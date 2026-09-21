package types

import (
	"database/sql/driver"
	"encoding/json"
)

// WS3.3 答案级论断审计（M3 G4，docs/07 WS4）：类型定义供 PipelineState 持有
// 与前端/消息层消费；判定逻辑在 chat_pipeline/claim_gate.go 的 AnalyzeClaims。

// ClaimVerdict 单条论断的审计结论。
type ClaimVerdict string

const (
	ClaimGrounded        ClaimVerdict = "grounded"         // 证据支撑（文本重叠达标）
	ClaimGroundedNumeric ClaimVerdict = "grounded_numeric" // 全部数值可在证据中复核
	ClaimUngrounded      ClaimVerdict = "ungrounded"       // 无证据重叠 → 建议标"推断"
	ClaimNumericMismatch ClaimVerdict = "numeric_mismatch" // 数字交叉校验失败
)

// Claim 单条论断。
type Claim struct {
	Text    string       `json:"text"`
	Verdict ClaimVerdict `json:"verdict"`
	Numbers []string     `json:"numbers,omitempty"` // 校验失败的数值（numeric_mismatch 时）
}

// ClaimReport 答案级审计报告。
type ClaimReport struct {
	Total             int     `json:"total"`
	Grounded          int     `json:"grounded"`
	Ungrounded        int     `json:"ungrounded"`
	NumericMismatch   int     `json:"numeric_mismatch"`
	GroundingCoverage float64 `json:"grounding_coverage"` // (Total-NumericMismatch-Ungrounded)/Total
	Claims            []Claim `json:"claims,omitempty"`
}

// Value implements the driver.Valuer interface for JSONB persistence.
func (c ClaimReport) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface for JSONB persistence.
func (c *ClaimReport) Scan(value interface{}) error {
	if value == nil {
		*c = ClaimReport{}
		return nil
	}
	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		*c = ClaimReport{}
		return nil
	}
	return json.Unmarshal(b, c)
}
