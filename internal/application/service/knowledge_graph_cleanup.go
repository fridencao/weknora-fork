package service

// ADR-008 §3.6 · 文档删除→图谱清理联动（docs/09 WS2.1）。
//
// 知识文档被删除时，通知 starkb-api 清理该文档在图谱中的全部数据
// （chunk 向量/实体/关系/KV/图节点/边），防止孤儿证据污染检索。
// 设计为 best-effort（不阻塞删除主链路），失败仅记日志。
//
// 开关：STARKB_GRAPH_CLEANUP_ON_DELETE=true 且 STARKB_API_URL 已配置。

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

const graphCleanupTimeout = 15 * time.Second

func graphCleanupOnDeleteEnabled() bool {
	return os.Getenv("STARKB_GRAPH_CLEANUP_ON_DELETE") == "true" && os.Getenv("STARKB_API_URL") != ""
}

// GraphCleanupOnDelete 在知识删除链中触发图谱数据清理。
func GraphCleanupOnDelete(ctx context.Context, knowledge *types.Knowledge) {
	if !graphCleanupOnDeleteEnabled() || knowledge == nil {
		return
	}
	if os.Getenv("STARKB_API_URL") == "" {
		return
	}

	payload, _ := json.Marshal(map[string]any{
		"doc_ids":   []string{knowledge.ID},
		"workspace": graphWorkspaceForKB(nil), // shared 模式 → default
	})
	url := os.Getenv("STARKB_API_URL") + "/graph/cleanup"
	client := &http.Client{Timeout: graphCleanupTimeout}
	resp, err := client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		logger.Warnf(ctx, "graph cleanup on delete: starkb-api 不可达（doc %s）: %v", knowledge.ID, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logger.Warnf(ctx, "graph cleanup on delete: starkb-api 返回 %d（doc %s）", resp.StatusCode, knowledge.ID)
		return
	}
	logger.Infof(ctx, "graph cleanup on delete: doc %s 图谱数据已清理", knowledge.ID)
}
