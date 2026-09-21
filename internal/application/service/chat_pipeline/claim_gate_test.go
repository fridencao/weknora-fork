package chatpipeline

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

func TestAnalyzeClaims_NumericCrossCheck(t *testing.T) {
	t.Parallel()

	evidence := []string{
		"2024年公司营业收入为3,218亿元，同比增长12.3%。",
		"零售业务贡献营收980亿元。",
	}
	answer := "公司2024年营业收入为3218亿元，同比增长12.3%。零售业务贡献营收980亿元。金融科技板块收入达到500亿元。"
	report := AnalyzeClaims(answer, evidence)

	require.Equal(t, 3, report.Total)
	require.Equal(t, 2, report.Grounded)             // 前两句数值均可复核
	require.Equal(t, 1, report.NumericMismatch)      // "500" 无证据
	require.Len(t, report.Claims[2].Numbers, 1)
	require.Equal(t, "500", report.Claims[2].Numbers[0])
	require.InDelta(t, 2.0/3.0, report.GroundingCoverage, 1e-9)
}

func TestAnalyzeClaims_TextOverlapGrounding(t *testing.T) {
	t.Parallel()

	evidence := []string{"该行持续推进零售转型战略，零售客户总数突破六千万户。"}
	grounded := AnalyzeClaims("该行持续推进零售转型战略，零售客户总数突破六千万户", evidence)
	require.Equal(t, 0, grounded.Ungrounded)

	off := AnalyzeClaims("量子计算机的纠错阈值决定了逻辑量子比特的规模上限", evidence)
	require.Equal(t, 1, off.Ungrounded)
	require.Equal(t, types.ClaimUngrounded, off.Claims[0].Verdict)
}

func TestAnalyzeClaims_YearExemptFromNumericCheck(t *testing.T) {
	t.Parallel()

	evidence := []string{"净利润为125亿元。"}
	// "2025" 为年份（1900-2099 豁免），核心数值 125 有据 → 不判失证。
	report := AnalyzeClaims("2025年净利润为125亿元。", evidence)
	require.Equal(t, 0, report.NumericMismatch)
}

func TestAnalyzeClaims_EmptyInputs(t *testing.T) {
	t.Parallel()

	require.Equal(t, 0, AnalyzeClaims("", []string{"x"}).Total)
	require.Equal(t, 0, AnalyzeClaims("有内容的一段话，足够长", nil).Total)
}

func TestAnalyzeClaims_ThousandSeparatorNormalization(t *testing.T) {
	t.Parallel()

	evidence := []string{"总资产 1,234.5 亿元。"}
	report := AnalyzeClaims("总资产为1234.5亿元", evidence)
	require.Equal(t, 0, report.NumericMismatch)
}
