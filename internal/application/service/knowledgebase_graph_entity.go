package service

// M6-1 WS1.2（docs/11 §2.2）· 图谱实体下钻。
//
// 图谱浏览器点一个节点需要三样东西：实体本身（视图接口截到 200 字不够）、邻居关系、
// 以及**可点开的证据**——最后一项是缺口的核心：LightRAG 的 source_id 只是
// `{docID}-chunk-NNN` 形式的 chunk key，前端拿它跳不到溯源面板。
//
// 取数、权限过滤、契约锚点回跳的公共部分见 knowledgebase_graph_evidence.go。

import (
	"context"
	"net/url"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// graphEntityPayload starkb-api /graph/entity 的响应（字段与 Python 侧对齐）。
type graphEntityPayload struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
	Workspace string `json:"workspace"`
	Entity    struct {
		ID           string   `json:"id"`
		EntityType   string   `json:"entity_type"`
		Description  string   `json:"description"`
		Descriptions []string `json:"descriptions"`
		Degree       int      `json:"degree"`
		SourceKeys   []string `json:"source_keys"`
	} `json:"entity"`
	Neighbors []struct {
		ID            string   `json:"id"`
		EntityType    string   `json:"entity_type"`
		RelationType  string   `json:"relation_type"`
		RelationTypes []string `json:"relation_types"`
		Description   string   `json:"description"`
		Descriptions  []string `json:"descriptions"`
		Direction     string   `json:"direction"`
		SourceKeys    []string `json:"source_keys"`
	} `json:"neighbors"`
	Chunks        []graphEvidenceChunk `json:"chunks"`
	ChunkTotal    int                  `json:"chunk_total"`
	NeighborTotal int                  `json:"neighbor_total"`
	Truncated     bool                 `json:"truncated"`
}

// GraphEntityDetail 图谱实体下钻：实体 + 邻居 + 回跳到 WeKnora 子 chunk 的证据。
//
// 永不返回 error：前端点节点时给不出数据应当是「面板空着」，而不是弹错误——
// 图谱是可选增强通道，其数据面缺失不该让页面变成错误页。
func (s *knowledgeBaseService) GraphEntityDetail(
	ctx context.Context, kbID, name string,
) (map[string]any, error) {
	name = strings.TrimSpace(name)
	if kbID == "" || name == "" {
		return graphUnavailable("缺少参数"), nil
	}
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return graphUnavailable("无法确定租户"), nil
	}

	allowed, titleByDoc := s.graphAllowedDocs(ctx, tenantID, kbID)
	if len(allowed) == 0 {
		// 空间隔离模式下没有文档就不可能有权回跳任何证据；仍返回实体本体，
		// 只是证据为空——比整块面板报错更有用。
		logger.Infof(ctx, "graph entity: KB %s 范围内无可回跳文档", kbID)
	}

	// WS4.4：shared 模式把归属清单带给数据面，实体与邻居先在源头过滤
	query := url.Values{
		"workspace": {graphWorkspaceForKBID(kbID)},
		"name":      {name},
	}
	if ids := graphDocIdsForFilter(graphWorkspaceForKBID(kbID), allowed); ids != "" {
		query.Set("doc_ids", ids)
	}
	payload := fetchStarkbGraph[graphEntityPayload](ctx, "/graph/entity", query)
	if payload == nil {
		return graphUnavailable("图谱数据面不可用（starkb-api 未配置或不可达）"), nil
	}
	if !payload.Available {
		reason := payload.Reason
		if reason == "" {
			reason = "实体不存在"
		}
		return graphUnavailable(reason), nil
	}

	refs, dropped := buildGraphEvidenceRefs(payload.Chunks, allowed)
	evidence := s.resolveGraphEvidence(ctx, tenantID, refs, titleByDoc)

	neighbors := make([]map[string]any, 0, len(payload.Neighbors))
	for _, n := range payload.Neighbors {
		neighbors = append(neighbors, map[string]any{
			"id":             n.ID,
			"entity_type":    n.EntityType,
			"relation_type":  n.RelationType,
			"relation_types": n.RelationTypes,
			"description":    n.Description,
			"descriptions":   n.Descriptions,
			"direction":      n.Direction,
		})
	}

	logger.Infof(ctx, "graph entity: KB %s 实体 %q 邻居 %d，证据 chunk %d/%d 条回跳为 %d 个子 chunk（%d 条越权/无锚点已丢弃）",
		kbID, name, len(neighbors), len(refs), payload.ChunkTotal, len(evidence), dropped)

	return map[string]any{
		"available": true,
		"workspace": payload.Workspace,
		"entity": map[string]any{
			"id":           payload.Entity.ID,
			"entity_type":  payload.Entity.EntityType,
			"description":  payload.Entity.Description,
			"descriptions": payload.Entity.Descriptions,
			"degree":       payload.Entity.Degree,
		},
		"neighbors":      neighbors,
		"neighbor_total": payload.NeighborTotal,
		"evidence":       evidence,
		"evidence_total": len(evidence),
		"chunk_total":    payload.ChunkTotal,
		"dropped_chunks": dropped,
		"truncated":      payload.Truncated,
	}, nil
}
