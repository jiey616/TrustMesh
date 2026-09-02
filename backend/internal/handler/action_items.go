package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

// ActionItemsHandler serves the action-items → tasks conversion endpoints
// used by the project "待办" tab.
type ActionItemsHandler struct {
	store *store.Store
	log   *zap.Logger
}

func NewActionItemsHandler(st *store.Store, log *zap.Logger) *ActionItemsHandler {
	return &ActionItemsHandler{store: st, log: log}
}

type actionItemDTO struct {
	TaskID          string `json:"task_id"`
	TodoID          string `json:"todo_id"`
	TaskTitle       string `json:"task_title"`
	TodoTitle       string `json:"todo_title"`
	ItemIndex       int    `json:"item_index"`
	Title           string `json:"title"`
	Description     string `json:"description,omitempty"`
	AssigneeNodeID  string `json:"assignee_node_id,omitempty"`
	AssigneeRole    string `json:"assignee_role,omitempty"`
	Status          string `json:"status"`
	ConvertedTaskID string `json:"converted_task_id,omitempty"`
	CreatedAt       string `json:"created_at"`
}

func toActionItemDTO(ref store.ActionItemRef, itemIdx int) actionItemDTO {
	it := ref.Item
	return actionItemDTO{
		TaskID:          ref.TaskID,
		TodoID:          ref.TodoID,
		TaskTitle:       ref.Task.Title,
		TodoTitle:       ref.Todo.Title,
		ItemIndex:       itemIdx,
		Title:           it.Title,
		Description:     it.Description,
		AssigneeNodeID:  it.AssigneeNodeID,
		AssigneeRole:    it.AssigneeRole,
		Status:          it.Status,
		ConvertedTaskID: it.ConvertedTaskID,
		CreatedAt:       it.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// List lists action items for the current user, optionally filtered by
// project and status.
func (h *ActionItemsHandler) List(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	projectID := c.Query("project_id")
	status := c.Query("status")
	refs := h.store.ListActionItems(userID, projectID, status, 200)

	out := make([]actionItemDTO, 0, len(refs))
	for i, ref := range refs {
		out = append(out, toActionItemDTO(ref, i))
	}
	transport.WriteData(c, http.StatusOK, gin.H{"items": out, "total": len(out)})
}

type convertRequest struct {
	ItemIDs []string `json:"item_ids"` // item keys to convert (see List output)
	// Assignees maps item key → agent id for UI overrides of the executor.
	Assignees map[string]string `json:"assignees,omitempty"`
}

// Convert turns the selected action items into new tasks, grouped by target
// agent (one task per assignee, one todo per action item).
func (h *ActionItemsHandler) Convert(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	projectID := c.Query("project_id")

	var body convertRequest
	if err := c.ShouldBindJSON(&body); err != nil || len(body.ItemIDs) == 0 {
		transport.WriteError(c, transport.Validation("invalid convert payload", map[string]any{"item_ids": "required"}))
		return
	}

	// Re-fetch all pending items for the user+project and pick the requested
	// ones by their stable key (taskID:todoID:index encoded in item_ids).
	refs := h.store.ListActionItems(userID, projectID, "", 0)
	wanted := make(map[string]struct{}, len(body.ItemIDs))
	for _, id := range body.ItemIDs {
		wanted[id] = struct{}{}
	}
	selected := make([]store.ActionItemRef, 0, len(body.ItemIDs))
	sourceTaskID := ""
	// Apply UI assignee overrides: item key → agent id. Resolve agent id → node id.
	overrides := make(map[string]string) // item key -> node id
	for key, agentID := range body.Assignees {
		if agent, err := h.store.GetAgent(userID, agentID); err == nil && agent != nil {
			overrides[key] = agent.NodeID
		}
	}
	for i, ref := range refs {
		key := actionItemKey(ref, i)
		if _, ok := wanted[key]; !ok {
			continue
		}
		if nodeID, has := overrides[key]; has && nodeID != "" {
			ref.Item.AssigneeNodeID = nodeID
			ref.Item.AssigneeRole = ""
		}
		selected = append(selected, ref)
		if sourceTaskID == "" {
			sourceTaskID = ref.TaskID
		}
	}
	if len(selected) == 0 {
		transport.WriteError(c, transport.NotFound("no matching action items"))
		return
	}

	created, appErr := h.store.ConvertActionItems(userID, selected, projectID, sourceTaskID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	out := make([]string, 0, len(created))
	for _, t := range created {
		out = append(out, t.ID)
	}
	transport.WriteData(c, http.StatusOK, gin.H{"created_task_ids": out, "count": len(out)})
}

// actionItemKey builds a stable identifier for an action item ref so the
// frontend can reference items across requests.
func actionItemKey(ref store.ActionItemRef, index int) string {
	return ref.TaskID + ":" + ref.TodoID + ":" + strconv.Itoa(index)
}
