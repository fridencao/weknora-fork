package handler

import (
	"errors"
	"net/http"
	"strconv"


	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
)

// OJKRunHandler handles OJK run lifecycle endpoints.
type OJKRunHandler struct {
	svc *service.OJKService
}

// NewOJKRunHandler creates a new OJK run handler.
func NewOJKRunHandler(svc *service.OJKService) *OJKRunHandler {
	return &OJKRunHandler{svc: svc}
}

// CreateRunRequest is the request body for starting a new extraction run.
type CreateRunRequest struct {
	KBID         string `json:"kb_id" binding:"required,uuid"`
	SkillVersion string `json:"skill_version" binding:"omitempty,max=64"`
}

// CreateRun starts a new OJK rule extraction run.
//
// POST /api/v1/ojk/runs
// @Summary      Start OJK checklist extraction run
// @Description  Creates a new run record (status=pending) and returns the run_id.
// @Tags         OJK
// @Produce      json
// @Param        body  body      CreateRunRequest  true  "Run parameters"
// @Success      201   {object}  map[string]string "run_id"
// @Failure      400   {object}  map[string]string "bad request"
// @Router       /api/v1/ojk/runs [post]
func (h *OJKRunHandler) CreateRun(c *gin.Context) {
	tenantID := types.MustTenantIDFromContext(c.Request.Context())
	var req CreateRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	status, err := h.svc.CreateRun(c.Request.Context(), tenantID, req.KBID, req.SkillVersion)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOJKKBNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrOJKKBNoDocs):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusCreated, status)
}

// ListRuns returns recent extraction runs (latest first).
//
// GET /api/v1/ojk/runs?limit=20
// @Summary      List OJK runs
// @Description  最近 extraction runs（含 KB 名与切片进度），供「最近生成」表使用。
// @Tags         OJK
// @Param        limit  query  int  false  "返回条数（默认 20，上限 100）"
// @Success      200    {object}  map[string]interface{}
// @Router       /api/v1/ojk/runs [get]
func (h *OJKRunHandler) ListRuns(c *gin.Context) {
	tenantID := types.MustTenantIDFromContext(c.Request.Context())
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	runs, err := h.svc.ListRuns(c.Request.Context(), tenantID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"runs": runs, "total": len(runs)})
}

// GetRun returns the current status of a run.
//
// GET /api/v1/ojk/runs/:run_id
// @Summary      Get OJK run status
// @Description  Returns the run status and statistics. Frontend polls this endpoint.
// @Tags         OJK
// @Produce      json
// @Param        run_id  path      string  true  "Run ID"
// @Success      200     {object}  service.RunStatus
// @Failure      404     {object}  map[string]string "not found"
// @Router       /api/v1/ojk/runs/{run_id} [get]
func (h *OJKRunHandler) GetRun(c *gin.Context) {
	tenantID := types.MustTenantIDFromContext(c.Request.Context())
	runID := c.Param("run_id")

	status, err := h.svc.GetRun(c.Request.Context(), tenantID, runID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, status)
}
