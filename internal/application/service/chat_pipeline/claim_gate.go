package chatpipeline

// WS3.3 完整版（M3 G4，docs/07 WS4）：答案文本级论断审计。
//
// 语义边界：流式场景答案文本已发出，"拦截"只能是事后审计——无证据论断在
// PipelineState.ClaimReport 打标并记录日志（强绑定的源头控制在
// PluginProvenanceGate 完成：无 L4 指针的 chunk 不进生成上下文）。
// 审计纯函数 AnalyzeClaims 独立于管线，消息保存层（后续接线）可直接复用。
//
// 开关：STARKB_CLAIM_GATE=true 启用；默认关闭不增加生成链路开销。

import (
	"context"
	"os"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// 数字与标点归一用的预编译正则（包级，避免热路径重复编译）。
var (
	numericToken = regexp.MustCompile(`-?\d[\d,]*(?:\.\d+)?`)
	thousandSep  = regexp.MustCompile(`,(\d{3})`)
)

// splitSentences 按中英文终止标点与换行切分答案为候选论断句。
func splitSentences(answer string) []string {
	fields := strings.FieldsFunc(answer, func(r rune) bool {
		switch r {
		case '。', '！', '？', '；', '\n', '\r', '.', '!', '?', ';':
			return true
		}
		return false
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if utf8.RuneCountInString(f) >= 6 { // 过短片段不构成论断
			out = append(out, f)
		}
	}
	return out
}

// normalizeNumeric 去千分位分隔符，便于跨格式数值比对（"3,218" ≍ "3218"）。
func normalizeNumeric(s string) string {
	return thousandSep.ReplaceAllString(s, "$1")
}

// bigramCoverage 句子在证据文本中的字符 2-gram 覆盖率（中文无分词的文本重叠度量）。
func bigramCoverage(sentence, evidence string) float64 {
	rs := []rune(sentence)
	if len(rs) < 2 {
		return 0
	}
	total, hit := 0, 0
	for i := 0; i+2 <= len(rs); i++ {
		total++
		if strings.Contains(evidence, string(rs[i:i+2])) {
			hit++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(hit) / float64(total)
}

// extractVerifiableNumbers 取句中数值（归一千分位），剔除年份（1900-2099 的 4 位整数
// 在证据中常以"2023年"形态出现且非数据论断核心，年份缺失不构成数字失证）。
func extractVerifiableNumbers(sentence string) []string {
	var out []string
	for _, m := range numericToken.FindAllString(sentence, -1) {
		n := normalizeNumeric(m)
		if len(n) == 4 && !strings.Contains(n, ".") && n >= "1900" && n <= "2099" {
			continue // 年份豁免
		}
		out = append(out, n)
	}
	return out
}

// AnalyzeClaims 论断审计纯函数。
//
// 判定规则（docs/07 WS4.1/4.2）：
//  1. 句含数值 → 数值必须逐一在证据文本中出现（千分位归一后子串匹配），
//     任一缺失即 numeric_mismatch（数字交叉校验失败）；
//  2. 句无数值 → 字符 2-gram 覆盖率 ≥0.50 视为有证据支撑，
//     <0.20 视为无证据论断（标"推断"候选），区间内为模糊带不判定。
func AnalyzeClaims(answer string, evidenceChunks []string) types.ClaimReport {
	report := types.ClaimReport{}
	if strings.TrimSpace(answer) == "" || len(evidenceChunks) == 0 {
		return report
	}
	evidence := strings.ToLower(strings.Join(evidenceChunks, "\n"))
	evidence = strings.ReplaceAll(evidence, ",", "") // 与数值归一同口径

	for _, s := range splitSentences(answer) {
		lower := strings.ToLower(s)
		report.Total++

		numbers := extractVerifiableNumbers(s)
		if len(numbers) > 0 {
			var missing []string
			for _, n := range numbers {
				if !strings.Contains(evidence, n) {
					missing = append(missing, n)
				}
			}
			if len(missing) > 0 {
				report.NumericMismatch++
				report.Claims = append(report.Claims, types.Claim{Text: s, Verdict: types.ClaimNumericMismatch, Numbers: missing})
				continue
			}
			report.Grounded++
			report.Claims = append(report.Claims, types.Claim{Text: s, Verdict: types.ClaimGroundedNumeric})
			continue
		}

		cov := bigramCoverage(lower, evidence)
		switch {
		case cov >= 0.50:
			report.Grounded++
			report.Claims = append(report.Claims, types.Claim{Text: s, Verdict: types.ClaimGrounded})
		case cov < 0.20 && hasContentLetters(lower):
			report.Ungrounded++
			report.Claims = append(report.Claims, types.Claim{Text: s, Verdict: types.ClaimUngrounded})
		default:
			// 模糊带：不标记（保守，避免误伤改写幅度大的正确表述）
			report.Grounded++
			report.Claims = append(report.Claims, types.Claim{Text: s, Verdict: types.ClaimGrounded})
		}
	}
	if report.Total > 0 {
		report.GroundingCoverage = float64(report.Total-report.Ungrounded-report.NumericMismatch) / float64(report.Total)
	}
	return report
}

func hasContentLetters(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// PluginClaimGate 答案论断审计插件（CHAT_COMPLETION 链，next() 之后执行——
// 依赖注册顺序位于 PluginChatCompletion 之后，container 接线保证）。
type PluginClaimGate struct{}

// NewPluginClaimGate 创建插件（container.Invoke 接线）。
func NewPluginClaimGate(eventManager *EventManager) *PluginClaimGate {
	p := &PluginClaimGate{}
	eventManager.Register(p)
	return p
}

// ActivationEvents 与生成共用 CHAT_COMPLETION 事件。
func (p *PluginClaimGate) ActivationEvents() []types.EventType {
	return []types.EventType{types.CHAT_COMPLETION}
}

// OnEvent 在生成完成后审计答案（上游未生成则跳过）。
func (p *PluginClaimGate) OnEvent(
	ctx context.Context, eventType types.EventType, chatManage *types.ChatManage, next func() *PluginError,
) *PluginError {
	if err := next(); err != nil {
		return err
	}
	if os.Getenv("STARKB_CLAIM_GATE") != "true" {
		return nil
	}
	if chatManage.ChatResponse == nil || strings.TrimSpace(chatManage.ChatResponse.Content) == "" {
		return nil
	}
	evidence := make([]string, 0, len(chatManage.MergeResult))
	for _, r := range chatManage.MergeResult {
		if r != nil && r.Content != "" {
			evidence = append(evidence, r.Content)
		}
	}
	report := AnalyzeClaims(chatManage.ChatResponse.Content, evidence)
	chatManage.ClaimReport = &report
	logger.Infof(ctx, "claim gate: 论断 %d，有据 %d，无据 %d（标推断候选），数字失证 %d，覆盖率 %.2f",
		report.Total, report.Grounded, report.Ungrounded, report.NumericMismatch,
		report.GroundingCoverage)
	return nil
}
