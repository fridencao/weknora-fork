package service

// M6-1 WS1.2 · 图谱下钻的公共部分（实体与边共用）。
//
// 两条下钻路径（点节点 / 点边）在数据面上只有一处不同：**取哪些图谱 chunk**。
// 实体取「节点 ∪ 邻居」，边只取该边自己。取到之后的事情完全一样——按 KB 文档范围
// 过滤（权限边界）、解析契约锚点、回跳成 WeKnora 子 chunk。抽在这里是为了避免两处
// 各写一遍过滤逻辑：这类安全口径一旦分叉，通常只有一处被修。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	chatpipeline "github.com/Tencent/WeKnora/internal/application/service/chat_pipeline"
	"github.com/Tencent/WeKnora/internal/logger"
)

// graphDrillTimeout 下钻是交互式请求（用户点了节点/边在等），比建图意图的 30s 短。
const graphDrillTimeout = 10 * time.Second

// 单次下钻返回的证据量级：下钻面板是给人看的，不需要检索通道那种配额，
// 但也不能把整篇文档铺出来。
const (
	graphDrillEvidencePerHit = 3
	graphDrillEvidenceTotal  = 15
	// graphDrillContextChars 图谱原文上下文的展示上限。它是「LLM 抽取时看到的
	// 原文段落」，通常 1-3k 字符；截到 1200 够读出上下文且不淹没面板。
	graphDrillContextChars = 1200
)

// graphDrillMaxDocIds shared 模式传给数据面的归属过滤文档数上限（与 Python 侧
// MAX_FILTER_DOC_IDS 对齐，双方都挡「当全量扫描接口用」）。
const graphDrillMaxDocIds = 500

// graphDocIdsForFilter shared 模式下把 KB 文档清单交给数据面做归属过滤
// （WS4.4：workspace="" 是全局图，路由只校验了单 KB 读权限）。kb 模式返回空——
// workspace 隔离已生效，过滤是多余的。
// allowed 已按 KB 文档范围收口（graphAllowedDocs），这里只是转成逗号串。
func graphDocIdsForFilter(workspace string, allowed map[string]struct{}) string {
	if workspace != "" || len(allowed) == 0 {
		return ""
	}
	ids := make([]string, 0, len(allowed))
	for id := range allowed {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) > graphDrillMaxDocIds {
		ids = ids[:graphDrillMaxDocIds]
	}
	return strings.Join(ids, ",")
}

// graphEvidenceChunk 图谱侧的一条证据 chunk（key + 正文）。
//
// 正文必需而非仅 key：两侧 chunk 粒度差约 16 倍、无法按序号映射，回跳只能靠正文里
// 的契约锚点 `<!--sbk:pXXX-bXXX-->` 与 WeKnora 子 chunk 的 metadata.sbk_blocks 求交。
type graphEvidenceChunk struct {
	Key     string `json:"key"`
	Content string `json:"content"`
}

// fetchStarkbGraph GET 一个图谱数据端点并解析。任何失败都返回 nil（调用方降级）。
func fetchStarkbGraph[T any](ctx context.Context, path string, query url.Values) *T {
	base := os.Getenv("STARKB_API_URL")
	if base == "" {
		return nil
	}
	endpoint := base + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	cctx, cancel := context.WithTimeout(ctx, graphDrillTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, endpoint, nil)
	if err != nil {
		logger.Warnf(ctx, "graph drill: 构造请求失败 %s: %v", path, err)
		return nil
	}
	resp, err := (&http.Client{Timeout: graphDrillTimeout}).Do(req)
	if err != nil {
		logger.Warnf(ctx, "graph drill: 调用 starkb-api %s 失败: %v", path, err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logger.Warnf(ctx, "graph drill: starkb-api %s 返回 %d", path, resp.StatusCode)
		return nil
	}
	// 描述 + 证据正文是自由文本，限制读取量避免异常大响应拖垮进程。
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		logger.Warnf(ctx, "graph drill: 读取 %s 响应失败: %v", path, err)
		return nil
	}
	var out T
	if err := json.Unmarshal(body, &out); err != nil {
		logger.Warnf(ctx, "graph drill: 解析 %s 响应失败: %v", path, err)
		return nil
	}
	return &out
}

// graphAllowedDocs KB 文档范围：既是权限边界（只回跳本 KB 文档），也是标题回填来源。
func (s *knowledgeBaseService) graphAllowedDocs(
	ctx context.Context, tenantID uint64, kbID string,
) (map[string]struct{}, map[string]string) {
	allowed := make(map[string]struct{})
	titleByDoc := make(map[string]string)
	if s.kgRepo == nil {
		return allowed, titleByDoc
	}
	knowles, err := s.kgRepo.ListKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID)
	if err != nil {
		logger.Warnf(ctx, "graph drill: 列 KB %s 文档失败: %v", kbID, err)
	}
	for _, k := range knowles {
		if k == nil || k.ID == "" {
			continue
		}
		allowed[k.ID] = struct{}{}
		titleByDoc[k.ID] = k.Title
	}
	return allowed, titleByDoc
}

// buildGraphEvidenceRefs 把图谱证据 chunk 转成可回跳引用。
//
// 两类证据会被丢弃并计数：
//   - 键非法，或文档不在本 KB 范围内（共享图谱空间下会真实出现别的 KB 的证据，
//     而路由只校验了当前 KB 的读权限——不过滤就能顺着图点到别人的文档）；
//   - 正文里没有契约锚点（实测约 3%，多为纯表格/图片区域），无法定位到子 chunk。
//
// 返回值二 contextByKey：图谱 chunk key → 剥离锚点后的原文（P1-6：作为
// 「这条关系/实体是从什么上下文抽出来的」跟随证据展示）。上限见
// graphDrillContextChars。
func buildGraphEvidenceRefs(
	chunks []graphEvidenceChunk, allowed map[string]struct{},
) ([]chatpipeline.GraphEvidenceRef, int, map[string]string) {
	refs := make([]chatpipeline.GraphEvidenceRef, 0, len(chunks))
	contextByKey := make(map[string]string, len(chunks))
	dropped := 0
	for _, c := range chunks {
		docID, _, err := chatpipeline.ParseLightragChunkKey(c.Key)
		if err != nil {
			dropped++
			continue
		}
		if _, ok := allowed[docID]; !ok {
			dropped++
			continue
		}
		blocks := chatpipeline.ParseGraphChunkAnchors(c.Content)
		if len(blocks) == 0 {
			dropped++
			continue
		}
		refs = append(refs, chatpipeline.GraphEvidenceRef{
			Key: c.Key, DocID: docID, BlockIDs: blocks,
		})
		contextByKey[c.Key] = snippet(
			chatpipeline.StripGraphChunkAnchors(c.Content), graphDrillContextChars)
	}
	return refs, dropped, contextByKey
}

// resolveGraphEvidence 回跳为 WeKnora 子 chunk，并组装成前端可直接喂溯源面板的形状。
// contextByKey（P1-6）把图谱原文上下文挂到对应证据条目上：子 chunk 片段回答
// 「定位在哪」，图谱原文回答「关系是从什么上下文抽出来的」。
func (s *knowledgeBaseService) resolveGraphEvidence(
	ctx context.Context, tenantID uint64,
	refs []chatpipeline.GraphEvidenceRef, titleByDoc map[string]string,
	contextByKey map[string]string,
) []map[string]any {
	evidence := make([]map[string]any, 0, len(refs))
	if len(refs) == 0 {
		return evidence
	}
	chunks, sourceByChunk, err := chatpipeline.ResolveGraphEvidence(
		ctx, s.chunkRepo, tenantID, refs, graphDrillEvidencePerHit, graphDrillEvidenceTotal)
	if err != nil {
		logger.Warnf(ctx, "graph drill: 证据回跳失败（前端仅展示图谱侧信息）: %v", err)
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
		if ctx2 := sourceByChunk[c.ID]; ctx2 != "" {
			if raw, ok := contextByKey[ctx2]; ok && raw != "" {
				item["source_context"] = raw
			}
		}
		evidence = append(evidence, item)
	}
	return evidence
}

// graphUnavailable 统一的降级形状（前端按「面板空着」渲染，而不是错误页）。
func graphUnavailable(reason string) map[string]any {
	return map[string]any{
		"available": false,
		"reason":    reason,
		"neighbors": []map[string]any{},
		"relations": []map[string]any{},
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
