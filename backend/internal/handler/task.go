package handler

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"trustmesh/backend/internal/agentfile"
	"trustmesh/backend/internal/clawsynapse"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/protocol"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

type addTaskCommentResponse struct {
	Comment           model.Comment                  `json:"comment"`
	MentionDeliveries []model.CommentMentionDelivery `json:"mention_deliveries,omitempty"`
}

type TaskHandler struct {
	store          *store.Store
	publisher      *clawsynapse.Client
	webhookHandler *clawsynapse.WebhookHandler
	externalURL    string
	jwtSecret      []byte
	downloadTTL    time.Duration
	log            *zap.Logger
}

func NewTaskHandler(s *store.Store, publisher *clawsynapse.Client, wh *clawsynapse.WebhookHandler, externalURL string, jwtSecret []byte, downloadTTL time.Duration, log *zap.Logger) *TaskHandler {
	return &TaskHandler{
		store:          s,
		publisher:      publisher,
		webhookHandler: wh,
		externalURL:    externalURL,
		jwtSecret:      jwtSecret,
		downloadTTL:    downloadTTL,
		log:            log,
	}
}

func (h *TaskHandler) Create(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}


	var body struct {
		Title           string          `json:"title"`
		Description     string          `json:"description"`
		Priority        string          `json:"priority"`
		AssigneeAgentID string          `json:"assignee_agent_id"`
		FileIDs         []string        `json:"file_ids"`
		Workflow        *model.Workflow `json:"workflow,omitempty"`
		WorkflowIndex   *int            `json:"workflow_index"`
		StepFrom        int             `json:"step_from"`
		StepTo          int             `json:"step_to"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid request body"))
		return
	}

	task, appErr := h.store.CreateTaskByUser(sc, store.UserTaskCreateInput{
		ProjectID:       c.Param("projectId"),
		Title:           body.Title,
		Description:     body.Description,
		Priority:        body.Priority,
		AssigneeAgentID: body.AssigneeAgentID,
		FileIDs:         body.FileIDs,
		Workflow:        body.Workflow,
		WorkflowIndex:   body.WorkflowIndex,
		StepFrom:        body.StepFrom,
		StepTo:          body.StepTo,
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	task = h.autoDispatchFirstTodo(c.Request.Context(), sc, task)
	transport.WriteData(c, http.StatusCreated, task)
}

// CreateFromText creates a task from a free-text description.
// If agent_id is provided (set by the frontend @mention), the task is created in
// building mode and dispatched immediately. Otherwise it enters planning mode.
func (h *TaskHandler) CreateFromText(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}


	var body struct {
		Content       string          `json:"content"`
		AgentID       string          `json:"agent_id"`
		FileIDs       []string        `json:"file_ids"`
		Workflow      *model.Workflow `json:"workflow,omitempty"`
		WorkflowIndex *int            `json:"workflow_index"`
		StepFrom      int             `json:"step_from"`
		StepTo        int             `json:"step_to"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Content) == "" {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "content is required"))
		return
	}

	projectID := c.Param("projectId")

	if strings.TrimSpace(body.AgentID) != "" {
		task, appErr := h.store.CreateTaskByUser(sc, store.UserTaskCreateInput{
			ProjectID:       projectID,
			Title:           deriveTitle(body.Content),
			Description:     body.Content,
			Priority:        "medium",
			AssigneeAgentID: body.AgentID,
			FileIDs:         body.FileIDs,
			Workflow:        body.Workflow,
			WorkflowIndex:   body.WorkflowIndex,
			StepFrom:        body.StepFrom,
			StepTo:          body.StepTo,
		})
		if appErr != nil {
			transport.WriteError(c, appErr)
			return
		}
		task = h.autoDispatchFirstTodo(c.Request.Context(), sc, task)
		transport.WriteData(c, http.StatusCreated, task)
		return
	}

	h.createPlanningTask(c, sc, projectID, body.Content, body.FileIDs, body.Workflow, body.WorkflowIndex, body.StepFrom, body.StepTo)
}

// deriveTitle extracts a short title from free-form content.
// Uses the first line if it is short enough, otherwise truncates.
func deriveTitle(content string) string {
	content = strings.TrimSpace(content)
	// Use the first line as the title candidate
	firstLine := content
	if idx := strings.IndexAny(content, "\n\r"); idx != -1 {
		firstLine = strings.TrimSpace(content[:idx])
	}
	// Strip leading @mentions so the title reads naturally
	for strings.HasPrefix(firstLine, "@") {
		if idx := strings.IndexByte(firstLine, ' '); idx != -1 {
			firstLine = strings.TrimSpace(firstLine[idx+1:])
		} else {
			firstLine = ""
			break
		}
	}
	if firstLine == "" {
		firstLine = content
	}
	return truncateTitle(firstLine, 30)
}

func truncateTitle(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

// createPlanningTask creates a planning-mode task and notifies the PM agent.
func (h *TaskHandler) createPlanningTask(c *gin.Context, sc store.Scope, projectID, content string, fileIDs []string, workflow *model.Workflow, workflowIndex *int, stepFrom, stepTo int) {
	task, appErr := h.store.CreateTaskPlanningWithFiles(sc, projectID, content, fileIDs, workflow, workflowIndex, stepFrom, stepTo)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	h.notifyPMTaskMessage(c, sc, projectID, task.ID, content, true, nil)
	transport.WriteData(c, http.StatusCreated, task)
}

// autoDispatchFirstTodo publishes a todo.assigned event for the first todo and records the dispatch.
// It returns the updated task if dispatch succeeds, or the original task if it fails (non-fatal).
func (h *TaskHandler) autoDispatchFirstTodo(ctx context.Context, sc store.Scope, task *model.TaskDetail) *model.TaskDetail {
	if h.publisher == nil || len(task.Todos) == 0 {
		return task
	}
	todo := &task.Todos[0]
	payload := protocol.TodoAssignedPayload{
		TaskID:      task.ID,
		TodoID:      todo.ID,
		Title:       todo.Title,
		Description: todo.Description,
		Content:     "你收到了一个新的 Todo 任务。请使用 /tm-task-exec skill 执行此任务，按要求回报进度和结果。",
		ExecBrief: &protocol.TodoExecBrief{
			Objective:    "执行分派的 Todo 任务；及时回报进度；完成后提交结果，失败时说明原因。",
			MustUseSkill: "tm-task-exec",
		},
		AttachedFiles: h.enrichAttachedFiles(task.AttachedFiles),
		Inputs:        h.webhookHandler.BuildTodoInputs(task, todo),
		Outputs:       h.webhookHandler.BuildTodoOutputs(task, todo),
	}
	if _, err := h.publisher.Publish(ctx, todo.Assignee.NodeID, "todo.assigned", payload, task.ID, map[string]any{"source": "user_created"}); err != nil {
		if h.log != nil {
			h.log.Warn("auto dispatch todo failed", zap.String("task_id", task.ID), zap.String("todo_id", todo.ID), zap.Error(err))
		}
		return task
	}
	dispatched, dispatchErr := h.store.RecordTodoDispatch(sc, task.ID, todo.ID)
	if dispatchErr != nil {
		if h.log != nil {
			h.log.Warn("record todo dispatch failed", zap.String("task_id", task.ID), zap.String("todo_id", todo.ID), zap.Error(dispatchErr))
		}
		return task
	}
	return dispatched
}

func (h *TaskHandler) ListByProject(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	status := c.Query("status")
	items, appErr := h.store.ListTasks(sc, c.Param("projectId"), status)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteList(c, items, len(items))
}

func (h *TaskHandler) Get(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	task, appErr := h.store.GetTask(sc, c.Param("id"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, task)
}

func (h *TaskHandler) ListEvents(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	events, appErr := h.store.ListTaskEvents(sc, c.Param("id"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteList(c, events, len(events))
}

func (h *TaskHandler) DispatchTodo(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}


	task, appErr := h.store.GetTask(sc, c.Param("id"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if appErr := h.store.CheckTaskProjectActive(task.ID); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	todo := findTaskTodo(task, c.Param("todoId"))
	if todo == nil {
		transport.WriteError(c, transport.NotFound("todo not found"))
		return
	}
	if todo.Status != "pending" {
		transport.WriteError(c, transport.Conflict("TODO_NOT_PENDING", "todo is not pending"))
		return
	}
	if !task.CanDispatchTodo(todo.ID) {
		transport.WriteError(c, transport.Conflict("TODO_BLOCKED_BY_PREVIOUS", "todo is blocked by previous todos"))
		return
	}
	if h.publisher == nil {
		transport.WriteError(c, transport.NewError(http.StatusServiceUnavailable, "CLAWSYNAPSE_DISABLED", "clawsynapse client is disabled"))
		return
	}

	payload := protocol.TodoAssignedPayload{
		TaskID:      task.ID,
		TodoID:      todo.ID,
		Title:       todo.Title,
		Description: todo.Description,
		Content:     "你收到了一个新的 Todo 任务。请使用 /tm-task-exec skill 执行此任务，按要求回报进度和结果。",
		ExecBrief: &protocol.TodoExecBrief{
			Objective:    "执行分派的 Todo 任务；及时回报进度；完成后提交结果，失败时说明原因。",
			MustUseSkill: "tm-task-exec",
		},
		AttachedFiles: h.enrichAttachedFiles(task.AttachedFiles),
		Inputs:        h.webhookHandler.BuildTodoInputs(task, todo),
		Outputs:       h.webhookHandler.BuildTodoOutputs(task, todo),
	}
	if _, err := h.publisher.Publish(context.Background(), todo.Assignee.NodeID, "todo.assigned", payload, task.ID, map[string]any{"source": "manual_dispatch"}); err != nil {
		if h.log != nil {
			h.log.Warn("manual todo dispatch failed", zap.String("task_id", task.ID), zap.String("todo_id", todo.ID), zap.String("target_node", todo.Assignee.NodeID), zap.Error(err))
		}
		transport.WriteError(c, transport.NewError(http.StatusBadGateway, "TODO_DISPATCH_FAILED", "failed to dispatch todo to agent"))
		return
	}

	task, appErr = h.store.RecordTodoDispatch(sc, task.ID, todo.ID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, task)
}

// ReviewTodo handles a human reviewer approving or rejecting a completed todo
// that is awaiting review. approve unblocks the pipeline (next todo dispatched);
// reject cascade-resets and re-dispatches the audited predecessor for rework.
// BindTodoOutput attaches an already-filed artifact to a workflow output slot,
// promoting it from a process file to a declared deliverable so it shows up on
// the workflow diagram and becomes resolvable as a downstream step input.
// This is the manual remedy for uploads that arrived without
// `--metadata outputName` and could not be auto-matched (ambiguous multi-slot
// steps, unexpected mime types).
func (h *TaskHandler) BindTodoOutput(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	var body struct {
		ArtifactID string `json:"artifact_id"`
		OutputName string `json:"output_name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid request body"))
		return
	}

	artifact, appErr := h.store.BindArtifactOutput(sc, c.Param("id"), c.Param("todoId"), body.ArtifactID, body.OutputName)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, artifact)
}

func (h *TaskHandler) ReviewTodo(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	var body struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid request body"))
		return
	}

	task, reworked, appErr := h.store.ReviewTodo(sc, "", c.Param("id"), c.Param("todoId"), body.Action, body.Reason)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if h.publisher != nil {
		if body.Action == "approve" {
			// Approved review unblocks the sequential pipeline: dispatch the next
			// ready todo.
			task = h.dispatchNextTodo(c.Request.Context(), sc, task)
		} else if reworked != nil {
			// Rejected review triggered a rework: the audited predecessor was
			// reset and must be re-dispatched to its assignee immediately.
			h.publishReworkDispatch(c.Request.Context(), sc, task, reworked, body.Reason)
		}
	}
	transport.WriteData(c, http.StatusOK, task)
}

// AnswerTodo answers a question the assignee agent asked via todo.ask: the
// todo resumes to in_progress and todo.answer is forwarded to the agent so it
// keeps executing in the same context.
func (h *TaskHandler) AnswerTodo(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	var body struct {
		QuestionID string `json:"question_id"`
		Answer     string `json:"answer"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid request body"))
		return
	}
	if strings.TrimSpace(body.QuestionID) == "" || strings.TrimSpace(body.Answer) == "" {
		transport.WriteError(c, transport.Validation("missing fields", map[string]any{"question_id": "required", "answer": "required"}))
		return
	}

	task, question, appErr := h.store.AnswerTodo(sc, c.Param("id"), c.Param("todoId"), body.QuestionID, body.Answer, sc.UserID, false)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	h.PublishTodoAnswer(c.Request.Context(), task, c.Param("todoId"), question)
	transport.WriteData(c, http.StatusOK, gin.H{"status": "ok", "question_id": question.ID})
}

// PublishTodoAnswer forwards the user's answer to the assignee agent so its
// session resumes after the todo.ask pause.
func (h *TaskHandler) PublishTodoAnswer(ctx context.Context, task *model.TaskDetail, todoID string, question *model.TodoQuestion) {
	if h.publisher == nil || task == nil || question == nil {
		return
	}
	var todo *model.Todo
	for i := range task.Todos {
		if task.Todos[i].ID == todoID {
			todo = &task.Todos[i]
			break
		}
	}
	if todo == nil || todo.Assignee.NodeID == "" {
		return
	}
	answeredAt := time.Now().UTC()
	if question.AnsweredAt != nil {
		answeredAt = *question.AnsweredAt
	}
	payload := protocol.TodoAnswerPayload{
		TaskID:     task.ID,
		TodoID:     todo.ID,
		QuestionID: question.ID,
		Question:   question.Question,
		Answer:     question.Answer,
		AnsweredBy: question.AnsweredBy,
		AnsweredAt: answeredAt,
		TimedOut:   question.TimedOut,
	}
	if _, err := h.publisher.Publish(ctx, todo.Assignee.NodeID, "todo.answer", payload, task.ID, nil); err != nil && h.log != nil {
		h.log.Warn("todo.answer publish failed", zap.String("task_id", task.ID), zap.String("todo_id", todo.ID), zap.String("target_node", todo.Assignee.NodeID), zap.Error(err))
	}
}

// publishReworkDispatch publishes a todo.assigned message for a todo that was
// sent back for rework (review rejection), telling the assignee to redo it.
func (h *TaskHandler) publishReworkDispatch(ctx context.Context, sc store.Scope, task *model.TaskDetail, todo *model.Todo, reason string) {
	if h.publisher == nil || task == nil || todo == nil {
		return
	}
	payload := protocol.TodoAssignedPayload{
		TaskID:      task.ID,
		TodoID:      todo.ID,
		Title:       todo.Title,
		Description: todo.Description,
		Content:     "该 Todo 的产出未通过审核，已被退回重做。请重新执行并按要求回报进度和结果。退回原因：" + reason,
		ExecBrief: &protocol.TodoExecBrief{
			Objective:    "重做被退回的 Todo；重新提交产出直至通过审核。",
			MustUseSkill: "tm-task-exec",
		},
		AttachedFiles: h.enrichAttachedFiles(task.AttachedFiles),
		Inputs:        h.webhookHandler.BuildTodoInputs(task, todo),
		Outputs:       h.webhookHandler.BuildTodoOutputs(task, todo),
	}
	if _, err := h.publisher.Publish(ctx, todo.Assignee.NodeID, "todo.assigned", payload, task.ID, map[string]any{"source": "rework", "reason": reason}); err != nil {
		if h.log != nil {
			h.log.Warn("rework todo dispatch failed", zap.String("task_id", task.ID), zap.String("todo_id", todo.ID), zap.String("target_node", todo.Assignee.NodeID), zap.Error(err))
		}
	}
}

// dispatchNextTodo publishes todo.assigned for the next dispatchable todo (if
// any) and records the dispatch, mirroring autoDispatchFirstTodo for later
// positions in the chain.
func (h *TaskHandler) dispatchNextTodo(ctx context.Context, sc store.Scope, task *model.TaskDetail) *model.TaskDetail {
	if h.publisher == nil {
		return task
	}
	todo := task.NextDispatchableTodo()
	if todo == nil {
		return task
	}
	payload := protocol.TodoAssignedPayload{
		TaskID:      task.ID,
		TodoID:      todo.ID,
		Title:       todo.Title,
		Description: todo.Description,
		Content:     "你收到了一个新的 Todo 任务。请使用 /tm-task-exec skill 执行此任务，按要求回报进度和结果。",
		ExecBrief: &protocol.TodoExecBrief{
			Objective:    "执行分派的 Todo 任务；及时回报进度；完成后提交结果，失败时说明原因。",
			MustUseSkill: "tm-task-exec",
		},
		AttachedFiles: h.enrichAttachedFiles(task.AttachedFiles),
		Inputs:        h.webhookHandler.BuildTodoInputs(task, todo),
		Outputs:       h.webhookHandler.BuildTodoOutputs(task, todo),
	}
	if _, err := h.publisher.Publish(ctx, todo.Assignee.NodeID, "todo.assigned", payload, task.ID, map[string]any{"source": "review_approved"}); err != nil {
		if h.log != nil {
			h.log.Warn("dispatch next todo after review failed", zap.String("task_id", task.ID), zap.String("todo_id", todo.ID), zap.String("target_node", todo.Assignee.NodeID), zap.Error(err))
		}
		return task
	}
	dispatched, dispatchErr := h.store.RecordTodoDispatch(sc, task.ID, todo.ID)
	if dispatchErr != nil {
		if h.log != nil {
			h.log.Warn("record next todo dispatch failed", zap.String("task_id", task.ID), zap.String("todo_id", todo.ID), zap.Error(dispatchErr))
		}
		return task
	}
	return dispatched
}

func (h *TaskHandler) AddComment(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	var body struct {
		Content  string `json:"content"`
		TodoID   string `json:"todo_id"`
		Mentions []struct {
			AgentID string `json:"agent_id"`
		} `json:"mentions"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid request body"))
		return
	}

	mentions := make([]store.TaskCommentMentionInput, 0, len(body.Mentions))
	for _, mention := range body.Mentions {
		mentions = append(mentions, store.TaskCommentMentionInput{AgentID: mention.AgentID})
	}

	comment, appErr := h.store.AddTaskComment(sc, c.Param("id"), store.TaskCommentInput{
		TaskID:   c.Param("id"),
		TodoID:   body.TodoID,
		Content:  body.Content,
		Mentions: mentions,
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	task, appErr := h.store.GetTask(sc, c.Param("id"))
	if appErr != nil {
		if h.log != nil {
			h.log.Warn("load task after comment failed", zap.String("task_id", c.Param("id")), zap.Error(appErr))
		}
		transport.WriteData(c, http.StatusCreated, addTaskCommentResponse{Comment: *comment})
		return
	}

	transport.WriteData(c, http.StatusCreated, addTaskCommentResponse{
		Comment:           *comment,
		MentionDeliveries: h.publishTaskCommentMentions(c.Request.Context(), task, comment),
	})
}

func (h *TaskHandler) Cancel(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid request body"))
		return
	}

	task, appErr := h.store.CancelTask(sc, store.TaskCancelInput{
		TaskID: c.Param("id"),
		Reason: strings.TrimSpace(body.Reason),
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, task)
}

func (h *TaskHandler) ListComments(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	comments, appErr := h.store.ListTaskComments(sc, c.Param("id"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteList(c, comments, len(comments))
}

func findTaskTodo(task *model.TaskDetail, todoID string) *model.Todo {
	for i := range task.Todos {
		if task.Todos[i].ID == todoID {
			return &task.Todos[i]
		}
	}
	return nil
}

func (h *TaskHandler) publishTaskCommentMentions(ctx context.Context, task *model.TaskDetail, comment *model.Comment) []model.CommentMentionDelivery {
	if comment == nil || len(comment.Mentions) == 0 {
		return nil
	}

	deliveries := make([]model.CommentMentionDelivery, 0, len(comment.Mentions))
	for _, mention := range comment.Mentions {
		delivery := model.CommentMentionDelivery{
			AgentID:   mention.AgentID,
			AgentName: mention.AgentName,
			Status:    "sent",
		}

		if h.publisher == nil {
			delivery.Status = "failed"
			delivery.Error = "clawsynapse client is disabled"
			deliveries = append(deliveries, delivery)
			continue
		}
		if strings.TrimSpace(mention.NodeID) == "" {
			delivery.Status = "failed"
			delivery.Error = "target agent node is missing"
			deliveries = append(deliveries, delivery)
			continue
		}

		payload := h.buildTaskMentionPayload(task, comment, mention)
		if _, err := h.publisher.Publish(ctx, mention.NodeID, "task.mention", payload, task.ID, map[string]any{
			"source":     "task_comment_mention",
			"task_id":    task.ID,
			"comment_id": comment.ID,
		}); err != nil {
			delivery.Status = "failed"
			delivery.Error = err.Error()
			if h.log != nil {
				h.log.Warn(
					"publish task mention failed",
					zap.String("task_id", task.ID),
					zap.String("comment_id", comment.ID),
					zap.String("target_agent_id", mention.AgentID),
					zap.String("target_node", mention.NodeID),
					zap.Error(err),
				)
			}
		}

		deliveries = append(deliveries, delivery)
	}

	return deliveries
}

func (h *TaskHandler) buildTaskMentionPayload(task *model.TaskDetail, comment *model.Comment, mention model.CommentMention) protocol.TaskMentionPayload {
	payload := protocol.TaskMentionPayload{
		TaskID:          task.ID,
		ProjectID:       task.ProjectID,
		CommentID:       comment.ID,
		TodoID:          comment.TodoID,
		TaskTitle:       task.Title,
		TaskDescription: task.Description,
		TaskStatus:      task.Status,
		TaskPriority:    task.Priority,
		AuthorName:      comment.ActorName,
		UserContent:     comment.Content,
		Content: fmt.Sprintf(
			"用户在任务评论中 @%s。请结合任务上下文阅读这条评论；如需回应，请通过 task.comment 回复并携带 task_id=%s。",
			mention.AgentName,
			task.ID,
		),
	}

	if todo := findTaskTodo(task, comment.TodoID); todo != nil {
		payload.TodoTitle = todo.Title
	}

	return payload
}

func (h *TaskHandler) ApprovePlan(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	taskID := c.Param("id")
	task, appErr := h.store.ApprovePlan(sc, taskID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	if h.webhookHandler != nil {
		h.webhookHandler.PublishTaskCreated(task)
		task = h.webhookHandler.DispatchNextTodo(c.Request.Context(), task)
	}
	transport.WriteData(c, http.StatusOK, task)
}

func (h *TaskHandler) RejectPlan(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}


	var body struct {
		Feedback string `json:"feedback"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid request body"))
		return
	}

	taskID := c.Param("id")
	task, appErr := h.store.RejectPlan(sc, taskID, body.Feedback)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	h.notifyPMTaskMessage(c, sc, task.ProjectID, task.ID, body.Feedback, false, nil)
	transport.WriteData(c, http.StatusOK, task)
}

func (h *TaskHandler) CreatePlanning(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}


	var body struct {
		Content       string          `json:"content"`
		FileIDs       []string        `json:"file_ids"`
		Workflow      *model.Workflow `json:"workflow,omitempty"`
		WorkflowIndex *int            `json:"workflow_index"`
		StepFrom      int             `json:"step_from"`
		StepTo        int             `json:"step_to"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid request body"))
		return
	}

	projectID := c.Param("projectId")
	task, appErr := h.store.CreateTaskPlanningWithFiles(sc, projectID, body.Content, body.FileIDs, body.Workflow, body.WorkflowIndex, body.StepFrom, body.StepTo)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	h.notifyPMTaskMessage(c, sc, projectID, task.ID, body.Content, true, nil)
	transport.WriteData(c, http.StatusCreated, task)
}

func (h *TaskHandler) AppendTaskMessage(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}


	var body struct {
		Content    string            `json:"content"`
		UIResponse *model.UIResponse `json:"ui_response,omitempty"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid request body"))
		return
	}

	taskID := c.Param("id")
	task, appErr := h.store.AppendTaskMessage(sc, taskID, body.Content, body.UIResponse)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	h.notifyPMTaskMessage(c, sc, task.ProjectID, task.ID, body.Content, false, body.UIResponse)
	transport.WriteData(c, http.StatusOK, task)
}

func (h *TaskHandler) notifyPMTaskMessage(c *gin.Context, sc store.Scope, projectID, taskID, content string, initial bool, uiResponse *model.UIResponse) {
	if h.publisher == nil {
		return
	}
	pmNodeID, appErr := h.store.GetTaskPMPublishTarget(sc, taskID)
	if appErr != nil {
		if h.log != nil {
			h.log.Warn("skip notify task.message", zap.String("task_id", taskID), zap.String("code", appErr.Code))
		}
		return
	}
	payload := h.buildPMTaskMessage(sc, projectID, taskID, content, initial, uiResponse)
	if _, err := h.publisher.Publish(c.Request.Context(), pmNodeID, "task.message", payload, taskID, nil); err != nil {
		if h.log != nil {
			h.log.Warn("notify task.message failed", zap.String("task_id", taskID), zap.Error(err))
		}
	}
}

func (h *TaskHandler) buildPMTaskMessage(sc store.Scope, projectID, taskID, userContent string, initial bool, uiResponse *model.UIResponse) protocol.PMTaskMessage {
	payload := protocol.PMTaskMessage{
		SchemaVersion:  "1.0",
		TaskID:         taskID,
		ProjectID:      projectID,
		UserContent:    userContent,
		IsInitial:      initial,
		UserUIResponse: uiResponse,
	}

	// Enrich with attached files from the task.
	task, err := h.store.GetTask(sc, taskID)
	if err == nil && task != nil {
		payload.AttachedFiles = h.enrichAttachedFiles(task.AttachedFiles)
		// Carry the workflow snapshot so the PM can plan against it.
		if task.Workflow != nil {
			payload.Workflow = task.Workflow
		}
		if h.log != nil && len(payload.AttachedFiles) > 0 {
			h.log.Info("enriched attached files for PM message",
				zap.String("task_id", taskID),
				zap.String("external_url", h.externalURL),
				zap.Int("jwt_secret_len", len(h.jwtSecret)),
				zap.String("first_download_url", payload.AttachedFiles[0].DownloadUrl),
			)
		}
	}

	if initial {
		payload.Content = "请使用 /tm-task-plan skill 处理本次需求。"
	} else {
		payload.Content = "用户发送了新消息，请使用 /tm-task-plan skill 继续处理。"
	}

	if !initial {
		return payload
	}

	project, appErr := h.store.GetProject(sc, projectID)
	if appErr != nil {
		if h.log != nil {
			h.log.Warn("build initial pm task message missing project context", zap.String("project_id", projectID))
		}
		return payload
	}

	candidates := buildCandidateAgents(project.PMAgent.ID, h.store.ListAgents(sc))
	payload.Project = &protocol.PMTaskProject{
		Name:        project.Name,
		Description: project.Description,
	}
	payload.CandidateAgents = candidates
	return payload
}

func buildCandidateAgents(pmAgentID string, agents []model.Agent) []protocol.PMTaskAgent {
	out := make([]protocol.PMTaskAgent, 0, len(agents))
	for _, agent := range agents {
		if agent.ID == pmAgentID {
			continue
		}
		out = append(out, protocol.PMTaskAgent{
			ID:           agent.ID,
			Name:         agent.Name,
			NodeID:       agent.NodeID,
			Role:         agent.Role,
			Status:       agent.Status,
			Capabilities: append([]string(nil), agent.Capabilities...),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		left := agentStatusRank(out[i].Status)
		right := agentStatusRank(out[j].Status)
		if left != right {
			return left < right
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func agentStatusRank(status string) int {
	switch status {
	case "online":
		return 0
	case "busy":
		return 1
	default:
		return 2
	}
}

// enrichAttachedFiles converts model attached files to protocol refs with download URLs.
func (h *TaskHandler) enrichAttachedFiles(files []model.TaskAttachedFile) []protocol.TaskAttachedFileRef {
	return agentfile.EnrichWithDownloadURLs(files, h.externalURL, h.jwtSecret, h.downloadTTL)
}

// AddTodo creates a new TODO at the end of a task's todo list.
func (h *TaskHandler) AddTodo(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	var body struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		AssigneeID  string `json:"assignee_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid request body"))
		return
	}

	task, appErr := h.store.AppendTodo(sc, c.Param("id"), store.TodoModifyInput{
		Title:       body.Title,
		Description: body.Description,
		AssigneeID:  body.AssigneeID,
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// Dispatch via ClawSynapse if this todo is the next one in sequence
	if h.webhookHandler != nil {
		task = h.webhookHandler.DispatchNextTodo(c.Request.Context(), task)
	}
	transport.WriteData(c, http.StatusCreated, task)
}

// InsertTodo creates a new TODO before a specified todo.
func (h *TaskHandler) InsertTodo(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	var body struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		AssigneeID  string `json:"assignee_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid request body"))
		return
	}

	task, appErr := h.store.InsertTodo(sc, c.Param("id"), c.Param("todoId"), store.TodoModifyInput{
		Title:       body.Title,
		Description: body.Description,
		AssigneeID:  body.AssigneeID,
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// Dispatch via ClawSynapse if this todo is the next one in sequence
	if h.webhookHandler != nil {
		task = h.webhookHandler.DispatchNextTodo(c.Request.Context(), task)
	}
	transport.WriteData(c, http.StatusCreated, task)
}

// UpdateTodo updates a todo's title, description, and/or assignee.
func (h *TaskHandler) UpdateTodo(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	var body struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		AssigneeID  string `json:"assignee_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid request body"))
		return
	}

	task, appErr := h.store.UpdateTodo(sc, c.Param("id"), c.Param("todoId"), store.TodoModifyInput{
		Title:       body.Title,
		Description: body.Description,
		AssigneeID:  body.AssigneeID,
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, task)
}

// RemoveTodo deletes a pending todo from a task.
func (h *TaskHandler) RemoveTodo(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	task, appErr := h.store.RemoveTodo(sc, c.Param("id"), c.Param("todoId"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// Dispatch via ClawSynapse if there is a next pending todo
	if h.webhookHandler != nil {
		task = h.webhookHandler.DispatchNextTodo(c.Request.Context(), task)
	}
	transport.WriteData(c, http.StatusOK, task)
}

// ReorderTodos reorders all todos in a task.
func (h *TaskHandler) ReorderTodos(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	var body struct {
		TodoIDs []string `json:"todo_ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_PAYLOAD", "invalid request body"))
		return
	}

	task, appErr := h.store.ReorderTodos(sc, c.Param("id"), body.TodoIDs)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, task)
}
