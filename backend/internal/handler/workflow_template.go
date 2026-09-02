package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

type WorkflowTemplateHandler struct {
	store *store.Store
}

func NewWorkflowTemplateHandler(s *store.Store) *WorkflowTemplateHandler {
	return &WorkflowTemplateHandler{store: s}
}

type workflowTemplatePayload struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Steps       []model.WorkflowStep `json:"steps"`
}

// ---------- 全局模板 CRUD ----------

func (h *WorkflowTemplateHandler) Create(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req workflowTemplatePayload
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	t, appErr := h.store.CreateWorkflowTemplate(userID, req.Name, req.Description, req.Steps)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusCreated, t)
}

func (h *WorkflowTemplateHandler) List(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	items := h.store.ListWorkflowTemplates(userID)
	transport.WriteList(c, items, len(items))
}

func (h *WorkflowTemplateHandler) Get(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	t, appErr := h.store.GetWorkflowTemplate(userID, c.Param("templateId"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, t)
}

func (h *WorkflowTemplateHandler) Update(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req workflowTemplatePayload
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	t, appErr := h.store.UpdateWorkflowTemplate(userID, c.Param("templateId"), &req.Name, &req.Description, req.Steps)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, t)
}

func (h *WorkflowTemplateHandler) Copy(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	t, appErr := h.store.CopyWorkflowTemplate(userID, c.Param("templateId"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusCreated, t)
}

func (h *WorkflowTemplateHandler) Delete(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	t, appErr := h.store.DeleteWorkflowTemplate(userID, c.Param("templateId"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, t)
}

// ---------- 项目继承 / 同步 ----------

type inheritWorkflowRequest struct {
	TemplateID string `json:"template_id"`
}

func (h *WorkflowTemplateHandler) Inherit(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req inheritWorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	project, appErr := h.store.InheritWorkflowTemplate(userID, c.Param("projectId"), req.TemplateID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, project)
}

func (h *WorkflowTemplateHandler) SyncDiff(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	diff, appErr := h.store.ComputeWorkflowSyncDiff(userID, c.Param("projectId"), c.Param("workflowId"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, diff)
}

type applySyncRequest struct {
	RemoveSteps []string `json:"remove_steps"`
}

func (h *WorkflowTemplateHandler) ApplySync(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req applySyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	project, appErr := h.store.ApplyWorkflowSync(userID, c.Param("projectId"), c.Param("workflowId"), req.RemoveSteps)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, project)
}

func (h *WorkflowTemplateHandler) Detach(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	project, appErr := h.store.DetachWorkflowFromTemplate(userID, c.Param("projectId"), c.Param("workflowId"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, project)
}
