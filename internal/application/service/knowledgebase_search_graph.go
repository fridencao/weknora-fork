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
	"time"

	chatpipeline "github.com/Tencent/WeKnora/internal/application/service/chat_pipeline"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// graphRecallTimeout 图谱通道软超时：检索主链路不应被图谱抖动拖死。
const graphRecallTimeout = 8 * time.Second

// graphChannelEnabled 与 chat 通道共用同一开关（GRAPH_CHANNEL_ENABLED）。
func graphChannelEnabled() bool {
	return os.Getenv("GRAPH_CHANNEL_ENABLED") == "true"
}

// graphRecallForSearch 对 API 层检索执行图谱召回：
// 查询词 → LightRAG mix 查询 → 证据 chunk key 解析 → KB 范围文档过滤 → chunk 回查。
// 任何失败返回 nil（调用方按二通道继续），不向调用方透出错误。
func (s *knowledgeBaseService) graphRecallForSearch(
	ctx context.Context, kbIDs []string, query string, topK int,
) []*types.IndexWithScore {
	if !graphChannelEnabled() || query == "" || len(kbIDs) == 0 {
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
	gctx, cancel := context.WithTimeout(ctx, graphRecallTimeout)
	defer cancel()
	data, err := client.QueryData(gctx, query, topK)
	if err != nil {
		logger.Warnf(ctx, "graph recall: 查询失败（降级二通道）: %v", err)
		return nil
	}

	chunkIDs := make([]string, 0, len(data.Data.Chunks))
	for _, c := range data.Data.Chunks {
		docID, chunkID, err := chatpipeline.ParseLightragChunkKey(c.ChunkID)
		if err != nil {
			continue
		}
		if _, ok := allowed[docID]; ok {
			chunkIDs = append(chunkIDs, chunkID)
		}
	}
	if len(chunkIDs) == 0 {
		logger.Infof(ctx, "graph recall: 图谱证据均在 KB 范围外或键非法")
		return nil
	}
	chunks, err := s.chunkRepo.ListChunksByID(ctx, tenantID, chunkIDs)
	if err != nil {
		logger.Warnf(ctx, "graph recall: chunk 回查失败: %v", err)
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
	logger.Infof(ctx, "graph recall: 命中 %d chunk（候选 %d，KB 范围 %d 文档）",
		len(results), len(data.Data.Chunks), len(allowed))
	return results
}
