package handler

import (
	"net/http"
	
	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
)

// OJKItemsHandler handles checklist item CRUD for the review workflow.
type OJKItemsHandler struct {
	svc *service.OJKService
}

// NewOJKItemsHandler creates a new items handler.
func NewOJKItemsHandler(svc *service.OJKService) *OJKItemsHandler {
	return &OJKItemsHandler{svc: svc}
}

// ListItemsRequest query params for GET /api/v1/ojk/items
type ListItemsRequest struct {
	RunID     string `form:"run_id" binding:"required"`
	Status    *string `form:"status"`
	Severity  *string `form:"severity"`
	SortBy    *string `form:"sort_by" binding:"omitempty,oneof=severity status"`
	SortOrder *string `form:"sort_order" binding:"omitempty,oneof=asc desc"`
	Page      int    `form:"page" binding:"min=1"`
	Size      int    `form:"page_size" binding:"min=1,max=100"`
}

// ListItems returns paginated checklist items.
//
// GET /api/v1/ojk/items
// @Summary      List OJK checklist items
// @Description  Paginated list of extracted checklist items for a run.
// @Tags         OJK
// @Produce      json
// @Param        run_id    query     string  true  "Run ID"
// @Param        status    query     string  false  "Filter by status"
// @Param        page      query     int     false  "Page number (default 1)"
// @Param        page_size query     int     false  "Page size (default 20, max 100)"
// @Success      200       {object}  map[string]interface{} "{items:[...], total:N, page:N, page_size:N}"
// @Router       /api/v1/ojk/items [get]
func (h *OJKItemsHandler) ListItems(c *gin.Context) {
	tenantID := types.MustTenantIDFromContext(c.Request.Context())
	var req ListItemsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	items, total, err := h.svc.ListItems(c.Request.Context(), tenantID, req.RunID, req.Status, req.Severity, req.SortBy, req.SortOrder, req.Page, req.Size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":      items,
		"total":      total,
		"page":       req.Page,
		"page_size":  req.Size,
	})
}

// ResolveItemRequest is the request body for approving/rejecting an item.
type ResolveItemRequest struct {
	Status     string `json:"status" binding:"required,oneof=confirmed rejected"`
	Note       string `json:"note"`
}

// ResolveItem updates a single item's review status.
//
// PATCH /api/v1/ojk/items/:item_id/resolve
// @Summary      Resolve an OJK checklist item
// @Description  Mark an item as confirmed or rejected by a human reviewer.
// @Tags         OJK
// @Produce      json
// @Param        item_id   path      string                  true  "Item ID"
// @Param        body      body      ResolveItemRequest      true  "Resolution"
// @Success      200       {object}  map[string]string       "resolved item id"
// @Failure      400       {object}  map[string]string       "bad request"
// @Failure      404       {object}  map[string]string       "not found"
// @Router       /api/v1/ojk/items/{item_id}/resolve [patch]
func (h *OJKItemsHandler) ResolveItem(c *gin.Context) {
	tenantID := types.MustTenantIDFromContext(c.Request.Context())
	itemID := c.Param("item_id")
	var req ResolveItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.ResolveItem(c.Request.Context(), tenantID, itemID, req.Status, req.Note); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": itemID, "status": req.Status})
}

// ItemStats returns status counts for a run.
//
// GET /api/v1/ojk/runs/:run_id/stats
// @Summary      Get OJK item stats
// @Description  Returns item counts grouped by status for a run.
// @Tags         OJK
// @Produce      json
// @Param        run_id  path      string  true  "Run ID"
// @Success      200     {object}  map[string]int64
// @Failure      404     {object}  map[string]string "not found"
// @Router       /api/v1/ojk/runs/{run_id}/stats [get]
func (h *OJKItemsHandler) ItemStats(c *gin.Context) {
	tenantID := types.MustTenantIDFromContext(c.Request.Context())
	runID := c.Param("run_id")

	stats, err := h.svc.ItemStats(c.Request.Context(), tenantID, runID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}
