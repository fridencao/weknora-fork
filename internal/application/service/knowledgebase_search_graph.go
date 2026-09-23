package service

// M3 WS3.1（docs/07）：图谱召回进 API 层 HybridSearch，与向量/关键词组成三通道 RRF。
// 复用 chat_pipeline 的 LightRAG 客户端与溯源键解析；失败降级为二通道（不阻断主检索）。
//
// 回跳过滤口径修复（M2 遗留）：建图时 LightRAG 的 document_id = WeKnora knowledge
// doc_id（tools/build_graph_a3.py 以契约目录名入队），因此图谱 chunk key 前段必须
// 对照**KB 范围内的文档 ID 集合**，而非 M2 search_graph.go 中使用的 KB ID 集合——
// 两者同为 UUID，语义错位在真实数据上表现为全部命中被过滤。

import (
	"context"
	"os"
	"sync"
	"time"

	chatpipeline "github.com/Tencent/WeKnora/internal/application/service/chat_pipeline"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// graphRecallTimeout 图谱通道软超时：检索主链路不应被图谱抖动拖死。
// 经系统设置解析（DB > ENV > 默认），改后立即生效，无需重启。
func (s *knowledgeBaseService) graphRecallTimeout(ctx context.Context) time.Duration {
	if s.settings == nil {
		return 8 * time.Second
	}
	n := s.settings.GetInt(ctx,
		types.SettingKeyGraphChannelTimeoutS, types.SettingEnvGraphChannelTimeoutS, 8)
	if n > 0 && n <= 120 {
		return time.Duration(n) * time.Second
	}
	return 8 * time.Second
}

// graphChannelEnvDefault 部署级默认（graph.channel.enabled），与 chat 通道共用。
// 默认 true，与 registry 条目一致（迁移前 compose 注入 GRAPH_CHANNEL_ENABLED=true）。
func (s *knowledgeBaseService) graphChannelEnvDefault(ctx context.Context) bool {
	if s.settings == nil {
		return true
	}
	return s.settings.GetBool(ctx,
		types.SettingKeyGraphChannelEnabled, types.SettingEnvGraphChannelEnabled, true)
}

// graphRecallForSearch 对 API 层检索执行图谱召回：
// 查询词 → LightRAG mix 查询 → KB 文档范围过滤 → 正文契约锚点回跳 WeKnora 子 chunk。
// 开关：tenant 检索配置（设置 UI）优先，未配置时回落部署默认。
// 任何失败返回 nil（调用方按二通道继续），不向调用方透出错误。
func (s *knowledgeBaseService) graphRecallForSearch(
	ctx context.Context, kbIDs []string, query string, topK int,
	retrievalCfg *types.RetrievalConfig,
) []*types.IndexWithScore {
	if !retrievalCfg.GetGraphChannelEnabled(s.graphChannelEnvDefault(ctx)) || query == "" || len(kbIDs) == 0 {
		return nil
	}
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return nil
	}

	// KB 范围 → 允许回跳的文档 ID 集合（修复口径：对照 knowledge doc_id）。
	allowed := make(map[string]struct{})
	for _, kbID := range kbIDs {
		knowles, err := s.kgRepo.ListKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID)
		if err != nil {
			logger.Warnf(ctx, "graph recall: 列 KB 文档失败（跳过 %s）: %v", kbID, err)
			continue
		}
		for _, k := range knowles {
			allowed[k.ID] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		logger.Infof(ctx, "graph recall: KB 范围内无可回跳文档，跳过")
		return nil
	}

	client := chatpipeline.NewLightragClientFromEnv()
	if client.BaseURL() == "" {
		logger.Infof(ctx, "graph recall: LIGHT_RAG_BASE_URL 未配置，跳过")
		return nil
	}
	gctx, cancel := context.WithTimeout(ctx, s.graphRecallTimeout(ctx))
	defer cancel()
	data, err := s.graphQueryMerged(gctx, client, kbIDs, query, topK)
	if err != nil {
		logger.Warnf(ctx, "graph recall: 查询失败（降级二通道）: %v", err)
		return nil
	}

	// 图谱证据 → WeKnora 子 chunk：KB 文档范围过滤 + 正文契约锚点回跳（A0 修复）。
	// 旧实现按 73 字符定长键解析 `{doc36}-{chunk36}`，而 LightRAG 实际产出
	// `{docID}-chunk-NNN`，真实数据上恒失败 → 图谱通道静默召回 0 条。
	refs := chatpipeline.CollectGraphEvidence(data, allowed)
	if len(refs) == 0 {
		logger.Infof(ctx, "graph recall: 图谱证据均在 KB 范围外或键非法")
		return nil
	}
	chunks, err := chatpipeline.ResolveGraphEvidence(
		ctx, s.chunkRepo, tenantID, refs, s.graphChunksPerHit(ctx), topK)
	if err != nil {
		logger.Warnf(ctx, "graph recall: chunk 回跳失败: %v", err)
		return nil
	}
	if len(chunks) == 0 {
		logger.Infof(ctx, "graph recall: %d 条证据均无契约锚点可定位", len(refs))
		return nil
	}

	results := make([]*types.IndexWithScore, 0, len(chunks))
	for _, c := range chunks {
		results = append(results, &types.IndexWithScore{
			ChunkID:     c.ID,
			Content:     c.Content,
			KnowledgeID: c.KnowledgeID,
			Score:       1.0,
			MatchType:   types.MatchTypeGraph,
			IsEnabled:   true,
		})
	}
	logger.Infof(ctx, "graph recall: 命中 %d chunk（图谱证据 %d 条，KB 范围 %d 文档）",
		len(results), len(refs), len(allowed))
	return results
}

// graphChunksPerHit 单条图谱证据最多回跳的 WeKnora 子 chunk 数。
// 经系统设置解析（DB > ENV > 默认），与 chat 通道的 graph.channel.chunks_per_hit
// 共用同一个键——迁移前两处各自 os.Getenv，改一处另一处不生效。
func (s *knowledgeBaseService) graphChunksPerHit(ctx context.Context) int {
	def := int64(chatpipeline.DefaultGraphChunksPerHit)
	if s.settings == nil {
		return int(def)
	}
	n := s.settings.GetInt(ctx,
		types.SettingKeyGraphChannelChunksPerHit, types.SettingEnvGraphChannelChunksPerHit, def)
	if n < 1 {
		return int(def)
	}
	return int(n)
}

// graphQueryMerged 图谱查询（WS6.2）：shared 模式单次查询默认空间；kb 模式按
// 用户可访问 KB 各自的图谱空间并行查询（docs/03 §4「不可达的图直接不查」），
// 证据 chunk 按 chunk_id 去重合并后交给统一的 KB 范围过滤与锚点回跳。
func (s *knowledgeBaseService) graphQueryMerged(
	ctx context.Context, client *chatpipeline.LightragClient,
	kbIDs []string, query string, topK int,
) (*chatpipeline.LightragQueryData, error) {
	if os.Getenv("STARKB_GRAPH_WORKSPACE_MODE") != "kb" {
		return client.QueryDataInWorkspace(ctx, query, topK, "")
	}
	merged := &chatpipeline.LightragQueryData{}
	seen := make(map[string]struct{})
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, kbID := range kbIDs {
		wg.Add(1)
		go func(kbID string) {
			defer wg.Done()
			d, err := client.QueryDataInWorkspace(ctx, query, topK, kbID)
			if err != nil {
				logger.Warnf(ctx, "graph recall: 空间 %s 查询失败（跳过）: %v", kbID, err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, c := range d.Data.Chunks {
				if _, dup := seen[c.ChunkID]; dup {
					continue
				}
				seen[c.ChunkID] = struct{}{}
				merged.Data.Chunks = append(merged.Data.Chunks, c)
			}
			merged.Data.Entities = append(merged.Data.Entities, d.Data.Entities...)
			merged.Data.Relationships = append(merged.Data.Relationships, d.Data.Relationships...)
		}(kbID)
	}
	wg.Wait()
	return merged, nil
}
