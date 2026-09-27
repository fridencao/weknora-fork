package service_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/application/service"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&service.OJKRun{}, &service.OJKChecklistItem{}))
	return db
}

func TestOJKRunLifecycle(t *testing.T) {
	db := openTestDB(t)

	run := service.OJKRun{
		RunID:        "ojk-t-1",
		TenantID:     10011,
		SkillVersion: "1.0.0",
		Status:       "pending",
	}
	require.NoError(t, db.Create(&run).Error)

	var got service.OJKRun
	require.NoError(t, db.Where("run_id = ?", "ojk-t-1").First(&got).Error)
	assert.Equal(t, "ojk-t-1", got.RunID)
	assert.Equal(t, uint64(10011), got.TenantID)
	assert.Equal(t, "pending", got.Status)

	require.NoError(t, db.Model(&run).Updates(map[string]interface{}{
		"status":      "running",
		"total_items": 42,
	}).Error)
	require.NoError(t, db.Where("run_id = ?", "ojk-t-1").First(&got).Error)
	assert.Equal(t, "running", got.Status)
	assert.Equal(t, 42, got.TotalItems)
}

func TestOJKChecklistItemCRUD(t *testing.T) {
	db := openTestDB(t)

	run := service.OJKRun{RunID: "r1", TenantID: 10011, Status: "done"}
	require.NoError(t, db.Create(&run).Error)

	items := []service.OJKChecklistItem{
		{
			ID:            "c-r1-0001",
			TenantID:      10011,
			RunID:         "r1",
			Regulation:    "POJK 17/POJK.03/2023",
			Pasal:         "Pasal 48 ayat (1)",
			PasalText:     "Direksi wajib menyampaikan daftar jabatan lain",
			Area:          strPtr("Structure"),
			Requirement:   "Disclose all concurrent positions",
			Severity:      "critical",
			Source:        "normal",
			Status:        "pending",
		},
		{
			ID:            "c-r1-0002",
			TenantID:      10011,
			RunID:         "r1",
			Regulation:    "POJK 27/POJK.03/2016",
			Pasal:         "Pasal 5",
			PasalText:     "Calon tidak pernah dijatuhi hukuman pidana",
			Area:          strPtr("Integrity"),
			Requirement:   "No criminal record",
			Severity:      "info",
			Source:        "normal",
			Status:        "pending",
		},
	}
	for _, it := range items {
		require.NoError(t, db.Create(&it).Error)
	}

	var list []service.OJKChecklistItem
	require.NoError(t, db.Where("run_id = ? AND tenant_id = ?", "r1", 10011).
		Order("created_at ASC").Find(&list).Error)
	assert.Len(t, list, 2)
	assert.Equal(t, "c-r1-0001", list[0].ID)

	require.NoError(t, db.Model(&service.OJKChecklistItem{}).
		Where("id = ?", "c-r1-0001").
		Updates(map[string]interface{}{"status": "confirmed", "reviewer_note": "OK"}).
		Error)
	var confirmed service.OJKChecklistItem
	require.NoError(t, db.Where("id = ?", "c-r1-0001").First(&confirmed).Error)
	assert.Equal(t, "confirmed", confirmed.Status)
	assert.Equal(t, "OK", *confirmed.ReviewerNote)
}

func strPtr(s string) *string { return &s }
