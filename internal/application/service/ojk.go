// Package service — OJK Fit & Proper checklist management.
//
// This package provides the business logic for the OJK plugin:
//   - Running the fp-rule-skill to extract checklist items from regulation KBs
//   - Managing the extraction run lifecycle (pending → running → done/failed)
//   - CRUD for extracted checklist items with human-review support
package service

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	)

// OJKRun represents one execution of the fp-rule-skill.
type OJKRun struct {
	RunID         string     `gorm:"primaryKey;type:text" json:"run_id"`
	TenantID      uint64     `gorm:"type:integer;not null;index:idx_ojk_runs_tenant_status" json:"tenant_id"`
	SkillVersion  string     `gorm:"type:text;not null;default:'1.0.0'" json:"skill_version"`
	Status        string     `gorm:"type:text;not null;default:'pending'" json:"status"`
	TotalSlices   *int       `gorm:"type:integer" json:"total_slices"`
	TotalItems    int        `gorm:"type:integer;default:0" json:"total_items"`
	FlaggedItems  int        `gorm:"type:integer;default:0" json:"flagged_items"`
	Error         string     `gorm:"type:text" json:"error"`
	CreatedAt     time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
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
	ApplicableRoles []string      `gorm:"type:text[]" json:"applicable_roles"`
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

// OJKService handles OJK Fit & Proper checklist operations.
type OJKService struct {
	db *gorm.DB
}

// NewOJKService creates a new OJK service.
func NewOJKService(db *gorm.DB) *OJKService {
	return &OJKService{db: db}
}

// RunStatus is the current status of an OJK run.
type RunStatus struct {
	RunID        string `json:"run_id"`
	TenantID     uint64 `json:"tenant_id"`
	SkillVersion string `json:"skill_version"`
	Status       string `json:"status"`
	TotalSlices  *int   `json:"total_slices,omitempty"`
	TotalItems   int    `json:"total_items"`
	FlaggedItems int    `json:"flagged_items"`
	Error        string `json:"error,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// CreateRun inserts a new run record and returns its ID.
func (s *OJKService) CreateRun(ctx context.Context, tenantID uint64, skillVersion string) (string, error) {
	runID := fmt.Sprintf("ojk-%d-%d", tenantID, time.Now().UnixNano())
	run := &OJKRun{
		RunID:        runID,
		TenantID:     tenantID,
		SkillVersion: skillVersion,
		Status:       "pending",
	}
	if err := s.db.WithContext(ctx).Create(run).Error; err != nil {
		return "", fmt.Errorf("create ojk run: %w", err)
	}
	return runID, nil
}

// GetRun returns the current status of a run.
func (s *OJKService) GetRun(ctx context.Context, tenantID uint64, runID string) (*RunStatus, error) {
	var run OJKRun
	if err := s.db.WithContext(ctx).
		Where("run_id = ? AND tenant_id = ?", runID, tenantID).
		First(&run).Error; err != nil {
		return nil, fmt.Errorf("get ojk run: %w", err)
	}
	return &RunStatus{
		RunID:        run.RunID,
		TenantID:     run.TenantID,
		SkillVersion: run.SkillVersion,
		Status:       run.Status,
		TotalSlices:  run.TotalSlices,
		TotalItems:   run.TotalItems,
		FlaggedItems: run.FlaggedItems,
		Error:        run.Error,
		CreatedAt:    run.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    run.UpdatedAt.Format(time.RFC3339),
	}, nil
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

// EnsureTables creates the OJK tables if they don't exist.
// Called during app startup to handle migrations gracefully.
func EnsureTables(db *gorm.DB) error {
	return db.AutoMigrate(&OJKRun{}, &OJKChecklistItem{})
}
