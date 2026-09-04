package handler

import (
	"net/http"

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
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	progress, appErr := h.store.GetProjectWorkflowProgress(userID, c.Param("projectId"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, progress)
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
