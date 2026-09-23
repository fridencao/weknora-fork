package service

// A1 · 上传自动建图（docs/09 WS1.2；ADR-008 决策 3 定版形态）。
//
// 知识解析完成（post-process）后，若所属 KB 开启了建图开关
// （graph_config.auto_build，A3 页面控制），向 starkb-api /graph/backfill 置
// pending——由补齐 worker 按限速消费（ADR 决策 3.4：不立即入队，可反悔）。
// 设计沿 AlignProvenanceOnIngest 先例：best-effort、绝不阻断入库主链路；
// 文档级执行状态（pending/building/ready/failed/stale）与重试在 starkb-api
// graph_doc_state（决策 3.3），本钩子不做本地重试。
//
// workspace 路由（WS6 预留）：STARKB_GRAPH_WORKSPACE_MODE=kb 时按 KB 维度
// 建 workspace（=KB id），shared（默认）落到全局图谱空间，与存量混建数据同域。
//
// 开关：STARKB_GRAPH_ON_INGEST=true 且 STARKB_API_URL 已配置，且 KB 开关已启用。

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

const graphBuildOnIngestTimeout = 30 * time.Second

type graphBackfillRequest struct {
	TenantID  string            `json:"tenant_id"`
	KBID      string            `json:"kb_id"`
	DocIDs    []string          `json:"doc_ids"`
	Workspace string            `json:"workspace,omitempty"`
	// FileNames: knowledge_id → 原始文件名。starkb 链的契约目录名是
	// 「文件名主干-8hex」≠ knowledge_id——starkb-api 据此反查 jobs 表定位契约包。
	FileNames map[string]string `json:"file_names,omitempty"`
}

func graphBuildOnIngestEnabled() bool {
	return os.Getenv("STARKB_GRAPH_ON_INGEST") == "true" && os.Getenv("STARKB_API_URL") != ""
}

// graphWorkspaceForKB 决定该 KB 的图谱投喂落哪个 workspace。
// shared（默认）= 全局空间（与存量单图谱空间一致）；kb = 按 KB 隔离（WS6 形态）。
func graphWorkspaceForKB(kb *types.KnowledgeBase) string {
	if os.Getenv("STARKB_GRAPH_WORKSPACE_MODE") == "kb" && kb != nil {
		return kb.ID
	}
	return ""
}

// GraphBuildOnIngest 在 post-process 完成阶段触发 KB 开启的自动建图。
// knowledge 无契约语义（FileName 为空）或 KB 未开启时为 no-op。
func GraphBuildOnIngest(ctx context.Context, kb *types.KnowledgeBase,
	knowledge *types.Knowledge,
) {
	if !graphBuildOnIngestEnabled() || kb == nil || knowledge == nil {
		return
	}
	if kb.GraphConfig == nil || !kb.GraphConfig.AutoBuild {
		return
	}
	if knowledge.FileName == "" {
		// 无文件语义的知识（粘贴文本等）没有契约目录，跳过并计数
		return
	}
	if os.Getenv("STARKB_API_URL") == "" {
		return
	}

	req := graphBackfillRequest{
		TenantID:  strconv.FormatUint(kb.TenantID, 10),
		KBID:      kb.ID,
		DocIDs:    []string{knowledge.ID},
		FileNames: map[string]string{knowledge.ID: knowledge.FileName},
	}
	if ws := graphWorkspaceForKB(kb); ws != "" {
		req.Workspace = ws
	}
	body, err := json.Marshal(req)
	if err != nil {
		logger.Warnf(ctx, "graph on ingest: marshal request: %v", err)
		return
	}

	// ADR-008 决策 3.4 触发点 1：只置 pending，由 starkb-api 补齐 worker 按限速
	// 消费；backfill 入口统一幂等（已 ready 的文档自动跳过）。
	url := os.Getenv("STARKB_API_URL") + "/graph/backfill"
	client := &http.Client{Timeout: graphBuildOnIngestTimeout}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		logger.Warnf(ctx, "graph on ingest: 调用 starkb-api 失败（文档 %s 建图未触发）: %v",
			knowledge.ID, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logger.Warnf(ctx, "graph on ingest: starkb-api 返回 %d（文档 %s 建图未触发）",
			resp.StatusCode, knowledge.ID)
		return
	}
	var out struct {
		Marked       int `json:"marked"`
		SkippedReady int `json:"skipped_ready"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		logger.Warnf(ctx, "graph on ingest: 解析响应失败: %v", err)
		return
	}
	logger.Infof(ctx, "graph on ingest: knowledge %s（KB %s）已置 pending（补齐队列；ready 跳过 %d）",
		knowledge.ID, kb.ID, out.SkippedReady)
}
