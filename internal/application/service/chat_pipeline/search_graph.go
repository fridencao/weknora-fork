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

// PluginSearchGraph 图谱召回通道（M2，docs03 §4 / ADR-005；A0 回跳修复见 graph_anchor.go）。
// 与 PluginSearchEntity 互补：实体词经 LightRAG mix 查询拿 KG 上下文与证据 chunk，
// 证据经**正文契约锚点**回跳为 WeKnora 子 chunk（chunk_type=text，与向量/关键词
// 通道同粒度），补入检索结果；
// 权限继承：仅返回当前租户/KB 范围内可读的 chunk（KB 文档 ID 集合过滤 + 租户查询）。
type PluginSearchGraph struct {
	lightrag      *LightragClient
	chunkRepo     interfaces.ChunkRepository
	knowledgeRepo interfaces.KnowledgeRepository
	enabled       bool
	topK          int
	chunksPerHit  int
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
		chunksPerHit:  DefaultGraphChunksPerHit,
	}
	if v := os.Getenv("GRAPH_CHANNEL_TOP_K"); v != "" {
		fmt.Sscanf(v, "%d", &p.topK)
	}
	if v := os.Getenv("GRAPH_CHANNEL_CHUNKS_PER_HIT"); v != "" {
		fmt.Sscanf(v, "%d", &p.chunksPerHit)
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

	// 证据 chunk → WeKnora 子 chunk：KB 范围文档过滤 + 正文契约锚点回跳
	// （见 graph_anchor.go；A0 修复前此处按 73 字符定长键解析，真实数据恒失败）。
	allowed := expandAllowedDocIDs(ctx, p.knowledgeRepo, tenantID, chatManage.EntityKBIDs)
	refs := CollectGraphEvidence(data, allowed)
	if len(refs) == 0 {
		logger.Infof(ctx, "graph search: 无可回跳证据（查询词域外或全部被过滤）")
		return next()
	}
	chunks, err := ResolveGraphEvidence(ctx, p.chunkRepo, tenantID, refs, p.chunksPerHit, p.topK)
	if err != nil {
		logger.Errorf(ctx, "graph search: chunk 回跳失败: %v", err)
		return next()
	}
	if len(chunks) == 0 {
		logger.Infof(ctx, "graph search: %d 条证据均无契约锚点可定位", len(refs))
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
	logger.Infof(ctx, "graph search: 命中 %d 个新 chunk（图谱证据 %d 条，实体 %d）",
		appended, len(refs), len(chatManage.Entity))
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
