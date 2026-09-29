package handler

import (
	"net/http"

	"gorm.io/gorm"
	"github.com/gin-gonic/gin"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// OJKCandidateHandler 阶段 2 候选人材料摄入入口：
// 创建候选人（自动建专属材料 KB）→ 前端把材料文件走
// /knowledge-bases/:kb_id/knowledge/file 上传（复用解析管线）。
type OJKCandidateHandler struct {
	db        *gorm.DB
	kbService interfaces.KnowledgeBaseService
	svc       *service.OJKService
}

// NewOJKCandidateHandler creates a new OJK candidate handler.
func NewOJKCandidateHandler(
	db *gorm.DB,
	kbService interfaces.KnowledgeBaseService,
	svc *service.OJKService,
) *OJKCandidateHandler {
	return &OJKCandidateHandler{db: db, kbService: kbService, svc: svc}
}

// CreateCandidateRequest is the request body for registering a candidate.
type CreateCandidateRequest struct {
	Name        string `json:"name" binding:"required,max=128"`
	NIK         string `json:"nik" binding:"max=32"`
	Position    string `json:"position" binding:"max=128"`
	Institution string `json:"institution" binding:"max=128"`
}

// Create registers a candidate and provisions its dedicated material KB.
//
// POST /api/v1/ojk/candidates
// @Summary      Register OJK candidate
// @Description  创建候选人登记并自动创建其专属材料知识库（后续材料文件上传至该 KB）。
// @Tags         OJK
// @Produce      json
// @Param        body  body      CreateCandidateRequest  true  "Candidate info"
// @Success      201   {object}  service.CandidateStatus
// @Failure      400   {object}  map[string]string
// @Router       /api/v1/ojk/candidates [post]
func (h *OJKCandidateHandler) Create(c *gin.Context) {
	tenantID := types.MustTenantIDFromContext(c.Request.Context())
	var req CreateCandidateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 继承工作区在用的 embedding 模型（新 KB 不带模型时解析会失败）
	var inheritedModel string
	h.db.WithContext(c.Request.Context()).Raw(
		"SELECT embedding_model_id FROM knowledge_bases "+
			"WHERE tenant_id = ? AND embedding_model_id <> '' AND deleted_at IS NULL "+
			"ORDER BY created_at DESC LIMIT 1", tenantID).Scan(&inheritedModel)

	kb, err := h.kbService.CreateKnowledgeBase(c.Request.Context(), &types.KnowledgeBase{
		Name:             "OJK 候选人-" + req.Name,
		Description:      "候选人材料卷宗（" + req.Name + " · " + req.Position + "）",
		Type:             types.KnowledgeBaseTypeDocument,
		EmbeddingModelID: inheritedModel,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create candidate knowledge base: " + err.Error()})
		return
	}

	cand := &service.OJKCandidate{
		TenantID:    tenantID,
		Name:        req.Name,
		NIK:         strPtrOrNil(req.NIK),
		Position:    strPtrOrNil(req.Position),
		Institution: strPtrOrNil(req.Institution),
		KBID:        kb.ID,
	}
	if err := h.svc.CreateCandidate(c.Request.Context(), cand); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	st, err := h.svc.GetCandidate(c.Request.Context(), tenantID, cand.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, st)
}

// List returns registered candidates with aggregated parse status.
//
// GET /api/v1/ojk/candidates
// @Summary      List OJK candidates
// @Tags         OJK
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Router       /api/v1/ojk/candidates [get]
func (h *OJKCandidateHandler) List(c *gin.Context) {
	tenantID := types.MustTenantIDFromContext(c.Request.Context())
	list, err := h.svc.ListCandidates(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"candidates": list, "total": len(list)})
}

// Get returns one candidate with its material document list.
//
// GET /api/v1/ojk/candidates/:id
// @Summary      Get OJK candidate detail
// @Tags         OJK
// @Produce      json
// @Param        id   path  string  true  "Candidate ID"
// @Success      200  {object}  map[string]interface{}
// @Failure      404  {object}  map[string]string
// @Router       /api/v1/ojk/candidates/{id} [get]
func (h *OJKCandidateHandler) Get(c *gin.Context) {
	tenantID := types.MustTenantIDFromContext(c.Request.Context())
	id := c.Param("id")
	st, err := h.svc.GetCandidate(c.Request.Context(), tenantID, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	var docs []struct {
		ID          string
		Title       string
		ParseStatus string
	}
	h.db.WithContext(c.Request.Context()).Raw(
		"SELECT id, title, parse_status FROM knowledges "+
			"WHERE knowledge_base_id = ? AND tenant_id = ? AND deleted_at IS NULL "+
			"ORDER BY created_at", st.KBID, tenantID).Scan(&docs)
	c.JSON(http.StatusOK, gin.H{"candidate": st, "documents": docs})
}

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
