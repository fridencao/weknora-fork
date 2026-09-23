package session

// M3 G4（docs/07 WS4）：答案级论断审计的消息层持久化。
//
// completeAssistantMessage 在答案落库前调用 AuditMessageClaims：以消息携带的
// KnowledgeReferences（= 检索 SearchResult 全量，含证据 Content）为证据集跑
// chatpipeline.AnalyzeClaims，报告写入 Message.ClaimReport（jsonb），随消息
// 一起持久化并透出前端。开关与管线侧一致：STARKB_CLAIM_GATE=true。

import (
	"context"
	"strings"

	chatpipeline "github.com/Tencent/WeKnora/internal/application/service/chat_pipeline"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// AuditMessageClaims 在消息落库前执行论断审计并写入 msg.ClaimReport。
// 无答案、无引用证据或开关关闭时为 no-op（报告保持 nil）。
//
// 开关经 chatpipeline.ClaimGateEnabled 解析（starkb.claim_gate 系统设置，
// DB > ENV > 默认），与生成链路上的 PluginClaimGate 共用同一口径——迁移前
// 两处各自直读 STARKB_CLAIM_GATE 环境变量，存在漂移风险。
func AuditMessageClaims(ctx context.Context, settings interfaces.SystemSettingService, msg *types.Message) {
	if !chatpipeline.ClaimGateEnabled(ctx, settings) || msg == nil {
		return
	}
	if strings.TrimSpace(msg.Content) == "" || len(msg.KnowledgeReferences) == 0 {
		return
	}
	evidence := make([]string, 0, len(msg.KnowledgeReferences))
	for _, r := range msg.KnowledgeReferences {
		if r != nil && r.Content != "" {
			evidence = append(evidence, r.Content)
		}
	}
	if len(evidence) == 0 {
		return
	}
	report := chatpipeline.AnalyzeClaims(msg.Content, evidence)
	if report.Total == 0 {
		return
	}
	msg.ClaimReport = &report
	logger.Infof(ctx, "claim audit: message %s 论断 %d，无据 %d，数字失证 %d，覆盖率 %.2f",
		msg.ID, report.Total, report.Ungrounded, report.NumericMismatch,
		report.GroundingCoverage)
}
