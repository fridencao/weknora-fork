package session

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

// envOnlySettings 是只实现 GetBool 的设置服务替身：DB 层视为空，直接
// 回落 ENV > 默认。开关迁到 system_settings 后（DB > ENV > 默认），这些
// 用例仍用 t.Setenv 驱动，替身保证语义与迁移前一致。
// 其余方法由内嵌接口满足（这些用例不会调用到）。
type envOnlySettings struct{ interfaces.SystemSettingService }

func (envOnlySettings) GetBool(_ context.Context, _ string, envName string, def bool) bool {
	if v := strings.TrimSpace(os.Getenv(envName)); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func refWithContent(content string) *types.SearchResult {
	return &types.SearchResult{ID: content, Content: content}
}

func TestAuditMessageClaims_PersistsReportWhenEnabled(t *testing.T) {
	t.Setenv("STARKB_CLAIM_GATE", "true")

	msg := &types.Message{
		Content: "公司2024年营业收入为3218亿元。金融科技板块收入达到500亿元。",
		KnowledgeReferences: types.References{
			refWithContent("2024年公司营业收入为3,218亿元。"),
		},
	}

	AuditMessageClaims(context.Background(), envOnlySettings{}, msg)

	require.NotNil(t, msg.ClaimReport)
	require.Equal(t, 2, msg.ClaimReport.Total)
	require.Equal(t, 1, msg.ClaimReport.Grounded)
	require.Equal(t, 1, msg.ClaimReport.NumericMismatch) // "500" 无证据
}

func TestAuditMessageClaims_NoOpWhenDisabled(t *testing.T) {
	t.Setenv("STARKB_CLAIM_GATE", "false")

	msg := &types.Message{
		Content:             "一段足够长的答案文本，含论断。",
		KnowledgeReferences: types.References{refWithContent("证据内容")},
	}

	AuditMessageClaims(context.Background(), envOnlySettings{}, msg)

	require.Nil(t, msg.ClaimReport)
}

func TestAuditMessageClaims_NoOpWithoutEvidence(t *testing.T) {
	t.Setenv("STARKB_CLAIM_GATE", "true")

	msg := &types.Message{Content: "一段没有引用证据的答案。"}
	AuditMessageClaims(context.Background(), envOnlySettings{}, msg)
	require.Nil(t, msg.ClaimReport)

	empty := &types.Message{Content: "一段没有引用证据的答案。", KnowledgeReferences: types.References{{}}}
	AuditMessageClaims(context.Background(), envOnlySettings{}, empty)
	require.Nil(t, empty.ClaimReport)
}

func TestClaimReportSerializationRoundTrip(t *testing.T) {
	t.Setenv("STARKB_CLAIM_GATE", "true")

	msg := &types.Message{
		Content:             "净利润为125亿元。",
		KnowledgeReferences: types.References{refWithContent("净利润为125亿元。")},
	}
	AuditMessageClaims(context.Background(), envOnlySettings{}, msg)
	require.NotNil(t, msg.ClaimReport)

	// jsonb 往返：Value → Scan 还原等值报告
	v, err := msg.ClaimReport.Value()
	require.NoError(t, err)
	var back types.ClaimReport
	require.NoError(t, back.Scan(v))
	require.Equal(t, *msg.ClaimReport, back)
	require.NoError(t, back.Scan(nil)) // NULL → 零值，不报错
}
