package handler

// A3（docs/09 WS1.3）：KB 级 LightRAG 图谱状态代理。
//
// 前端 GraphSettings 页需要图谱健康度（实体/关系/chunk 计数、最近建图任务、
// 失败清单）。真实存储在 starkb-api 侧的 starkb_graph PG 库（A4），Go 侧只做
// 带权限的代理：复用 KBAccessRead 中间件（路由层挂），把 workspace 按
// STARKB_GRAPH_WORKSPACE_MODE 解析后转发 starkb-api /graph/status。
// 代理不可达时返回 degraded 数据（graph_config 仍可用），页面不空白。

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/infrastructure/docparser"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

const graphStatusProxyTimeout = 10 * time.Second

// graphDocStatusMaxIDs 是单次徽标查询的文档数上限。列表页一页最多几十行，
// 500 留足余量，同时挡住「拿它当全量扫描接口」的用法。
const graphDocStatusMaxIDs = 500

// ownedKBDocIDsForGraph 列出 KB 内**参与建图**的文档 id（粘贴类 FileName==""
// 从不进图谱，带上只会白占参数）。WS4.4 的 shared 模式视图过滤用。
// 与 Python 侧 MAX_FILTER_DOC_IDS(=500) 对齐。
func (h *KnowledgeBaseHandler) ownedKBDocIDsForGraph(ctx context.Context, kbID string) []string {
	if h.knowledgeService == nil || kbID == "" {
		return nil
	}
	docs, err := h.knowledgeService.ListKnowledgeByKnowledgeBaseID(ctx, kbID)
	if err != nil {
		logger.Warnf(ctx, "graph view: 列 KB %s 文档失败（跳过归属过滤）: %v", kbID, err)
		return nil
	}
	out := make([]string, 0, len(docs))
	for _, d := range docs {
		if d != nil && d.ID != "" && d.FileName != "" {
			out = append(out, d.ID)
		}
	}
	if len(out) > graphDocStatusMaxIDs {
		out = out[:graphDocStatusMaxIDs]
	}
	return out
}

// graphWorkspaceForKBHandler 与 service 侧 graphWorkspaceForKB 同口径：
// shared（默认）= 全局图谱空间；kb = 按 KB 隔离（WS6 形态）。
//
// M6-4 WS4.3 per-KB 化（用户决策：切档走前端设置，docs/10 §6.3 「不迁」原
// 决议被推翻）——优先级：KB 设置 > 系统 env > 默认 shared；显式 shared 优先
// 于 env，避免「系统默认升 kb 后老 KB 误切」。
func graphWorkspaceForKBHandler(kb *types.KnowledgeBase) string {
	if kb != nil && kb.GraphConfig != nil {
		switch kb.GraphConfig.WorkspaceMode {
		case "kb":
			return kb.ID
		case "shared":
			return ""
			// "" / 未知值 = 跟随系统
		}
	}
	if os.Getenv("STARKB_GRAPH_WORKSPACE_MODE") == "kb" && kb != nil {
		return kb.ID
	}
	return ""
}

// graphWorkspaceModeForKB 报告 KB 的有效切档模式（KB 设置或回退 env），
// 供 /graph/status 端点如实返回「当前这个 KB 实际走哪种图谱空间」。
func graphWorkspaceModeForKB(kb *types.KnowledgeBase) string {
	if kb != nil && kb.GraphConfig != nil && kb.GraphConfig.WorkspaceMode != "" {
		return kb.GraphConfig.WorkspaceMode
	}
	if os.Getenv("STARKB_GRAPH_WORKSPACE_MODE") == "kb" {
		return "kb"
	}
	return "shared"
}

// GetKnowledgeBaseGraphStatus GET /knowledge-bases/:id/graph/status
func (h *KnowledgeBaseHandler) GetKnowledgeBaseGraphStatus(c *gin.Context) {
	kb, _, _, _, err := h.validateAndGetKnowledgeBase(c)
	if err != nil {
		c.Error(err)
		return
	}

	out := gin.H{
		"graph_config":   kb.GraphConfig,
		"workspace_mode": graphWorkspaceModeForKB(kb),
	}

	starkbURL := os.Getenv("STARKB_API_URL")
	if starkbURL == "" {
		out["graph"] = gin.H{"available": false, "reason": "STARKB_API_URL 未配置"}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
		return
	}

	url := starkbURL + "/graph/status?tenant_id=" +
		strconv.FormatUint(kb.TenantID, 10) +
		"&workspace=" + graphWorkspaceForKBHandler(kb)
	ctx, cancel := context.WithTimeout(c.Request.Context(), graphStatusProxyTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		out["graph"] = gin.H{"available": false, "reason": err.Error()}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Warnf(c.Request.Context(), "graph status proxy: %v", err)
		out["graph"] = gin.H{"available": false, "reason": "starkb-api 不可达"}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil || resp.StatusCode != http.StatusOK {
		out["graph"] = gin.H{"available": false, "reason": "starkb-api 返回异常"}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
		return
	}
	var graph map[string]any
	if err := json.Unmarshal(body, &graph); err != nil {
		out["graph"] = gin.H{"available": false, "reason": "响应解析失败"}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
		return
	}
	out["graph"] = graph
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// postStarkbGraph 把 JSON 载荷转发给 starkb-api 并解回 map。
// 返回的 reason 非空表示降级（未配置/不可达/非 200/解析失败），由调用方决定呈现方式。
func postStarkbGraph(ctx context.Context, path string, payload any) (map[string]any, string) {
	base := os.Getenv("STARKB_API_URL")
	if base == "" {
		return nil, "STARKB_API_URL 未配置"
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err.Error()
	}
	ctx, cancel := context.WithTimeout(ctx, graphStatusProxyTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "starkb-api 不可达"
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, "starkb-api 返回异常"
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, "响应解析失败"
	}
	return out, ""
}

// ownedGraphDocIDs 把请求里的 id 收敛到「该 KB 且该租户」真实拥有的文档。
//
// 不信任客户端传来的 id 是必须的：starkb-api 的 graph_doc_state 以 knowledge_id
// 为唯一键、查询不按租户过滤（对账 CLI 写入的行 tenant_id/kb_id 为空，数据层也
// 没法过滤）。若原样转发，任何有 KB 读权限的人都能拿别人的 doc id 探到其图谱
// 状态与 last_error —— 而 doc id 里存在「工商变更通知-2.27」这类可猜的中文名，
// 并非全是 UUID。
//
// 用按 id 点查（GetKnowledgeByID 本身按租户过滤，跨租户返回 NotFound）而不是
// 拉全 KB 文档列表：成本随页大小增长，不随 KB 规模增长。
func ownedGraphDocIDs(
	ctx context.Context, h *KnowledgeBaseHandler, kb *types.KnowledgeBase, requested []string,
) []string {
	if len(requested) == 0 {
		return nil
	}
	if len(requested) > graphDocStatusMaxIDs {
		requested = requested[:graphDocStatusMaxIDs]
	}
	seen := make(map[string]struct{}, len(requested))
	uniq := make([]string, 0, len(requested))
	for _, id := range requested {
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	if h.knowledgeService == nil {
		// 测试装配下可能没有 knowledgeService；此处不 fail-open 到「全放行」，
		// 而是按调用方自己给的 id 走 —— 与旧行为一致，生产装配始终非 nil。
		return uniq
	}
	out := make([]string, 0, len(uniq))
	for _, id := range uniq {
		k, err := h.knowledgeService.GetKnowledgeByID(ctx, id)
		if err != nil || k == nil || k.KnowledgeBaseID != kb.ID {
			continue
		}
		out = append(out, id)
	}
	return out
}

// GetKnowledgeBaseGraphDocStatus POST /knowledge-bases/:id/graph/doc-status
//
// ADR-008 决策 4：文档列表页每行的图谱状态徽标。
//
// 为什么经 Go 代理，而不是按 ADR 字面让浏览器直连 starkb-api：
// starkb-api 只监听 127.0.0.1:8300 且**没有任何认证**（/graph/docs/delete 还是
// 破坏性端点），直连要求把它绑到 0.0.0.0 并开 CORS —— 那是把控制面暴露给浏览器。
// ADR 真正要守的是「不把跨服务调用塞进 WeKnora 的列表热路径」，本方案完全满足：
// 列表接口一行没改，前端拿到列表后自己批量补一次状态。
func (h *KnowledgeBaseHandler) GetKnowledgeBaseGraphDocStatus(c *gin.Context) {
	kb, _, _, _, err := h.validateAndGetKnowledgeBase(c)
	if err != nil {
		_ = c.Error(err)
		return
	}
	var in struct {
		KnowledgeIDs []string `json:"knowledge_ids"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		_ = c.Error(apperrors.NewBadRequestError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	out := gin.H{"states": gin.H{}, "details": gin.H{}, "available": true}
	ids := ownedGraphDocIDs(c.Request.Context(), h, kb, in.KnowledgeIDs)
	if len(ids) == 0 {
		// 没有一个 id 属于这个 KB：直接回空，不打 starkb-api。
		c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
		return
	}

	graph, reason := postStarkbGraph(c.Request.Context(), "/graph/doc-status", gin.H{
		"knowledge_ids": ids,
	})
	if reason != "" {
		logger.Warnf(c.Request.Context(), "graph doc-status proxy: %s", reason)
		out["available"] = false
		out["reason"] = reason
		c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
		return
	}
	if v, ok := graph["states"]; ok {
		out["states"] = v
	}
	if v, ok := graph["details"]; ok {
		out["details"] = v
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// RetryKnowledgeBaseGraphDocs POST /knowledge-bases/:id/graph/doc-retry
//
// ADR-008 决策 4：失败徽标点开后的「重试」入口。复用 starkb-api 的
// /graph/backfill —— 把文档重新标成 pending，后台 worker 会捡起来重建。
func (h *KnowledgeBaseHandler) RetryKnowledgeBaseGraphDocs(c *gin.Context) {
	kb, _, _, _, err := h.validateAndGetKnowledgeBase(c)
	if err != nil {
		_ = c.Error(err)
		return
	}
	var in struct {
		KnowledgeIDs []string `json:"knowledge_ids"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		_ = c.Error(apperrors.NewBadRequestError("Invalid request parameters").WithDetails(err.Error()))
		return
	}
	ids := ownedGraphDocIDs(c.Request.Context(), h, kb, in.KnowledgeIDs)
	if len(ids) == 0 {
		_ = c.Error(apperrors.NewBadRequestError("no valid knowledge ids"))
		return
	}
	res, reason := postStarkbGraph(c.Request.Context(), "/graph/backfill", gin.H{
		"tenant_id": strconv.FormatUint(kb.TenantID, 10),
		"kb_id":     kb.ID,
		"doc_ids":   ids,
		"workspace": graphWorkspaceForKBHandler(kb),
	})
	if reason != "" {
		logger.Warnf(c.Request.Context(), "graph doc-retry proxy: %s", reason)
		_ = c.Error(apperrors.NewInternalServerError(reason))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": res})
}

// GetKnowledgeBaseGraphView GET /knowledge-bases/:id/graph/view
// M5-1：返回 KB 图谱可视化数据（节点+边），Go 代理 starkb-api /graph/view。
//
// M6-4 WS4.4：shared 模式（workspace=""）下数据面是**全局图**，而本路由只要求
// 该 KB 的读权限——不过滤就是越权泄漏（他 KB 的实体、描述、证据来源全可见）。
// 处置选「过滤」而非拒绝服务：把 KB 文档清单带给数据面（doc_ids 参数），
// kb 模式下 workspace 隔离已生效，不带该参数、行为不变。
func (h *KnowledgeBaseHandler) GetKnowledgeBaseGraphView(c *gin.Context) {
	kb, _, _, _, err := h.validateAndGetKnowledgeBase(c)
	if err != nil {
		c.Error(err)
		return
	}
	starkbURL := os.Getenv("STARKB_API_URL")
	if starkbURL == "" {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"available": false, "reason": "STARKB_API_URL 未配置"}})
		return
	}
	ws := graphWorkspaceForKBHandler(kb)
	viewURL := starkbURL + "/graph/view?workspace=" + ws + "&limit=300"
	if ws == "" {
		if ids := h.ownedKBDocIDsForGraph(c.Request.Context(), kb.ID); len(ids) > 0 {
			viewURL += "&doc_ids=" + url.QueryEscape(strings.Join(ids, ","))
		}
	}
	// P0（图谱浏览器规划 2026-09-25）：ego/types 参数透传。非法值直接忽略
	// （starkb-api 侧亦兜底），这里只做枚举与长度边界控制防滥用。
	if c.Query("mode") == "ego" {
		viewURL += "&mode=ego"
		if center := strings.TrimSpace(c.Query("center")); center != "" &&
			len([]rune(center)) <= graphEntityNameMaxRunes {
			viewURL += "&center=" + url.QueryEscape(center)
		}
		if d, err := strconv.Atoi(c.Query("depth")); err == nil && d >= 1 && d <= 3 {
			viewURL += "&depth=" + strconv.Itoa(d)
		}
	}
	if types := strings.TrimSpace(c.Query("types")); types != "" && len(types) <= 256 {
		viewURL += "&types=" + url.QueryEscape(types)
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, viewURL, nil)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"available": false, "reason": err.Error()}})
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"available": false, "reason": "starkb-api 不可达"}})
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil || resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"available": false, "reason": "starkb-api 返回异常"}})
		return
	}
	var graph map[string]any
	if err := json.Unmarshal(body, &graph); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"available": false, "reason": "响应解析失败"}})
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// graphEntityNameMaxRunes 实体名长度上限。实体名是 LLM 抽取的自由文本，但它是
// 查询条件而非常量级载荷；512 个字符足够容纳真实实体（最长实测为机构全称）。
const graphEntityNameMaxRunes = 512

// GetKnowledgeBaseGraphEntity GET /knowledge-bases/:id/graph/entity?name=
//
// M6-1 WS1.2：图谱浏览器点节点后的下钻数据（实体 + 邻居 + 可点开的证据）。
//
// 为什么不像 /graph/view 那样在 handler 里直连 starkb-api：证据必须**回跳**成
// WeKnora 子 chunk（图谱 chunk key → 契约锚点 → 子 chunk），这需要 ChunkRepository
// 与租户上下文，属于 service 层职责；handler 只做参数收口与权限（路由层
// KBAccessRead）。回跳同时是权限边界：共享图谱空间下会返回别的 KB 的实体，
// service 按本 KB 文档范围过滤后才可能出现在证据列表里。
func (h *KnowledgeBaseHandler) GetKnowledgeBaseGraphEntity(c *gin.Context) {
	kb, _, _, _, err := h.validateAndGetKnowledgeBase(c)
	if err != nil {
		_ = c.Error(err)
		return
	}
	name := strings.TrimSpace(c.Query("name"))
	if name == "" {
		_ = c.Error(apperrors.NewBadRequestError("name is required"))
		return
	}
	if len([]rune(name)) > graphEntityNameMaxRunes {
		_ = c.Error(apperrors.NewBadRequestError("name too long"))
		return
	}
	data, err := h.service.GraphEntityDetail(c.Request.Context(), kb.ID, name)
	if err != nil {
		_ = c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// GetKnowledgeBaseGraphEntitySearch GET /knowledge-bases/:id/graph/entity/search?q=
//
// P0-1（图谱浏览器规划 2026-09-25）：实体名搜索——图谱浏览器搜索框与
// 「万物可达 pivot」（搜索命中不在当前子图时自动切 ego 视图）的数据源。
// 越权收口与 /graph/view 相同：shared 模式注入本 KB 文档清单，越权实体不可见。
func (h *KnowledgeBaseHandler) GetKnowledgeBaseGraphEntitySearch(c *gin.Context) {
	kb, _, _, _, err := h.validateAndGetKnowledgeBase(c)
	if err != nil {
		_ = c.Error(err)
		return
	}
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		_ = c.Error(apperrors.NewBadRequestError("q is required"))
		return
	}
	if len([]rune(q)) > graphEntityNameMaxRunes {
		_ = c.Error(apperrors.NewBadRequestError("q too long"))
		return
	}
	starkbURL := os.Getenv("STARKB_API_URL")
	if starkbURL == "" {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"available": false, "reason": "STARKB_API_URL 未配置"}})
		return
	}
	ws := graphWorkspaceForKBHandler(kb)
	searchURL := starkbURL + "/graph/entity/search?workspace=" + ws +
		"&q=" + url.QueryEscape(q) + "&limit=20"
	if ws == "" {
		if ids := h.ownedKBDocIDsForGraph(c.Request.Context(), kb.ID); len(ids) > 0 {
			searchURL += "&doc_ids=" + url.QueryEscape(strings.Join(ids, ","))
		}
	}
	if types := strings.TrimSpace(c.Query("types")); types != "" && len(types) <= 256 {
		searchURL += "&types=" + url.QueryEscape(types)
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"available": false, "reason": err.Error()}})
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"available": false, "reason": "starkb-api 不可达"}})
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil || resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"available": false, "reason": "starkb-api 返回异常"}})
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// GetKnowledgeBaseGraphCharts GET /knowledge-bases/:id/graph/charts?doc_ids=a,b
//
// P1-8（图谱浏览器规划 2026-09-25）：实体/证据下钻面板的图表资产联动。
// 图表资产在 starkb-api chart_assets 表（解析管线注册），starkb-api 无认证且
// 只监听内网，必须经本代理：先校验每个 doc_id 归属本 KB（shared 图谱空间下
// doc_id 可伪造，图表标题/数据点也是不能越权泄漏的信息），再逐 doc 转发
// starkb-api /charts/assets。任一 doc 失败降级跳过，不阻断整体。
func (h *KnowledgeBaseHandler) GetKnowledgeBaseGraphCharts(c *gin.Context) {
	kb, _, _, _, err := h.validateAndGetKnowledgeBase(c)
	if err != nil {
		_ = c.Error(err)
		return
	}
	starkbURL := os.Getenv("STARKB_API_URL")
	if starkbURL == "" {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"available": false, "assets": []any{}}})
		return
	}
	requested := strings.Split(c.Query("doc_ids"), ",")
	owned := h.ownedKBDocIDsForGraph(c.Request.Context(), kb.ID)
	ownedSet := make(map[string]struct{}, len(owned))
	for _, id := range owned {
		ownedSet[id] = struct{}{}
	}
	// 保持请求顺序、去重、越权丢弃，上限与图浏览器的证据量级对齐。
	docIDs := make([]string, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, id := range requested {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		if _, ok := ownedSet[id]; !ok {
			continue
		}
		docIDs = append(docIDs, id)
		if len(docIDs) >= 10 {
			break
		}
	}
	assets := make([]map[string]any, 0)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	for _, docID := range docIDs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			starkbURL+"/charts/assets?tenant_id="+url.QueryEscape(strconv.FormatUint(kb.TenantID, 10))+
				"&doc_id="+url.QueryEscape(docID)+"&limit=20", nil)
		if err != nil {
			continue
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}
		var parsed struct {
			Assets []map[string]any `json:"assets"`
		}
		if json.Unmarshal(body, &parsed) != nil {
			continue
		}
		for _, a := range parsed.Assets {
			a["doc_id"] = docID
			assets = append(assets, a)
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"available": true, "assets": assets}})
}

// GetKnowledgeBaseGraphEntityMerge POST /knowledge-bases/:id/graph/entity/merge
//
// P2-12（图谱浏览器规划 2026-09-25）：人工修图——实体合并/改名。写操作：
// OwnedKBOrAdmin + KBAccessWrite（路由层挂）。shared 模式的图是全局的，
// 归属校验（source/target 实体须有指向本 KB 文档的证据）在 starkb-api 侧做，
// owned doc 清单由本层注入——与读路径的越权收口同一口径。
func (h *KnowledgeBaseHandler) GetKnowledgeBaseGraphEntityMerge(c *gin.Context) {
	kb, _, _, _, err := h.validateAndGetKnowledgeBase(c)
	if err != nil {
		_ = c.Error(err)
		return
	}
	var in struct {
		SourceIDs []string `json:"source_ids"`
		TargetID  string   `json:"target_id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		_ = c.Error(apperrors.NewBadRequestError("invalid body: " + err.Error()))
		return
	}
	in.SourceIDs = trimGraphEntityNames(in.SourceIDs)
	in.TargetID = strings.TrimSpace(in.TargetID)
	if len(in.SourceIDs) == 0 || in.TargetID == "" {
		_ = c.Error(apperrors.NewBadRequestError("source_ids and target_id are required"))
		return
	}
	for _, n := range append(append([]string{}, in.SourceIDs...), in.TargetID) {
		if len([]rune(n)) > graphEntityNameMaxRunes {
			_ = c.Error(apperrors.NewBadRequestError("entity name too long"))
			return
		}
	}
	starkbURL := os.Getenv("STARKB_API_URL")
	if starkbURL == "" {
		c.JSON(http.StatusOK, gin.H{"available": false, "reason": "STARKB_API_URL 未配置"})
		return
	}
	payload := map[string]any{
		"tenant_id":  strconv.FormatUint(kb.TenantID, 10),
		"kb_id":      kb.ID,
		"workspace":  graphWorkspaceForKBHandler(kb),
		"source_ids": in.SourceIDs,
		"target_id":  in.TargetID,
		"owned_doc_ids": func() []string {
			ids := h.ownedKBDocIDsForGraph(c.Request.Context(), kb.ID)
			if ids == nil {
				ids = []string{}
			}
			return ids
		}(),
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"available": false, "reason": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		starkbURL+"/graph/entity/merge", bytes.NewReader(buf))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"available": false, "reason": err.Error()})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"available": false, "reason": "starkb-api 不可达"})
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil || resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusOK, gin.H{"available": false, "reason": "starkb-api 返回异常"})
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// trimGraphEntityNames 去空白、丢空项（合并来源清单的入口归一）。
func trimGraphEntityNames(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// GetKnowledgeBaseGraphEdge GET /knowledge-bases/:id/graph/edge?source=&target=
// M6-1 WS1.2：点边下钻。实体的下钻证据是「节点 ∪ 全部邻居」的合并集，答不了
// 「**这条**关系是从哪句话抽出来的」，所以边需要单独的入口。
func (h *KnowledgeBaseHandler) GetKnowledgeBaseGraphEdge(c *gin.Context) {
	kb, _, _, _, err := h.validateAndGetKnowledgeBase(c)
	if err != nil {
		_ = c.Error(err)
		return
	}
	source := strings.TrimSpace(c.Query("source"))
	target := strings.TrimSpace(c.Query("target"))
	if source == "" || target == "" {
		_ = c.Error(apperrors.NewBadRequestError("source and target are required"))
		return
	}
	if len([]rune(source)) > graphEntityNameMaxRunes || len([]rune(target)) > graphEntityNameMaxRunes {
		_ = c.Error(apperrors.NewBadRequestError("source or target too long"))
		return
	}
	data, err := h.service.GraphEdgeDetail(c.Request.Context(), kb.ID, source, target)
	if err != nil {
		_ = c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// graphExemptDoc 无文件语义的知识（粘贴文本等）没有契约目录，从不进图谱。
// 这是入队侧的豁免口径（knowledge_graph_ingest.go 的 FileName == "" 跳过分支），
// 覆盖率的分母必须用同一把尺子，否则粘贴类文档会被算成「未覆盖」。
func graphExemptDoc(k *types.Knowledge) bool {
	return k == nil || k.ID == "" || k.FileName == ""
}

// countGraphEligibleDocs 统计 KB 内参与建图的文档数与豁免数。
func countGraphEligibleDocs(docs []*types.Knowledge) (total, exempt int) {
	for _, d := range docs {
		if graphExemptDoc(d) {
			exempt++
		}
	}
	return len(docs), exempt
}

// GetKnowledgeBaseGraphCoverage GET /knowledge-bases/:id/graph/coverage
//
// M6-1 WS1.5：KB 设置页覆盖进度条 + 图谱页「未覆盖」标注的数据源。
//
// 两边的数据在这一层合流：starkb-api /graph/coverage 给状态计数（它的
// graph_doc_state 只有进过建图队列的文档），WeKnora 侧补上**分母**——KB 文档总数
// 与粘贴类豁免数（D3 口径）。没有分母，"ready: 12" 无法回答「覆盖了多少」。
func (h *KnowledgeBaseHandler) GetKnowledgeBaseGraphCoverage(c *gin.Context) {
	kb, _, _, _, err := h.validateAndGetKnowledgeBase(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	out := gin.H{"available": false}
	if h.knowledgeService != nil {
		docs, err := h.knowledgeService.ListKnowledgeByKnowledgeBaseID(c.Request.Context(), kb.ID)
		if err != nil {
			logger.Warnf(c.Request.Context(), "graph coverage: 列 KB %s 文档失败: %v", kb.ID, err)
		}
		total, exempt := countGraphEligibleDocs(docs)
		out["total_docs"] = total
		out["exempt_manual"] = exempt
		out["eligible"] = total - exempt

		// M6-1 后扩展：KB 解析引擎画像（仅 starkb 引擎的产物才会写图谱契约）。
		// 出两条信息供前端：
		//   parser_engine_consistent=true：所有规则都锁定 starkb
		//   parser_engine_consistent=false：覆盖不全或为默认（非 starkb）
		// unsupported_count 是 eligible_docs 中 inferred engine != starkb 的数量；
		// 用解析引擎默认 + KB 规则合并推断（与 docparser 侧 ResolveParserEngine
		// 同口径，避免出现「前端以为 ok 后端判 fail」的不一致）。
		unsupported := countUnsupportedEngineDocs(docs, kb.ChunkingConfig)
		if unsupported >= 0 {
			out["unsupported_engine_count"] = unsupported
		}
		out["parser_engine_consistent"] = isStarkbConsistent(kb.ChunkingConfig)
	}

	starkbURL := os.Getenv("STARKB_API_URL")
	if starkbURL == "" {
		out["reason"] = "STARKB_API_URL 未配置"
		c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
		return
	}

	// starkb-api /graph/coverage：graph_doc_state 状态计数（ready/pending/
	// building/failed），是前端覆盖进度条的 ready 分子。失败不阻断响应——
	// 分子缺失时 coverageSummary 退化为「全部未覆盖」，分母（本侧已填）仍在。
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		starkbURL+"/graph/coverage?kb_id="+url.QueryEscape(kb.ID), nil)
	if err != nil {
		out["reason"] = err.Error()
		c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		out["reason"] = "starkb-api 不可达"
		c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil || resp.StatusCode != http.StatusOK {
		out["reason"] = "starkb-api 返回异常"
		c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
		return
	}
	var cov map[string]any
	if err := json.Unmarshal(body, &cov); err != nil {
		out["reason"] = "响应解析失败"
		c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
		return
	}
	out["coverage"] = cov
	out["available"] = true
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// isStarkbConsistent 判断 KB 的 chunking_config.parser_engine_rules 是否
// 一致锁定到 StarKB 引擎。docs/11 §3：仅 starkb 引擎才会写图谱契约；其它
// 引擎（builtin / simple / anydoc / mineru / mineru_cloud / paddleocr_vl /
// _cloud）解析完成的文档不会进入建图管线。
//
// 判定：
// - 没有 ParserEngineRules：视为使用默认引擎 ≠ starkb，返回 false；
// - 存在规则但全部 Engine == "starkb"：返回 true；
// - 存在规则但 Engine 不是 starkb：false（用户决策"全部用 starkb"的可视化）；
// - 混合（部分 starkb 部分不是）：false 并附汇总由前端呈现。
func isStarkbConsistent(cfg types.ChunkingConfig) bool {
	if len(cfg.ParserEngineRules) == 0 {
		return false
	}
	for _, r := range cfg.ParserEngineRules {
		if r.Engine != docparser.StarkbEngineName {
			return false
		}
	}
	return true
}

// countUnsupportedEngineDocs 统计 KB 内 inferred engine != starkb 的
// eligible_docs 数（粘贴类豁免的不计入）。与 docparser.engines 中
// ResolveParserEngine 同口径：若 KB 有规则先按规则，否则用全局默认。
// 返回 -1 时表示无法推断（缺 file_type 字段等），调用方按 -1 不出键即可。
func countUnsupportedEngineDocs(docs []*types.Knowledge, cfg types.ChunkingConfig) int {
	n := 0
	for _, d := range docs {
		if graphExemptDoc(d) {
			continue
		}
		engine := cfg.ResolveParserEngine(d.FileType)
		if engine != docparser.StarkbEngineName {
			n++
		}
	}
	return n
}
