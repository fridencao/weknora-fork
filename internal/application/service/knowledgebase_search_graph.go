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
	"strings"
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
//
// 第二个返回值是本次图谱召回命中的实体名清单（M6-1 WS1.3），查询级——调用方把它
// 盖到 Channels 含 graph 的最终结果上，前端引用抽屉据此深链图谱浏览器。
func (s *knowledgeBaseService) graphRecallForSearch(
	ctx context.Context, kbIDs []string, query string, topK int,
	retrievalCfg *types.RetrievalConfig,
) ([]*types.IndexWithScore, []string) {
	if !retrievalCfg.GetGraphChannelEnabled(s.graphChannelEnvDefault(ctx)) || query == "" || len(kbIDs) == 0 {
		return nil, nil
	}
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return nil, nil
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
		return nil, nil
	}

	client := chatpipeline.NewLightragClientFromEnv()
	if client.BaseURL() == "" {
		logger.Infof(ctx, "graph recall: LIGHT_RAG_BASE_URL 未配置，跳过")
		return nil, nil
	}
	gctx, cancel := context.WithTimeout(ctx, s.graphRecallTimeout(ctx))
	defer cancel()
	data, err := s.graphQueryMerged(gctx, client, kbIDs, query, topK)
	if err != nil {
		logger.Warnf(ctx, "graph recall: 查询失败（降级二通道）: %v", err)
		return nil, nil
	}
	entityNames := CollectGraphEntityNames(data, maxGraphEntityNames)

	// 图谱证据 → WeKnora 子 chunk：KB 文档范围过滤 + 正文契约锚点回跳（A0 修复）。
	// 旧实现按 73 字符定长键解析 `{doc36}-{chunk36}`，而 LightRAG 实际产出
	// `{docID}-chunk-NNN`，真实数据上恒失败 → 图谱通道静默召回 0 条。
	refs := chatpipeline.CollectGraphEvidence(data, allowed)
	if len(refs) == 0 {
		logger.Infof(ctx, "graph recall: 图谱证据均在 KB 范围外或键非法")
		return nil, entityNames
	}
	chunks, _, err := chatpipeline.ResolveGraphEvidence(
		ctx, s.chunkRepo, tenantID, refs, s.graphChunksPerHit(ctx), topK)
	if err != nil {
		logger.Warnf(ctx, "graph recall: chunk 回跳失败: %v", err)
		return nil, entityNames
	}
	if len(chunks) == 0 {
		logger.Infof(ctx, "graph recall: %d 条证据均无契约锚点可定位", len(refs))
		return nil, entityNames
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
			Channels:    []types.RetrieverType{types.GraphRetrieverType},
		})
	}
	logger.Infof(ctx, "graph recall: 命中 %d chunk（图谱证据 %d 条，KB 范围 %d 文档，实体 %v）",
		len(results), len(refs), len(allowed), entityNames)
	return results, entityNames
}

// maxGraphEntityNames 单次检索透出的实体名上限。引用抽屉只展示一小排 chips，
// 更多只会把 UI 撑爆；排序保持 LightRAG 的相关性顺序（截断即"最相关的 N 个"）。
const maxGraphEntityNames = 8

// CollectGraphEntityNames 收集图谱召回命中的实体名（保序去重，封顶 max）。
//
// 抽成纯函数便于单测：实体名会被前端拿去深链图谱浏览器，空名/重复必须在这里清掉。
func CollectGraphEntityNames(data *chatpipeline.LightragQueryData, max int) []string {
	if data == nil || max <= 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(data.Data.Entities))
	out := make([]string, 0, len(data.Data.Entities))
	for _, e := range data.Data.Entities {
		name := strings.TrimSpace(e.EntityName)
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
		if len(out) >= max {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
