package handler

// A3（docs/09 WS1.3）：KB 级 LightRAG 图谱状态代理。
//
// 前端 GraphSettings 页需要图谱健康度（实体/关系/chunk 计数、最近建图任务、
// 失败清单）。真实存储在 starkb-api 侧的 starkb_graph PG 库（A4），Go 侧只做
// 带权限的代理：复用 KBAccessRead 中间件（路由层挂），把 workspace 按
// STARKB_GRAPH_WORKSPACE_MODE 解析后转发 starkb-api /graph/status。
// 代理不可达时返回 degraded 数据（graph_config 仍可用），页面不空白。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

const graphStatusProxyTimeout = 10 * time.Second

// graphWorkspaceForKBHandler 与 service 侧 graphWorkspaceForKB 同口径：
// shared（默认）= 全局图谱空间；kb = 按 KB 隔离（WS6 形态）。
func graphWorkspaceForKBHandler(kb *types.KnowledgeBase) string {
	if os.Getenv("STARKB_GRAPH_WORKSPACE_MODE") == "kb" && kb != nil {
		return kb.ID
	}
	return ""
}

// GetKnowledgeBaseGraphStatus GET /knowledge-bases/:id/graph/status
func (h *KnowledgeBaseHandler) GetKnowledgeBaseGraphStatus(c *gin.Context) {
	kb, _, _, _, err := h.validateAndGetKnowledgeBase(c)
	if err != nil {
		c.Error(err)
		return
	}

	out := gin.H{
		"graph_config":       kb.GraphConfig,
		"workspace_mode":     map[string]string{"kb": "kb"}[os.Getenv("STARKB_GRAPH_WORKSPACE_MODE")],
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
