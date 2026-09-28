package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

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
	// :memory: 每个连接是独立库——锁单连接，避免迁移/写入/查询分到不同连接
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
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

// ---- RW 向导重构（2026-09-28）：KB 绑定 + Go 执行器 ----

func TestOJKSlicePasal(t *testing.T) {
	text := "# POJK 12/2020\n" +
		"## Pasal 1\n" +
		"Ketentuan dalam Peraturan Ini adalah: (definitions text here)\n" +
		"## Pasal 2 ayat (1)\n" +
		"Setiap bank wajib memiliki sistem tata kelola yang sehat.\n" +
		"PENJELASAN ATAS PERATURAN\n" +
		"## Pasal 3\n" +
		"Penjelasan pasal tiga berisi keterangan tambahan yang cukup panjang.\n" +
		"## Pasal 4\n" +
		"short\n"

	slices := service.SliceOJKPasal(text, "POJK 12/2020.pdf", "POJK 12/2020")
	require.Len(t, slices, 3) // huruf a 段太短被跳过
	assert.Equal(t, "Pasal 1", slices[0].Ref)
	assert.Equal(t, "Pasal 2 ayat (1)", slices[1].Ref)
	assert.Equal(t, "Pasal 3 (Penjelasan)", slices[2].Ref)
	assert.False(t, slices[0].IsPenjelasan)
	assert.True(t, slices[2].IsPenjelasan)
	// 正文清洗：锚点/标题标记被剥除、空白折叠
	assert.NotContains(t, slices[0].Body, "##")
	assert.NotContains(t, slices[0].Body, "\n")
}

func TestOJKBatchSlices(t *testing.T) {
	mk := func(n int) service.OJKSlice {
		return service.OJKSlice{Body: strings.Repeat("x", n)}
	}
	batches := service.BatchOJKSlices([]service.OJKSlice{mk(7000), mk(6000), mk(500)}, 12000)
	require.Len(t, batches, 2)
	assert.Len(t, batches[0], 1)
	assert.Len(t, batches[1], 2)
}

func TestOJKValidateItem(t *testing.T) {
	refs := map[string]bool{"Pasal 2": true}
	item := map[string]interface{}{
		"pasal": "Pasal 2", "severity": "critical",
		"check_method": "document_presence",
	}
	got, flags := service.ValidateOJKItem(item, refs)
	assert.Empty(t, flags)
	assert.Equal(t, []interface{}{"*"}, got["applicable_roles"], "roles 兜底 *")

	bad := map[string]interface{}{"pasal": "Pasal 99", "severity": "nope", "area": "??", "check_method": "??"}
	_, flags = service.ValidateOJKItem(bad, refs)
	assert.ElementsMatch(t, []string{"invalid_area", "invalid_severity", "invalid_check_method", "pasal_unverified"}, flags)
}

func TestOJKCreateRunValidatesKB(t *testing.T) {
	db := openTestDB(t)
	// 同库里有 knowledge_bases/knowledges 表（校验查询用）
	require.NoError(t, db.Exec("CREATE TABLE knowledge_bases (id text primary key, name text, tenant_id integer, deleted_at integer)").Error)
	require.NoError(t, db.Exec("CREATE TABLE knowledges (id text primary key, knowledge_base_id text, deleted_at integer, parse_status text)").Error)
	require.NoError(t, db.Exec("INSERT INTO knowledge_bases VALUES ('kb-1','OJK 法规','10011',NULL)").Error)
	require.NoError(t, db.Exec("INSERT INTO knowledges VALUES ('d1','kb-1',NULL,'completed')").Error)

	svc := service.NewOJKService(db)
	// 未知 KB → 404 语义
	_, err := svc.CreateRun(context.Background(), 10011, "kb-none", "")
	require.ErrorIs(t, err, service.ErrOJKKBNotFound)
	// 空文档 KB → 400 语义
	require.NoError(t, db.Exec("INSERT INTO knowledge_bases VALUES ('kb-2','空库','10011',NULL)").Error)
	_, err = svc.CreateRun(context.Background(), 10011, "kb-2", "")
	require.ErrorIs(t, err, service.ErrOJKKBNoDocs)
	// 合法 KB → 创建成功且状态 pending
	st, err := svc.CreateRun(context.Background(), 10011, "kb-1", "")
	require.NoError(t, err)
	assert.Equal(t, "kb-1", st.KBID)
	assert.Equal(t, "OJK 法规", st.KBName)
}

func TestOJKExecutorEndToEnd(t *testing.T) {
	db := openTestDB(t)
	svc := service.NewOJKService(db)

	// 注入数据面与 LLM：跑一条 2 批的迷你管线
	svc.LoadRegulationTextFn = func(ctx context.Context, tenantID uint64, kbID string) (string, error) {
		return "# 法规\n## Pasal 1\n银行应当建立稳健的公司治理结构并持续满足监管要求。\n", nil
	}
	svc.LoadModelConfigFn = func(ctx context.Context, tenantID uint64) (service.LLMConfig, error) {
		return service.LLMConfig{APIKey: "k", BaseURL: "http://mock", Model: "glm"}, nil
	}
	batchCount := 0
	svc.InvokeLLMFn = func(ctx context.Context, cfg service.LLMConfig, system, user string) (string, error) {
		batchCount++
		return `{"items":[{"regulation":"法规","pasal":"Pasal 1","pasal_text":"…","requirement":"建立治理结构","severity":"critical","check_method":"document_presence","applicable_roles":["board"]}]}`, nil
	}

	require.NoError(t, db.Exec("CREATE TABLE knowledge_bases (id text primary key, name text, tenant_id integer, deleted_at integer)").Error)
	require.NoError(t, db.Exec("INSERT INTO knowledge_bases VALUES ('kb-1','法规库','10011',NULL)").Error)
	require.NoError(t, db.Exec("CREATE TABLE knowledges (id text primary key, knowledge_base_id text, deleted_at integer, parse_status text)").Error)
	require.NoError(t, db.Exec("INSERT INTO knowledges VALUES ('d1','kb-1',NULL,'completed')").Error)

	st, err := svc.CreateRun(context.Background(), 10011, "kb-1", "")
	require.NoError(t, err)

	// 执行器是异步的：轮询等终态（测试里注入的 LLM 立即返回，秒级内应完成）
	deadline := time.Now().Add(5 * time.Second)
	for {
		st, err = svc.GetRun(context.Background(), 10011, st.RunID)
		require.NoError(t, err)
		if st.Status == "done" || st.Status == "failed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run not finished in time: %+v", st)
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.Equal(t, "done", st.Status, st.Error)
	assert.Equal(t, 1, st.TotalItems)
	assert.Equal(t, 1, batchCount)

	items, total, err := svc.ListItems(context.Background(), 10011, st.RunID, nil, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "Pasal 1", items[0].Pasal)
}
