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
