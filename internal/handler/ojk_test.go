package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/types"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&service.OJKRun{}, &service.OJKChecklistItem{}))
	return db
}

func TestOJKRunHandler_CreateRun(t *testing.T) {
	db := testDB(t)
	svc := service.NewOJKService(db)
	h := handler.NewOJKRunHandler(svc)

	r := gin.New()
	r.POST("/ojk/runs", h.CreateRun)

	body := bytes.NewReader([]byte(`{"skill_version":"1.0.0"}`))
	req := httptest.NewRequest("POST", "/ojk/runs", body)
	req.Header.Set("Content-Type", "application/json")
	// Inject tenant ID into request context (mirrors middleware behavior)
	ctx := context.WithValue(req.Context(), types.TenantIDContextKey, uint64(10011))
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	runID := resp["run_id"]
	assert.NotEmpty(t, runID)

	var run service.OJKRun
	require.NoError(t, db.Where("run_id = ?", runID).First(&run).Error)
	assert.Equal(t, "pending", run.Status)
	assert.Equal(t, uint64(10011), run.TenantID)
}

func TestOJKRunHandler_GetRun(t *testing.T) {
	db := testDB(t)
	svc := service.NewOJKService(db)
	h := handler.NewOJKRunHandler(svc)

	r := gin.New()
	r.POST("/ojk/runs", h.CreateRun)
	r.GET("/ojk/runs/:run_id", h.GetRun)

	// Create run
	body, _ := json.Marshal(map[string]string{"skill_version": "1.0.0"})
	req := httptest.NewRequest("POST", "/ojk/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), types.TenantIDContextKey, uint64(10011)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var created map[string]string
	json.Unmarshal(w.Body.Bytes(), &created)
	runID := created["run_id"]

	// Get run
	req2 := httptest.NewRequest("GET", "/ojk/runs/"+runID, nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), types.TenantIDContextKey, uint64(10011)))
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	var status service.RunStatus
	json.Unmarshal(w2.Body.Bytes(), &status)
	assert.Equal(t, runID, status.RunID)
	assert.Equal(t, "pending", status.Status)
}

func TestOJKItemsHandler_ListItems(t *testing.T) {
	db := testDB(t)
	svc := service.NewOJKService(db)
	h := handler.NewOJKItemsHandler(svc)

	run := service.OJKRun{RunID: "r-test", TenantID: 10011, Status: "done"}
	require.NoError(t, db.Create(&run).Error)
	for i := 1; i <= 5; i++ {
		item := service.OJKChecklistItem{
			ID:          fmt.Sprintf("c-r-test-%04d", i),
			TenantID:    10011,
			RunID:       "r-test",
			Regulation:  "POJK 17/POJK.03/2023",
			Pasal:       fmt.Sprintf("Pasal %d", i),
			Requirement: fmt.Sprintf("Test requirement %d", i),
			Severity:    "info",
			Status:      "pending",
		}
		require.NoError(t, db.Create(&item).Error)
	}

	r := gin.New()
	r.GET("/ojk/items", h.ListItems)

	req := httptest.NewRequest("GET", "/ojk/items?run_id=r-test&page=1&page_size=3", nil)
	req = req.WithContext(context.WithValue(req.Context(), types.TenantIDContextKey, uint64(10011)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, float64(3), resp["page_size"])
	assert.Equal(t, float64(5), resp["total"])
	items := resp["items"].([]interface{})
	assert.Len(t, items, 3)
}

func TestOJKRunHandler_DeleteRun(t *testing.T) {
	db := testDB(t)
	svc := service.NewOJKService(db)
	h := handler.NewOJKRunHandler(svc)

	seed := func(runID, status string, items int) {
		run := service.OJKRun{RunID: runID, TenantID: 10011, Status: status}
		require.NoError(t, db.Create(&run).Error)
		for i := 1; i <= items; i++ {
			item := service.OJKChecklistItem{
				ID: fmt.Sprintf("c-%s-%04d", runID, i), TenantID: 10011, RunID: runID,
				Regulation: "POJK 17", Pasal: fmt.Sprintf("Pasal %d", i),
				Requirement: fmt.Sprintf("req %d", i), Severity: "info", Status: "pending",
			}
			require.NoError(t, db.Create(&item).Error)
		}
	}
	seed("r-done", "done", 3)
	seed("r-live", "running", 1)

	r := gin.New()
	r.DELETE("/ojk/runs/:run_id", h.DeleteRun)

	// 进行中的版本不允许删除
	req := httptest.NewRequest("DELETE", "/ojk/runs/r-live", nil)
	req = req.WithContext(context.WithValue(req.Context(), types.TenantIDContextKey, uint64(10011)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusConflict, w.Code)

	// done 版本连条目一起删
	req2 := httptest.NewRequest("DELETE", "/ojk/runs/r-done", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), types.TenantIDContextKey, uint64(10011)))
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	var runCount, itemCount int64
	db.Model(&service.OJKRun{}).Where("run_id = ?", "r-done").Count(&runCount)
	db.Model(&service.OJKChecklistItem{}).Where("run_id = ?", "r-done").Count(&itemCount)
	assert.Zero(t, runCount)
	assert.Zero(t, itemCount)

	// 不存在的版本 404
	req3 := httptest.NewRequest("DELETE", "/ojk/runs/r-missing", nil)
	req3 = req3.WithContext(context.WithValue(req3.Context(), types.TenantIDContextKey, uint64(10011)))
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusNotFound, w3.Code)
}

func TestOJKRunHandler_UpdateRunVersion(t *testing.T) {
	db := testDB(t)
	svc := service.NewOJKService(db)
	h := handler.NewOJKRunHandler(svc)

	run := service.OJKRun{RunID: "r-rename", TenantID: 10011, Status: "done", SkillVersion: "1.0.0"}
	require.NoError(t, db.Create(&run).Error)

	r := gin.New()
	r.PATCH("/ojk/runs/:run_id", h.UpdateRun)

	// 带首字母 v 也能存进去（展示层统一补 v）
	body, _ := json.Marshal(map[string]string{"skill_version": "v2026.09-review"})
	req := httptest.NewRequest("PATCH", "/ojk/runs/r-rename", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), types.TenantIDContextKey, uint64(10011)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var status service.RunStatus
	json.Unmarshal(w.Body.Bytes(), &status)
	assert.Equal(t, "2026.09-review", status.SkillVersion)

	// 空版本号 → 400
	body2, _ := json.Marshal(map[string]string{"skill_version": "   "})
	req2 := httptest.NewRequest("PATCH", "/ojk/runs/r-rename", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	req2 = req2.WithContext(context.WithValue(req2.Context(), types.TenantIDContextKey, uint64(10011)))
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusBadRequest, w2.Code)
}

// 跨 run 的同 requirement_id 不再全局唯一（版本=同一定义的快照），
// 唯一性收窄到 (run_id, requirement_id) —— migration 000113 的回归测试。
func TestOJKItems_RequirementIDUniquePerRun(t *testing.T) {
	db := testDB(t)

	mk := func(runID, reqID string) *service.OJKChecklistItem {
		rid := reqID
		return &service.OJKChecklistItem{
			ID: fmt.Sprintf("c-%s-0001", runID), TenantID: 10011, RunID: runID,
			Regulation: "POJK 17", Pasal: "Pasal 3", Requirement: "req",
			RequirementID: &rid, Severity: "info", Status: "pending",
		}
	}
	require.NoError(t, db.Create(mk("r-a", "SEOJA-P3-1")).Error)
	require.NoError(t, db.Create(mk("r-b", "SEOJA-P3-1")).Error, "same requirement_id in another run must be allowed")
	require.Error(t, db.Create(mk("r-a", "SEOJA-P3-1")).Error, "duplicate within one run must violate the composite unique")
}

// StrList 往返：PG 字面量与 JSON 两种形态都要能扫回；含逗号/引号的值不失真。
func TestOJKItems_StrListRoundTrip(t *testing.T) {
	db := testDB(t)

	run := service.OJKRun{RunID: "r-sl", TenantID: 10011, Status: "done"}
	require.NoError(t, db.Create(&run).Error)
	item := service.OJKChecklistItem{
		ID: "c-r-sl-0001", TenantID: 10011, RunID: "r-sl",
		Regulation: "POJK 17", Pasal: "Pasal 3", Requirement: "req",
		ApplicableRoles: service.StrList{"Director, Compliance", "Risk \"Owner\""},
		Keywords:        service.StrList{"a,b", "c"},
		Severity:        "info", Status: "pending",
	}
	require.NoError(t, db.Create(&item).Error)

	var got service.OJKChecklistItem
	require.NoError(t, db.Where("id = ?", "c-r-sl-0001").First(&got).Error)
	assert.Equal(t, []string{"Director, Compliance", "Risk \"Owner\""}, []string(got.ApplicableRoles))
	assert.Equal(t, []string{"a,b", "c"}, []string(got.Keywords))
}

// 创建新版时版本号自动递增（2026-09-28 用户需求）：
// 无历史 → 1.0.0；历史含 1.0.0/1.0.3/2.0 → 2.0.1；非数字标签不参与递增。
func TestOJKService_NextSkillVersion(t *testing.T) {
	db := testDB(t)
	svc := service.NewOJKService(db)

	seed := func(ver string) {
		run := service.OJKRun{RunID: "r-" + ver, TenantID: 10011, Status: "done", SkillVersion: ver}
		require.NoError(t, db.Create(&run).Error)
	}

	next := func() string {
		v, err := svc.NextSkillVersion(context.Background(), 10011)
		require.NoError(t, err)
		return v
	}

	assert.Equal(t, "1.0.0", next(), "no history -> 1.0.0")
	seed("1.0.0")
	assert.Equal(t, "1.0.1", next())
	seed("1.0.3")
	seed("v2.0")
	assert.Equal(t, "2.1", next(), "末段 +1：2.0 -> 2.1")
	seed("2026.09-review") // 非纯数字，不参与
	assert.Equal(t, "2.1", next())
	seed("10.0")
	assert.Equal(t, "10.1", next())
}
