package chatpipeline

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// PluginSearchGraph 图谱召回通道（M2，docs03 §4 / ADR-005）。
// 与 PluginSearchEntity 互补：实体词经 LightRAG mix 查询拿 KG 上下文与证据 chunk
// （chunk key 定长 {doc36}-{chunk36}，无损解析回 WeKnora chunk），补入检索结果；
// 权限继承：仅返回当前租户/KB 范围内可读的 chunk（下推 ListChunksByID + KB 过滤）。
type PluginSearchGraph struct {
	lightrag      *LightragClient
	chunkRepo     interfaces.ChunkRepository
	knowledgeRepo interfaces.KnowledgeRepository
	enabled       bool
	topK          int
}

// NewPluginSearchGraph 创建图谱召回通道插件（container.Invoke 接线）。
func NewPluginSearchGraph(
	eventManager *EventManager,
	chunkRepository interfaces.ChunkRepository,
	knowledgeRepository interfaces.KnowledgeRepository,
) *PluginSearchGraph {
	p := &PluginSearchGraph{
		lightrag:      NewLightragClientFromEnv(),
		chunkRepo:     chunkRepository,
		knowledgeRepo: knowledgeRepository,
		// p.enabled = 部署级默认；运行时被 tenant 检索配置（UI）覆盖（见 OnEvent）。
		enabled:       os.Getenv("GRAPH_CHANNEL_ENABLED") == "true",
		topK:          20,
	}
	if v := os.Getenv("GRAPH_CHANNEL_TOP_K"); v != "" {
		fmt.Sscanf(v, "%d", &p.topK)
	}
	eventManager.Register(p)
	return p
}

// ActivationEvents 图谱通道在实体检索事件后触发（复用实体抽取产物 chatManage.Entity）。
func (p *PluginSearchGraph) ActivationEvents() []types.EventType {
	return []types.EventType{types.GRAPH_SEARCH}
}

// OnEvent 执行图谱召回：实体词 → LightRAG mix 查询 → 证据 chunk → 追加检索结果。
func (p *PluginSearchGraph) OnEvent(
	ctx context.Context,
	eventType types.EventType,
	chatManage *types.ChatManage,
	next func() *PluginError,
) *PluginError {
	// 开关：tenant 检索配置（设置 UI 显式设置）优先，未配置回落部署默认。
	enabled := p.enabled
	if info, ok := types.TenantInfoFromContext(ctx); ok && info != nil && info.RetrievalConfig != nil {
		enabled = info.RetrievalConfig.GetGraphChannelEnabled(p.enabled)
	}
	if !enabled {
		return next()
	}
	if len(chatManage.Entity) == 0 || len(chatManage.EntityKBIDs) == 0 {
		logger.Infof(ctx, "graph search: 无实体词或无图谱空间，跳过")
		return next()
	}

	tenantID := types.MustTenantIDFromContext(ctx)
	query := strings.Join(chatManage.Entity, " ")

	data, err := p.lightrag.QueryData(ctx, query, p.topK)
	if err != nil {
		// 图谱通道失败不阻断主检索（降级链：三通道退二通道）
		logger.Warnf(ctx, "graph search 查询失败（降级跳过）: %v", err)
		return next()
	}

	// 证据 chunk key → WeKnora chunk（跨权限双重过滤：KB 范围文档校验 + 租户查询）。
	// M3 口径对齐（与 service 层 knowledgebase_search_graph.go 一致）：建图时
	// LightRAG 的 document_id = WeKnora knowledge doc_id，因此 key 前段必须对照
	// KB 范围内的**文档 ID** 集合——M2 直接用 KB ID 对照，真实数据上全部命中被过滤。
	allowed := expandAllowedDocIDs(ctx, p.knowledgeRepo, tenantID, chatManage.EntityKBIDs)
	chunkIDs := make([]string, 0, len(data.Data.Chunks))
	keyToKB := make(map[string]string)
	for _, c := range data.Data.Chunks {
		if docID, chunkID, err := ParseLightragChunkKey(c.ChunkID); err == nil {
			if _, ok := allowed[docID]; ok && docID != chunkID {
				chunkIDs = append(chunkIDs, chunkID)
				keyToKB[chunkID] = docID
			}
		}
	}
	if len(chunkIDs) == 0 {
		logger.Infof(ctx, "graph search: 无可回跳 chunk（查询词域外或全部被过滤）")
		return next()
	}
	chunks, err := p.chunkRepo.ListChunksByID(ctx, tenantID, chunkIDs)
	if err != nil {
		logger.Errorf(ctx, "graph search: chunk 回查失败: %v", err)
		return next()
	}

	seen := map[string]bool{}
	for _, r := range chatManage.SearchResult {
		seen[r.ID] = true
	}
	appended := 0
	for _, c := range chunks {
		if seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		chatManage.SearchResult = append(chatManage.SearchResult, &types.SearchResult{
			ID:            c.ID,
			Content:       c.Content,
			KnowledgeID:   c.KnowledgeID,
			ChunkIndex:    c.ChunkIndex,
			StartAt:       c.StartAt,
			EndAt:         c.EndAt,
			Seq:           c.ChunkIndex,
			Score:         1.0,
			MatchType:     types.MatchTypeGraph,
			ChunkType:     string(c.ChunkType),
			ParentChunkID: c.ParentChunkID,
			ChunkMetadata: c.Metadata,
		})
		appended++
	}
	logger.Infof(ctx, "graph search: 命中 %d 个新 chunk（实体 %d）", appended, len(chatManage.Entity))
	return next()
}

// 并发安全占位：LightRAG 查询在各 KB 并行时由 http.Client 保证连接复用。
var _ = sync.Mutex{}

// expandAllowedDocIDs 把 KB ID 集合展开为允许回跳的文档 ID 集合（M3 口径）。
// 单 KB 列举失败仅跳过该 KB（降级，不阻断通道）。
func expandAllowedDocIDs(
	ctx context.Context,
	repo interfaces.KnowledgeRepository,
	tenantID uint64,
	kbIDs []string,
) map[string]struct{} {
	allowed := make(map[string]struct{}, len(kbIDs))
	for _, kbID := range kbIDs {
		knowles, err := repo.ListKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID)
		if err != nil {
			logger.Warnf(ctx, "graph search: 列 KB 文档失败（跳过 %s）: %v", kbID, err)
			continue
		}
		for _, k := range knowles {
			allowed[k.ID] = struct{}{}
		}
	}
	return allowed
}
