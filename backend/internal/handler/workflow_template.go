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
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	var req workflowTemplatePayload
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	t, appErr := h.store.CreateWorkflowTemplate(sc, req.Name, req.Description, req.Steps)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusCreated, t)
}

func (h *WorkflowTemplateHandler) List(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	items := h.store.ListWorkflowTemplates(sc)
	transport.WriteList(c, items, len(items))
}

func (h *WorkflowTemplateHandler) Get(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	t, appErr := h.store.GetWorkflowTemplate(sc, c.Param("templateId"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, t)
}

func (h *WorkflowTemplateHandler) Update(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	var req workflowTemplatePayload
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	t, appErr := h.store.UpdateWorkflowTemplate(sc, c.Param("templateId"), &req.Name, &req.Description, req.Steps)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, t)
}

func (h *WorkflowTemplateHandler) Copy(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	t, appErr := h.store.CopyWorkflowTemplate(sc, c.Param("templateId"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusCreated, t)
}

func (h *WorkflowTemplateHandler) Delete(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	t, appErr := h.store.DeleteWorkflowTemplate(sc, c.Param("templateId"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, t)
}

// ---------- 从成功任务一键沉淀（T1.9） ----------

type distillWorkflowRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Distill 把一个成功任务沉淀为全局工作流模板。任务 id 走路径参数 :id（挂在
// /tasks/:id/distill-template 下，避免与 /workflow-templates/:templateId 的
// 通配段冲突）。请求体可选：缺省时模板名回退为任务标题。
func (h *WorkflowTemplateHandler) Distill(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	var req distillWorkflowRequest
	// 请求体是可选的（「一键」场景可以不带 body）；绑定失败只意味着用默认命名，
	// 不应因此拒绝请求。
	_ = c.ShouldBindJSON(&req)
	t, appErr := h.store.DistillWorkflowTemplateFromTask(sc, c.Param("id"), req.Name, req.Description)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusCreated, t)
}

// ---------- 项目继承 / 同步 ----------

type inheritWorkflowRequest struct {
	TemplateID string `json:"template_id"`
}

func (h *WorkflowTemplateHandler) Inherit(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	var req inheritWorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	project, appErr := h.store.InheritWorkflowTemplate(sc, c.Param("projectId"), req.TemplateID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, project)
}

func (h *WorkflowTemplateHandler) SyncDiff(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	diff, appErr := h.store.ComputeWorkflowSyncDiff(sc, c.Param("projectId"), c.Param("workflowId"))
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
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	var req applySyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	project, appErr := h.store.ApplyWorkflowSync(sc, c.Param("projectId"), c.Param("workflowId"), req.RemoveSteps)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, project)
}

func (h *WorkflowTemplateHandler) Detach(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	project, appErr := h.store.DetachWorkflowFromTemplate(sc, c.Param("projectId"), c.Param("workflowId"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, project)
}
