// Package service — OJK Fit & Proper checklist management.
//
// This package provides the business logic for the OJK plugin:
//   - Running the fp-rule-skill to extract checklist items from regulation KBs
//   - Managing the extraction run lifecycle (pending → running → done/failed)
//   - CRUD for extracted checklist items with human-review support
package service

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
	)

// OJKRun represents one execution of the fp-rule-skill.
type OJKRun struct {
	RunID        string    `gorm:"primaryKey;type:text" json:"run_id"`
	TenantID     uint64    `gorm:"type:integer;not null;index:idx_ojk_runs_tenant_status" json:"tenant_id"`
	KBID         string    `gorm:"type:text;not null;default:'';index:idx_ojk_runs_kb" json:"kb_id"`
	SkillVersion string    `gorm:"type:text;not null;default:'1.0.0'" json:"skill_version"`
	Status       string    `gorm:"type:text;not null;default:'pending'" json:"status"`
	SlicesTotal  *int      `gorm:"type:integer" json:"slices_total"`
	SlicesDone   *int      `gorm:"type:integer" json:"slices_done"`
	TotalSlices  *int      `gorm:"type:integer" json:"total_slices"`
	TotalItems   int       `gorm:"type:integer;default:0" json:"total_items"`
	FlaggedItems int       `gorm:"type:integer;default:0" json:"flagged_items"`
	Error        string    `gorm:"type:text" json:"error"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (OJKRun) TableName() string { return "ojk_runs" }

// OJKChecklistItem is one extracted requirement awaiting human review.
type OJKChecklistItem struct {
	ID             string         `gorm:"primaryKey;type:text" json:"id"`
	TenantID       uint64         `gorm:"type:integer;not null;index:idx_ojk_items_tenant_status" json:"tenant_id"`
	RunID          string         `gorm:"type:text;not null;index:idx_ojk_items_run" json:"run_id"`
	Regulation     string         `gorm:"type:text;not null" json:"regulation"`
	Pasal          string         `gorm:"type:text;not null" json:"pasal"`
	PasalText      string         `gorm:"type:text;not null" json:"pasal_text"`
	Area           *string        `gorm:"type:text" json:"area"`
	Requirement    string         `gorm:"type:text;not null" json:"requirement"`
	RequirementID  *string        `gorm:"type:text;uniqueIndex" json:"requirement_id"`
	EvidenceType   *string        `gorm:"type:text" json:"evidence_type"`
	CheckMethod    *string        `gorm:"type:text" json:"check_method"`
	ApplicableRoles StrList       `gorm:"type:text[]" json:"applicable_roles"`
	Severity       string         `gorm:"type:text;not null;default:'info'" json:"severity"`
	Keywords       []string       `gorm:"type:text[]" json:"keywords"`
	Source         string         `gorm:"type:text;not null;default:'normal'" json:"source"`
	Flag           *string        `gorm:"column:_flag;type:text" json:"_flag"`
	Status         string         `gorm:"type:text;not null;default:'pending'" json:"status"`
	ReviewerNote   *string        `gorm:"type:text" json:"reviewer_note"`
	CreatedAt      time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
}

func (OJKChecklistItem) TableName() string { return "ojk_checklist_items" }

// StrList 是 PG text[] 与 sqlite text 两栖的字符串列表：
// 写入用 PG 数组字面量 {a,b}，扫描同时接受数组字面量 / JSON / 裸串。
// （执行器在 PG 生产与 sqlite 单测两种环境都要能读写同一模型。）
type StrList []string

func (s StrList) Value() (driver.Value, error) {
	if len(s) == 0 {
		return "{}", nil
	}
	parts := make([]string, len(s))
	for i, v := range s {
		parts[i] = `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
	}
	return "{" + strings.Join(parts, ",") + "}", nil
}

func (s *StrList) Scan(v interface{}) error {
	switch t := v.(type) {
	case nil:
		*s = nil
	case []byte:
		return s.parse(string(t))
	case string:
		return s.parse(t)
	default:
		return fmt.Errorf("ojk strlist: unsupported scan type %T", v)
	}
	return nil
}

func (s *StrList) parse(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		*s = nil
		return nil
	}
	// 优先 JSON（执行器自写的是 PG 字面量，但历史上可能存在 JSON 形态）
	if strings.HasPrefix(raw, "[") {
		var out []string
		if err := json.Unmarshal([]byte(raw), &out); err == nil {
			*s = out
			return nil
		}
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(raw, "{"), "}")
	if inner == "" {
		*s = nil
		return nil
	}
	var out []string
	var cur strings.Builder
	inQuote := false
	esc := false
	for _, r := range inner {
		switch {
		case esc:
			cur.WriteRune(r)
			esc = false
		case r == '\\':
			esc = true
		case r == '"':
			inQuote = !inQuote
		case r == ',' && !inQuote:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	out = append(out, cur.String())
	*s = out
	return nil
}

// OJKService handles OJK Fit & Proper checklist operations.
type OJKService struct {
	db *gorm.DB
	// invokeLLM 可在测试中替换（生产实现 callLLM）。
	InvokeLLMFn func(ctx context.Context, cfg LLMConfig, system, user string) (string, error)
	// loadRegulationText / loadModelConfig 同理可注入。
	LoadRegulationTextFn func(ctx context.Context, tenantID uint64, kbID string) (string, error)
	LoadModelConfigFn    func(ctx context.Context, tenantID uint64) (LLMConfig, error)
}

// NewOJKService creates a new OJK service.
func NewOJKService(db *gorm.DB) *OJKService {
	s := &OJKService{db: db}
	s.InvokeLLMFn = s.callLLM
	s.LoadRegulationTextFn = s.loadRegulationTextFromDB
	s.LoadModelConfigFn = s.loadModelConfigFromDB
	return s
}

// RunStatus is the current status of an OJK run.
type RunStatus struct {
	RunID        string `json:"run_id"`
	TenantID     uint64 `json:"tenant_id"`
	KBID         string `json:"kb_id"`
	KBName       string `json:"kb_name,omitempty"`
	SkillVersion string `json:"skill_version"`
	Status       string `json:"status"`
	SlicesTotal  *int   `json:"slices_total,omitempty"`
	SlicesDone   *int   `json:"slices_done,omitempty"`
	TotalSlices  *int   `json:"total_slices,omitempty"`
	TotalItems   int    `json:"total_items"`
	FlaggedItems int    `json:"flagged_items"`
	Error        string `json:"error,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// ErrKBNotFound / ErrKBNoDocs：CreateRun 的 KB 校验失败语义（handler 映射 404/400）。
var (
	ErrOJKKBNotFound = errors.New("knowledge base not found in this workspace")
	ErrOJKKBNoDocs   = errors.New("knowledge base has no parsed documents to extract from")
)

// CreateRun 校验 KB 后落一条 pending run，并异步启动执行器
// （重建全文 → Pasal 切片 → 分批调 LLM → 校验入库）。
func (s *OJKService) CreateRun(ctx context.Context, tenantID uint64, kbID, skillVersion string) (*RunStatus, error) {
	if skillVersion == "" {
		skillVersion = "1.0.0"
	}
	var kb struct {
		ID   string
		Name string
	}
	if err := s.db.WithContext(ctx).Raw(
		"SELECT id, name FROM knowledge_bases WHERE id = ? AND tenant_id = ? AND deleted_at IS NULL",
		kbID, tenantID).Scan(&kb).Error; err != nil {
		return nil, fmt.Errorf("check knowledge base: %w", err)
	}
	if kb.ID == "" {
		return nil, ErrOJKKBNotFound
	}
	var docCount int64
	if err := s.db.WithContext(ctx).Raw(
		"SELECT count(*) FROM knowledges WHERE knowledge_base_id = ? AND deleted_at IS NULL "+
			"AND (parse_status = 'completed' OR parse_status = 'finished' OR parse_status = '' )",
		kbID).Scan(&docCount).Error; err != nil {
		return nil, fmt.Errorf("count kb documents: %w", err)
	}
	if docCount == 0 {
		return nil, ErrOJKKBNoDocs
	}

	runID := fmt.Sprintf("ojk-%d-%d", tenantID, time.Now().UnixNano())
	run := &OJKRun{
		RunID:        runID,
		TenantID:     tenantID,
		KBID:         kbID,
		SkillVersion: skillVersion,
		Status:       "pending",
	}
	if err := s.db.WithContext(ctx).Create(run).Error; err != nil {
		return nil, fmt.Errorf("create ojk run: %w", err)
	}
	go s.processRun(runID, tenantID, kbID, kb.Name)
	return s.GetRun(ctx, tenantID, runID)
}

// ListRuns 返回最近 runs（含 KB 名），最新在前。
func (s *OJKService) ListRuns(ctx context.Context, tenantID uint64, limit int) ([]RunStatus, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	var runs []OJKRun
	if err := s.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").Limit(limit).
		Find(&runs).Error; err != nil {
		return nil, fmt.Errorf("list ojk runs: %w", err)
	}
	kbIDs := make([]string, 0, len(runs))
	for _, r := range runs {
		if r.KBID != "" {
			kbIDs = append(kbIDs, r.KBID)
		}
	}
	kbNames := map[string]string{}
	if len(kbIDs) > 0 {
		var kbs []struct {
			ID   string
			Name string
		}
		if err := s.db.WithContext(ctx).Raw(
			"SELECT id, name FROM knowledge_bases WHERE id IN ?",
			kbIDs).Scan(&kbs).Error; err == nil {
			for _, kb := range kbs {
				kbNames[kb.ID] = kb.Name
			}
		}
	}
	out := make([]RunStatus, 0, len(runs))
	for _, r := range runs {
		st := runToStatus(r)
		st.KBName = kbNames[r.KBID]
		out = append(out, st)
	}
	return out, nil
}

func runToStatus(r OJKRun) RunStatus {
	return RunStatus{
		RunID:        r.RunID,
		TenantID:     r.TenantID,
		KBID:         r.KBID,
		SkillVersion: r.SkillVersion,
		Status:       r.Status,
		SlicesTotal:  r.SlicesTotal,
		SlicesDone:   r.SlicesDone,
		TotalSlices:  r.TotalSlices,
		TotalItems:   r.TotalItems,
		FlaggedItems: r.FlaggedItems,
		Error:        r.Error,
		CreatedAt:    r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    r.UpdatedAt.Format(time.RFC3339),
	}
}

// ResumeInterrupted 在进程启动时调用：上次进程中断留下的 running 标记为失败；
// pending（含旧版无 KB 绑定的遗留行）重新驱动或显式失败。
func (s *OJKService) ResumeInterrupted(ctx context.Context) {
	s.db.WithContext(ctx).Model(&OJKRun{}).
		Where("status = 'running'").
		Updates(map[string]interface{}{
			"status": "failed",
			"error":  "interrupted by server restart",
		})
	var pending []OJKRun
	s.db.WithContext(ctx).
		Where("status = 'pending'").Order("created_at ASC").Find(&pending)
	for _, r := range pending {
		if r.KBID == "" {
			s.db.WithContext(ctx).Model(&OJKRun{}).Where("run_id = ?", r.RunID).
				Updates(map[string]interface{}{
					"status": "failed",
					"error":  "legacy run without kb binding",
				})
			continue
		}
		var kb struct {
			Name string
		}
		s.db.WithContext(ctx).Raw(
			"SELECT name FROM knowledge_bases WHERE id = ?", r.KBID).Scan(&kb)
		go s.processRun(r.RunID, r.TenantID, r.KBID, kb.Name)
	}
}

// GetRun returns the current status of a run.
func (s *OJKService) GetRun(ctx context.Context, tenantID uint64, runID string) (*RunStatus, error) {
	var run OJKRun
	if err := s.db.WithContext(ctx).
		Where("run_id = ? AND tenant_id = ?", runID, tenantID).
		First(&run).Error; err != nil {
		return nil, fmt.Errorf("get ojk run: %w", err)
	}
	st := runToStatus(run)
	if run.KBID != "" {
		var kb struct{ Name string }
		s.db.WithContext(ctx).Raw(
			"SELECT name FROM knowledge_bases WHERE id = ?", run.KBID).Scan(&kb)
		st.KBName = kb.Name
	}
	return &st, nil
}

// UpdateRunStatus updates the status fields of a run.
func (s *OJKService) UpdateRunStatus(ctx context.Context, runID string, updates map[string]interface{}) error {
	if err := s.db.WithContext(ctx).Model(&OJKRun{}).
		Where("run_id = ?", runID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("update ojk run status: %w", err)
	}
	return nil
}

// ItemStatus represents a checklist item for the review workflow.
type ItemStatus struct {
	ID              string   `json:"id"`
	RunID           string   `json:"run_id"`
	Regulation      string   `json:"regulation"`
	Pasal           string   `json:"pasal"`
	PasalText       string   `json:"pasal_text"`
	Area            string   `json:"area,omitempty"`
	Requirement     string   `json:"requirement"`
	RequirementID   string   `json:"requirement_id,omitempty"`
	EvidenceType    string   `json:"evidence_type,omitempty"`
	CheckMethod     string   `json:"check_method,omitempty"`
	ApplicableRoles []string `json:"applicable_roles,omitempty"`
	Severity        string   `json:"severity"`
	Keywords        []string `json:"keywords,omitempty"`
	Source          string   `json:"source"`
	Flag            string   `json:"_flag,omitempty"`
	Status          string   `json:"status"`
	ReviewerNote    string   `json:"reviewer_note,omitempty"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

// ListItems returns paginated checklist items for a run.
func (s *OJKService) ListItems(ctx context.Context, tenantID uint64, runID string, status *string, page, pageSize int) ([]ItemStatus, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	query := s.db.WithContext(ctx).
		Where("run_id = ? AND tenant_id = ?", runID, tenantID)
	if status != nil && *status != "" {
		query = query.Where("status = ?", *status)
	}

	var total int64
	if err := query.Model(&OJKChecklistItem{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count ojk items: %w", err)
	}

	var items []OJKChecklistItem
	if err := query.Order("created_at ASC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list ojk items: %w", err)
	}

	out := make([]ItemStatus, len(items))
	for i, it := range items {
		out[i] = ItemStatus{
			ID:              it.ID,
			RunID:           it.RunID,
			Regulation:      it.Regulation,
			Pasal:           it.Pasal,
			PasalText:       it.PasalText,
			Area:            stringPtr(it.Area),
			Requirement:     it.Requirement,
			RequirementID:   stringPtr(it.RequirementID),
			EvidenceType:    stringPtr(it.EvidenceType),
			CheckMethod:     stringPtr(it.CheckMethod),
			ApplicableRoles: it.ApplicableRoles,
			Severity:        it.Severity,
			Keywords:        it.Keywords,
			Source:          it.Source,
			Flag:            stringPtr(it.Flag),
			Status:          it.Status,
			ReviewerNote:    stringPtr(it.ReviewerNote),
			CreatedAt:       it.CreatedAt.Format(time.RFC3339),
			UpdatedAt:       it.UpdatedAt.Format(time.RFC3339),
		}
	}
	return out, total, nil
}

// ResolveItem updates a single item's review status.
func (s *OJKService) ResolveItem(ctx context.Context, tenantID uint64, itemID, status, note string) error {
	if status != "confirmed" && status != "rejected" {
		return fmt.Errorf("invalid status: %q (must be confirmed or rejected)", status)
	}
	updates := map[string]interface{}{
		"status":    status,
		"updated_at": time.Now(),
	}
	if note != "" {
		updates["reviewer_note"] = note
	}
	result := s.db.WithContext(ctx).
		Model(&OJKChecklistItem{}).
		Where("id = ? AND tenant_id = ?", itemID, tenantID).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("resolve ojk item: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("item not found: %q", itemID)
	}
	return nil
}

// ItemStats returns counts by status for a run.
func (s *OJKService) ItemStats(ctx context.Context, tenantID uint64, runID string) (map[string]int64, error) {
	var results []struct {
		Status string
		Count  int64
	}
	if err := s.db.WithContext(ctx).
		Model(&OJKChecklistItem{}).
		Where("run_id = ? AND tenant_id = ?", runID, tenantID).
		Group("status").
		Pluck("status, count(*)", &results).Error; err != nil {
		return nil, fmt.Errorf("stats ojk items: %w", err)
	}
	out := make(map[string]int64)
	for _, r := range results {
		out[r.Status] = r.Count
	}
	return out, nil
}

// stringPtr returns a pointer to s, or nil if s is empty.
func stringPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ---- 提示词（与 examples/skills/fp-rule-skill/scripts/run.py 逐字一致）----

const ojkSystemPrompt = `You are an expert OJK (Otoritas Jasa Keuangan) regulatory analyst
specializing in Indonesian financial services fit-and-proper
assessment regulations.

Your task: Given structured sections (Pasal) of an OJK regulation,
extract EVERY testable compliance requirement as a structured
ChecklistItem. Your output will be reviewed line-by-line by an OJK
business expert before publication — accuracy and completeness
matter more than brevity.

STRICT RULES:
- R1: Extract ONLY requirements explicitly stated in the provided
  Pasal text. Never invent, infer beyond the text, or add
  requirements from your general knowledge.
- R2: For each requirement, you MUST quote the exact Pasal reference
  (Pasal number and ayat/huruf if applicable). If you cannot cite
  a Pasal, do not create the item.
- R3: Quote the key phrase from the original regulation text
  (pasal_text) verbatim.
- R4: If a Pasal contains multiple distinct testable requirements,
  split them into separate ChecklistItems, each citing the same Pasal.
- R5: If a Pasal is procedural/administrative (not a testable
  requirement on a candidate), skip it.
- R6: Classify compliance area strictly as one of:
  "Integrity" | "Financial Reputation" | "Competence" |
  "Structure" | "Completeness"
- R7: Classify severity strictly as one of:
  "critical" | "clarification" | "info"
- R8: Define check_method strictly as one of:
  "document_presence" | "cross_document" | "rule_computation"
- R9: evidence_type must be a concrete, nameable document or data
  source. Do not write vague values.
- R10: applicable_roles must be a non-empty subset of:
  ["Direktur", "Komisaris", "Direktur Utama", "Dewan Pengawas",
   "Pejabat Puncak", "Pemegang Saham Pengendali", "Eksekutif"]
  If applies to all, use ["*"].
- R11: keywords — extract 2-5 distinctive terms from regulation text.
- R12: Output ONLY valid JSON conforming to the schema. No markdown fences.

If a Pasal yields zero requirements, omit it silently.`

const ojkUserTemplate = `Regulation: {regulation_no}
Title: {regulation_title}
Source: {regulation_file}

--- PASAL SECTIONS ---
{pasal_slices}
--- END PASAL SECTIONS ---

Extract all testable requirements as JSON array of ChecklistItem.`


// ---- 执行器（run.py 管线的 Go 移植，docs/17 同源提示词）----
// Note: 提示词常量见文件末尾（与 examples/skills/fp-rule-skill 逐字一致）。

const (
	ojkBatchMaxChars   = 12000 // 单批切片正文总长上限（与 run.py 一致）
	ojkMinBodyChars    = 20    // 短于该长度的 Pasal 段跳过
	ojkLLMTimeout      = 120 * time.Second
	ojkLLMMaxRetries   = 2
	ojkDefaultLLMModel = "glm-5.3-flash"
)

// LLMConfig 是执行器调 LLM 所需的最小配置（从 models 表解析）。
type LLMConfig struct {
	APIKey   string
	BaseURL  string
	Model    string
}

// invokeLLM 可在测试中替换；生产实现见 callLLM。
var _ = func(s *OJKService) {}

// processRun 是一次 run 的完整管线。错误一律落 run.error 并置 failed。
func (s *OJKService) processRun(runID string, tenantID uint64, kbID, kbName string) {
	ctx := context.Background()
	fail := func(format string, args ...interface{}) {
		msg := fmt.Sprintf(format, args...)
		s.db.Model(&OJKRun{}).Where("run_id = ?", runID).
			Updates(map[string]interface{}{"status": "failed", "error": msg})
	}
	// 领取：pending → running（并发/重复触发时只有一个赢家）
	claim := s.db.Model(&OJKRun{}).
		Where("run_id = ? AND status = 'pending'", runID).
		Updates(map[string]interface{}{"status": "running"})
	if claim.Error != nil || claim.RowsAffected == 0 {
		return
	}

	text, err := s.LoadRegulationTextFn(ctx, tenantID, kbID)
	if err != nil {
		fail("rebuild regulation text: %v", err)
		return
	}
	if text == "" {
		fail("no parsable chunks in knowledge base %q", kbName)
		return
	}
	regName := regexp.MustCompile(`\s*\(OJK[^)]*\)\s*`).ReplaceAllString(kbName, "")
	regName = strings.TrimSpace(regName)
	slices := slicePasal(text, kbName, regName)
	if len(slices) == 0 {
		fail("no Pasal sections found in knowledge base %q", kbName)
		return
	}
	refs := make(map[string]bool, len(slices))
	for _, sl := range slices {
		refs[sl.Ref] = true
	}
	batches := batchSlices(slices, ojkBatchMaxChars)

	total := len(batches)
	s.db.Model(&OJKRun{}).Where("run_id = ?", runID).
		Updates(map[string]interface{}{"slices_total": total, "slices_done": 0})

	cfg, err := s.LoadModelConfigFn(ctx, tenantID)
	if err != nil {
		fail("resolve llm config: %v", err)
		return
	}

	type ojkItem = map[string]interface{}
	var allItems []ojkItem
	var allFlags []string
	skippedBatches := 0
	for i, batch := range batches {
		var pasals strings.Builder
		for j, sl := range batch {
			if j > 0 {
				pasals.WriteString("\n\n")
			}
			pasals.WriteString(sl.Ref)
			pasals.WriteString(": ")
			pasals.WriteString(sl.Body)
		}
		user := strings.ReplaceAll(ojkUserTemplate, "{regulation_no}", batch[0].Regulation)
		user = strings.ReplaceAll(user, "{regulation_title}", batch[0].Regulation)
		user = strings.ReplaceAll(user, "{regulation_file}", batch[0].File)
		user = strings.ReplaceAll(user, "{pasal_slices}", pasals.String())

		var content string
		var lastErr error
		for attempt := 0; attempt <= ojkLLMMaxRetries; attempt++ {
			content, lastErr = s.InvokeLLMFn(ctx, cfg, ojkSystemPrompt, user)
			if lastErr == nil {
				break
			}
			select {
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
			}
		}
		if lastErr != nil {
			fail("llm batch %d/%d: %v", i+1, total, lastErr)
			return
		}
		// 模型可能返回 {"items":[...]} 或裸 [...] —— 两种都接受；
		// 单批解析持续失败按 run.py 口径跳过该批继续（计数留痕）。
		var items []ojkItem
		var parsed struct {
			Items []ojkItem `json:"items"`
		}
		if err := json.Unmarshal([]byte(content), &parsed); err == nil {
			items = parsed.Items
		} else if err := json.Unmarshal([]byte(content), &items); err != nil {
			skippedBatches++
			done := i + 1
			s.db.Model(&OJKRun{}).Where("run_id = ?", runID).
				Updates(map[string]interface{}{"slices_done": done})
			continue
		}
		for _, item := range items {
			v, flags := validateOJKItem(item, refs)
			allItems = append(allItems, v)
			allFlags = append(allFlags, flags...)
		}
		done := i + 1
		s.db.Model(&OJKRun{}).Where("run_id = ?", runID).
			Updates(map[string]interface{}{"slices_done": done})
	}

	// 去重（requirement_id）
	seen := map[string]bool{}
	var deduped []ojkItem
	for _, item := range allItems {
		rid, _ := item["requirement_id"].(string)
		if rid != "" && seen[rid] {
			continue
		}
		if rid != "" {
			seen[rid] = true
		}
		deduped = append(deduped, item)
	}

	now := time.Now()
	for idx, item := range deduped {
		itemID := fmt.Sprintf("c-%s-%04d", runID, idx+1)
		reg, _ := item["regulation"].(string)
		pasal, _ := item["pasal"].(string)
		pasalText, _ := item["pasal_text"].(string)
		area, _ := item["area"].(string)
		requirement, _ := item["requirement"].(string)
		reqID, _ := item["requirement_id"].(string)
		evidence, _ := item["evidence_type"].(string)
		checkMethod, _ := item["check_method"].(string)
		severity, _ := item["severity"].(string)
		if severity == "" {
			severity = "info"
		}
		source := "normal"
		if strings.Contains(pasal, "Penjelasan") {
			source = "penjelasan"
		}
		roles, _ := item["applicable_roles"].([]interface{})
		roleStrings := StrList{}
		for _, r := range roles {
			if rs, ok := r.(string); ok {
				roleStrings = append(roleStrings, rs)
			}
		}
		keywords, _ := item["keywords"].([]interface{})
		kwStrings := StrList{}
		for _, k := range keywords {
			if ks, ok := k.(string); ok {
				kwStrings = append(kwStrings, ks)
			}
		}
		var flag *string
		if len(allFlags) > 0 {
			f := strings.Join(allFlags, "; ")
			flag = &f
		}
		row := OJKChecklistItem{
			ID: itemID, TenantID: tenantID, RunID: runID,
			Regulation: reg, Pasal: pasal, PasalText: pasalText,
			Requirement: requirement, Severity: severity,
			ApplicableRoles: roleStrings, Keywords: kwStrings,
			Source: source, Status: "pending", Flag: flag,
		}
		if area != "" {
			row.Area = &area
		}
		if reqID != "" {
			row.RequirementID = &reqID
		}
		if evidence != "" {
			row.EvidenceType = &evidence
		}
		if checkMethod != "" {
			row.CheckMethod = &checkMethod
		}
		if err := s.db.Create(&row).Error; err != nil {
			// 幂等：同 id 冲突跳过（重跑同 run 时）
			s.db.Exec(
				"INSERT INTO ojk_checklist_items (id,tenant_id,run_id,created_at,updated_at) "+
					"VALUES (?,?,?,?,?) ON CONFLICT (id) DO NOTHING",
				itemID, tenantID, runID, now, now)
		}
		_ = pasalText
	}

	finalStatus := "done"
	finalErr := ""
	if skippedBatches > 0 {
		finalErr = fmt.Sprintf("%d/%d batches skipped (invalid LLM output)", skippedBatches, total)
	}
	s.db.Model(&OJKRun{}).Where("run_id = ?", runID).
		Updates(map[string]interface{}{
			"status":        finalStatus,
			"total_items":   len(deduped),
			"flagged_items": len(allFlags),
			"slices_done":   total,
			"error":         finalErr,
		})
}

// loadRegulationText 从 chunks 重建法规全文（与 run.py 同口径：冲突检测 + 按
// start_at 拼接；过滤 sbk_method=failed 与已删除 chunk）。
func (s *OJKService) loadRegulationTextFromDB(ctx context.Context, tenantID uint64, kbID string) (string, error) {
	var rows []struct {
		Content string
		StartAt int64
		EndAt   int64
	}
	if err := s.db.WithContext(ctx).Raw(
		"SELECT content, start_at, end_at FROM chunks "+
			"WHERE knowledge_base_id = ? AND tenant_id = ? AND deleted_at IS NULL "+
			"AND coalesce(metadata->>'sbk_method','') != 'failed' "+
			"ORDER BY start_at", kbID, tenantID).Scan(&rows).Error; err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	// 重叠处理：现役 KB 走 parent-child 分块，父块与子片段天然内容重叠
	// （同源不同粒度）。run.py 的硬冲突检查在 parent-child 下必然误报
	// （2026-09-28 OJK 法律法规库实测），这里改为宽容策略：
	// 已覆盖区间内的 chunk 跳过，部分重叠裁剪起点，按序拼接。
	var parts strings.Builder
	cursor := int64(0)
	for _, r := range rows {
		start, end := r.StartAt, r.EndAt
		if end <= cursor {
			continue // 完全被前文覆盖（父子分块的子片段）
		}
		if start < cursor {
			start = cursor
		}
		if start >= end {
			continue
		}
		parts.WriteString(r.Content)
		cursor = end
	}
	return parts.String(), nil
}

// 测试与向导侧使用的导出包装。
func SliceOJKPasal(text, fileName, regName string) []OJKSlice {
	return slicePasal(text, fileName, regName)
}

func BatchOJKSlices(slices []OJKSlice, maxChars int) [][]OJKSlice {
	return batchSlices(slices, maxChars)
}

func ValidateOJKItem(item map[string]interface{}, allRefs map[string]bool) (map[string]interface{}, []string) {
	return validateOJKItem(item, allRefs)
}

// OJKSlice 是一个 Pasal 段。
type OJKSlice struct {
	Ref          string
	Regulation   string
	File         string
	Body         string
	IsPenjelasan bool
}

// slicePasal 按 `## Pasal N [ayat (X)] [huruf y]` 标题切片（run.py 同款正则）。
func slicePasal(fullText, fileName, regName string) []OJKSlice {
	pasalRe := regexp.MustCompile(
		`(?im)^##\s+Pasal\s+(\d+)(?:\s+ayat\s+\((\d+)\))?(?:\s+huruf\s+([a-z]))?\s*$`)
	matches := pasalRe.FindAllStringSubmatchIndex(fullText, -1)
	penjIdx := strings.Index(strings.ToLower(fullText), "penjelasan atas peraturan")

	slices := []OJKSlice{}
	for i, m := range matches {
		num := string(fullText[m[2]:m[3]])
		label := "Pasal " + num
		if m[4] >= 0 {
			label += " ayat (" + string(fullText[m[4]:m[5]]) + ")"
		}
		if m[6] >= 0 {
			label += " huruf " + string(fullText[m[6]:m[7]])
		}
		isPenj := penjIdx >= 0 && m[0] > penjIdx
		if isPenj {
			label += " (Penjelasan)"
		}
		bodyStart := m[1]
		bodyEnd := len(fullText)
		if i+1 < len(matches) {
			bodyEnd = matches[i+1][0]
		}
		body := fullText[bodyStart:bodyEnd]
		body = regexp.MustCompile(`<!--sbk:p\d+-b\d+-->`).ReplaceAllString(body, "")
		body = regexp.MustCompile(`(?m)^#{1,4}\s+`).ReplaceAllString(body, "")
		body = strings.Join(strings.Fields(body), " ")
		if len(body) < ojkMinBodyChars {
			continue
		}
		slices = append(slices, OJKSlice{
			Ref: label, Regulation: regName, File: fileName,
			Body: body, IsPenjelasan: isPenj,
		})
	}
	return slices
}

// batchSlices 按 body 长度上限把切片分批（12k，run.py 同款贪心）。
func batchSlices(slices []OJKSlice, maxChars int) [][]OJKSlice {
	var batches [][]OJKSlice
	var cur []OJKSlice
	curSize := 0
	for _, sl := range slices {
		if curSize+len(sl.Body) > maxChars && len(cur) > 0 {
			batches = append(batches, cur)
			cur, curSize = nil, 0
		}
		cur = append(cur, sl)
		curSize += len(sl.Body)
	}
	if len(cur) > 0 {
		batches = append(batches, cur)
	}
	return batches
}

// validateOJKItem 校验单条 item 并返回（规整后的 item, flags）。
// 与 run.py validate_item 同口径：非法枚举打标而非丢弃；roles 兜底 ["*"]。
func validateOJKItem(item map[string]interface{}, allRefs map[string]bool) (map[string]interface{}, []string) {
	var flags []string
	str := func(k string) string { v, _ := item[k].(string); return v }
	if a := str("area"); a != "" && !map[string]bool{
		"Integrity": true, "Financial Reputation": true, "Competence": true,
		"Structure": true, "Completeness": true,
	}[a] {
		flags = append(flags, "invalid_area")
	}
	if sv := str("severity"); sv != "" && !map[string]bool{
		"critical": true, "clarification": true, "info": true,
	}[sv] {
		flags = append(flags, "invalid_severity")
	}
	if cm := str("check_method"); cm != "" && !map[string]bool{
		"document_presence": true, "cross_document": true, "rule_computation": true,
	}[cm] {
		flags = append(flags, "invalid_check_method")
	}
	if pasal := str("pasal"); pasal != "" && !allRefs[pasal] {
		flags = append(flags, "pasal_unverified")
	}
	roles, _ := item["applicable_roles"].([]interface{})
	if len(roles) == 0 {
		item["applicable_roles"] = []interface{}{"*"}
	}
	return item, flags
}

// loadModelConfig 从 models 表解析租户的 LLM 配置（优先 KnowledgeQA 默认模型）。
func (s *OJKService) loadModelConfigFromDB(ctx context.Context, tenantID uint64) (LLMConfig, error) {
	var rows []struct {
		APIKey   string
		BaseURL  string
		Name     string
	}
	if err := s.db.WithContext(ctx).Raw(
		"SELECT parameters->>'api_key' AS api_key, parameters->>'base_url' AS base_url, name "+
			"FROM models WHERE tenant_id = ? AND status = 'active' AND type = 'KnowledgeQA' "+
			"ORDER BY is_default DESC, updated_at DESC LIMIT 1", tenantID).Scan(&rows).Error; err != nil {
		return LLMConfig{}, err
	}
	if len(rows) == 0 || rows[0].APIKey == "" {
		if err := s.db.WithContext(ctx).Raw(
			"SELECT parameters->>'api_key' AS api_key, parameters->>'base_url' AS base_url, name "+
				"FROM models WHERE tenant_id = ? AND status = 'active' "+
				"ORDER BY is_default DESC, updated_at DESC LIMIT 1", tenantID).Scan(&rows).Error; err != nil {
			return LLMConfig{}, err
		}
	}
	if len(rows) == 0 || rows[0].APIKey == "" {
		return LLMConfig{}, errors.New("no active llm model configured for this workspace")
	}
	baseURL := rows[0].BaseURL
	if baseURL == "" {
		baseURL = "https://open.bigmodel.cn/api/coding/paas/v4"
	}
	model := rows[0].Name
	if model == "" {
		model = ojkDefaultLLMModel
	}
	return LLMConfig{APIKey: rows[0].APIKey, BaseURL: baseURL, Model: model}, nil
}

// callLLM 直接调 OpenAI 兼容 chat/completions（与 run.py call_glm 同形）。
// 返回剥离 ```json 围栏后的内容。
func (s *OJKService) callLLM(ctx context.Context, cfg LLMConfig, system, user string) (string, error) {
	payload, err := json.Marshal(map[string]interface{}{
		"model": cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"max_tokens": 8192,
		"temperature": 0,
		"thinking":   map[string]string{"type": "disabled"},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(cfg.BaseURL, "/")+"/chat/completions",
		bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	client := &http.Client{Timeout: ojkLLMTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("llm http %d: %s", resp.StatusCode, ojkTruncate(string(body), 300))
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode llm response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", errors.New("llm response has no choices")
	}
	content := result.Choices[0].Message.Content
	content = regexp.MustCompile("^```json\\s*").ReplaceAllString(content, "")
	content = regexp.MustCompile("\\s*```$").ReplaceAllString(content, "")
	return content, nil
}

func ojkTruncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}


// EnsureTables creates the OJK tables if they don't exist.
// Called during app startup to handle migrations gracefully.
// 表结构的生产事实源是迁移 000112；AutoMigrate 只补新增列（kb_id/slices_*）。
// gorm 对已有唯一约束的命名与迁移不一致会报 DROP CONSTRAINT 失败——
// 这里降级为日志告警，不让插件表把整个进程拖死。
func (s *OJKService) EnsureTables(db *gorm.DB) error {
	if err := db.AutoMigrate(&OJKRun{}, &OJKChecklistItem{}); err != nil {
		fmt.Printf("[ojk] automigrate warn (tables managed by migration 000112): %v\n", err)
	}
	return nil
}
