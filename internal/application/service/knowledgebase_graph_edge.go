package service

// M6-1 WS1.2 · 图谱边下钻。
//
// 为什么与点节点分开：点节点给的是「节点 ∪ 全部邻居」的合并证据集，答不了
// 「**这条**关系是从哪句话抽出来的」。边的证据在 edge.properties.source_id 里，
// 与节点的证据互不相干，所以要按 (source, target) 单取。
//
// 同一对实体之间可能有多条边（不同关系类型，或反向），故 relations 是列表；
// 反向边也取回并用 direction 标注——图上画的是有向边，用户点的方向未必等于存储方向。

import (
	"context"
	"net/url"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// graphEdgePayload starkb-api /graph/edge 的响应（字段与 Python 侧对齐）。
type graphEdgePayload struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
	Workspace string `json:"workspace"`
	Source    string `json:"source"`
	Target    string `json:"target"`
	Relations []struct {
		Source        string   `json:"source"`
		Target        string   `json:"target"`
		Direction     string   `json:"direction"`
		RelationType  string   `json:"relation_type"`
		RelationTypes []string `json:"relation_types"`
		Description   string   `json:"description"`
		Descriptions  []string `json:"descriptions"`
		SourceKeys    []string `json:"source_keys"`
	} `json:"relations"`
	Chunks     []graphEvidenceChunk `json:"chunks"`
	ChunkTotal int                  `json:"chunk_total"`
	Truncated  bool                 `json:"truncated"`
}

// GraphEdgeDetail 图谱边下钻：关系属性 + 该边自己的证据（已回跳为 WeKnora 子 chunk）。
//
// 与 GraphEntityDetail 同样永不返回 error（见该方法的说明）。
func (s *knowledgeBaseService) GraphEdgeDetail(
	ctx context.Context, kbID, source, target string,
) (map[string]any, error) {
	source = strings.TrimSpace(source)
	target = strings.TrimSpace(target)
	if kbID == "" || source == "" || target == "" {
		return graphUnavailable("缺少参数"), nil
	}
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return graphUnavailable("无法确定租户"), nil
	}

	allowed, titleByDoc := s.graphAllowedDocs(ctx, tenantID, kbID)

	// WS4.4：shared 模式把归属清单带给数据面（与实体端点同口径）
	query := url.Values{
		"workspace": {graphWorkspaceForKBID(kbID)},
		"source":    {source},
		"target":    {target},
	}
	if ids := graphDocIdsForFilter(graphWorkspaceForKBID(kbID), allowed); ids != "" {
		query.Set("doc_ids", ids)
	}
	payload := fetchStarkbGraph[graphEdgePayload](ctx, "/graph/edge", query)
	if payload == nil {
		return graphUnavailable("图谱数据面不可用（starkb-api 未配置或不可达）"), nil
	}
	if !payload.Available {
		reason := payload.Reason
		if reason == "" {
			reason = "关系不存在"
		}
		return graphUnavailable(reason), nil
	}

	refs, dropped, contextByKey := buildGraphEvidenceRefs(payload.Chunks, allowed)
	evidence := s.resolveGraphEvidence(ctx, tenantID, refs, titleByDoc, contextByKey)

	relations := make([]map[string]any, 0, len(payload.Relations))
	for _, r := range payload.Relations {
		relations = append(relations, map[string]any{
			"source":         r.Source,
			"target":         r.Target,
			"direction":      r.Direction,
			"relation_type":  r.RelationType,
			"relation_types": r.RelationTypes,
			"description":    r.Description,
			"descriptions":   r.Descriptions,
		})
	}

	logger.Infof(ctx, "graph edge: KB %s %s→%s 关系 %d 条，证据 chunk %d/%d 条回跳为 %d 个子 chunk（%d 条越权/无锚点已丢弃）",
		kbID, source, target, len(relations), len(refs), payload.ChunkTotal, len(evidence), dropped)

	return map[string]any{
		"available":      true,
		"workspace":      payload.Workspace,
		"source":         payload.Source,
		"target":         payload.Target,
		"relations":      relations,
		"evidence":       evidence,
		"evidence_total": len(evidence),
		"chunk_total":    payload.ChunkTotal,
		"dropped_chunks": dropped,
		"truncated":      payload.Truncated,
	}, nil
}
