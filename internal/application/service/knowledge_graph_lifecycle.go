package service

// ADR-008 决策 3.4（off→on 边沿）/ 3.6（删除清理）· 图谱生命周期钩子。
//
// M4 R1 已实现决策 3.3/3.4/3.5/3.7 的**主干**：graph_doc_state 文档级状态机、
// 补齐 worker（按配额从 pending 取数）、/graph/backfill 与 /graph/coverage 端点、
// 存量对账 CLI。本文件补的是它没覆盖的两条边：
//
//   1. 决策 3.4 的**触发边沿**。M4 的 GraphBuildOnIngest 挂在"解析完成"事件上，
//      只有开关当时是开的才会置 pending。而 ADR 要补的核心场景恰恰相反——
//      **存量 KB 默认关闭，用户后来才打开**：此时库里已有的文档永远不会被投喂，
//      开关看起来生效了、图谱却始终是空的。这里在 KB 配置更新时识别 off→on 边沿，
//      把既有文档一次性置 pending，交给 M4 的补齐 worker 按限速消费。
//
//   2. 决策 3.6 删除清理。M4 完全没有删除侧：文档在 WeKnora 侧删掉后，LightRAG
//      里的 chunk/实体/关系还在，图谱通道会召回已删文档的 chunk，而 A0 的锚点
//      回跳按 doc_id 反查契约目录会落空——用户看到一个**点不开的坏引用**。
//
// 两者都是 best-effort：绝不阻断主链路，失败只记日志（与 A1 钩子同款）。

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
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

const graphLifecycleTimeout = 30 * time.Second

// graphWorkspaceForKBID 与 M4 的 graphWorkspaceForKB 同口径，但只吃 id。
func graphWorkspaceForKBID(kbID string) string {
	if os.Getenv("STARKB_GRAPH_WORKSPACE_MODE") == "kb" && kbID != "" {
		return kbID
	}
	return ""
}

// postStarkbGraph 向 starkb-api 投递一个图谱意图（best-effort）。
func postStarkbGraph(ctx context.Context, path string, payload any) map[string]any {
	base := os.Getenv("STARKB_API_URL")
	if base == "" {
		return nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		logger.Warnf(ctx, "graph lifecycle: marshal %s: %v", path, err)
		return nil
	}
	client := &http.Client{Timeout: graphLifecycleTimeout}
	resp, err := client.Post(base+path, "application/json", bytes.NewReader(body))
	if err != nil {
		logger.Warnf(ctx, "graph lifecycle: 调用 starkb-api %s 失败: %v", path, err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logger.Warnf(ctx, "graph lifecycle: starkb-api %s 返回 %d", path, resp.StatusCode)
		return nil
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		logger.Warnf(ctx, "graph lifecycle: 解析 %s 响应失败: %v", path, err)
		return nil
	}
	return out
}

// graphCleanupOnDeleteEnabled 删除清理开关。默认随 STARKB_API_URL 启用——
// 图谱里有内容就该在文档删除时清掉，这与"是否自动建图"是两件事（决策 3.1）。
// 开关本体读 starkb.graph_cleanup_on_delete 系统设置（DB > ENV > 默认 true；
// 迁移前是 STARKB_GRAPH_CLEANUP_ON_DELETE != "false" 的反向默认语义）。
func (s *knowledgeService) graphCleanupOnDeleteEnabled(ctx context.Context) bool {
	if os.Getenv("STARKB_API_URL") == "" || s.settings == nil {
		return false
	}
	return s.settings.GetBool(ctx,
		types.SettingKeyStarkbGraphCleanupOnDelete, types.SettingEnvStarkbGraphCleanupOnDelete, true)
}

// GraphCleanupOnDelete 文档删除后登记图谱清理（ADR-008 决策 3.6）。
//
// 挂载点：executeKnowledgeDelete（单删/批删的唯一漏斗），与
// cleanupWikiOnKnowledgeDelete 并列。只登记墓碑不等结果——清理由 starkb-api 的
// GraphCleanupService 异步消化（失败留墓碑重试），不占删除事务的时间。
func (s *knowledgeService) GraphCleanupOnDelete(ctx context.Context, knowledgeList []*types.Knowledge) {
	if !s.graphCleanupOnDeleteEnabled(ctx) || len(knowledgeList) == 0 {
		return
	}
	tenantInfo, ok := types.TenantInfoFromContext(ctx)
	if !ok || tenantInfo == nil {
		return
	}

	// 按 KB 分组：workspace 是 KB 维度的（WS6 形态），一次请求只对应一个 KB。
	byKB := map[string][]string{}
	for _, k := range knowledgeList {
		if k == nil || k.ID == "" || k.FileName == "" {
			// 无文件语义的知识（粘贴文本等）从未进过图谱，跳过
			continue
		}
		byKB[k.KnowledgeBaseID] = append(byKB[k.KnowledgeBaseID], k.ID)
	}

	for kbID, docIDs := range byKB {
		out := postStarkbGraph(ctx, "/graph/docs/delete", map[string]any{
			"tenant_id": strconv.FormatUint(tenantInfo.ID, 10),
			"kb_id":     kbID,
			"workspace": graphWorkspaceForKBID(kbID),
			"doc_ids":   docIDs,
		})
		if out == nil {
			logger.Warnf(ctx, "graph cleanup: 文档 %v 的图谱清理未登记（starkb-api 不可达）", docIDs)
			continue
		}
		logger.Infof(ctx, "graph cleanup: KB %s 的 %d 篇文档已登记图谱清理（待清理 %v）",
			kbID, len(docIDs), out["pending"])
	}
}

// GraphCleanupOnKBDelete 整库删除时登记该 KB 全部文档的图谱清理。
//
// 此前 KB 删除只清理了内置图谱引擎（graphEngine.DelGraph，D2 已废弃），
// LightRAG 侧的 graph_doc_state 行与 chunk/实体/向量全部残留——删库重建后
// 陈旧状态会以幽灵行的形式污染新图谱（用户实测）。与单篇删除共用 starkb-api
// 墓碑机制（异步消化，失败留墓碑重试）；best-effort，失败仅告警。
//
// 必须在知识条目尚未软删时调用（需要 FileName 判定「进过图谱」的文档）。
func GraphCleanupOnKBDelete(ctx context.Context, tenantID uint64, kbID string,
	knowledgeList []*types.Knowledge) {
	if os.Getenv("STARKB_API_URL") == "" || kbID == "" {
		return
	}
	docIDs := make([]string, 0, len(knowledgeList))
	for _, k := range knowledgeList {
		if k != nil && k.ID != "" && k.FileName != "" {
			docIDs = append(docIDs, k.ID)
		}
	}
	if len(docIDs) == 0 {
		return
	}
	out := postStarkbGraph(ctx, "/graph/docs/delete", map[string]any{
		"tenant_id": strconv.FormatUint(tenantID, 10),
		"kb_id":     kbID,
		"workspace": graphWorkspaceForKBID(kbID),
		"doc_ids":   docIDs,
	})
	if out == nil {
		logger.Warnf(ctx, "graph cleanup: KB %s 整库清理未登记（starkb-api 不可达，%d 篇）",
			kbID, len(docIDs))
		return
	}
	logger.Infof(ctx, "graph cleanup: KB %s 整库删除已登记 %d 篇图谱清理", kbID, len(docIDs))
}

// graphAutoBuildTurnedOn 判断图谱自动建图开关是否发生 off→on 边沿。
//
// 抽成纯函数是因为这个判断只有两种错法，且都难在联调中发现：
// 判宽了（每次都算边沿）→ 每次保存 KB 设置就把全库文档重新置 pending，
// 触发一轮无谓的建图配额消耗；判窄了（边沿识别不到）→ ADR 决策 3.4 的
// 存量补齐永远不触发，开关打开后图谱一直是空的。
func graphAutoBuildTurnedOn(wasOn bool, kb *types.KnowledgeBase) bool {
	return !wasOn && kb != nil && kb.GraphConfig != nil && kb.GraphConfig.AutoBuild
}

// GraphBackfillOnEnable KB 自动建图开关从关到开时，为**既有**文档补齐图谱
// （ADR-008 决策 3.4）。
//
// 为什么必须有这个钩子：M4 的 GraphBuildOnIngest 挂在"解析完成"事件上，只在
// 开关当时为开时才置 pending。存量 KB 默认关闭、用户后来才打开时，库里已有的
// 文档永远不会被投喂——开关看起来生效了，图谱却一直是空的。
//
// 调用方（knowledgeBaseService.UpdateKnowledgeBase）负责判断 off→on 边沿：
// 只有边沿才该触发，否则每次保存 KB 设置都会把全库重新置 pending。
//
// 只置 pending、不立即全量建图：M4 的补齐 worker 按配额（默认 3 篇/小时）
// 串行消费，与 jobs 共用串行锁（ADR 3.4 的限速口径）。
func GraphBackfillOnEnable(ctx context.Context, repo interfaces.KnowledgeRepository,
	tenantID uint64, kbID string,
) {
	if os.Getenv("STARKB_API_URL") == "" || kbID == "" || repo == nil {
		return
	}
	docs, err := repo.ListKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID)
	if err != nil {
		logger.Warnf(ctx, "graph backfill: 列举 KB %s 文档失败: %v", kbID, err)
		return
	}
	docIDs := make([]string, 0, len(docs))
	for _, d := range docs {
		if d != nil && d.ID != "" && d.FileName != "" {
			docIDs = append(docIDs, d.ID)
		}
	}
	if len(docIDs) == 0 {
		logger.Infof(ctx, "graph backfill: KB %s 无可补齐文档", kbID)
		return
	}

	out := postStarkbGraph(ctx, "/graph/backfill", map[string]any{
		"tenant_id": strconv.FormatUint(tenantID, 10),
		"kb_id":     kbID,
		"workspace": graphWorkspaceForKBID(kbID),
		"doc_ids":   docIDs,
	})
	if out == nil {
		logger.Warnf(ctx, "graph backfill: KB %s 的补齐未登记（starkb-api 不可达）", kbID)
		return
	}
	logger.Infof(ctx, "graph backfill: KB %s 开关已开启，%d 篇既有文档置 pending（新增 %v，已就绪跳过 %v）",
		kbID, len(docIDs), out["marked"], out["skipped_ready"])
}
