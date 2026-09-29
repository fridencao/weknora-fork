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
	"os"
	"regexp"
	"strconv"
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
//
// applicable_roles / keywords 存 TEXT（PG 数组字面量 {a,b}，Scan 兼容 JSON）——
// 不能用 text[]：pgx 把 Go string 参数绑成 text，服务器拒绝隐式转 text[]
// （42804），曾致整版条目全部插入失败却记下 total_items 的假成功（migration 000113）。
// requirement_id 的唯一性限定在 run 内（每个版本是同一定义的快照，跨 run 必然重复）。
type OJKChecklistItem struct {
	ID             string         `gorm:"primaryKey;type:text" json:"id"`
	TenantID       uint64         `gorm:"type:integer;not null;index:idx_ojk_items_tenant_status" json:"tenant_id"`
	RunID          string         `gorm:"type:text;not null;index:idx_ojk_items_run;uniqueIndex:idx_ojk_items_run_req" json:"run_id"`
	Regulation     string         `gorm:"type:text;not null" json:"regulation"`
	Pasal          string         `gorm:"type:text;not null" json:"pasal"`
	PasalText      string         `gorm:"type:text;not null" json:"pasal_text"`
	Area           *string        `gorm:"type:text" json:"area"`
	Requirement    string         `gorm:"type:text;not null" json:"requirement"`
	RequirementID  *string        `gorm:"type:text;uniqueIndex:idx_ojk_items_run_req" json:"requirement_id"`
	EvidenceType   *string        `gorm:"type:text" json:"evidence_type"`
	CheckMethod    *string        `gorm:"type:text" json:"check_method"`
	ApplicableRoles StrList       `gorm:"type:text" json:"applicable_roles"`
	Severity       string         `gorm:"type:text;not null;default:'info'" json:"severity"`
	Keywords       StrList        `gorm:"type:text" json:"keywords"`
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
	// ErrOJKRunActive：互斥（2026-09-28 用户需求）——同一时刻只允许一个
	// 抽取任务。并发 run 会叠加 LLM 调用并让 STAGING 卡无法如实展示。
	ErrOJKRunActive = errors.New("another extraction run is already active")
	// ErrOJKRunDeletingActive：进行中的版本不允许删除（先等它跑完/失败）
	ErrOJKRunDeletingActive = errors.New("cannot delete a run that is pending or running")
	// ErrOJKInvalidVersion：版本号重命名校验失败
	ErrOJKInvalidVersion = errors.New("skill_version must be 1-32 characters after trimming")
)

// CreateRun 校验 KB 后落一条 pending run，并异步启动执行器
// （重建全文 → Pasal 切片 → 分批调 LLM → 校验入库）。
// skillVersion 留空时自动递增（2026-09-28 用户需求），不再固定 v1.0.0。
func (s *OJKService) CreateRun(ctx context.Context, tenantID uint64, kbID, skillVersion string) (*RunStatus, error) {
	if skillVersion == "" {
		skillVersion = s.nextSkillVersion(ctx, tenantID)
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
	var activeRuns int64
	if err := s.db.WithContext(ctx).Raw(
		"SELECT count(*) FROM ojk_runs WHERE status IN ('pending','running')").Scan(&activeRuns).Error; err != nil {
		return nil, fmt.Errorf("check active runs: %w", err)
	}
	if activeRuns > 0 {
		return nil, ErrOJKRunActive
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

// PreflightResult 是向导确认步的预检结论。
type PreflightResult struct {
	KBID          string   `json:"kb_id"`
	KBName        string   `json:"kb_name"`
	Docs          int64    `json:"docs"`
	PasalSections int      `json:"pasal_sections"`
	Titles        []string `json:"titles,omitempty"`
}

// Preflight 对指定 KB 做轻量预检：文档数 + Pasal 段数。
// pasal_sections=0 意味着该库没有法规结构（多为申请人材料库），
// 前端在确认步直接拦下，避免跑到一半才失败。
func (s *OJKService) Preflight(ctx context.Context, tenantID uint64, kbID string) (*PreflightResult, error) {
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
	out := &PreflightResult{KBID: kb.ID, KBName: kb.Name}
	if err := s.db.WithContext(ctx).Raw(
		"SELECT count(*) FROM knowledges WHERE knowledge_base_id = ? AND deleted_at IS NULL "+
			"AND (parse_status = 'completed' OR parse_status = 'finished' OR parse_status = '')",
		kbID).Scan(&out.Docs).Error; err != nil {
		return nil, fmt.Errorf("count kb documents: %w", err)
	}
	text, err := s.LoadRegulationTextFn(ctx, tenantID, kbID)
	if err != nil {
		return nil, fmt.Errorf("rebuild regulation text: %w", err)
	}
	out.PasalSections = len(slicePasal(text, kb.Name, kb.Name))
	var titles []string
	s.db.WithContext(ctx).Raw(
		"SELECT title FROM knowledges WHERE knowledge_base_id = ? AND deleted_at IS NULL ORDER BY title",
		kbID).Scan(&titles)
	out.Titles = titles
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

// nextSkillVersion 生成下一个版本号：取本租户历史版本里"点分数字"形态
// （如 1.0.0 / 2.1，容忍 v 前缀）的最大值末段 +1；无可解析历史则从 1.0.0 起步。
// 非数字形态（如重命名过的 2026.09-review）不参与递增。
func (s *OJKService) nextSkillVersion(ctx context.Context, tenantID uint64) string {
	var versions []string
	if err := s.db.WithContext(ctx).Model(&OJKRun{}).
		Where("tenant_id = ?", tenantID).
		Distinct().Pluck("skill_version", &versions).Error; err != nil {
		return "1.0.0"
	}
	var max []int
	for _, v := range versions {
		nums := parseVersionTuple(v)
		if nums == nil {
			continue
		}
		if versionLess(max, nums) {
			max = nums
		}
	}
	if max == nil {
		return "1.0.0"
	}
	max[len(max)-1]++
	out := make([]string, len(max))
	for i, n := range max {
		out[i] = strconv.Itoa(n)
	}
	return strings.Join(out, ".")
}

// parseVersionTuple 把 "v1.0.2" 解析成 [1,0,2]；非纯点分数字返回 nil。
func parseVersionTuple(v string) []int {
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".")
	if len(parts) == 0 || len(parts) > 4 {
		return nil
	}
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 {
			return nil
		}
		nums = append(nums, n)
	}
	return nums
}

// versionLess 按 数值/缺段补 0 比较；a 为 nil 表示"尚无最大值"，恒小于 b。
func versionLess(a, b []int) bool {
	if a == nil {
		return b != nil
	}
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		av, bv := 0, 0
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		if av != bv {
			return av < bv
		}
	}
	return false
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

// DeleteRun 删除一个历史版本（run + 其全部条目，同事务）。
// pending/running 的抽取不允许删——它还在被执行器推进，删了只会产生僵尸写入。
func (s *OJKService) DeleteRun(ctx context.Context, tenantID uint64, runID string) error {
	var run OJKRun
	if err := s.db.WithContext(ctx).
		Where("run_id = ? AND tenant_id = ?", runID, tenantID).
		First(&run).Error; err != nil {
		return fmt.Errorf("get ojk run: %w", err)
	}
	if run.Status == "pending" || run.Status == "running" {
		return ErrOJKRunDeletingActive
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("run_id = ? AND tenant_id = ?", runID, tenantID).
			Delete(&OJKChecklistItem{}).Error; err != nil {
			return fmt.Errorf("delete ojk checklist items: %w", err)
		}
		if err := tx.Where("run_id = ? AND tenant_id = ?", runID, tenantID).
			Delete(&OJKRun{}).Error; err != nil {
			return fmt.Errorf("delete ojk run: %w", err)
		}
		return nil
	})
}

// UpdateRunVersion 重命名一个版本的 skill_version 标签。
// 去掉首字母 v（展示层统一补 v），并限制 1-32 字符。
func (s *OJKService) UpdateRunVersion(ctx context.Context, tenantID uint64, runID, skillVersion string) (*RunStatus, error) {
	skillVersion = strings.TrimSpace(skillVersion)
	skillVersion = strings.TrimPrefix(skillVersion, "v")
	if skillVersion == "" || len(skillVersion) > 32 {
		return nil, ErrOJKInvalidVersion
	}
	res := s.db.WithContext(ctx).Model(&OJKRun{}).
		Where("run_id = ? AND tenant_id = ?", runID, tenantID).
		Update("skill_version", skillVersion)
	if res.Error != nil {
		return nil, fmt.Errorf("update ojk run version: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return nil, fmt.Errorf("get ojk run: not found")
	}
	return s.GetRun(ctx, tenantID, runID)
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

// OJKCandidate 登记一位 fit & proper 候选人：材料进入其专属知识库
// （kb_id），文件走 WeKnora 现成的上传→解析→向量化管线。
type OJKCandidate struct {
	ID          string    `gorm:"primaryKey;type:text" json:"id"`
	TenantID    uint64    `gorm:"type:integer;not null;index:idx_ojk_candidates_tenant" json:"tenant_id"`
	Name        string    `gorm:"type:text;not null" json:"name"`
	NIK         *string   `gorm:"type:text" json:"nik"`
	Position    *string   `gorm:"type:text" json:"position"`
	Institution *string   `gorm:"type:text" json:"institution"`
	KBID        string    `gorm:"type:text;not null" json:"kb_id"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (OJKCandidate) TableName() string { return "ojk_candidates" }

// CandidateStatus 候选人列表/详情视图（材料解析态由 knowledges 聚合派生）。
type CandidateStatus struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	NIK         string `json:"nik"`
	Position    string `json:"position"`
	Institution string `json:"institution"`
	KBID        string `json:"kb_id"`
	Status      string `json:"status"`           // queued | parsing | parsed | failed
	Docs        int    `json:"docs"`             // 已上传材料份数
	Parsed      int    `json:"parsed"`           // 已解析份数
	ParsePct    int    `json:"parse_pct"`        // 解析进度百分比
	CreatedAt   string `json:"created_at"`
}

// deriveCandidateStatus 从文档解析态聚合候选人状态：
// 无材料=queued；任一在解析=parsing；任一失败=failed；全部解析完=parsed。
func deriveCandidateStatus(docs, parsed, failed int) string {
	switch {
	case docs == 0:
		return "queued"
	case failed > 0:
		return "failed"
	case parsed < docs:
		return "parsing"
	default:
		return "parsed"
	}
}

// CreateCandidate 登记候选人并落库（KB 由 handler 先行创建后传入 kb_id）。
func (s *OJKService) CreateCandidate(ctx context.Context, cand *OJKCandidate) error {
	cand.ID = fmt.Sprintf("cand-%d-%d", cand.TenantID, time.Now().UnixNano())
	return s.db.WithContext(ctx).Create(cand).Error
}

// ListCandidates 返回候选人列表（按创建时间倒序），解析态实时聚合。
func (s *OJKService) ListCandidates(ctx context.Context, tenantID uint64) ([]CandidateStatus, error) {
	var cands []OJKCandidate
	if err := s.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").Find(&cands).Error; err != nil {
		return nil, fmt.Errorf("list ojk candidates: %w", err)
	}
	out := make([]CandidateStatus, 0, len(cands))
	for _, c := range cands {
		docs, parsed, failed := s.candidateDocStats(ctx, c.KBID, tenantID)
		out = append(out, CandidateStatus{
			ID: c.ID, Name: c.Name,
			NIK: stringPtr(c.NIK), Position: stringPtr(c.Position),
			Institution: stringPtr(c.Institution), KBID: c.KBID,
			Status:  deriveCandidateStatus(docs, parsed, failed),
			Docs:    docs, Parsed: parsed,
			ParsePct: func() int {
				if docs == 0 {
					return 0
				}
				return parsed * 100 / docs
			}(),
			CreatedAt: c.CreatedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

// GetCandidate 单个候选人详情（同列表聚合口径）。
func (s *OJKService) GetCandidate(ctx context.Context, tenantID uint64, id string) (*CandidateStatus, error) {
	var c OJKCandidate
	if err := s.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		First(&c).Error; err != nil {
		return nil, fmt.Errorf("get ojk candidate: %w", err)
	}
	docs, parsed, failed := s.candidateDocStats(ctx, c.KBID, tenantID)
	st := deriveCandidateStatus(docs, parsed, failed)
	pct := 0
	if docs > 0 {
		pct = parsed * 100 / docs
	}
	return &CandidateStatus{
		ID: c.ID, Name: c.Name,
		NIK: stringPtr(c.NIK), Position: stringPtr(c.Position),
		Institution: stringPtr(c.Institution), KBID: c.KBID,
		Status: st, Docs: docs, Parsed: parsed, ParsePct: pct,
		CreatedAt: c.CreatedAt.Format(time.RFC3339),
	}, nil
}

// candidateDocStats 聚合候选人材料 KB 的文档解析态。
func (s *OJKService) candidateDocStats(ctx context.Context, kbID string, tenantID uint64) (docs, parsed, failed int) {
	var rows []struct {
		ParseStatus string
	}
	s.db.WithContext(ctx).Raw(
		"SELECT parse_status FROM knowledges "+
			"WHERE knowledge_base_id = ? AND tenant_id = ? AND deleted_at IS NULL",
		kbID, tenantID).Scan(&rows)
	for _, r := range rows {
		switch r.ParseStatus {
		case "completed", "finished":
			parsed++
		case "failed":
			failed++
		}
		docs++
	}
	return
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
func (s *OJKService) ListItems(ctx context.Context, tenantID uint64, runID string, status *string, severity *string, sortBy *string, sortOrder *string, page, pageSize int) ([]ItemStatus, int64, error) {
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
	if severity != nil && *severity != "" {
		query = query.Where("severity = ?", *severity)
	}
	// 排序：severity/status 按业务等级（critical > clarification > info、
	// pending > confirmed > rejected），sort_by 白名单之外的值不排序
	if sortBy != nil && (*sortBy == "severity" || *sortBy == "status") {
		order := "ASC"
		if sortOrder != nil && *sortOrder == "desc" {
			order = "DESC"
		}
		if *sortBy == "severity" {
			query = query.Order(
				"CASE severity WHEN 'critical' THEN 0 WHEN 'clarification' THEN 1 ELSE 2 END " + order)
		} else {
			query = query.Order(
				"CASE status WHEN 'pending' THEN 0 WHEN 'confirmed' THEN 1 ELSE 2 END " + order)
		}
	}
	query = query.Order("created_at ASC") // 稳定次序兜底

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
			Keywords:        []string(it.Keywords),
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
- R5: ONLY extract requirements whose obligation falls on the
  CANDIDATE (the person being assessed: Direktur, Komisaris,
  Direktur Utama, Pemegang Saham Pengendali, Pejabat Puncak) or that
  can be verified from documents the candidate personally submits
  (appointment letters, certificates, financial statements,
  compliance statements).
  SKIP entirely:
  * obligations on the BANK as an institution ("Bank wajib ...",
    "Bank harus ...") — IT governance, reporting procedures,
    internal committees, infrastructure requirements;
  * procedural/administrative clauses addressed to the bank's
    organs as bodies (Dewan Komisaris charter contents, committee
    meeting rules, report delivery addresses);
  * penalty and sanction mechanics between OJK and the Bank.
  These are institutional duties: no candidate material can ever
  prove them, so they MUST NOT become ChecklistItems.
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

OUTPUT SCHEMA — a JSON array of ChecklistItem objects, each with EXACTLY
these keys:
  "pasal"             string, REQUIRED, MUST be non-empty: the exact Pasal
                      reference the requirement comes from (e.g.
                      "Pasal 5 ayat (2)"). Copy it from the section header
                      shown in PASAL SECTIONS. NEVER null, never "".
  "pasal_text"        string: verbatim key phrase from that Pasal.
  "area"              "Integrity" | "Financial Reputation" | "Competence"
                      | "Structure" | "Completeness" — always fill one.
  "requirement"       string: one testable requirement.
  "severity"          "critical" | "clarification" | "info"
  "check_method"      "document_presence" | "cross_document" | "rule_computation"
  "evidence_type"     string: concrete document or data source name.
  "applicable_roles"  array of strings (see R10).
  "keywords"          array of 2-5 strings.
An object missing "pasal" or leaving it empty is INVALID — omit it
instead of emitting it.

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
		fail("no regulation sections (Pasal) in knowledge base %q — pick the KB that contains regulation full texts; candidate-material KBs are review subjects, not extraction sources", kbName)
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
	// flags 必须逐条随身携带：此前用全局 allFlags 收集再拼成一条长串盖到
	// 每一行，导致所有条目的 Flag 都是全 run 校验告警的重复拼接。
	type ojkExtracted struct {
		item  ojkItem
		flags []string
	}
	var allItems []ojkExtracted
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
			allItems = append(allItems, ojkExtracted{item: v, flags: flags})
		}
		done := i + 1
		s.db.Model(&OJKRun{}).Where("run_id = ?", runID).
			Updates(map[string]interface{}{"slices_done": done})
	}

	// 去重（requirement_id）
	seen := map[string]bool{}
	var deduped []ojkExtracted
	for _, it := range allItems {
		rid, _ := it.item["requirement_id"].(string)
		if rid != "" && seen[rid] {
			continue
		}
		if rid != "" {
			seen[rid] = true
		}
		deduped = append(deduped, it)
	}

	// R2 强制：引用不出 Pasal 的条目不落库（与 run.py 契约一致——
	// "If you cannot cite a Pasal, do not create the item"）。
	// 此前空 pasal 条目照收，导致 512/529 条无法溯源。
	droppedNoPasal := 0
	persistedCount := 0
	persistedFlagged := 0
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		persisted := 0
		for _, it := range deduped {
			item := it.item
			pasal0, _ := item["pasal"].(string)
			if strings.TrimSpace(pasal0) == "" {
				droppedNoPasal++
				continue
			}
			persisted++
			itemID := fmt.Sprintf("c-%s-%04d", runID, persisted)
			pasal := pasal0
			reg, _ := item["regulation"].(string)
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
			if len(it.flags) > 0 {
				f := strings.Join(it.flags, "; ")
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
			if err := tx.Create(&row).Error; err != nil {
				return fmt.Errorf("persist checklist item %s: %w", itemID, err)
			}
			if len(it.flags) > 0 {
				persistedFlagged++
			}
			_ = pasalText
		}
		persistedCount = persisted
		return nil
	}); err != nil {
		fail("persist checklist items: %v", err)
		return
	}

	finalStatus := "done"
	finalErr := ""
	notes := []string{}
	if skippedBatches > 0 {
		notes = append(notes, fmt.Sprintf("%d/%d batches skipped (invalid LLM output)", skippedBatches, total))
	}
	if droppedNoPasal > 0 {
		notes = append(notes, fmt.Sprintf("%d items dropped without Pasal anchor (R2)", droppedNoPasal))
	}
	finalErr = strings.Join(notes, "; ")
	// flagged_items 统计"带告警的落库条数"而非告警总次数
	s.db.Model(&OJKRun{}).Where("run_id = ?", runID).
		Updates(map[string]interface{}{
			"status":        finalStatus,
			"total_items":   persistedCount,
			"flagged_items": persistedFlagged,
			"slices_done":   total,
			"error":         finalErr,
		})
}

// ojkDocFilter 返回法规文档标题的关键词白名单（不区分大小写子串匹配）。
// 默认只抽 Fit & Proper 核心法规（POJK 27/2016 FitProper + SEOJK 39/2016 FPT）：
// 全量抽取会把与候选人无关的银行机构义务条款（POJK 11/2022 IT 治理等）带进来，
// 这类条款无法用申请人材料核验。OJK_DOC_FILTER=- 关闭过滤；逗号分隔自定义关键词。
func ojkDocFilter() []string {
	v := os.Getenv("OJK_DOC_FILTER")
	if v == "-" {
		return nil
	}
	if v == "" {
		v = "FitProper,FPT"
	}
	return strings.Split(v, ",")
}

// loadRegulationText 从 chunks 重建法规全文（与 run.py 同口径：冲突检测 + 按
// start_at 拼接；过滤 sbk_method=failed 与已删除 chunk）。
func (s *OJKService) loadRegulationTextFromDB(ctx context.Context, tenantID uint64, kbID string) (string, error) {
	var rows []struct {
		Content   string
		StartAt   int64
		EndAt     int64
		DocTitle  string
	}
	if err := s.db.WithContext(ctx).Raw(
		"SELECT c.content, c.start_at, c.end_at, k.title AS doc_title "+
			"FROM chunks c LEFT JOIN knowledges k ON k.id = c.knowledge_id "+
			"WHERE c.knowledge_base_id = ? AND c.tenant_id = ? AND c.deleted_at IS NULL "+
			"AND coalesce(c.metadata->>'sbk_method','') != 'failed' "+
			"ORDER BY c.start_at", kbID, tenantID).Scan(&rows).Error; err != nil {
		return "", err
	}
	if kw := ojkDocFilter(); len(kw) > 0 {
		filtered := rows[:0]
		for _, r := range rows {
			title := strings.ToLower(r.DocTitle)
			for _, k := range kw {
				if k != "" && strings.Contains(title, strings.ToLower(k)) {
					filtered = append(filtered, r)
					break
				}
			}
		}
		rows = filtered
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
	if str("area") == "" {
		flags = append(flags, "area_missing")
	}
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
		// 前缀归一：LLM 常返回比切片锚点更细的引用（"Pasal 19 ayat (2) huruf c"
		// vs 切片 ref "Pasal 19"）——前缀命中即视为已验证，避免误报
		verified := false
		for ref := range allRefs {
			if ref != "" && strings.HasPrefix(pasal, ref) {
				verified = true
				break
			}
		}
		if !verified {
			flags = append(flags, "pasal_unverified")
		}
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
	if err := db.AutoMigrate(&OJKRun{}, &OJKChecklistItem{}, &OJKCandidate{}); err != nil {
		fmt.Printf("[ojk] automigrate warn (tables managed by migration 000112): %v\n", err)
	}
	return nil
}

// NextSkillVersion 导出给 handler/测试用的下一个版本号推导。
func (s *OJKService) NextSkillVersion(ctx context.Context, tenantID uint64) (string, error) {
	return s.nextSkillVersion(ctx, tenantID), nil
}
