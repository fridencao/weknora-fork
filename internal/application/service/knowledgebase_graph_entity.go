package service

// M6-1 WS1.2（docs/11 §2.2）· 图谱实体下钻。
//
// 图谱浏览器点一个节点需要三样东西：实体本身（完整描述，视图接口截到 200 字不够）、
// 邻居关系、以及**可点开的证据**——最后一项是缺口的核心：LightRAG 的 source_id 只是
// `{docID}-chunk-NNN` 形式的 chunk key，前端拿它跳不到溯源面板。
//
// 回跳沿用 A0 已修好的口径（chat_pipeline/graph_anchor.go），而不是另起一套：
//   1. 图谱 chunk key 前段是 WeKnora knowledge doc id，但**必须按 KB 文档范围过滤**，
//      否则共享空间下能顺着图点到别的 KB 的文档（与 /graph/doc-status 同类越权）；
//   2. 两侧 chunk 粒度差约 16 倍，无法按序号映射，只能靠图谱 chunk 正文里的契约锚点
//      `<!--sbk:pXXX-bXXX-->` 与 WeKnora 子 chunk 的 metadata.sbk_blocks 求交。
// 因此 Python 侧必须回传 chunk 正文，这里才能解析锚点。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	chatpipeline "github.com/Tencent/WeKnora/internal/application/service/chat_pipeline"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// graphEntityTimeout 下钻是交互式请求（用户点了节点在等），比建图意图的 30s 短。
const graphEntityTimeout = 10 * time.Second

// graphEntityEvidencePerHit / Total 单次下钻返回的证据量级：下钻面板是给人看的，
// 不需要检索通道那种配额，但也不能把整篇文档铺出来。
const (
	graphEntityEvidencePerHit = 3
	graphEntityEvidenceTotal  = 15
)

// graphEntityPayload starkb-api /graph/entity 的响应（字段与 Python 侧对齐）。
type graphEntityPayload struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
	Workspace string `json:"workspace"`
	Entity    struct {
		ID          string   `json:"id"`
		EntityType  string   `json:"entity_type"`
		Description string   `json:"description"`
		Degree      int      `json:"degree"`
		SourceKeys  []string `json:"source_keys"`
	} `json:"entity"`
	Neighbors []struct {
		ID           string   `json:"id"`
		EntityType   string   `json:"entity_type"`
		RelationType string   `json:"relation_type"`
		Description  string   `json:"description"`
		Direction    string   `json:"direction"`
		SourceKeys   []string `json:"source_keys"`
	} `json:"neighbors"`
	Chunks []struct {
		Key     string `json:"key"`
		Content string `json:"content"`
	} `json:"chunks"`
	ChunkTotal    int  `json:"chunk_total"`
	NeighborTotal int  `json:"neighbor_total"`
	Truncated     bool `json:"truncated"`
}

// fetchStarkbGraphEntity 拉取实体详情；不可达/非 200/解析失败一律返回 nil（降级）。
func fetchStarkbGraphEntity(ctx context.Context, workspace, name string) *graphEntityPayload {
	base := os.Getenv("STARKB_API_URL")
	if base == "" {
		return nil
	}
	endpoint := base + "/graph/entity?workspace=" + url.QueryEscape(workspace) +
		"&name=" + url.QueryEscape(name)
	cctx, cancel := context.WithTimeout(ctx, graphEntityTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, endpoint, nil)
	if err != nil {
		logger.Warnf(ctx, "graph entity: 构造请求失败: %v", err)
		return nil
	}
	resp, err := (&http.Client{Timeout: graphEntityTimeout}).Do(req)
	if err != nil {
		logger.Warnf(ctx, "graph entity: 调用 starkb-api 失败: %v", err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logger.Warnf(ctx, "graph entity: starkb-api 返回 %d", resp.StatusCode)
		return nil
	}
	// 实体描述 + 证据正文是自由文本，限制读取量避免异常大响应拖垮进程。
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		logger.Warnf(ctx, "graph entity: 读取响应失败: %v", err)
		return nil
	}
	var out graphEntityPayload
	if err := json.Unmarshal(body, &out); err != nil {
		logger.Warnf(ctx, "graph entity: 解析响应失败: %v", err)
		return nil
	}
	return &out
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
		return graphEntityUnavailable("缺少参数"), nil
	}
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return graphEntityUnavailable("无法确定租户"), nil
	}

	// KB 文档范围：既是权限边界（只回跳本 KB 文档），也是标题回填的来源。
	allowed := make(map[string]struct{})
	titleByDoc := make(map[string]string)
	if s.kgRepo != nil {
		knowles, err := s.kgRepo.ListKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID)
		if err != nil {
			logger.Warnf(ctx, "graph entity: 列 KB %s 文档失败: %v", kbID, err)
		}
		for _, k := range knowles {
			if k == nil || k.ID == "" {
				continue
			}
			allowed[k.ID] = struct{}{}
			titleByDoc[k.ID] = k.Title
		}
	}
	if len(allowed) == 0 {
		// 空间隔离模式下没有文档就不可能有权回跳任何证据；仍返回实体本体，
		// 只是证据为空——比整块面板报错更有用。
		logger.Infof(ctx, "graph entity: KB %s 范围内无可回跳文档", kbID)
	}

	payload := fetchStarkbGraphEntity(ctx, graphWorkspaceForKBID(kbID), name)
	if payload == nil {
		return graphEntityUnavailable("图谱数据面不可用（starkb-api 未配置或不可达）"), nil
	}
	if !payload.Available {
		reason := payload.Reason
		if reason == "" {
			reason = "实体不存在"
		}
		return graphEntityUnavailable(reason), nil
	}

	// 证据 chunk → 契约锚点 → WeKnora 子 chunk（A0 口径）。
	refs := make([]chatpipeline.GraphEvidenceRef, 0, len(payload.Chunks))
	dropped := 0
	anchored := 0
	for _, c := range payload.Chunks {
		docID, _, err := chatpipeline.ParseLightragChunkKey(c.Key)
		if err != nil {
			dropped++
			continue
		}
		if _, ok := allowed[docID]; !ok {
			// 跨 KB 的图谱证据：共享空间下会真实出现，直接丢弃（权限边界）
			dropped++
			continue
		}
		blocks := chatpipeline.ParseGraphChunkAnchors(c.Content)
		if len(blocks) == 0 {
			// 实测约 3% 的图谱 chunk 无锚点（纯表格/图片区域），无法定位到子 chunk
			dropped++
			continue
		}
		anchored++
		refs = append(refs, chatpipeline.GraphEvidenceRef{
			Key: c.Key, DocID: docID, BlockIDs: blocks,
		})
	}

	evidence := make([]map[string]any, 0, len(refs))
	if len(refs) > 0 {
		chunks, err := chatpipeline.ResolveGraphEvidence(
			ctx, s.chunkRepo, tenantID, refs, graphEntityEvidencePerHit, graphEntityEvidenceTotal)
		if err != nil {
			logger.Warnf(ctx, "graph entity: 证据回跳失败（前端仅展示实体与邻居）: %v", err)
		}
		for _, c := range chunks {
			item := map[string]any{
				"chunk_id":     c.ID,
				"knowledge_id": c.KnowledgeID,
				"title":        titleByDoc[c.KnowledgeID],
				"snippet":      snippet(c.Content, 400),
			}
			// chunk_metadata 原样透出：溯源面板的 L3（页码）与 L4（区块锚点）都从
			// 这里读 sbk_pages / sbk_blocks，不透出就只能给到文档级、点不到具体位置。
			if len(c.Metadata) > 0 {
				item["chunk_metadata"] = c.Metadata
			}
			evidence = append(evidence, item)
		}
	}

	neighbors := make([]map[string]any, 0, len(payload.Neighbors))
	for _, n := range payload.Neighbors {
		neighbors = append(neighbors, map[string]any{
			"id":            n.ID,
			"entity_type":   n.EntityType,
			"relation_type": n.RelationType,
			"description":   n.Description,
			"direction":     n.Direction,
		})
	}

	logger.Infof(ctx, "graph entity: KB %s 实体 %q 邻居 %d，证据 chunk %d/%d 条回跳为 %d 个子 chunk（%d 条越权/无锚点已丢弃）",
		kbID, name, len(neighbors), anchored, payload.ChunkTotal, len(evidence), dropped)

	return map[string]any{
		"available": true,
		"workspace": payload.Workspace,
		"entity": map[string]any{
			"id":          payload.Entity.ID,
			"entity_type": payload.Entity.EntityType,
			"description": payload.Entity.Description,
			"degree":      payload.Entity.Degree,
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

func graphEntityUnavailable(reason string) map[string]any {
	return map[string]any{
		"available": false,
		"reason":    reason,
		"neighbors": []map[string]any{},
		"evidence":  []map[string]any{},
	}
}

// snippet 截断证据正文（按 rune，避免把多字节字符切成乱码）。
func snippet(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
