package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

type ProjectHandler struct {
	store *store.Store
}

func NewProjectHandler(s *store.Store) *ProjectHandler {
	return &ProjectHandler{store: s}
}

type createProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	PMAgentID   string `json:"pm_agent_id"`
	// TemplateID optionally inherits a global workflow template on creation.
	TemplateID string `json:"template_id"`
}

func (h *ProjectHandler) Create(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	var req createProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	project, appErr := h.store.CreateProject(sc, req.Name, req.Description, req.PMAgentID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if req.TemplateID != "" {
		project, appErr = h.store.InheritWorkflowTemplate(sc, project.ID, req.TemplateID)
		if appErr != nil {
			transport.WriteError(c, appErr)
			return
		}
	}
	transport.WriteData(c, http.StatusCreated, project)
}

func (h *ProjectHandler) List(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	items := h.store.ListProjects(sc)
	transport.WriteList(c, items, len(items))
}

func (h *ProjectHandler) Get(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	project, appErr := h.store.GetProject(sc, c.Param("projectId"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, project)
}

// WorkflowProgress handles GET /api/v1/projects/:projectId/workflow-progress.
func (h *ProjectHandler) WorkflowProgress(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	progress, appErr := h.store.GetProjectWorkflowProgress(sc, c.Param("projectId"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, progress)
}

// BindStepOutput handles
// POST /api/v1/projects/:projectId/workflow/steps/:stepIndex/outputs/bind.
//
// 项目流程 · 手工绑定交付物：把项目里任意一个文件（用户在文件区手工上传的、或别的
// 任务产出的）绑定为总流程某个步骤的最终交付物。地址用 project + stepIndex 而不是
// task + todo —— 后端自己解析承载任务与 todo，前端因此可以从任意入口发起，
// 不必先知道该步骤此刻对应哪个 todoId。
func (h *ProjectHandler) BindStepOutput(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	stepIndex, err := strconv.Atoi(c.Param("stepIndex"))
	if err != nil || stepIndex < 0 {
		transport.WriteError(c, transport.BadRequest("BAD_STEP_INDEX", "step index must be a non-negative integer"))
		return
	}
	var body store.BindStepOutputRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid request body"))
		return
	}
	artifact, appErr := h.store.BindStepOutput(sc, c.Param("projectId"), stepIndex, body)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, artifact)
}

type updateProjectRequest struct {
	Name        *string          `json:"name"`
	Description *string          `json:"description"`
	PMAgentID   *string          `json:"pm_agent_id"`
	Workflows   []model.Workflow `json:"workflows"`
	// PrimaryWorkflowIndex marks which workflow is the project's 总流程.
	// nil keeps the current value; -1 clears it.
	PrimaryWorkflowIndex *int `json:"primary_workflow_index"`
}

func (h *ProjectHandler) Update(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	var req updateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	project, appErr := h.store.UpdateProject(sc, c.Param("projectId"), store.UpdateProjectInput{
		Name:                 req.Name,
		Description:          req.Description,
		PMAgentID:            req.PMAgentID,
		Workflows:            req.Workflows,
		PrimaryWorkflowIndex: req.PrimaryWorkflowIndex,
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, project)
}

func (h *ProjectHandler) Archive(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	project, appErr := h.store.ArchiveProject(sc, c.Param("projectId"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, project)
}
