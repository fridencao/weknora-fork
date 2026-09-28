package router

import (
	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
)

// RegisterOJKRoutes registers the OJK Fit & Proper plugin routes under /api/v1/ojk.
//
// These routes are gated by the existing API-key / auth middleware chain.
// No new auth mechanisms are introduced here — they inherit the workspace's
// existing RBAC so that only viewers can list and admins can trigger runs.
func RegisterOJKRoutes(r *gin.RouterGroup, runH *handler.OJKRunHandler, itemsH *handler.OJKItemsHandler, g *rbacGuards) {
	ojk := g.apiKeyGroup(r.Group("/ojk"), apiKeyFullAccess())

	// Run lifecycle
	ojk.POST("/runs", g.Admin(), runH.CreateRun)
	ojk.GET("/runs", g.Viewer(), runH.ListRuns)
	ojk.GET("/runs/:run_id", g.Viewer(), runH.GetRun)
	ojk.GET("/runs/:run_id/stats", g.Viewer(), itemsH.ItemStats)

	// Checklist items (review workflow)
	ojk.GET("/items", g.Viewer(), itemsH.ListItems)
	ojk.PATCH("/items/:item_id/resolve", g.Admin(), itemsH.ResolveItem)
}
