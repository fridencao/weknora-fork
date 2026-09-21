package session

// M3 G4（docs/07 WS4）：答案级论断审计的消息层持久化。
//
// completeAssistantMessage 在答案落库前调用 AuditMessageClaims：以消息携带的
// KnowledgeReferences（= 检索 SearchResult 全量，含证据 Content）为证据集跑
// chatpipeline.AnalyzeClaims，报告写入 Message.ClaimReport（jsonb），随消息
// 一起持久化并透出前端。开关与管线侧一致：STARKB_CLAIM_GATE=true。

import (
	"context"
	"os"
	"strings"

	chatpipeline "github.com/Tencent/WeKnora/internal/application/service/chat_pipeline"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// claimAuditEnabled 与管线侧 PluginClaimGate 共用开关。
func claimAuditEnabled() bool {
	return os.Getenv("STARKB_CLAIM_GATE") == "true"
}

// AuditMessageClaims 在消息落库前执行论断审计并写入 msg.ClaimReport。
// 无答案、无引用证据或开关关闭时为 no-op（报告保持 nil）。
func AuditMessageClaims(ctx context.Context, msg *types.Message) {
	if !claimAuditEnabled() || msg == nil {
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
