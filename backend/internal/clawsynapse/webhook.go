package clawsynapse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"trustmesh/backend/internal/agentfile"
	"trustmesh/backend/internal/embedding"
	"trustmesh/backend/internal/knowledge"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/project"
	"trustmesh/backend/internal/protocol"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

type WebhookHandler struct {
	store    *store.Store
	client   *Client
	log      *zap.Logger
	embedder embedding.Client
	qdrant   *knowledge.QdrantClient

	projectFileStorage project.FileStorage

	externalURL string
	jwtSecret   []byte
	downloadTTL time.Duration
}

func NewWebhookHandler(st *store.Store, client *Client, log *zap.Logger) *WebhookHandler {
	return &WebhookHandler{
		store:  st,
		client: client,
		log:    log,
	}
}

// SetKnowledgeComponents injects optional knowledge base dependencies.
func (h *WebhookHandler) SetKnowledgeComponents(embedder embedding.Client, qdrant *knowledge.QdrantClient) {
	h.embedder = embedder
	h.qdrant = qdrant
}

// SetProjectFileStorage injects the project file storage for artifact auto-indexing.
func (h *WebhookHandler) SetProjectFileStorage(storage project.FileStorage) {
	h.projectFileStorage = storage
}

// SetAgentFileConfig injects external URL and JWT secret for building download URLs.
func (h *WebhookHandler) SetAgentFileConfig(externalURL string, jwtSecret []byte, downloadTTL time.Duration) {
	h.externalURL = externalURL
	h.jwtSecret = jwtSecret
	h.downloadTTL = downloadTTL
}

func (h *WebhookHandler) HandleWebhook(c *gin.Context) {
	var payload protocol.WebhookPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid webhook payload"))
		return
	}

	if payload.NodeID != "" {
		localNodeID, appErr := h.resolveLocalNodeID(c.Request.Context())
		if appErr != nil {
			transport.WriteError(c, appErr)
			return
		}
		if payload.NodeID != localNodeID {
			transport.WriteError(c, transport.Validation("invalid webhook target node", map[string]any{"nodeId": "does not match local node"}))
			return
		}
	}

	switch strings.TrimSpace(payload.Type) {
	case "chat.message":
		h.handleChatMessage(c, payload)
	case "task.create":
		h.handleTaskCreate(c, payload)
	case "task.reply":
		h.handleTaskReply(c, payload)
	case "task.plan_ready":
		h.handleTaskPlanReady(c, payload)
	case "task.todo_add":
		h.handleTodoAdd(c, payload)
	case "task.todo_modify":
		h.handleTodoModify(c, payload)
	case "todo.progress":
		h.handleTodoProgress(c, payload)
	case "todo.complete":
		h.handleTodoComplete(c, payload)
	case "todo.fail":
		h.handleTodoFail(c, payload)
	case "task.comment":
		h.handleTaskComment(c, payload)
	case "knowledge.query":
		h.handleKnowledgeQuery(c, payload)
	case "task.response":
		h.handleTaskResponse(c, payload)
	case "transfer.received":
		h.handleTransferReceived(c, payload)
	case "task.context.query":
		h.handleContextQuery(c, payload)
	default:
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "unsupported webhook type"))
	}
}

func (h *WebhookHandler) handleChatMessage(c *gin.Context, webhook protocol.WebhookPayload) {
	content := strings.TrimSpace(webhook.Message)
	if content == "" {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid chat.message payload"))
		return
	}

	detail, appErr := h.store.AppendAgentChatMessageByNode(webhook.From, webhook.SessionKey, content, messageIDFromMetadata(webhook.Metadata))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, detail)
}

func (h *WebhookHandler) resolveLocalNodeID(ctx context.Context) (string, *transport.AppError) {
	if h == nil || h.client == nil {
		return "", &transport.AppError{
			Status:  http.StatusServiceUnavailable,
			Code:    "CLAWSYNAPSE_UNAVAILABLE",
			Message: "暂时无法校验本地节点身份",
			Details: map[string]any{},
		}
	}

	nodeID, err := h.client.GetSelfNodeID(ctx)
	if err != nil {
		return "", &transport.AppError{
			Status:  http.StatusServiceUnavailable,
			Code:    "CLAWSYNAPSE_UNAVAILABLE",
			Message: "暂时无法校验本地节点身份",
			Details: map[string]any{"cause": err.Error()},
		}
	}

	return nodeID, nil
}

func (h *WebhookHandler) handleTaskCreate(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.TaskCreatePayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid task.create message"))
		return
	}

	in := store.TaskCreateInput{
		ProjectID:   payload.ProjectID,
		Title:       payload.Title,
		Description: payload.Description,
		Todos:       make([]store.TaskCreateTodoInput, 0, len(payload.Todos)),
	}
	for _, todo := range payload.Todos {
		in.Todos = append(in.Todos, store.TaskCreateTodoInput{
			ID:             todo.ID,
			Order:          todo.Order,
			Title:          todo.Title,
			Description:    todo.Description,
			AssigneeNodeID: todo.AssigneeNodeID,
		})
	}

	task, appErr := h.store.CreateTaskByPMNodeWithMessageID(webhook.From, messageIDFromMetadata(webhook.Metadata), in)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	h.publishTaskCreated(task)
	task = h.dispatchNextTodo(context.Background(), task)
	transport.WriteData(c, http.StatusOK, task)
}

func (h *WebhookHandler) handleTaskReply(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.TaskReplyPayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid task.reply message"))
		return
	}

	task, appErr := h.store.AppendPMTaskReply(webhook.From, payload.TaskID, payload.Content, payload.UIBlocks)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	transport.WriteData(c, http.StatusOK, task)
}

// handleTaskResponse processes task.response messages from PM agents.
// PM agents use this type for status updates, ACKs, and intermediate replies.
// It supports three message formats:
//  1. Structured: {"data":{"id":"...", ...}} — extracts task_id from data.id
//  2. JSON reply:  {"task_id":"...", "content":"..."} — treats as task.reply
//  3. Plain text:  plain status message — stored as PM reply
//
// Silent ACKs ("ACK ...\n\nWAITING", "↻ Resumed session") are ignored.
func (h *WebhookHandler) handleTaskResponse(c *gin.Context, webhook protocol.WebhookPayload) {
	msg := strings.TrimSpace(webhook.Message)

	// Ignore silent ACK messages that carry no actionable content.
	if isSilentACK(msg) {
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "ignored": true})
		return
	}

	// Try to extract structured content from the message.
	taskID, content := extractTaskResponseContent(msg, webhook.SessionKey)
	if taskID == "" || content == "" {
		transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "ignored": true})
		return
	}

	// Store as a PM reply so the user can see the status update.
	task, appErr := h.store.AppendPMTaskReply(webhook.From, taskID, content, nil)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	transport.WriteData(c, http.StatusOK, task)
}

// isSilentACK returns true for messages that should be silently ignored.
func isSilentACK(msg string) bool {
	if msg == "" {
		return true
	}
	// ACK messages from PM: "ACK task.reply\n\nWAITING"
	if strings.HasPrefix(msg, "ACK ") && strings.Contains(msg, "WAITING") {
		return true
	}
	// Session resume notifications
	if strings.HasPrefix(msg, "↻ Resumed session") {
		return true
	}
	return false
}

// extractTaskResponseContent parses a task.response message and returns
// (taskID, content). It handles three formats:
//
//	{"data": {"id": "task_xxx", ...}}        → extracts data.id, content from data
//	{"task_id": "task_xxx", "content": "..."}  → direct extraction
//	plain text                                → uses sessionKey as taskID
func extractTaskResponseContent(msg, sessionKey string) (taskID, content string) {
	// Try structured JSON first.
	var raw map[string]any
	if err := json.Unmarshal([]byte(msg), &raw); err != nil {
		// Plain text fallback.
		return sessionKey, msg
	}

	// Format 2: direct task_id + content fields.
	if tid, ok := raw["task_id"].(string); ok && tid != "" {
		if c, ok := raw["content"].(string); ok {
			return tid, c
		}
		return tid, msg
	}

	// Format 1: {data: {id: "...", ...}}
	if data, ok := raw["data"].(map[string]any); ok {
		tid, _ := data["id"].(string)
		if tid == "" {
			tid = sessionKey
		}

		// Try to extract meaningful content from the data wrapper.
		// Prefer status/title fields that convey progress.
		var parts []string
		if title, ok := data["title"].(string); ok && title != "" {
			parts = append(parts, "📋 当前任务：**"+title+"**")
		}
		if status, ok := data["status"].(string); ok && status != "" {
			statusMap := map[string]string{
				"planning":    "🔍 PM 规划中",
				"in_progress": "⚙️ 执行中",
				"done":        "✅ 已完成",
				"canceled":    "❌ 已取消",
			}
			label := statusMap[status]
			if label == "" {
				label = status
			}
			parts = append(parts, "状态："+label)
		}
		if todos, ok := data["todos"].([]any); ok && len(todos) > 0 {
			parts = append(parts, fmt.Sprintf("已规划 %d 个子任务", len(todos)))
		}

		if len(parts) > 0 {
			return tid, strings.Join(parts, "  |  ")
		}
		return tid, "" // only have task ID, no meaningful content
	}

	// Unknown JSON shape — try as plain text.
	return sessionKey, msg
}

// handleTodoAdd processes task.todo_add messages from PM Agent.
// PM Agent can dynamically add a TODO to an existing task after planning.
func (h *WebhookHandler) handleTodoAdd(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.TodoAddPayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid task.todo_add message"))
		return
	}

	// Validate that the sender is the PM agent for this task.
	task, appErr := h.store.GetTaskByNodeID(webhook.From, payload.TaskID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if task.PMAgent.NodeID != webhook.From {
		transport.WriteError(c, transport.Forbidden("only the task's PM agent can add todos"))
		return
	}

	// Find the assignee agent by node ID.
	assigneeAgent, appErr := h.store.GetAgentByNodeID(payload.AssigneeNodeID)
	if appErr != nil {
		transport.WriteError(c, transport.Validation("invalid assignee_node_id", map[string]any{"assignee_node_id": payload.AssigneeNodeID}))
		return
	}

	// Add the todo via store.
	if payload.BeforeTodoID != "" {
		task, appErr = h.store.InsertTodo(task.UserID, payload.TaskID, payload.BeforeTodoID, store.TodoModifyInput{
			Title:       payload.Title,
			Description: payload.Description,
			AssigneeID:  assigneeAgent.ID,
		})
	} else {
		task, appErr = h.store.AppendTodo(task.UserID, payload.TaskID, store.TodoModifyInput{
			Title:       payload.Title,
			Description: payload.Description,
			AssigneeID:  assigneeAgent.ID,
		})
	}
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// Dispatch via ClawSynapse if this todo is the next one in sequence
	task = h.dispatchNextTodo(c.Request.Context(), task)

	transport.WriteData(c, http.StatusCreated, task)
}

// handleTodoModify processes task.todo_modify messages from PM Agent.
// PM Agent can update a TODO's title, description, and/or assignee.
func (h *WebhookHandler) handleTodoModify(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.TodoModifyPM
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid task.todo_modify message"))
		return
	}

	// Validate that the sender is the PM agent for this task.
	task, appErr := h.store.GetTaskByNodeID(webhook.From, payload.TaskID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if task.PMAgent.NodeID != webhook.From {
		transport.WriteError(c, transport.Forbidden("only the task's PM agent can modify todos"))
		return
	}

	// Resolve assignee if provided.
	assigneeID := payload.AssigneeNodeID
	if assigneeID != "" {
		assigneeAgent, appErr := h.store.GetAgentByNodeID(assigneeID)
		if appErr != nil {
			transport.WriteError(c, transport.Validation("invalid assignee_node_id", map[string]any{"assignee_node_id": assigneeID}))
			return
		}
		assigneeID = assigneeAgent.ID
	}

	task, appErr = h.store.UpdateTodo(task.UserID, payload.TaskID, payload.TodoID, store.TodoModifyInput{
		Title:       payload.Title,
		Description: payload.Description,
		AssigneeID:  assigneeID,
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	transport.WriteData(c, http.StatusOK, task)
}

func (h *WebhookHandler) handleTaskPlanReady(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.TaskPlanReadyPayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid task.plan_ready message"))
		return
	}

	in := store.TaskPlanReadyInput{
		TaskID:      payload.TaskID,
		Title:       payload.Title,
		Description: payload.Description,
		Todos:       make([]store.TaskCreateTodoInput, 0, len(payload.Todos)),
	}
	for _, todo := range payload.Todos {
		in.Todos = append(in.Todos, store.TaskCreateTodoInput{
			ID:             todo.ID,
			Order:          todo.Order,
			Title:          todo.Title,
			Description:    todo.Description,
			AssigneeNodeID: todo.AssigneeNodeID,
		})
	}

	task, appErr := h.store.FinalizePlanByPMNode(webhook.From, messageIDFromMetadata(webhook.Metadata), in)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	transport.WriteData(c, http.StatusOK, task)
}

func (h *WebhookHandler) handleTodoProgress(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.TodoProgressPayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid todo.progress message"))
		return
	}

	task, appErr := h.store.UpdateTodoProgressByNode(webhook.From, store.TodoProgressInput{
		TaskID:  payload.TaskID,
		TodoID:  payload.TodoID,
		Message: payload.Message,
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	h.publishTaskAndTodoStatusChanges(task, payload.TodoID, payload.Message, webhook.From, "todo.progress")
	transport.WriteData(c, http.StatusOK, task)
}

func (h *WebhookHandler) handleTodoComplete(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.TodoCompletePayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid todo.complete message"))
		return
	}
	task, appErr := h.store.CompleteTodoByNodeWithMessageID(webhook.From, messageIDFromMetadata(webhook.Metadata), store.TodoCompleteInput{
		TaskID: payload.TaskID,
		TodoID: payload.TodoID,
		Result: payload.Result,
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	h.publishTaskAndTodoStatusChanges(task, payload.TodoID, "completed", webhook.From, "todo.complete")
	task = h.dispatchNextTodo(context.Background(), task)
	transport.WriteData(c, http.StatusOK, task)
}

func (h *WebhookHandler) handleTodoFail(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.TodoFailPayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid todo.fail message"))
		return
	}

	task, appErr := h.store.FailTodoByNodeWithMessageID(webhook.From, messageIDFromMetadata(webhook.Metadata), store.TodoFailInput{
		TaskID: payload.TaskID,
		TodoID: payload.TodoID,
		Error:  payload.Error,
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	h.publishTaskAndTodoStatusChanges(task, payload.TodoID, payload.Error, webhook.From, "todo.fail")
	transport.WriteData(c, http.StatusOK, task)
}

func (h *WebhookHandler) handleTaskComment(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.TaskCommentPayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid task.comment message"))
		return
	}

	event, appErr := h.store.AddTaskCommentByNode(webhook.From, store.TaskCommentInput{
		TaskID:  payload.TaskID,
		TodoID:  payload.TodoID,
		Content: payload.Content,
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, event)
}

func (h *WebhookHandler) handleContextQuery(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.ContextQueryPayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid task.context.query message"))
		return
	}

	task, appErr := h.store.GetTaskByNodeID(webhook.From, payload.TaskID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	resultPayload := protocol.ContextResultPayload{
		TaskID:      task.ID,
		TaskContext: buildTaskContext(task, ""),
	}
	for i := range task.Todos {
		t := &task.Todos[i]
		if t.Status == "done" {
			resultPayload.AllResults = append(resultPayload.AllResults, buildPriorResult(t))
		}
	}

	h.publish(context.Background(), webhook.From, "task.context.result", resultPayload, task.ID)
	transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "task_id": task.ID})
}

func (h *WebhookHandler) publishTaskCreated(task *model.TaskDetail) {
	h.publish(context.Background(), task.PMAgent.NodeID, "task.created", protocol.TaskCreatedPayload{
		TaskID:    task.ID,
		ProjectID: task.ProjectID,
		Title:     task.Title,
	}, task.ID)
}

// PublishTaskCreated notifies the PM agent that a task has been created/approved.
func (h *WebhookHandler) PublishTaskCreated(task *model.TaskDetail) {
	h.publishTaskCreated(task)
}

// DispatchNextTodo dispatches the next pending todo in a task to its assignee agent.
func (h *WebhookHandler) DispatchNextTodo(ctx context.Context, task *model.TaskDetail) *model.TaskDetail {
	return h.dispatchNextTodo(ctx, task)
}

func (h *WebhookHandler) dispatchNextTodo(ctx context.Context, task *model.TaskDetail) *model.TaskDetail {
	if h == nil || task == nil || h.client == nil {
		return task
	}
	if task.Status == "canceled" {
		return task
	}

	todo := task.NextDispatchableTodo()
	if todo == nil {
		return task
	}
	if appErr := h.store.CheckTaskProjectActive(task.ID); appErr != nil {
		if h.log != nil {
			h.log.Warn("skip sequential todo dispatch for archived task project", zap.String("task_id", task.ID), zap.Error(appErr))
		}
		return task
	}

	payload := h.buildTodoAssignedPayload(task, todo)
	if _, err := h.client.Publish(ctx, todo.Assignee.NodeID, "todo.assigned", payload, task.ID, nil); err != nil {
		if h.log != nil {
			h.log.Warn("sequential todo dispatch failed", zap.String("task_id", task.ID), zap.String("todo_id", todo.ID), zap.String("target_node", todo.Assignee.NodeID), zap.Error(err))
		}
		return task
	}

	updatedTask, appErr := h.store.RecordSequentialTodoDispatch(task.ID, todo.ID)
	if appErr != nil {
		if h.log != nil {
			h.log.Warn("failed to persist sequential todo dispatch", zap.String("task_id", task.ID), zap.String("todo_id", todo.ID), zap.Error(appErr))
		}
		return task
	}
	return updatedTask
}

func (h *WebhookHandler) buildTodoAssignedPayload(task *model.TaskDetail, todo *model.Todo) protocol.TodoAssignedPayload {
	payload := protocol.TodoAssignedPayload{
		TaskID:      task.ID,
		TodoID:      todo.ID,
		Title:       todo.Title,
		Description: todo.Description,
		Content:     "你收到了一个新的 Todo 任务。请使用 /tm-task-exec skill 执行此任务，按要求回报进度和结果。",
		ExecBrief:   defaultExecBrief(),
	}

	firstTime := !agentHasPriorTodoInTask(task, todo)
	if firstTime {
		payload.TaskContext = buildTaskContext(task, todo.ID)
		payload.PriorResults = buildAllPriorResults(task, todo)
		payload.AttachedFiles = h.enrichAttachedFiles(task.AttachedFiles)
	} else {
		payload.PriorResults = buildCrossAgentPriorResults(task, todo)
	}

	return payload
}

// agentHasPriorTodoInTask returns true if the same agent has already completed
// or failed an earlier todo in this task, meaning the agent's session already
// contains task-level context.
func agentHasPriorTodoInTask(task *model.TaskDetail, currentTodo *model.Todo) bool {
	for i := range task.Todos {
		t := &task.Todos[i]
		if t.ID == currentTodo.ID {
			continue
		}
		if t.Assignee.NodeID == currentTodo.Assignee.NodeID &&
			t.Order < currentTodo.Order &&
			(t.Status == "done" || t.Status == "failed") {
			return true
		}
	}
	return false
}

func buildTaskContext(task *model.TaskDetail, currentTodoID string) *protocol.TaskContext {
	ctx := &protocol.TaskContext{
		Title:       task.Title,
		Description: task.Description,
		Todos:       make([]protocol.TodoSummary, 0, len(task.Todos)),
	}
	for i := range task.Todos {
		t := &task.Todos[i]
		ctx.Todos = append(ctx.Todos, protocol.TodoSummary{
			TodoID:       t.ID,
			Order:        t.Order,
			Title:        t.Title,
			Status:       t.Status,
			AssigneeName: t.Assignee.Name,
			IsCurrent:    t.ID == currentTodoID,
		})
	}
	return ctx
}

// buildAllPriorResults returns results from all completed todos before the
// current one. Used when the agent has no prior session context for this task.
func buildAllPriorResults(task *model.TaskDetail, currentTodo *model.Todo) []protocol.TodoPriorResult {
	var results []protocol.TodoPriorResult
	for i := range task.Todos {
		t := &task.Todos[i]
		if t.Status != "done" || t.Order >= currentTodo.Order {
			continue
		}
		results = append(results, buildPriorResult(t))
	}
	return results
}

// buildCrossAgentPriorResults returns only results from todos completed by
// OTHER agents. The current agent's own prior results are already visible
// in its session via sessionKey continuity.
func buildCrossAgentPriorResults(task *model.TaskDetail, currentTodo *model.Todo) []protocol.TodoPriorResult {
	var results []protocol.TodoPriorResult
	for i := range task.Todos {
		t := &task.Todos[i]
		if t.Status != "done" || t.Order >= currentTodo.Order {
			continue
		}
		if t.Assignee.NodeID == currentTodo.Assignee.NodeID {
			continue
		}
		results = append(results, buildPriorResult(t))
	}
	return results
}

func buildPriorResult(todo *model.Todo) protocol.TodoPriorResult {
	return protocol.TodoPriorResult{
		TodoID:  todo.ID,
		Title:   todo.Title,
		Summary: todo.Result.Summary,
		Output:  todo.Result.Output,
	}
}

func (h *WebhookHandler) publishTaskAndTodoStatusChanges(task *model.TaskDetail, todoID, message, actorNodeID, cause string) {
	h.publish(context.Background(), task.PMAgent.NodeID, "task.status_changed", protocol.TaskStatusChangedPayload{
		TaskID:      task.ID,
		Status:      task.Status,
		ActorNodeID: strings.TrimSpace(actorNodeID),
		Cause:       strings.TrimSpace(cause),
		Reason:      message,
		Version:     task.Version,
	}, task.ID)

	todo := findTodo(task, todoID)
	if todo == nil {
		return
	}

	payload := protocol.TodoStatusChangedPayload{
		TaskID:      task.ID,
		TodoID:      todo.ID,
		Status:      todo.Status,
		ActorNodeID: strings.TrimSpace(actorNodeID),
		Cause:       strings.TrimSpace(cause),
		Reason:      message,
		Version:     task.Version,
		Message:     message,
	}
	if task.PMAgent.NodeID != "" {
		h.publish(context.Background(), task.PMAgent.NodeID, "todo.status_changed", payload, task.ID)
	}
	if todo.Assignee.NodeID != "" && todo.Assignee.NodeID != strings.TrimSpace(actorNodeID) && todo.Assignee.NodeID != task.PMAgent.NodeID {
		h.publish(context.Background(), todo.Assignee.NodeID, "todo.status_changed", payload, task.ID)
	}
}

func (h *WebhookHandler) publish(ctx context.Context, targetNode, msgType string, payload any, sessionKey string) {
	if h.client == nil {
		return
	}
	if _, err := h.client.Publish(ctx, targetNode, msgType, payload, sessionKey, nil); err != nil && h.log != nil {
		h.log.Warn("clawsynapse publish failed", zap.String("target_node", targetNode), zap.String("type", msgType), zap.Error(err))
	}
}

func defaultExecBrief() *protocol.TodoExecBrief {
	return &protocol.TodoExecBrief{
		Objective:    "执行分派的 Todo 任务；及时回报进度；完成后提交结果，失败时说明原因。",
		MustUseSkill: "tm-task-exec",
	}
}

func decodeWebhookMessage(raw string, out any) error {
	return json.Unmarshal([]byte(raw), out)
}

func messageIDFromMetadata(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	for _, key := range []string{"messageId", "message_id", "id"} {
		if value, ok := metadata[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func findTodo(task *model.TaskDetail, todoID string) *model.Todo {
	for i := range task.Todos {
		if task.Todos[i].ID == todoID {
			return &task.Todos[i]
		}
	}
	return nil
}

func (h *WebhookHandler) handleTransferReceived(c *gin.Context, webhook protocol.WebhookPayload) {
	var msg struct {
		TransferID string `json:"transferId"`
		FileName   string `json:"fileName"`
		FileSize   int64  `json:"fileSize"`
		LocalPath  string `json:"localPath"`
		MimeType   string `json:"mimeType"`
	}
	if err := decodeWebhookMessage(webhook.Message, &msg); err != nil || msg.TransferID == "" {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid transfer.received message"))
		return
	}

	taskID, _ := webhook.Metadata["taskId"].(string)
	if taskID == "" {
		transport.WriteError(c, transport.Validation("missing metadata", map[string]any{"taskId": "required"}))
		return
	}
	todoID, _ := webhook.Metadata["todoId"].(string)

	artifact := model.TaskArtifact{
		TransferID: msg.TransferID,
		TaskID:     taskID,
		TodoID:     todoID,
		FileName:   msg.FileName,
		FileSize:   msg.FileSize,
		LocalPath:  msg.LocalPath,
		MimeType:   msg.MimeType,
		FromNodeID: webhook.From,
		CreatedAt:  time.Now().UTC(),
	}

	if appErr := h.store.SaveArtifact(artifact); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// Auto-create ProjectFile and copy artifact to project files volume.
	if h.projectFileStorage != nil {
		if projectFile, err := h.store.SaveProjectFileFromArtifact(artifact); err != nil {
			if h.log != nil {
				h.log.Warn("failed to create project file from artifact",
					zap.String("transfer_id", msg.TransferID),
					zap.Error(err))
			}
		} else if projectFile != nil {
			// Copy the file from transfer volume to project files volume.
			dstPath, copyErr := h.projectFileStorage.CopyFromPath(
				c.Request.Context(),
				msg.LocalPath,
				projectFile.ProjectID,
				taskID,
				artifact.FromAgentID,
				msg.FileName,
			)
			if copyErr != nil {
				if h.log != nil {
					h.log.Warn("failed to copy artifact to project files",
						zap.String("transfer_id", msg.TransferID),
						zap.Error(copyErr))
				}
			} else {
				_ = h.store.SetProjectFileLocalPath(projectFile.ID, dstPath)
			}
		}
	}

	transport.WriteData(c, http.StatusOK, gin.H{
		"transfer_id": msg.TransferID,
		"task_id":     taskID,
	})
}

func (h *WebhookHandler) handleKnowledgeQuery(c *gin.Context, webhook protocol.WebhookPayload) {
	var payload protocol.KnowledgeQueryPayload
	if err := decodeWebhookMessage(webhook.Message, &payload); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid knowledge.query message"))
		return
	}

	if strings.TrimSpace(payload.Query) == "" {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "query is required"))
		return
	}
	if payload.TopK <= 0 {
		payload.TopK = 5
	}
	if payload.MinScore <= 0 {
		payload.MinScore = 0.7
	}

	// Resolve agent → user ownership
	userID, appErr := h.store.ResolveKnowledgeDocOwnerByAgentNode(webhook.From)
	if appErr != nil {
		h.sendKnowledgeError(webhook.From, payload, "agent not recognized: "+appErr.Message)
		transport.WriteError(c, appErr)
		return
	}

	// Validate project ownership
	if payload.ProjectID != "" {
		if appErr := h.store.ValidateProjectOwnership(userID, payload.ProjectID); appErr != nil {
			h.sendKnowledgeError(webhook.From, payload, "access denied to project")
			transport.WriteError(c, appErr)
			return
		}
	}

	// Perform search
	results, err := h.knowledgeSearch(c.Request.Context(), userID, payload)
	if err != nil {
		if h.log != nil {
			h.log.Error("knowledge query search failed", zap.Error(err))
		}
		h.sendKnowledgeError(webhook.From, payload, "search failed: "+err.Error())
		transport.WriteData(c, http.StatusOK, gin.H{"status": "error", "error": err.Error()})
		return
	}

	// Send results back to agent
	resultPayload := protocol.KnowledgeResultPayload{
		QueryID:   payload.QueryID,
		ProjectID: payload.ProjectID,
		Results:   results,
	}
	h.publish(context.Background(), webhook.From, "knowledge.result", resultPayload, payload.QueryID)
	transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "results_count": len(results)})
}

func (h *WebhookHandler) knowledgeSearch(ctx context.Context, userID string, payload protocol.KnowledgeQueryPayload) ([]protocol.KnowledgeResultItem, error) {
	// Try vector search first
	if h.embedder != nil && h.qdrant != nil {
		embeddings, err := h.embedder.Embed(ctx, []string{payload.Query})
		if err != nil {
			return nil, err
		}
		if len(embeddings) > 0 && len(embeddings[0]) > 0 {
			mustConditions := []knowledge.QdrantCondition{
				{Key: "user_id", Match: map[string]any{"value": userID}},
			}
			var filter *knowledge.QdrantFilter
			if payload.ProjectID != "" {
				filter = &knowledge.QdrantFilter{
					Must: mustConditions,
					Should: []knowledge.QdrantCondition{
						{Key: "project_id", Match: map[string]any{"value": payload.ProjectID}},
					},
				}
			} else {
				filter = &knowledge.QdrantFilter{Must: mustConditions}
			}

			hits, err := h.qdrant.Search(ctx, embeddings[0], filter, payload.TopK)
			if err != nil {
				return nil, err
			}

			var results []protocol.KnowledgeResultItem
			for _, hit := range hits {
				if hit.Score < payload.MinScore {
					continue
				}
				chunkID, _ := hit.Payload["chunk_id"].(string)
				docID, _ := hit.Payload["document_id"].(string)

				chunks, err := h.store.GetKnowledgeChunksByIDs([]string{chunkID})
				if err != nil || len(chunks) == 0 {
					continue
				}
				chunk := chunks[0]

				results = append(results, protocol.KnowledgeResultItem{
					ChunkID:       chunkID,
					DocumentID:    docID,
					DocumentTitle: h.store.GetKnowledgeDocTitle(docID),
					Content:       chunk.Content,
					Score:         hit.Score,
					ChunkIndex:    chunk.ChunkIndex,
					Metadata:      chunk.Metadata,
				})
			}
			return results, nil
		}
	}

	// Fallback to text search
	var projectID *string
	if payload.ProjectID != "" {
		projectID = &payload.ProjectID
	}
	chunks, err := h.store.SearchKnowledgeChunks(ctx, userID, projectID, payload.Query, payload.TopK)
	if err != nil {
		return nil, err
	}
	var results []protocol.KnowledgeResultItem
	for _, chunk := range chunks {
		results = append(results, protocol.KnowledgeResultItem{
			ChunkID:       chunk.ID,
			DocumentID:    chunk.DocumentID,
			DocumentTitle: h.store.GetKnowledgeDocTitle(chunk.DocumentID),
			Content:       chunk.Content,
			Score:         1.0,
			ChunkIndex:    chunk.ChunkIndex,
			Metadata:      chunk.Metadata,
		})
	}
	return results, nil
}

func (h *WebhookHandler) sendKnowledgeError(targetNode string, payload protocol.KnowledgeQueryPayload, errMsg string) {
	h.publish(context.Background(), targetNode, "knowledge.result", protocol.KnowledgeResultPayload{
		QueryID:   payload.QueryID,
		ProjectID: payload.ProjectID,
		Error:     errMsg,
	}, payload.QueryID)
}

func (h *WebhookHandler) enrichAttachedFiles(files []model.TaskAttachedFile) []protocol.TaskAttachedFileRef {
	return agentfile.EnrichWithDownloadURLs(files, h.externalURL, h.jwtSecret, h.downloadTTL)
}
