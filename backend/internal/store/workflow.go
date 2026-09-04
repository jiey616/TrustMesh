package store

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

type TaskCreateTodoInput struct {
	ID             string
	Order          int
	Title          string
	Description    string
	AssigneeNodeID string
}

type TaskCreateInput struct {
	ProjectID   string
	Title       string
	Description string
	Todos       []TaskCreateTodoInput
	// SourceTaskID links the created task back to the task whose action item
	// produced it (action-items → tasks conversion).
	SourceTaskID string
}

type TodoProgressInput struct {
	TaskID  string
	TodoID  string
	Message string
}

type TodoCompleteInput struct {
	TaskID string
	TodoID string
	Result model.TodoResult

	// NeedReview marks the todo as awaiting human/PM review after completion.
	// Sequential dispatch of later todos blocks until approval.
	NeedReview bool
	// ReturnPrevious asks to send the previous todo (order-1, the one this
	// review-style todo audits) back for rework.
	ReturnPrevious bool
	// ReworkReason is the reviewer's concrete verdict forwarded to the
	// reworked assignee (fallback text used when empty).
	ReworkReason string
}

type TodoFailInput struct {
	TaskID string
	TodoID string
	Error  string
}

// TodoAskInput carries an agent's human-input request (todo.ask). The todo is
// parked in waiting_user until the user answers (or, for non-required
// questions, until the timeout auto-resumes it).
type TodoAskInput struct {
	TaskID     string
	TodoID     string
	QuestionID string
	Question   string
	Options    []string
	Required   bool
}

// TodoAnswerInput carries a user's (or timeout's) answer to a pending question.
type TodoAnswerInput struct {
	TaskID     string
	TodoID     string
	QuestionID string
	Answer     string
	AnsweredBy string
	TimedOut   bool
}

type TaskCancelInput struct {
	TaskID string
	Reason string
}

type TaskCommentInput struct {
	TaskID   string
	TodoID   string
	Content  string
	Mentions []TaskCommentMentionInput
}

type TaskCommentMentionInput struct {
	AgentID string
}

func (s *Store) RecordTodoDispatch(userID, taskID, todoID string) (*model.TaskDetail, *transport.AppError) {
	taskID = strings.TrimSpace(taskID)
	todoID = strings.TrimSpace(todoID)
	if taskID == "" || todoID == "" {
		return nil, transport.Validation("invalid todo dispatch payload", map[string]any{"task_id": "required", "todo_id": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return nil, transport.NotFound("task not found")
	}
	if appErr := s.ensureTaskProjectActiveUnsafe(task); appErr != nil {
		return nil, appErr
	}

	todoIdx := findTodoIndex(task, todoID)
	if todoIdx < 0 {
		return nil, transport.NotFound("todo not found")
	}

	todo := &task.Todos[todoIdx]
	if appErr := ensureTaskAcceptingUpdates(task); appErr != nil {
		return nil, appErr
	}
	if appErr := ensureTodoAcceptingUpdates(todo); appErr != nil {
		return nil, appErr
	}
	if todo.Status != "pending" {
		return nil, transport.Conflict("TODO_NOT_PENDING", "todo is not pending")
	}
	if !task.CanDispatchTodo(todoID) {
		return nil, transport.Conflict("TODO_BLOCKED_BY_PREVIOUS", "todo is blocked by previous todos")
	}

	now := time.Now().UTC()
	userName := ""
	if u, ok := s.users[userID]; ok {
		userName = u.Name
	}
	message := fmt.Sprintf("手动派发给 %s", todo.Assignee.Name)
	s.recordTodoDispatchUnsafe(task, todo, "user", userID, userName, &message, map[string]any{
		"todo_id":           todo.ID,
		"assignee_agent_id": todo.Assignee.AgentID,
		"manual":            true,
	}, now)
	if todo.Status == "pending" {
		todo.Status = "in_progress"
		todo.StartedAt = &now
		todo.AssignedAt = &now
	}
	s.updateTaskStatusUnsafe(task, now)
	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	s.refreshAgentExecutionStatusUnsafe(todo.Assignee.AgentID, now)
	if err := s.persistAgentGraphUnsafe(todo.Assignee.AgentID); err != nil {
		return nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)
	return s.copyTaskWithArtifactsUnsafe(task), nil
}

func (s *Store) RecordSequentialTodoDispatch(taskID, todoID string) (*model.TaskDetail, *transport.AppError) {
	taskID = strings.TrimSpace(taskID)
	todoID = strings.TrimSpace(todoID)
	if taskID == "" || todoID == "" {
		return nil, transport.Validation("invalid todo dispatch payload", map[string]any{"task_id": "required", "todo_id": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return nil, transport.NotFound("task not found")
	}
	if appErr := s.ensureTaskProjectActiveUnsafe(task); appErr != nil {
		return nil, appErr
	}

	todoIdx := findTodoIndex(task, todoID)
	if todoIdx < 0 {
		return nil, transport.NotFound("todo not found")
	}

	todo := &task.Todos[todoIdx]
	if appErr := ensureTaskAcceptingUpdates(task); appErr != nil {
		return nil, appErr
	}
	if appErr := ensureTodoAcceptingUpdates(todo); appErr != nil {
		return nil, appErr
	}
	if todo.Status != "pending" {
		return nil, transport.Conflict("TODO_NOT_PENDING", "todo is not pending")
	}
	if !task.CanDispatchTodo(todoID) {
		return nil, transport.Conflict("TODO_BLOCKED_BY_PREVIOUS", "todo is blocked by previous todos")
	}

	now := time.Now().UTC()
	message := fmt.Sprintf("按顺序派发给 %s", todo.Assignee.Name)
	s.recordTodoDispatchUnsafe(task, todo, "system", "system", "System", &message, map[string]any{
		"todo_id":           todo.ID,
		"assignee_agent_id": todo.Assignee.AgentID,
		"dispatch_mode":     "sequential",
		"manual":            false,
	}, now)
	if todo.Status == "pending" {
		todo.Status = "in_progress"
		todo.StartedAt = &now
		todo.AssignedAt = &now
	}
	s.updateTaskStatusUnsafe(task, now)
	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	s.refreshAgentExecutionStatusUnsafe(todo.Assignee.AgentID, now)
	if err := s.persistAgentGraphUnsafe(todo.Assignee.AgentID); err != nil {
		return nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)
	return s.copyTaskWithArtifactsUnsafe(task), nil
}

func (s *Store) CreateTaskByPMNode(nodeID string, in TaskCreateInput) (*model.TaskDetail, *transport.AppError) {
	return s.CreateTaskByPMNodeWithMessageID(nodeID, "", in)
}

func (s *Store) CreateTaskByPMNodeWithMessageID(nodeID, messageID string, in TaskCreateInput) (*model.TaskDetail, *transport.AppError) {
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	if strings.TrimSpace(in.ProjectID) == "" || in.Title == "" || in.Description == "" {
		return nil, transport.Validation("invalid task.create payload", map[string]any{
			"project_id":  "required",
			"title":       "required",
			"description": "required",
		})
	}
	if len(in.Todos) == 0 {
		return nil, transport.Validation("invalid task.create payload", map[string]any{"todos": "must not be empty"})
	}
	normalizedTodos, todoErr := normalizeTaskCreateTodos(in.Todos)
	if todoErr != nil {
		return nil, todoErr
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if task, ok := s.findProcessedTaskUnsafe(processedMessageKey("task.create", nodeID, messageID)); ok {
		return task, nil
	}

	pmAgent, err := s.agentByNodeUnsafe(nodeID)
	if err != nil {
		return nil, err
	}
	if pmAgent.Role != "pm" {
		return nil, transport.Forbidden("only pm agent can create tasks")
	}

	project, ok := s.projects[in.ProjectID]
	if !ok {
		return nil, transport.NotFound("project not found")
	}
	if project.Status == "archived" {
		return nil, transport.Conflict("PROJECT_ARCHIVED", "archived project cannot create tasks")
	}
	if project.PMAgentID != pmAgent.ID {
		return nil, transport.Forbidden("pm agent is not bound to this project")
	}

	now := time.Now().UTC()
	s.markAgentSeenUnsafe(pmAgent.ID, now)
	seenTodoIDs := make(map[string]struct{}, len(normalizedTodos))
	todos := make([]model.Todo, 0, len(normalizedTodos))
	for i, todoIn := range normalizedTodos {
		title := strings.TrimSpace(todoIn.Title)
		desc := strings.TrimSpace(todoIn.Description)
		assigneeNode := strings.TrimSpace(todoIn.AssigneeNodeID)
		if title == "" || desc == "" || assigneeNode == "" {
			return nil, transport.Validation("invalid todo in task.create", map[string]any{"todo_index": i, "title": "required", "description": "required", "assignee_node_id": "required"})
		}

		assigneeAgent, assigneeErr := s.agentByNodeUnsafe(assigneeNode)
		if assigneeErr != nil {
			return nil, transport.Validation("invalid assignee_node_id", map[string]any{"todo_index": i, "assignee_node_id": assigneeNode})
		}
		if assigneeAgent.UserID != project.UserID {
			return nil, transport.Forbidden("assignee agent does not belong to same user")
		}

		todoID := strings.TrimSpace(todoIn.ID)
		if todoID == "" {
			todoID = uuid.NewString()
		}
		if _, dup := seenTodoIDs[todoID]; dup {
			return nil, transport.Validation("duplicated todo id", map[string]any{"todo_id": todoID})
		}
		seenTodoIDs[todoID] = struct{}{}

		todos = append(todos, model.Todo{
			ID:          todoID,
			Order:       todoIn.Order,
			Title:       title,
			Description: desc,
			Status:      "pending",
			Assignee: model.TodoAssignee{
				AgentID: assigneeAgent.ID,
				Name:    assigneeAgent.Name,
				NodeID:  assigneeAgent.NodeID,
			},
			StartedAt:    nil,
			CompletedAt:  nil,
			FailedAt:     nil,
			CanceledAt:   nil,
			Error:        nil,
			CancelReason: nil,
			Result: model.TodoResult{
				Summary:  "",
				Output:   "",
				Metadata: map[string]any{},
			},
			CreatedAt: now,
		})
	}

	task := &model.TaskDetail{
		ID:           newID(),
		UserID:       project.UserID,
		OrgID:        s.personalOrgOfUnsafe(project.UserID),
		ProjectID:    project.ID,
		Title:        in.Title,
		Description:  in.Description,
		Status:       "pending",
		Priority:     "medium",
		SourceTaskID: in.SourceTaskID,
		PMAgentID:    pmAgent.ID,
		PMAgent:      toPMSummary(pmAgent),
		Todos:        todos,

		Result: model.TaskResult{
			Summary:     "",
			FinalOutput: "",
			Metadata:    map[string]any{},
		},
		Version:      1,
		CanceledAt:   nil,
		CanceledBy:   nil,
		CancelReason: nil,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	s.tasks[task.ID] = task
	s.projectTasks[task.ProjectID] = append(s.projectTasks[task.ProjectID], task.ID)
	taskTitle := task.Title
	s.addEventUnsafe(project.UserID, project.ID, task.ID, "", "agent", pmAgent.ID, pmAgent.Name, "task_created", &taskTitle, map[string]any{"task_title": task.Title}, now)

	s.rememberProcessedMessageUnsafe(processedMessageKey("task.create", nodeID, messageID), "task.create", task.ID)
	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	if err := s.persistAgentGraphUnsafe(pmAgent.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	if err := s.persistProcessedMessageUnsafe(processedMessageKey("task.create", nodeID, messageID)); err != nil {
		return nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)

	return s.copyTaskWithArtifactsUnsafe(task), nil
}

type UserTaskCreateInput struct {
	ProjectID       string
	Title           string
	Description     string
	Priority        string
	AssigneeAgentID string
	FileIDs         []string
	// SourceTaskID links the created task back to the source task whose
	// action item produced it (action-items → tasks conversion).
	SourceTaskID string
	// Workflow overrides the project default workflow snapshot; nil means
	// copy the project's workflow.
	Workflow *model.Workflow
	// WorkflowIndex + StepFrom/StepTo create the task against a slice of the
	// project's primary workflow ("项目总流程"). When WorkflowIndex is set the
	// snapshot is trimmed to the owned step range and WorkflowRef is recorded;
	// otherwise Workflow (if any) is used as-is and WorkflowRef stays empty.
	WorkflowIndex *int
	StepFrom      int
	StepTo        int
}

func (s *Store) CreateTaskByUser(userID string, in UserTaskCreateInput) (*model.TaskDetail, *transport.AppError) {
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	in.Priority = strings.TrimSpace(in.Priority)
	if strings.TrimSpace(in.ProjectID) == "" || in.Title == "" || in.Description == "" || strings.TrimSpace(in.AssigneeAgentID) == "" {
		return nil, transport.Validation("invalid task create payload", map[string]any{
			"project_id":        "required",
			"title":             "required",
			"description":       "required",
			"assignee_agent_id": "required",
		})
	}
	priority := in.Priority
	if priority == "" {
		priority = "medium"
	}
	validPriorities := map[string]bool{"low": true, "medium": true, "high": true, "urgent": true}
	if !validPriorities[priority] {
		return nil, transport.Validation("invalid priority", map[string]any{"priority": "must be low, medium, high, or urgent"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	project, ok := s.projects[in.ProjectID]
	if !ok || project.UserID != userID {
		return nil, transport.NotFound("project not found")
	}
	if project.Status == "archived" {
		return nil, transport.Conflict("PROJECT_ARCHIVED", "archived project cannot create tasks")
	}

	assignee, ok := s.agents[in.AssigneeAgentID]
	if !ok || assignee.UserID != userID {
		return nil, transport.NotFound("assignee agent not found")
	}
	if assignee.Archived {
		return nil, transport.Conflict("AGENT_ARCHIVED", "cannot assign to archived agent")
	}

	now := time.Now().UTC()
	todoID := uuid.NewString()

	todo := model.Todo{
		ID:          todoID,
		Order:       1,
		Title:       in.Title,
		Description: in.Description,
		Status:      "pending",
		Assignee: model.TodoAssignee{
			AgentID: assignee.ID,
			Name:    assignee.Name,
			NodeID:  assignee.NodeID,
		},
		Result: model.TodoResult{
			Summary:  "",
			Output:   "",
			Metadata: map[string]any{},
		},
		CreatedAt: now,
	}

	// Workflow snapshot: when the task is created against a slice of the
	// project's primary workflow, trim the snapshot to the owned step range
	// and record the WorkflowRef; otherwise copy the explicitly chosen
	// workflow (or the project's default) as before.
	var wfSnapshot *model.Workflow
	var wfRef *model.WorkflowRef
	if in.WorkflowIndex != nil {
		wfIdx := *in.WorkflowIndex
		if wfIdx < 0 || wfIdx >= len(project.Workflows) {
			return nil, transport.Validation("invalid workflow_index", map[string]any{
				"workflow_index": wfIdx,
			})
		}
		pw := project.Workflows[wfIdx]
		if in.StepFrom < 0 || in.StepTo >= len(pw.Steps) || in.StepFrom > in.StepTo {
			return nil, transport.Validation("invalid step range", map[string]any{
				"step_from": in.StepFrom,
				"step_to":   in.StepTo,
				"max_step":  len(pw.Steps) - 1,
			})
		}
		wfSnapshot = &model.Workflow{
			Name:  pw.Name,
			Steps: append([]model.WorkflowStep(nil), pw.Steps[in.StepFrom:in.StepTo+1]...),
		}
		wfRef = &model.WorkflowRef{
			WorkflowIndex: wfIdx,
			WorkflowName:  pw.Name,
			StepFrom:      in.StepFrom,
			StepTo:        in.StepTo,
		}
	} else {
		wfSnapshot = workflowSnapshot(in.Workflow)
	}

	task := &model.TaskDetail{
		ID:            newID(),
		UserID:        userID,
		OrgID:         s.personalOrgOfUnsafe(userID),
		ProjectID:     project.ID,
		Title:         in.Title,
		Description:   in.Description,
		Status:        "pending",
		Priority:      priority,
		SourceTaskID:  in.SourceTaskID,
		Workflow:      wfSnapshot,
		WorkflowRef:   wfRef,
		PMAgentID:     "",
		PMAgent:       model.PMAgentSummary{},
		Todos:         []model.Todo{todo},
		AttachedFiles: s.resolveAttachedFilesUnsafe(in.FileIDs, project.ID),

		Result: model.TaskResult{
			Summary:     "",
			FinalOutput: "",
			Metadata:    map[string]any{},
		},
		Version:      1,
		CanceledAt:   nil,
		CanceledBy:   nil,
		CancelReason: nil,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	s.tasks[task.ID] = task
	s.projectTasks[task.ProjectID] = append(s.projectTasks[task.ProjectID], task.ID)

	taskTitle := task.Title
	s.addEventUnsafe(userID, project.ID, task.ID, "", "user", userID, "", "task_created", &taskTitle, map[string]any{"task_title": task.Title}, now)

	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)

	return s.copyTaskWithArtifactsUnsafe(task), nil
}

func (s *Store) recordTodoDispatchUnsafe(task *model.TaskDetail, todo *model.Todo, actorType, actorID, actorName string, message *string, metadata map[string]any, now time.Time) {
	metadata["task_title"] = task.Title
	metadata["todo_title"] = todo.Title
	s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, todo.ID, actorType, actorID, actorName, "todo_assigned", message, metadata, now)
	task.UpdatedAt = now
	task.Version++
}

func normalizeTaskCreateTodos(in []TaskCreateTodoInput) ([]TaskCreateTodoInput, *transport.AppError) {
	out := make([]TaskCreateTodoInput, len(in))
	copy(out, in)

	seenOrders := make(map[int]struct{}, len(out))
	for i := range out {
		order := out[i].Order
		if order == 0 {
			order = i + 1
		}
		if order <= 0 {
			return nil, transport.Validation("invalid todo order", map[string]any{"todo_index": i, "order": "must be greater than zero"})
		}
		if _, exists := seenOrders[order]; exists {
			return nil, transport.Validation("duplicated todo order", map[string]any{"todo_index": i, "order": order})
		}
		seenOrders[order] = struct{}{}
		out[i].Order = order
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Order < out[j].Order
	})
	return out, nil
}

// resolveAttachedFilesUnsafe looks up file metadata for the given file IDs.
// Caller must hold s.mu.
func (s *Store) resolveAttachedFilesUnsafe(fileIDs []string, projectID string) []model.TaskAttachedFile {
	if len(fileIDs) == 0 {
		return nil
	}
	out := make([]model.TaskAttachedFile, 0, len(fileIDs))
	for _, fid := range fileIDs {
		f, ok := s.projectFiles[fid]
		if !ok || f.ProjectID != projectID || f.IsFolder {
			continue
		}
		out = append(out, model.TaskAttachedFile{
			ID:       f.ID,
			FileName: f.FileName,
			FileSize: f.FileSize,
			MimeType: f.MimeType,
			Source:   f.Source,
		})
	}
	return out
}

func hasIncompletePredecessor(task *model.TaskDetail, todoIdx int) bool {
	if task == nil || todoIdx <= 0 {
		return false
	}
	for i := 0; i < todoIdx; i++ {
		if task.Todos[i].Status != "done" {
			return true
		}
	}
	return false
}

func (s *Store) UpdateTodoProgressByNode(nodeID string, in TodoProgressInput) (*model.TaskDetail, *transport.AppError) {
	in.TaskID = strings.TrimSpace(in.TaskID)
	in.TodoID = strings.TrimSpace(in.TodoID)
	in.Message = strings.TrimSpace(in.Message)
	if in.TaskID == "" || in.TodoID == "" || in.Message == "" {
		return nil, transport.Validation("invalid todo.progress payload", map[string]any{"task_id": "required", "todo_id": "required", "message": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	agent, err := s.agentByNodeUnsafe(nodeID)
	if err != nil {
		return nil, err
	}
	task, ok := s.tasks[in.TaskID]
	if !ok || task.UserID != agent.UserID {
		return nil, transport.NotFound("task not found")
	}
	if appErr := s.ensureTaskProjectActiveUnsafe(task); appErr != nil {
		return nil, appErr
	}

	todoIdx := findTodoIndex(task, in.TodoID)
	if todoIdx < 0 {
		return nil, transport.NotFound("todo not found")
	}
	todo := &task.Todos[todoIdx]
	if appErr := ensureTaskAcceptingUpdates(task); appErr != nil {
		return nil, appErr
	}
	if appErr := ensureTodoAcceptingUpdates(todo); appErr != nil {
		return nil, appErr
	}
	if todo.Assignee.AgentID != agent.ID {
		return nil, transport.Forbidden("todo is not assigned to this agent")
	}
	if todo.Status == "done" || todo.Status == "failed" {
		return nil, transport.Conflict("TODO_FINALIZED", "todo already finalized")
	}
	if hasIncompletePredecessor(task, todoIdx) {
		return nil, transport.Conflict("TODO_BLOCKED_BY_PREVIOUS", "todo is blocked by previous todos")
	}

	now := time.Now().UTC()
	s.markAgentSeenUnsafe(agent.ID, now)
	if todo.Status == "pending" {
		todo.Status = "in_progress"
		todo.StartedAt = &now
		todo.AssignedAt = &now
		todo.CanceledAt = nil
		todo.CancelReason = nil
		started := fmt.Sprintf("todo started: %s", todo.Title)
		s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, todo.ID, "agent", agent.ID, agent.Name, "todo_started", &started, map[string]any{"todo_id": todo.ID, "task_title": task.Title, "todo_title": todo.Title}, now)
	}
	// Any progress report keeps the todo alive: the timeout monitor uses
	// LastActivityAt so long-running tasks that keep reporting are not
	// spuriously reset/retried.
	todo.LastActivityAt = &now
	todo.RemindCount = 0
	todo.RemindAt = nil
	progress := in.Message
	s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, todo.ID, "agent", agent.ID, agent.Name, "todo_progress", &progress, map[string]any{"todo_id": todo.ID, "task_title": task.Title, "todo_title": todo.Title}, now)

	s.updateTaskStatusUnsafe(task, now)
	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	s.refreshAgentExecutionStatusUnsafe(agent.ID, now)
	if err := s.persistAgentGraphUnsafe(agent.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)
	return s.copyTaskWithArtifactsUnsafe(task), nil
}

func (s *Store) CompleteTodoByNode(nodeID string, in TodoCompleteInput) (*model.TaskDetail, *model.Todo, *transport.AppError) {
	return s.CompleteTodoByNodeWithMessageID(nodeID, "", in)
}

func (s *Store) CompleteTodoByNodeWithMessageID(nodeID, messageID string, in TodoCompleteInput) (*model.TaskDetail, *model.Todo, *transport.AppError) {
	in.TaskID = strings.TrimSpace(in.TaskID)
	in.TodoID = strings.TrimSpace(in.TodoID)
	if in.TaskID == "" || in.TodoID == "" {
		return nil, nil, transport.Validation("invalid todo.complete payload", map[string]any{"task_id": "required", "todo_id": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if task, ok := s.findProcessedTaskUnsafe(processedMessageKey("todo.complete", nodeID, messageID)); ok {
		return task, nil, nil
	}

	agent, err := s.agentByNodeUnsafe(nodeID)
	if err != nil {
		return nil, nil, err
	}
	task, ok := s.tasks[in.TaskID]
	if !ok || task.UserID != agent.UserID {
		return nil, nil, transport.NotFound("task not found")
	}
	if appErr := s.ensureTaskProjectActiveUnsafe(task); appErr != nil {
		return nil, nil, appErr
	}
	todoIdx := findTodoIndex(task, in.TodoID)
	if todoIdx < 0 {
		return nil, nil, transport.NotFound("todo not found")
	}
	todo := &task.Todos[todoIdx]
	if appErr := ensureTaskAcceptingUpdates(task); appErr != nil {
		return nil, nil, appErr
	}
	if appErr := ensureTodoAcceptingUpdates(todo); appErr != nil {
		return nil, nil, appErr
	}
	if todo.Assignee.AgentID != agent.ID {
		return nil, nil, transport.Forbidden("todo is not assigned to this agent")
	}
	if todo.Status == "done" {
		return nil, nil, transport.Conflict("TODO_ALREADY_DONE", "todo already done")
	}
	if todo.Status == "failed" {
		return nil, nil, transport.Conflict("TODO_ALREADY_FAILED", "todo already failed")
	}
	if hasIncompletePredecessor(task, todoIdx) {
		return nil, nil, transport.Conflict("TODO_BLOCKED_BY_PREVIOUS", "todo is blocked by previous todos")
	}

	now := time.Now().UTC()
	s.markAgentSeenUnsafe(agent.ID, now)
	if todo.StartedAt == nil {
		todo.StartedAt = &now
	}
	todo.Status = "done"
	todo.CompletedAt = &now
	todo.LastActivityAt = &now
	todo.RemindCount = 0
	todo.RemindAt = nil
	todo.FailedAt = nil
	todo.CanceledAt = nil
	todo.Error = nil
	todo.CancelReason = nil
	todo.Result = model.TodoResult{
		Summary:     strings.TrimSpace(in.Result.Summary),
		Output:      strings.TrimSpace(in.Result.Output),
		Metadata:    copyMap(in.Result.Metadata),
		ActionItems: normalizeActionItems(in.Result.ActionItems, now),
	}
	// Surface the agent's returned deliverable in the task conversation.
	// The result text is already persisted on todo.Result, but the user only
	// sees task.messages (user + pm_agent roles) — so execution output was
	// invisible and the task looked like it "completed with no deliverable".
	// Appending it here makes the returned content show up in the thread.
	deliverableContent := todo.Result.Output
	if strings.TrimSpace(deliverableContent) == "" {
		deliverableContent = todo.Result.Summary
	}
	if strings.TrimSpace(deliverableContent) != "" {
		task.Messages = append(task.Messages, model.TaskMessage{
			ID:        uuid.NewString(),
			Role:      "agent",
			Content:   fmt.Sprintf("**%s** 完成了「%s」：\n\n%s", agent.Name, todo.Title, deliverableContent),
			CreatedAt: now,
		})
	}
	completed := fmt.Sprintf("todo completed: %s", todo.Title)
	s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, todo.ID, "agent", agent.ID, agent.Name, "todo_completed", &completed, map[string]any{"todo_id": todo.ID, "task_title": task.Title, "todo_title": todo.Title}, now)

	// ── Review gate / rework trigger ──────────────────────────────
	// 1) ReturnPrevious: this todo is a review-style todo whose verdict is
	//    that its predecessor (order-1) failed. Cascade-reset the predecessor
	//    and every later todo (including this one) back to pending, then the
	//    predecessor is re-dispatched and the chain re-runs.
	var reworked *model.Todo
	if in.ReturnPrevious {
		reason := strings.TrimSpace(in.ReworkReason)
		if reason == "" {
			reason = "数字员工判定前序产出不合格，退回重做"
		}
		var reworkErr *transport.AppError
		// Agent review-style todo: its verdict audits the predecessor.
		reworked, reworkErr = s.triggerReworkUnsafe(task, todoIdx-1, reason)
		if reworkErr != nil {
			return nil, nil, reworkErr
		}
	} else if in.NeedReview || todo.NeedReview {
		// 2) NeedReview: completed todo awaits human/PM approval. Status stays
		//    "done" (terminal execution state) but the review gate blocks
		//    dispatch of later todos until approved.
		//    todo.NeedReview is inherited from the workflow step at plan time.
		todo.ReviewStatus = model.ReviewPending
		msg := fmt.Sprintf("todo awaiting review: %s", todo.Title)
		s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, todo.ID, "agent", agent.ID, agent.Name, "todo_awaiting_review", &msg, map[string]any{"todo_id": todo.ID, "task_title": task.Title, "todo_title": todo.Title}, now)
	} else {
		// 3) Plain completion: ensure any stale review gate is cleared.
		todo.ReviewStatus = ""
		todo.ReviewReason = nil
	}

	s.updateTaskStatusUnsafe(task, now)
	s.rememberProcessedMessageUnsafe(processedMessageKey("todo.complete", nodeID, messageID), "todo.complete", task.ID)
	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, nil, mongoWriteError(err)
	}
	s.refreshAgentExecutionStatusUnsafe(agent.ID, now)
	if err := s.persistAgentGraphUnsafe(agent.ID); err != nil {
		return nil, nil, mongoWriteError(err)
	}
	if err := s.persistProcessedMessageUnsafe(processedMessageKey("todo.complete", nodeID, messageID)); err != nil {
		return nil, nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)
	return s.copyTaskWithArtifactsUnsafe(task), reworked, nil
}

func (s *Store) FailTodoByNode(nodeID string, in TodoFailInput) (*model.TaskDetail, *transport.AppError) {
	return s.FailTodoByNodeWithMessageID(nodeID, "", in)
}

// AskTodoByNode records a todo.ask from the assignee agent and parks the todo
// in waiting_user until the user answers (or, for non-required questions,
// until the timeout auto-resumes it).
func (s *Store) AskTodoByNode(nodeID string, in TodoAskInput) (*model.TaskDetail, *model.TodoQuestion, *transport.AppError) {
	in.TaskID = strings.TrimSpace(in.TaskID)
	in.TodoID = strings.TrimSpace(in.TodoID)
	in.QuestionID = strings.TrimSpace(in.QuestionID)
	in.Question = strings.TrimSpace(in.Question)
	if in.TaskID == "" || in.TodoID == "" || in.Question == "" {
		return nil, nil, transport.Validation("invalid todo.ask payload", map[string]any{"task_id": "required", "todo_id": "required", "question": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	agent, err := s.agentByNodeUnsafe(nodeID)
	if err != nil {
		return nil, nil, err
	}
	task, ok := s.tasks[in.TaskID]
	if !ok || task.UserID != agent.UserID {
		return nil, nil, transport.NotFound("task not found")
	}
	if appErr := s.ensureTaskProjectActiveUnsafe(task); appErr != nil {
		return nil, nil, appErr
	}
	todoIdx := findTodoIndex(task, in.TodoID)
	if todoIdx < 0 {
		return nil, nil, transport.NotFound("todo not found")
	}
	todo := &task.Todos[todoIdx]
	if appErr := ensureTaskAcceptingUpdates(task); appErr != nil {
		return nil, nil, appErr
	}
	if appErr := ensureTodoAcceptingUpdates(todo); appErr != nil {
		return nil, nil, appErr
	}
	if todo.Assignee.AgentID != agent.ID {
		return nil, nil, transport.Forbidden("todo is not assigned to this agent")
	}
	if todo.Status != "in_progress" && todo.Status != "dispatched" {
		return nil, nil, transport.Conflict("TODO_NOT_ACTIVE", "todo is not active")
	}

	now := time.Now().UTC()
	s.markAgentSeenUnsafe(agent.ID, now)

	qid := in.QuestionID
	if qid == "" {
		qid = "q_" + newID()
	}
	q := model.TodoQuestion{
		ID:       qid,
		Question: in.Question,
		Options:  in.Options,
		Required: in.Required,
		AskedAt:  now,
	}
	todo.Questions = append(todo.Questions, q)
	todo.Status = model.TodoStatusWaitingUser
	todo.LastActivityAt = &now
	todo.RemindCount = 0
	todo.RemindAt = nil

	msg := fmt.Sprintf("agent asked user: %s", q.Question)
	s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, todo.ID, "agent", agent.ID, agent.Name, "todo_ask_received", &msg, map[string]any{
		"question_id": q.ID,
		"todo_id":     todo.ID,
		"question":    q.Question,
		"options":     q.Options,
		"required":    q.Required,
		"task_title":  task.Title,
		"todo_title":  todo.Title,
	}, now)

	s.updateTaskStatusUnsafe(task, now)
	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, nil, mongoWriteError(err)
	}
	s.refreshAgentExecutionStatusUnsafe(agent.ID, now)
	if err := s.persistAgentGraphUnsafe(agent.ID); err != nil {
		return nil, nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)
	return s.copyTaskWithArtifactsUnsafe(task), &q, nil
}

// AnswerTodo records a user's (or timeout's) answer to a parked question and
// resumes the todo back to in_progress. Returns the answered question so the
// caller can forward todo.answer to the assignee agent.
func (s *Store) AnswerTodo(userID, taskID, todoID, questionID, answer, answeredBy string, timedOut bool) (*model.TaskDetail, *model.TodoQuestion, *transport.AppError) {
	taskID = strings.TrimSpace(taskID)
	todoID = strings.TrimSpace(todoID)
	questionID = strings.TrimSpace(questionID)
	answer = strings.TrimSpace(answer)
	if taskID == "" || todoID == "" || questionID == "" {
		return nil, nil, transport.Validation("invalid answer payload", map[string]any{"task_id": "required", "todo_id": "required", "question_id": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return nil, nil, transport.NotFound("task not found")
	}
	if appErr := s.ensureTaskProjectActiveUnsafe(task); appErr != nil {
		return nil, nil, appErr
	}
	todoIdx := findTodoIndex(task, todoID)
	if todoIdx < 0 {
		return nil, nil, transport.NotFound("todo not found")
	}
	todo := &task.Todos[todoIdx]
	qIdx := -1
	for i := range todo.Questions {
		if todo.Questions[i].ID == questionID {
			qIdx = i
			break
		}
	}
	if qIdx < 0 {
		return nil, nil, transport.NotFound("question not found")
	}
	q := &todo.Questions[qIdx]
	if q.Answer != "" {
		return nil, nil, transport.Conflict("QUESTION_ALREADY_ANSWERED", "question already answered")
	}

	now := time.Now().UTC()
	q.Answer = answer
	q.AnsweredBy = answeredBy
	q.AnsweredAt = &now
	q.TimedOut = timedOut
	// Resume the parked todo.
	if todo.Status == model.TodoStatusWaitingUser {
		todo.Status = "in_progress"
	}
	todo.LastActivityAt = &now
	todo.RemindCount = 0
	todo.RemindAt = nil

	source := "user"
	if timedOut {
		source = "system"
	}
	msg := fmt.Sprintf("question answered: %s", q.Question)
	s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, todo.ID, source, userID, answeredBy, "todo_answer_received", &msg, map[string]any{"question_id": q.ID, "todo_id": todo.ID, "task_title": task.Title, "todo_title": todo.Title}, now)

	// Mark the original ask event answered so the timeline renders a readonly
	// state after refresh (the ask event itself stays as history).
	for i := range s.taskEvents[task.ID] {
		ev := &s.taskEvents[task.ID][i]
		if ev.EventType != "todo_ask_received" {
			continue
		}
		qid, _ := ev.Metadata["question_id"].(string)
		if qid != q.ID {
			continue
		}
		if ev.Metadata == nil {
			ev.Metadata = map[string]any{}
		}
		ev.Metadata["answer"] = answer
		ev.Metadata["answered_by"] = answeredBy
		ev.Metadata["answered_at"] = now
		ev.Metadata["timed_out"] = timedOut
		break
	}

	s.updateTaskStatusUnsafe(task, now)
	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)
	return s.copyTaskWithArtifactsUnsafe(task), q, nil
}

// PendingTimeoutAnswer identifies a non-required question whose timeout has
// elapsed, ready for the supervisor to auto-resume.
type PendingTimeoutAnswer struct {
	UserID     string
	TaskID     string
	TodoID     string
	QuestionID string
}

// ListTimedOutQuestions scans all tasks for waiting_user todos whose first
// unanswered non-required question has exceeded its timeout. Read-only; the
// supervisor calls AnswerTodo(timedOut=true) for each returned item.
func (s *Store) ListTimedOutQuestions(now time.Time, timeout time.Duration) []PendingTimeoutAnswer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []PendingTimeoutAnswer
	for _, task := range s.tasks {
		for i := range task.Todos {
			todo := &task.Todos[i]
			if todo.Status != model.TodoStatusWaitingUser {
				continue
			}
			for j := range todo.Questions {
				q := &todo.Questions[j]
				if q.Answer != "" || q.Required {
					continue
				}
				if now.Sub(q.AskedAt) >= timeout {
					out = append(out, PendingTimeoutAnswer{
						UserID:     task.UserID,
						TaskID:     task.ID,
						TodoID:     todo.ID,
						QuestionID: q.ID,
					})
				}
				break // only the first unanswered question gates the todo
			}
		}
	}
	return out
}

// ResolveActiveTodoForNode returns the ID of the in-progress (or dispatched) todo assigned to
// nodeID within the given task. This tolerates external ClawSynapse agents that omit todo_id in
// their todo.complete / todo.progress / todo.fail webhook payloads (only sending task_id).
func (s *Store) ResolveActiveTodoForNode(taskID, nodeID string) (string, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	task, ok := s.tasks[taskID]
	if !ok {
		return "", transport.NotFound("task not found")
	}
	for i := range task.Todos {
		t := &task.Todos[i]
		if t.Assignee.NodeID == nodeID && (t.Status == "in_progress" || t.Status == "dispatched") {
			return t.ID, nil
		}
	}
	return "", nil
}

func (s *Store) FailTodoByNodeWithMessageID(nodeID, messageID string, in TodoFailInput) (*model.TaskDetail, *transport.AppError) {
	in.TaskID = strings.TrimSpace(in.TaskID)
	in.TodoID = strings.TrimSpace(in.TodoID)
	in.Error = strings.TrimSpace(in.Error)
	if in.TaskID == "" || in.TodoID == "" || in.Error == "" {
		return nil, transport.Validation("invalid todo.fail payload", map[string]any{"task_id": "required", "todo_id": "required", "error": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if task, ok := s.findProcessedTaskUnsafe(processedMessageKey("todo.fail", nodeID, messageID)); ok {
		return task, nil
	}

	agent, err := s.agentByNodeUnsafe(nodeID)
	if err != nil {
		return nil, err
	}
	task, ok := s.tasks[in.TaskID]
	if !ok || task.UserID != agent.UserID {
		return nil, transport.NotFound("task not found")
	}
	if appErr := s.ensureTaskProjectActiveUnsafe(task); appErr != nil {
		return nil, appErr
	}
	todoIdx := findTodoIndex(task, in.TodoID)
	if todoIdx < 0 {
		return nil, transport.NotFound("todo not found")
	}
	todo := &task.Todos[todoIdx]
	if appErr := ensureTaskAcceptingUpdates(task); appErr != nil {
		return nil, appErr
	}
	if appErr := ensureTodoAcceptingUpdates(todo); appErr != nil {
		return nil, appErr
	}
	if todo.Assignee.AgentID != agent.ID {
		return nil, transport.Forbidden("todo is not assigned to this agent")
	}
	if todo.Status == "done" {
		return nil, transport.Conflict("TODO_ALREADY_DONE", "todo already done")
	}
	if todo.Status == "failed" {
		return nil, transport.Conflict("TODO_ALREADY_FAILED", "todo already failed")
	}
	if hasIncompletePredecessor(task, todoIdx) {
		return nil, transport.Conflict("TODO_BLOCKED_BY_PREVIOUS", "todo is blocked by previous todos")
	}

	now := time.Now().UTC()
	s.markAgentSeenUnsafe(agent.ID, now)
	if todo.StartedAt == nil {
		todo.StartedAt = &now
	}
	todo.Status = "failed"
	todo.CompletedAt = nil
	todo.FailedAt = &now
	todo.LastActivityAt = &now
	todo.RemindCount = 0
	todo.RemindAt = nil
	todo.CanceledAt = nil
	errCopy := in.Error
	todo.Error = &errCopy
	todo.CancelReason = nil
	failed := fmt.Sprintf("todo failed: %s", todo.Title)
	s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, todo.ID, "agent", agent.ID, agent.Name, "todo_failed", &failed, map[string]any{"todo_id": todo.ID, "error": in.Error, "task_title": task.Title, "todo_title": todo.Title}, now)

	s.updateTaskStatusUnsafe(task, now)
	s.rememberProcessedMessageUnsafe(processedMessageKey("todo.fail", nodeID, messageID), "todo.fail", task.ID)
	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	s.refreshAgentExecutionStatusUnsafe(agent.ID, now)
	if err := s.persistAgentGraphUnsafe(agent.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	if err := s.persistProcessedMessageUnsafe(processedMessageKey("todo.fail", nodeID, messageID)); err != nil {
		return nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)
	return s.copyTaskWithArtifactsUnsafe(task), nil
}

func (s *Store) AddTaskCommentByNode(nodeID string, in TaskCommentInput) (*model.Comment, *transport.AppError) {
	in.TaskID = strings.TrimSpace(in.TaskID)
	in.TodoID = strings.TrimSpace(in.TodoID)
	in.Content = strings.TrimSpace(in.Content)
	if in.TaskID == "" || in.Content == "" {
		return nil, transport.Validation("invalid task.comment payload", map[string]any{"task_id": "required", "content": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	agent, err := s.agentByNodeUnsafe(nodeID)
	if err != nil {
		return nil, err
	}
	task, ok := s.tasks[in.TaskID]
	if !ok || task.UserID != agent.UserID {
		return nil, transport.NotFound("task not found")
	}
	if in.TodoID != "" {
		if findTodoIndex(task, in.TodoID) < 0 {
			return nil, transport.NotFound("todo not found")
		}
	}
	mentions, appErr := s.normalizeTaskCommentMentionsUnsafe(task, in.Mentions)
	if appErr != nil {
		return nil, appErr
	}

	now := time.Now().UTC()
	s.markAgentSeenUnsafe(agent.ID, now)
	comment := s.addCommentUnsafe(task, in.TodoID, "agent", agent.ID, agent.Name, in.Content, mentions, now)
	// 执行者的任何评论上报都是活跃信号：重置其名下 in_progress todo 的
	// 超时催办计数（LastActivityAt/RemindCount/RemindAt）。
	// 否则 persona 型执行者（不走 todo.progress 而用 task.comment 汇报）
	// 会在持续正常工作的同时被超时监控累计 3 次提醒误判为失败。
	// 与 UpdateTodoProgress 的保活语义对齐（见该函数内注释）。
	for i := range task.Todos {
		td := &task.Todos[i]
		if td.Status != "in_progress" || td.Assignee.AgentID != agent.ID {
			continue
		}
		td.LastActivityAt = &now
		td.RemindCount = 0
		td.RemindAt = nil
	}
	if err := s.persistCommentUnsafe(comment); err != nil {
		return nil, mongoWriteError(err)
	}
	_ = s.persistTaskEventsUnsafe(task.ID)
	s.publishTaskUnsafe(task.ID)
	return comment, nil
}

// AppendSystemTaskComment records a platform-generated system comment (e.g.
// workflow-plan validation failures) so agent-side failures become visible in
// the UI. Without this, a rejected task.plan_ready is only reported back to
// the PM agent — which may misreport success, leaving the user misled while
// the task stays stuck in planning.
func (s *Store) AppendSystemTaskComment(taskID, content string) (*model.Comment, *transport.AppError) {
	taskID = strings.TrimSpace(taskID)
	content = strings.TrimSpace(content)
	if taskID == "" || content == "" {
		return nil, transport.Validation("invalid system comment payload", map[string]any{"task_id": "required", "content": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return nil, transport.NotFound("task not found")
	}

	now := time.Now().UTC()
	comment := s.addCommentUnsafe(task, "", "system", "system", "系统", content, nil, now)
	if err := s.persistCommentUnsafe(comment); err != nil {
		return nil, mongoWriteError(err)
	}
	_ = s.persistTaskEventsUnsafe(taskID)
	s.publishTaskUnsafe(taskID)
	return comment, nil
}

func (s *Store) CancelTask(userID string, in TaskCancelInput) (*model.TaskDetail, *transport.AppError) {
	in.TaskID = strings.TrimSpace(in.TaskID)
	in.Reason = strings.TrimSpace(in.Reason)
	if in.TaskID == "" {
		return nil, transport.Validation("invalid cancel payload", map[string]any{"task_id": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[in.TaskID]
	if !ok || task.UserID != userID {
		return nil, transport.NotFound("task not found")
	}
	if appErr := s.ensureTaskProjectActiveUnsafe(task); appErr != nil {
		return nil, appErr
	}

	switch task.Status {
	case "canceled":
		return nil, transport.Conflict("TASK_ALREADY_CANCELED", "task already canceled")
	case "done", "failed":
		return nil, transport.Conflict("TASK_ALREADY_TERMINAL", "task already finalized")
	}

	now := time.Now().UTC()
	userName := ""
	if u, ok := s.users[userID]; ok {
		userName = u.Name
	}

	affectedAgents := s.cancelTaskUnsafe(task, "user", userID, userName, in.Reason, now)
	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	for agentID := range affectedAgents {
		s.refreshAgentExecutionStatusUnsafe(agentID, now)
		if err := s.persistAgentGraphUnsafe(agentID); err != nil {
			return nil, mongoWriteError(err)
		}
	}
	s.publishTaskUnsafe(task.ID)
	return s.copyTaskWithArtifactsUnsafe(task), nil
}

func (s *Store) AddTaskComment(userID, taskID string, in TaskCommentInput) (*model.Comment, *transport.AppError) {
	taskID = strings.TrimSpace(taskID)
	in.Content = strings.TrimSpace(in.Content)
	in.TodoID = strings.TrimSpace(in.TodoID)
	if taskID == "" || in.Content == "" {
		return nil, transport.Validation("invalid comment payload", map[string]any{"task_id": "required", "content": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return nil, transport.NotFound("task not found")
	}
	if in.TodoID != "" {
		if findTodoIndex(task, in.TodoID) < 0 {
			return nil, transport.NotFound("todo not found")
		}
	}
	mentions, appErr := s.normalizeTaskCommentMentionsUnsafe(task, in.Mentions)
	if appErr != nil {
		return nil, appErr
	}

	now := time.Now().UTC()
	userName := ""
	if u, ok := s.users[userID]; ok {
		userName = u.Name
	}
	comment := s.addCommentUnsafe(task, in.TodoID, "user", userID, userName, in.Content, mentions, now)
	if err := s.persistCommentUnsafe(comment); err != nil {
		return nil, mongoWriteError(err)
	}
	_ = s.persistTaskEventsUnsafe(task.ID)
	s.publishTaskUnsafe(task.ID)
	return comment, nil
}

func (s *Store) ListTaskComments(userID, taskID string) ([]model.Comment, *transport.AppError) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, transport.Validation("task_id required", nil)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return nil, transport.NotFound("task not found")
	}
	comments := s.taskComments[taskID]
	out := make([]model.Comment, len(comments))
	copy(out, comments)
	return out, nil
}

func (s *Store) addCommentUnsafe(task *model.TaskDetail, todoID, actorType, actorID, actorName, content string, mentions []model.CommentMention, at time.Time) *model.Comment {
	comment := &model.Comment{
		ID:        newID(),
		UserID:    task.UserID,
		OrgID:     s.personalOrgOfUnsafe(task.UserID),
		TaskID:    task.ID,
		TodoID:    todoID,
		ActorType: actorType,
		ActorID:   actorID,
		ActorName: actorName,
		Content:   content,
		Mentions:  append([]model.CommentMention(nil), mentions...),
		CreatedAt: at,
	}
	s.taskComments[task.ID] = append(s.taskComments[task.ID], *comment)
	s.publishUserEventUnsafe(task.UserID, "task.comment.created", map[string]any{
		"task_id":    task.ID,
		"project_id": task.ProjectID,
		"comment":    *comment,
	}, at)

	metadata := map[string]any{"comment_id": comment.ID, "task_title": task.Title}
	if todoID != "" {
		metadata["todo_id"] = todoID
		for i := range task.Todos {
			if task.Todos[i].ID == todoID {
				metadata["todo_title"] = task.Todos[i].Title
				break
			}
		}
	}
	if len(comment.Mentions) > 0 {
		metadata["mentions"] = comment.Mentions
	}
	s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, todoID, actorType, actorID, actorName, "task_comment", &content, metadata, at)

	task.UpdatedAt = at
	task.Version++
	return comment
}

func (s *Store) normalizeTaskCommentMentionsUnsafe(task *model.TaskDetail, inputs []TaskCommentMentionInput) ([]model.CommentMention, *transport.AppError) {
	if len(inputs) == 0 {
		return nil, nil
	}

	targets := s.taskCommentMentionTargetsUnsafe(task)
	seen := make(map[string]struct{}, len(inputs))
	mentions := make([]model.CommentMention, 0, len(inputs))
	for i, input := range inputs {
		agentID := strings.TrimSpace(input.AgentID)
		if agentID == "" {
			return nil, transport.Validation("invalid mentions", map[string]any{
				fmt.Sprintf("mentions.%d.agent_id", i): "required",
			})
		}
		mention, ok := targets[agentID]
		if !ok {
			return nil, transport.Validation("invalid mentions", map[string]any{
				fmt.Sprintf("mentions.%d.agent_id", i): "must reference task participant",
			})
		}
		if _, exists := seen[agentID]; exists {
			continue
		}
		seen[agentID] = struct{}{}
		mentions = append(mentions, mention)
	}
	return mentions, nil
}

func (s *Store) taskCommentMentionTargetsUnsafe(task *model.TaskDetail) map[string]model.CommentMention {
	targets := make(map[string]model.CommentMention, len(task.Todos)+1)
	if task.PMAgent.ID != "" {
		targets[task.PMAgent.ID] = model.CommentMention{
			AgentID:   task.PMAgent.ID,
			AgentName: task.PMAgent.Name,
			NodeID:    task.PMAgent.NodeID,
			Role:      "pm",
		}
	}
	for _, todo := range task.Todos {
		agentID := strings.TrimSpace(todo.Assignee.AgentID)
		if agentID == "" {
			continue
		}
		if _, exists := targets[agentID]; exists {
			continue
		}
		targets[agentID] = model.CommentMention{
			AgentID:   agentID,
			AgentName: todo.Assignee.Name,
			NodeID:    todo.Assignee.NodeID,
			Role:      "executor",
		}
	}
	return targets
}

func (s *Store) updateTaskStatusUnsafe(task *model.TaskDetail, now time.Time) {
	if task.Status == "canceled" {
		task.Result = aggregateTaskResult(task.Todos, task.Status)
		task.UpdatedAt = now
		task.Version++
		return
	}
	prev := task.Status
	next := aggregateTaskStatus(*task)
	result := aggregateTaskResult(task.Todos, next)
	task.Status = next
	task.Result = result
	task.UpdatedAt = now
	task.Version++
	if prev != next {
		msg := fmt.Sprintf("task status changed: %s -> %s", prev, next)
		s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, "", "system", "system", "System", "task_status_changed", &msg, map[string]any{"from": prev, "to": next, "task_title": task.Title}, now)
	}
}

func aggregateTaskStatus(task model.TaskDetail) string {
	if len(task.Todos) == 0 {
		return "pending"
	}
	allDone := true
	hasWork := false
	hasAwaitingReview := false
	hasWaitingUser := false
	for _, todo := range task.Todos {
		switch todo.Status {
		case "failed":
			return "failed"
		case "done":
			hasWork = true
			if todo.ReviewStatus == model.ReviewPending {
				hasAwaitingReview = true
			} else if todo.ReviewStatus == model.ReviewRejected {
				// Defense: a rejected todo must never aggregate the task to
				// done, even if the rework cascade was somehow skipped.
				allDone = false
			}
		case "in_progress":
			allDone = false
			hasWork = true
		case "canceled":
			allDone = false
		case model.TodoStatusWaitingUser:
			allDone = false
			hasWork = true
			hasWaitingUser = true
		default:
			allDone = false
		}
	}
	// A parked todo takes priority: the user should address it before the
	// pipeline proceeds (even over review-gated todos).
	if hasWaitingUser {
		return model.TaskStatusWaitingUser
	}
	if allDone && !hasAwaitingReview {
		return "done"
	}
	if hasAwaitingReview {
		return model.TaskStatusAwaitingReview
	}
	if hasWork {
		return "in_progress"
	}
	return "pending"
}

func aggregateTaskResult(todos []model.Todo, status string) model.TaskResult {
	total := len(todos)
	doneCount := 0
	failedCount := 0
	inProgressCount := 0
	pendingCount := 0
	canceledCount := 0
	completedSummaries := make([]string, 0)
	completedOutputs := make([]string, 0)
	failedMessages := make([]string, 0)

	for _, todo := range todos {
		switch todo.Status {
		case "done":
			doneCount++
			if summary := strings.TrimSpace(todo.Result.Summary); summary != "" {
				completedSummaries = append(completedSummaries, fmt.Sprintf("%s: %s", todo.Title, summary))
			}
			if output := strings.TrimSpace(todo.Result.Output); output != "" {
				completedOutputs = append(completedOutputs, fmt.Sprintf("%s\n%s", todo.Title, output))
			}
		case "failed":
			failedCount++
			if todo.Error != nil && strings.TrimSpace(*todo.Error) != "" {
				failedMessages = append(failedMessages, fmt.Sprintf("%s: %s", todo.Title, strings.TrimSpace(*todo.Error)))
			} else {
				failedMessages = append(failedMessages, fmt.Sprintf("%s: failed", todo.Title))
			}
		case "in_progress":
			inProgressCount++
		case "canceled":
			canceledCount++
		default:
			pendingCount++
		}
	}

	summary := ""
	finalOutput := ""
	switch status {
	case "done":
		if len(completedSummaries) > 0 {
			summary = strings.Join(completedSummaries, "; ")
		} else if total > 0 {
			summary = fmt.Sprintf("All %d todos completed", total)
		}
		if len(completedOutputs) > 0 {
			finalOutput = strings.Join(completedOutputs, "\n\n")
		}
	case "failed":
		if len(failedMessages) > 0 {
			summary = "Task failed: " + strings.Join(failedMessages, "; ")
		} else {
			summary = "Task failed"
		}
		sections := make([]string, 0, 2)
		if len(completedOutputs) > 0 {
			sections = append(sections, strings.Join(completedOutputs, "\n\n"))
		}
		if len(failedMessages) > 0 {
			sections = append(sections, "Failed todos:\n"+strings.Join(failedMessages, "\n"))
		}
		finalOutput = strings.Join(sections, "\n\n")
	case "in_progress":
		summary = fmt.Sprintf("Task in progress: %d/%d completed, %d in progress, %d pending", doneCount, total, inProgressCount, pendingCount)
	case "pending":
		summary = fmt.Sprintf("Task pending: %d todos not started", total)
	case "canceled":
		summary = fmt.Sprintf("Task canceled: %d completed, %d canceled, %d pending before stop", doneCount, canceledCount, pendingCount)
		if len(completedOutputs) > 0 {
			finalOutput = strings.Join(completedOutputs, "\n\n")
		}
	}

	return model.TaskResult{
		Summary:     summary,
		FinalOutput: finalOutput,
		Metadata: map[string]any{
			"status":                 status,
			"todo_count":             total,
			"completed_todo_count":   doneCount,
			"failed_todo_count":      failedCount,
			"in_progress_todo_count": inProgressCount,
			"pending_todo_count":     pendingCount,
			"canceled_todo_count":    canceledCount,
		},
	}
}

func ensureTaskAcceptingUpdates(task *model.TaskDetail) *transport.AppError {
	if task != nil && task.Status == "canceled" {
		return transport.Conflict("TASK_CANCELED", "task has been canceled")
	}
	return nil
}

func ensureTodoAcceptingUpdates(todo *model.Todo) *transport.AppError {
	if todo != nil && todo.Status == "canceled" {
		return transport.Conflict("TODO_CANCELED", "todo has been canceled")
	}
	return nil
}

func (s *Store) cancelTaskUnsafe(task *model.TaskDetail, actorType, actorID, actorName, reason string, now time.Time) map[string]struct{} {
	affectedAgents := make(map[string]struct{})
	prev := task.Status
	reasonPtr := (*string)(nil)
	if reason != "" {
		reasonCopy := reason
		reasonPtr = &reasonCopy
	}
	for i := range task.Todos {
		todo := &task.Todos[i]
		switch todo.Status {
		case "pending", "in_progress":
			s.cancelTodoUnsafe(todo, reason, now)
			if todo.Assignee.AgentID != "" {
				affectedAgents[todo.Assignee.AgentID] = struct{}{}
			}
		}
	}

	task.Status = "canceled"
	task.CanceledAt = &now
	task.CanceledBy = &model.ActorRef{
		ActorType: actorType,
		ActorID:   actorID,
		ActorName: actorName,
	}
	task.CancelReason = reasonPtr
	task.Result = aggregateTaskResult(task.Todos, task.Status)
	task.UpdatedAt = now
	task.Version++

	msg := fmt.Sprintf("task status changed: %s -> canceled", prev)
	if reason != "" {
		msg = fmt.Sprintf("%s (%s)", msg, reason)
	}
	s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, "", actorType, actorID, actorName, "task_status_changed", &msg, map[string]any{
		"from":       prev,
		"to":         "canceled",
		"reason":     reason,
		"task_title": task.Title,
	}, now)
	return affectedAgents
}

func (s *Store) cancelTodoUnsafe(todo *model.Todo, reason string, now time.Time) {
	todo.Status = "canceled"
	todo.CompletedAt = nil
	todo.FailedAt = nil
	todo.CanceledAt = &now
	todo.Error = nil
	if reason == "" {
		todo.CancelReason = nil
		return
	}
	reasonCopy := reason
	todo.CancelReason = &reasonCopy
}

func findTodoIndex(task *model.TaskDetail, todoID string) int {
	for i := range task.Todos {
		if task.Todos[i].ID == todoID {
			return i
		}
	}
	return -1
}

// ─── dynamic todo management ───

type TodoModifyInput struct {
	Title       string
	Description string
	AssigneeID  string // agent ID to assign to
}

func (s *Store) AppendTodo(userID, taskID string, in TodoModifyInput) (*model.TaskDetail, *transport.AppError) {
	taskID = strings.TrimSpace(taskID)
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	in.AssigneeID = strings.TrimSpace(in.AssigneeID)
	if taskID == "" || in.Title == "" || in.AssigneeID == "" {
		return nil, transport.Validation("invalid todo payload", map[string]any{"title": "required", "assignee_id": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return nil, transport.NotFound("task not found")
	}
	// Only allow mutation while task is active
	if err := ensureTaskAcceptingUpdates(task); err != nil {
		return nil, err
	}
	assignee, ok := s.agents[in.AssigneeID]
	if !ok || assignee.UserID != userID {
		return nil, transport.Validation("invalid assignee_id", nil)
	}

	now := time.Now().UTC()
	maxOrder := 0
	for i := range task.Todos {
		if task.Todos[i].Order > maxOrder {
			maxOrder = task.Todos[i].Order
		}
	}

	todo := model.Todo{
		ID:          uuid.NewString(),
		Order:       maxOrder + 1,
		Title:       in.Title,
		Description: in.Description,
		Status:      "pending",
		Assignee: model.TodoAssignee{
			AgentID: assignee.ID,
			Name:    assignee.Name,
			NodeID:  assignee.NodeID,
		},
		Result: model.TodoResult{
			Summary:  "",
			Output:   "",
			Metadata: map[string]any{},
		},
		CreatedAt: now,
	}
	task.Todos = append(task.Todos, todo)
	task.UpdatedAt = now
	task.Version++

	s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, todo.ID, "user", userID, getAgentName(s, userID), "todo_appended", &in.Title, map[string]any{
		"todo_id":    todo.ID,
		"todo_title": todo.Title,
	}, now)

	// Note: ClawSynapse todo.assigned dispatch is handled by the caller (handler layer)
	// The store only persists the task; the caller calls DispatchNextTodo for ClawSynapse message delivery.

	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)
	return s.copyTaskWithArtifactsUnsafe(task), nil
}

func (s *Store) InsertTodo(userID, taskID, beforeTodoID string, in TodoModifyInput) (*model.TaskDetail, *transport.AppError) {
	taskID = strings.TrimSpace(taskID)
	beforeTodoID = strings.TrimSpace(beforeTodoID)
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	in.AssigneeID = strings.TrimSpace(in.AssigneeID)
	if taskID == "" || beforeTodoID == "" || in.Title == "" || in.AssigneeID == "" {
		return nil, transport.Validation("invalid todo payload", map[string]any{"title": "required", "assignee_id": "required", "before_todo_id": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return nil, transport.NotFound("task not found")
	}
	if err := ensureTaskAcceptingUpdates(task); err != nil {
		return nil, err
	}

	beforeIdx := findTodoIndex(task, beforeTodoID)
	if beforeIdx < 0 {
		return nil, transport.NotFound("before_todo not found")
	}

	assignee, ok := s.agents[in.AssigneeID]
	if !ok || assignee.UserID != userID {
		return nil, transport.Validation("invalid assignee_id", nil)
	}

	now := time.Now().UTC()
	todo := model.Todo{
		ID:          uuid.NewString(),
		Order:       0, // will be recalculated
		Title:       in.Title,
		Description: in.Description,
		Status:      "pending",
		Assignee: model.TodoAssignee{
			AgentID: assignee.ID,
			Name:    assignee.Name,
			NodeID:  assignee.NodeID,
		},
		Result: model.TodoResult{
			Summary:  "",
			Output:   "",
			Metadata: map[string]any{},
		},
		CreatedAt: now,
	}

	// Insert at position beforeIdx
	task.Todos = append(task.Todos, model.Todo{}) // grow
	copy(task.Todos[beforeIdx+1:], task.Todos[beforeIdx:])
	task.Todos[beforeIdx] = todo

	// Recalculate Order for all todos
	for i := range task.Todos {
		task.Todos[i].Order = i + 1
	}

	task.UpdatedAt = now
	task.Version++

	s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, todo.ID, "user", userID, getAgentName(s, userID), "todo_appended", &in.Title, map[string]any{
		"todo_id":    todo.ID,
		"todo_title": todo.Title,
	}, now)

	// Note: ClawSynapse todo.assigned dispatch is handled by the caller (handler layer).

	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)
	return s.copyTaskWithArtifactsUnsafe(task), nil
}

func (s *Store) UpdateTodo(userID, taskID, todoID string, in TodoModifyInput) (*model.TaskDetail, *transport.AppError) {
	taskID = strings.TrimSpace(taskID)
	todoID = strings.TrimSpace(todoID)
	if taskID == "" || todoID == "" {
		return nil, transport.Validation("invalid payload", map[string]any{"task_id": "required", "todo_id": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return nil, transport.NotFound("task not found")
	}
	if err := ensureTaskAcceptingUpdates(task); err != nil {
		return nil, err
	}

	todoIdx := findTodoIndex(task, todoID)
	if todoIdx < 0 {
		return nil, transport.NotFound("todo not found")
	}
	todo := &task.Todos[todoIdx]
	if err := ensureTodoAcceptingUpdates(todo); err != nil {
		return nil, err
	}

	now := time.Now().UTC()

	if in.Title != "" {
		todo.Title = strings.TrimSpace(in.Title)
	}
	if in.Description != "" {
		todo.Description = strings.TrimSpace(in.Description)
	}
	if in.AssigneeID != "" {
		assignee, ok := s.agents[in.AssigneeID]
		if !ok || assignee.UserID != userID {
			return nil, transport.Validation("invalid assignee_id", nil)
		}
		todo.Assignee = model.TodoAssignee{
			AgentID: assignee.ID,
			Name:    assignee.Name,
			NodeID:  assignee.NodeID,
		}
	}

	task.UpdatedAt = now
	task.Version++

	s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, todoID, "user", userID, getAgentName(s, userID), "todo_updated", nil, map[string]any{
		"todo_id":    todoID,
		"todo_title": todo.Title,
	}, now)

	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)
	return s.copyTaskWithArtifactsUnsafe(task), nil
}

func (s *Store) RemoveTodo(userID, taskID, todoID string) (*model.TaskDetail, *transport.AppError) {
	taskID = strings.TrimSpace(taskID)
	todoID = strings.TrimSpace(todoID)
	if taskID == "" || todoID == "" {
		return nil, transport.Validation("invalid payload", map[string]any{"task_id": "required", "todo_id": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return nil, transport.NotFound("task not found")
	}
	if err := ensureTaskAcceptingUpdates(task); err != nil {
		return nil, err
	}

	todoIdx := findTodoIndex(task, todoID)
	if todoIdx < 0 {
		return nil, transport.NotFound("todo not found")
	}
	todo := &task.Todos[todoIdx]
	if todo.Status != "pending" {
		return nil, transport.Conflict("TODO_NOT_PENDING", "only pending todos can be removed")
	}

	now := time.Now().UTC()
	todoTitle := todo.Title

	// Remove the todo
	task.Todos = append(task.Todos[:todoIdx], task.Todos[todoIdx+1:]...)

	// Recalculate Order
	for i := range task.Todos {
		task.Todos[i].Order = i + 1
	}

	task.UpdatedAt = now
	task.Version++

	s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, todoID, "user", userID, getAgentName(s, userID), "todo_removed", &todoTitle, map[string]any{
		"todo_id":    todoID,
		"todo_title": todoTitle,
	}, now)

	// Re-aggregate task status after removal
	s.updateTaskStatusUnsafe(task, now)
	// Note: ClawSynapse todo.assigned dispatch is handled by the caller (handler layer).

	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)
	return s.copyTaskWithArtifactsUnsafe(task), nil
}

func (s *Store) ReorderTodos(userID, taskID string, todoIDs []string) (*model.TaskDetail, *transport.AppError) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || len(todoIDs) == 0 {
		return nil, transport.Validation("invalid payload", map[string]any{"task_id": "required", "todo_ids": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return nil, transport.NotFound("task not found")
	}
	if err := ensureTaskAcceptingUpdates(task); err != nil {
		return nil, err
	}

	if len(todoIDs) != len(task.Todos) {
		return nil, transport.Validation("todo_ids count mismatch", map[string]any{"expected": len(task.Todos), "got": len(todoIDs)})
	}

	// Build a lookup
	todoMap := make(map[string]*model.Todo, len(task.Todos))
	for i := range task.Todos {
		todoMap[task.Todos[i].ID] = &task.Todos[i]
	}

	now := time.Now().UTC()
	reordered := make([]model.Todo, 0, len(todoIDs))
	for i, id := range todoIDs {
		todo, exists := todoMap[id]
		if !exists {
			return nil, transport.Validation("unknown todo_id", map[string]any{"todo_id": id})
		}
		todo.Order = i + 1
		reordered = append(reordered, *todo)
	}

	task.Todos = reordered
	task.UpdatedAt = now
	task.Version++

	s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, "", "user", userID, getAgentName(s, userID), "todos_reordered", nil, map[string]any{
		"count": len(todoIDs),
	}, now)

	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)
	return s.copyTaskWithArtifactsUnsafe(task), nil
}

// getAgentName is a helper to safely retrieve a user's display name from the store.
func getAgentName(s *Store, userID string) string {
	if u, ok := s.users[userID]; ok {
		return u.Name
	}
	return ""
}

// ── Review gate & rework ──────────────────────────────────────────

// ReviewTodo is the store entry point for the human (UI) or PM agent
// (todo.review message) approving or rejecting a completed todo that is
// awaiting review.
//   - action "approve": todo.ReviewStatus -> approved; the pipeline unblocks
//     and the next dispatchable todo is dispatched by the caller.
//   - action "reject": cascade-reset the todo and everything from its
//     predecessor onward back to pending, bump rework counters, then
//     re-dispatch the predecessor (the todo being audited).
func (s *Store) ReviewTodo(userID, nodeID, taskID, todoID, action, reason string) (*model.TaskDetail, *model.Todo, *transport.AppError) {
	taskID = strings.TrimSpace(taskID)
	todoID = strings.TrimSpace(todoID)
	action = strings.TrimSpace(action)
	if taskID == "" || todoID == "" {
		return nil, nil, transport.Validation("invalid todo.review payload", map[string]any{"task_id": "required", "todo_id": "required"})
	}
	if action != "approve" && action != "reject" {
		return nil, nil, transport.Validation("invalid todo.review action", map[string]any{"action": "must be approve or reject"})
	}
	if action == "reject" && strings.TrimSpace(reason) == "" {
		return nil, nil, transport.Validation("invalid todo.review payload", map[string]any{"reason": "required when rejecting"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return nil, nil, transport.NotFound("task not found")
	}
	// Authorize: either the owning user or an agent belonging to that user.
	if userID != "" && task.UserID != userID {
		return nil, nil, transport.Forbidden("task does not belong to this user")
	}
	if nodeID != "" {
		agent, err := s.agentByNodeUnsafe(nodeID)
		if err != nil {
			return nil, nil, err
		}
		if agent.UserID != task.UserID {
			return nil, nil, transport.Forbidden("agent does not belong to this task's user")
		}
	}
	if appErr := s.ensureTaskProjectActiveUnsafe(task); appErr != nil {
		return nil, nil, appErr
	}
	todoIdx := findTodoIndex(task, todoID)
	if todoIdx < 0 {
		return nil, nil, transport.NotFound("todo not found")
	}
	todo := &task.Todos[todoIdx]
	if todo.Status != "done" {
		return nil, nil, transport.Conflict("TODO_NOT_COMPLETED", "only a completed todo can be reviewed")
	}
	if todo.ReviewStatus != model.ReviewPending {
		return nil, nil, transport.Conflict("TODO_NOT_AWAITING_REVIEW", "todo is not awaiting review")
	}

	now := time.Now().UTC()

	var reworked *model.Todo
	switch action {
	case "approve":
		todo.ReviewStatus = model.ReviewApproved
		todo.ReviewReason = nil
		msg := fmt.Sprintf("todo review approved: %s", todo.Title)
		s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, todo.ID, "system", "reviewer", "人工确认", "todo_review_approved", &msg, map[string]any{"todo_id": todo.ID, "task_title": task.Title, "todo_title": todo.Title, "action": "approve"}, now)
	case "reject":
		todo.ReviewStatus = model.ReviewRejected
		todo.ReviewReason = &reason
		msg := fmt.Sprintf("todo review rejected, rework requested: %s", todo.Title)
		s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, todo.ID, "system", "reviewer", "人工确认", "todo_review_rejected", &msg, map[string]any{"todo_id": todo.ID, "task_title": task.Title, "todo_title": todo.Title, "action": "reject", "reason": reason}, now)
		// The audited todo depends on who reviews: an agent reviewer todo
		// audits its predecessor (order-1); a human review (need_review on
		// the todo itself, nodeID empty) audits the todo itself. Resetting
		// the wrong target made single-step human rejects a silent no-op and
		// the task flipped straight to done.
		auditedIdx := todoIdx - 1
		if nodeID == "" {
			auditedIdx = todoIdx
		}
		var reworkErr *transport.AppError
		reworked, reworkErr = s.triggerReworkUnsafe(task, auditedIdx, reason)
		if reworkErr != nil {
			return nil, nil, reworkErr
		}
	}

	s.updateTaskStatusUnsafe(task, now)
	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)
	return s.copyTaskWithArtifactsUnsafe(task), reworked, nil
}

// triggerReworkUnsafe cascades a rework request starting at the audited todo
// auditedIdx. The audited todo and every todo from that point onward (for
// agent reviews this includes the reviewer itself) are reset to pending,
// rework counters are bumped, and the audited todo is re-dispatched
// immediately so the chain re-runs. Callers pick auditedIdx by reviewer type:
// agent reviewer todo → its predecessor (order-1); human review → the todo
// under review itself.
//
// Preconditions (caller must hold s.mu): task is loaded, auditedIdx valid
// (>= 0; a negative index means there is nothing to audit).
func (s *Store) triggerReworkUnsafe(task *model.TaskDetail, auditedIdx int, reason string) (*model.Todo, *transport.AppError) {
	if auditedIdx < 0 {
		// Nothing to audit (e.g. agent review of the first todo): no cascade.
		return nil, nil
	}
	now := time.Now().UTC()

	// Reset the audited todo and everything after it (for agent reviews this
	// is inclusive of the reviewer todo) back to pending, so the chain
	// re-runs in order.
	for i := auditedIdx; i < len(task.Todos); i++ {
		t := &task.Todos[i]
		if t.Status == "canceled" {
			continue
		}
		resetTodoForReworkUnsafe(t, now)
	}

	// Rework counter: bump on the audited todo. Exceeding MaxReworks fails it.
	audited := &task.Todos[auditedIdx]
	maxReworks := audited.MaxReworks
	if maxReworks == 0 {
		maxReworks = defaultMaxReworks
	}
	audited.ReworkCount++
	if audited.ReworkCount > maxReworks {
		audited.Status = "failed"
		errMsg := fmt.Sprintf("重做次数超限（%d/%d）", audited.ReworkCount, maxReworks)
		audited.Error = &errMsg
		audited.FailedAt = &now
		s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, audited.ID, "system", "reviewer", "人工确认", "todo_rework_exhausted", &errMsg, map[string]any{"todo_id": audited.ID, "task_title": task.Title, "todo_title": audited.Title, "rework_count": audited.ReworkCount, "max_reworks": maxReworks}, now)
		return nil, nil
	}

	// Re-dispatch the audited predecessor (automatic rework).
	msg := fmt.Sprintf("退回重做：%s（原因：%s）", audited.Title, reason)
	s.recordTodoDispatchUnsafe(task, audited, "system", "reviewer", "人工确认", &msg, map[string]any{
		"todo_id":           audited.ID,
		"assignee_agent_id": audited.Assignee.AgentID,
		"rework":            true,
		"rework_count":      audited.ReworkCount,
		"reason":            reason,
	}, now)
	s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, audited.ID, "system", "reviewer", "人工确认", "todo_rework_requested", &msg, map[string]any{"todo_id": audited.ID, "task_title": task.Title, "todo_title": audited.Title, "rework_count": audited.ReworkCount, "reason": reason}, now)
	if audited.Status == "pending" {
		audited.Status = "in_progress"
		audited.StartedAt = &now
		audited.AssignedAt = &now
	}
	return audited, nil
}

// resetTodoForReworkUnsafe clears execution state so a todo can be executed
// again from scratch, keeping its result as historical reference.
func resetTodoForReworkUnsafe(t *model.Todo, now time.Time) {
	t.Status = "pending"
	t.StartedAt = nil
	t.AssignedAt = nil
	t.CompletedAt = nil
	t.FailedAt = nil
	t.CanceledAt = nil
	t.Error = nil
	t.CancelReason = nil
	t.LastActivityAt = nil
	t.ReviewStatus = ""
	t.ReviewReason = nil
}

// ── Action items → tasks conversion ───────────────────────────────

const maxActionItemsPerReport = 5

// normalizeActionItems cleans agent-provided action items: drops empties,
// defaults status to pending, stamps CreatedAt, and caps at 5 per report
// (agents should consolidate same-agent / same-category items themselves).
func normalizeActionItems(items []model.ActionItem, now time.Time) []model.ActionItem {
	out := make([]model.ActionItem, 0, len(items))
	for _, it := range items {
		it.Title = strings.TrimSpace(it.Title)
		if it.Title == "" {
			continue
		}
		if it.Status == "" {
			it.Status = model.ActionItemPending
		}
		if it.CreatedAt.IsZero() {
			it.CreatedAt = now
		}
		out = append(out, it)
		if len(out) >= maxActionItemsPerReport {
			break
		}
	}
	return out
}

// ActionItemRef locates an action item within the store: (taskID, todoID,
// itemIndex). The item lives in task.Todos[i].Result.ActionItems[j].
type ActionItemRef struct {
	TaskID  string
	TodoID  string
	ItemIdx int
	Item    *model.ActionItem
	Task    *model.TaskDetail
	Todo    *model.Todo
}

// ListActionItems returns action items across the user's tasks filtered by
// status (e.g. awaiting_confirmation for the project "待办" tab).
func (s *Store) ListActionItems(userID, projectID, status string, limit int) []ActionItemRef {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []ActionItemRef
	for _, task := range s.tasks {
		if task.UserID != userID {
			continue
		}
		if projectID != "" && task.ProjectID != projectID {
			continue
		}
		for i := range task.Todos {
			todo := &task.Todos[i]
			for j := range todo.Result.ActionItems {
				item := &todo.Result.ActionItems[j]
				if status != "" && item.Status != status {
					continue
				}
				out = append(out, ActionItemRef{
					TaskID:  task.ID,
					TodoID:  todo.ID,
					ItemIdx: j,
					Item:    item,
					Task:    task,
					Todo:    todo,
				})
				if limit > 0 && len(out) >= limit {
					return out
				}
			}
		}
	}
	return out
}

// ConvertActionItems turns the given action items into new tasks, grouping by
// target agent: items sharing the same resolved assignee become one task with
// multiple todos (one todo per action item). Each item is marked converted
// with the created task id (optimistic: fails if already converted).
func (s *Store) ConvertActionItems(userID string, refs []ActionItemRef, projectID, sourceTaskID string) ([]*model.TaskDetail, *transport.AppError) {
	if len(refs) == 0 {
		return nil, nil
	}
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Resolve assignee per item inside the lock (agentByNodeUnsafe requires it):
	// explicit node id first, then role-name matching against the user's agents.
	type resolvedItem struct {
		ref      *ActionItemRef
		agentKey string // assignee node id used for grouping
	}
	grouped := make(map[string][]resolvedItem)
	order := make([]string, 0)
	var unresolved []*ActionItemRef

	for i := range refs {
		ref := &refs[i]
		it := ref.Item
		if it.Status == model.ActionItemConverted {
			continue // already converted; skip silently
		}
		var agent *model.Agent
		if it.AssigneeNodeID != "" {
			if a, err := s.agentByNodeUnsafe(it.AssigneeNodeID); err == nil && a != nil && a.UserID == userID && !a.Archived {
				agent = a
			}
		}
		if agent == nil && it.AssigneeRole != "" {
			for _, a := range s.agents {
				if a.UserID == userID && !a.Archived && (a.Name == it.AssigneeRole || strings.Contains(a.Name, it.AssigneeRole)) {
					agent = a
					break
				}
			}
		}
		if agent == nil {
			unresolved = append(unresolved, ref)
			continue
		}
		key := agent.NodeID
		if _, ok := grouped[key]; !ok {
			grouped[key] = []resolvedItem{}
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], resolvedItem{ref: ref, agentKey: key})
	}

	if len(unresolved) > 0 {
		names := make([]string, 0, len(unresolved))
		for _, r := range unresolved {
			names = append(names, r.Item.Title)
		}
		return nil, transport.Validation("cannot resolve assignee for action items", map[string]any{"items": names, "hint": "provide assignee_node_id or assignee_role"})
	}

	created := make([]*model.TaskDetail, 0, len(order))
	for _, key := range order {
		items := grouped[key]
		assignee := items[0].ref.Item
		var agent *model.Agent
		if assignee.AssigneeNodeID != "" {
			if a, err := s.agentByNodeUnsafe(assignee.AssigneeNodeID); err == nil && a != nil && a.UserID == userID && !a.Archived {
				agent = a
			}
		}
		if agent == nil && assignee.AssigneeRole != "" {
			for _, a := range s.agents {
				if a.UserID == userID && !a.Archived && (a.Name == assignee.AssigneeRole || strings.Contains(a.Name, assignee.AssigneeRole)) {
					agent = a
					break
				}
			}
		}
		if agent == nil {
			return nil, transport.NotFound("assignee agent not found")
		}

		// Build one task with one todo per action item.
		todos := make([]model.Todo, 0, len(items))
		title := items[0].ref.Item.Title
		for idx, item := range items {
			it := item.ref.Item
			if it.Status == model.ActionItemConverted {
				return nil, transport.Conflict("ACTION_ITEM_ALREADY_CONVERTED", "action item already converted")
			}
			todos = append(todos, model.Todo{
				ID:          newID(),
				Order:       idx + 1,
				Title:       it.Title,
				Description: it.Description,
				Status:      "pending",
				Assignee: model.TodoAssignee{
					AgentID: agent.ID,
					Name:    agent.Name,
					NodeID:  agent.NodeID,
				},
				Result:    model.TodoResult{},
				CreatedAt: now,
			})
		}
		task := &model.TaskDetail{
			ID:           newID(),
			UserID:       userID,
			OrgID:        s.personalOrgOfUnsafe(userID),
			ProjectID:    projectID,
			Title:        title,
			Description:  fmt.Sprintf("由任务「%s」的结果待办自动转换生成（%d 项待办整合）", sourceTaskTitle(s, sourceTaskID), len(items)),
			Status:       "pending",
			Priority:     "medium",
			SourceTaskID: sourceTaskID,
			Todos:        todos,
			Result:       model.TaskResult{},
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		s.tasks[task.ID] = task

		// Mark items converted + record created task id.
		for _, item := range items {
			it := item.ref.Item
			it.Status = model.ActionItemConverted
			it.ConvertedTaskID = task.ID
			it.ConfirmedBy = "user:" + userID
		}

		if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
			return nil, mongoWriteError(err)
		}
		created = append(created, s.copyTaskWithArtifactsUnsafe(task))
		s.publishTaskUnsafe(task.ID)
	}
	return created, nil
}

func sourceTaskTitle(s *Store, taskID string) string {
	if taskID == "" {
		return ""
	}
	t, ok := s.tasks[taskID]
	if !ok {
		return taskID
	}
	return t.Title
}

// workflowSnapshot returns a deep copy of the workflow explicitly chosen at
// task creation time. Returns nil when the task is created without a workflow
// (PM plans freely). The project's workflow list is NOT auto-inherited — the
// frontend picks a workflow from the project list explicitly.
func workflowSnapshot(override *model.Workflow) *model.Workflow {
	if override != nil && len(override.Steps) > 0 {
		return override.Clone()
	}
	return nil
}

// applyWorkflowReviewFlags marks representative todos with NeedReview for each
// workflow step that has need_review=true. Matching is greedy over the ordered
// todos: exact agent id binding wins, otherwise fuzzy role/name match against
// the assignee role. This guarantees the workflow's human review gates exist
// on the plan regardless of whether the PM declared need_review per todo.
func applyWorkflowReviewFlags(steps []model.WorkflowStep, todos *[]model.Todo, todoRoles []string) {
	if len(steps) == 0 || len(*todos) == 0 {
		return
	}
	cur := 0 // next step to match
	for ti := 0; ti < len(*todos) && cur < len(steps); ti++ {
		step := steps[cur]
		if !stepMatchesTodo(step, (*todos)[ti], roleOfTodo(todoRoles, ti)) {
			// Extra todo (split/clarification) or future-role mismatch: skip it.
			continue
		}
		if step.NeedReview {
			(*todos)[ti].NeedReview = true
		}
		cur++
	}
}

func roleOfTodo(todoRoles []string, i int) string {
	if i < len(todoRoles) {
		return todoRoles[i]
	}
	return ""
}

// stepMatchesTodo reports whether the todo's assignee satisfies the step binding.
func stepMatchesTodo(step model.WorkflowStep, todo model.Todo, assigneeRole string) bool {
	if step.AgentID != "" {
		return step.AgentID == todo.Assignee.AgentID
	}
	role := strings.TrimSpace(step.Role)
	if role == "" {
		return false
	}
	return fuzzyMatch(todo.Assignee.Name, role) || fuzzyMatch(assigneeRole, role)
}

// fuzzyMatch is a loose containment-equality compare on either side.
func fuzzyMatch(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	return strings.Contains(a, b) || strings.Contains(b, a)
}

// alignTodosToStep returns the todos of a task that belong to the workflow
// step at stepIdx. Matching is agent_id based (stepMatchesTodo), so when one
// agent owns several steps of the same task every todo lights up all of them
// (measured 2026-09-04: the done 分镜拆解 todo also marked the not-yet-started
// 分镜分组 node done, because both steps share one agent). Disambiguation:
// todos whose title fuzzy-matches a step name are claimed by that step only;
// ambiguous todos without a title match join the step named by a same-agent
// sibling todo when possible; otherwise keep the old all-matches behavior so
// legacy data never matches less than before.
func alignTodosToStep(steps []model.WorkflowStep, todos []model.Todo, stepIdx int, roleOf func(model.Todo) string) []*model.Todo {
	cand := make([][]int, len(todos))
	for ti := range todos {
		for i := range steps {
			if stepMatchesTodo(steps[i], todos[ti], roleOf(todos[ti])) {
				cand[ti] = append(cand[ti], i)
			}
		}
	}
	claim := make([][]int, len(todos))
	for ti := range todos {
		cands := cand[ti]
		if len(cands) > 1 {
			var titled []int
			for _, i := range cands {
				if fuzzyMatch(todos[ti].Title, steps[i].Name) {
					titled = append(titled, i)
				}
			}
			if len(titled) > 0 {
				cands = titled
			} else {
				// Sibling fallback: a same-agent todo that names one of these
				// steps pins the whole sibling group to that step (a step may be
				// split into several todos, e.g. a preflight check).
				var pinned []int
				for tj := range todos {
					if tj == ti || todos[tj].Assignee.AgentID != todos[ti].Assignee.AgentID {
						continue
					}
					for _, i := range cand[tj] {
						if fuzzyMatch(todos[tj].Title, steps[i].Name) && intIn(cands, i) {
							pinned = append(pinned, i)
						}
					}
				}
				if len(pinned) > 0 {
					cands = dedupInts(pinned)
				}
			}
		}
		claim[ti] = cands
	}
	var out []*model.Todo
	for ti := range todos {
		if intIn(claim[ti], stepIdx) {
			td := todos[ti]
			out = append(out, &td)
		}
	}
	return out
}

func intIn(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func dedupInts(xs []int) []int {
	seen := make(map[int]bool, len(xs))
	out := xs[:0]
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// FindTaskForWorkflowStep returns the task in the same project whose owned
// slice of the primary workflow covers the named step, together with the task
// (artifacts filled) and the todo matched to that step. Used for cross-task
// step-input resolution so a downstream task can fetch the predecessor task's
// final produced output. Returns ok=false when no owning task/todo is found.
// FindTaskForWorkflowStep returns the most recently updated non-canceled task
// covering the requested workflow step, together with ALL todos matched to that
// step (a single pipeline step is frequently split into several same-agent
// todos, and the produced artifact may be linked to any of them — returning
// only the first matched todo would silently drop the step's real output).
// Ordered alignment mirrors applyWorkflowReviewFlags.
func (s *Store) FindTaskForWorkflowStep(userID, projectID, workflowName, stepName string) (*model.TaskDetail, []*model.Todo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	wantStep := strings.TrimSpace(stepName)
	if wantStep == "" || workflowName == "" {
		return nil, nil, false
	}
	var best *model.TaskDetail
	var bestTodos []*model.Todo
	for _, taskID := range s.projectTasks[projectID] {
		task, ok := s.tasks[taskID]
		if !ok || task.UserID != userID {
			continue
		}
		// Skip canceled tasks so a terminated test run never supplies a step's
		// inputs, and prefer the most recently updated active task when several
		// tasks cover the same step.
		if task.Status == "canceled" {
			continue
		}
		ref := task.WorkflowRef
		if ref == nil || ref.WorkflowName != workflowName || task.Workflow == nil || len(task.Workflow.Steps) == 0 {
			continue
		}
		stepIdx := -1
		for i := range task.Workflow.Steps {
			if strings.EqualFold(strings.TrimSpace(task.Workflow.Steps[i].Name), wantStep) {
				stepIdx = i
				break
			}
		}
		if stepIdx < 0 {
			continue
		}
		// Ordered alignment: walk steps against todos in Order (mirror
		// applyWorkflowReviewFlags) and collect every todo matched to the step.
		sorted := make([]model.Todo, len(task.Todos))
		copy(sorted, task.Todos)
		sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Order < sorted[j].Order })
		cur := 0
		for ti := 0; ti < len(sorted) && cur < len(task.Workflow.Steps); ti++ {
			step := task.Workflow.Steps[cur]
			if !stepMatchesTodo(step, sorted[ti], s.assigneeRoleUnsafe(sorted[ti])) {
				continue
			}
			if cur == stepIdx {
				if best == nil || task.UpdatedAt.After(best.UpdatedAt) {
					best = s.copyTaskWithArtifactsUnsafe(task)
					bestTodos = nil
				}
				if task.ID == best.ID {
					todoCopy := sorted[ti]
					bestTodos = append(bestTodos, &todoCopy)
				}
				continue
			}
			cur++
		}
	}
	if best == nil || len(bestTodos) == 0 {
		return nil, nil, false
	}
	return best, bestTodos, true
}

func (s *Store) assigneeRoleUnsafe(todo model.Todo) string {
	if ag, ok := s.agents[todo.Assignee.AgentID]; ok {
		return ag.Role
	}
	return ""
}

// GetProjectWorkflowProgress returns the progress of the project's primary
// workflow ("项目总流程"): one entry per step with the owning task, derived
// execution status and the produced output files. Returns an empty progress
// when the project has no primary workflow configured.
func (s *Store) GetProjectWorkflowProgress(userID, projectID string) (*model.WorkflowProgress, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	project, ok := s.projects[projectID]
	if !ok || project.UserID != userID {
		return nil, transport.NotFound("project not found")
	}
	if !hasPrimaryWorkflow(project) {
		// 空 progress 用非 nil 的 Steps，避免序列化为 null 导致前端崩溃。
		return &model.WorkflowProgress{Steps: []model.WorkflowStepProgress{}}, nil
	}
	pw := primaryWorkflowUnsafe(project)
	progress := &model.WorkflowProgress{WorkflowName: pw.Name}
	for i := range pw.Steps {
		step := pw.Steps[i]
		ps := model.WorkflowStepProgress{
			Index:   i,
			Name:    step.Name,
			Role:    step.Role,
			AgentID: step.AgentID,
			Status:  "unassigned",
		}
		if task, todos, ok := s.taskForPrimaryStepUnsafe(projectID, pw.Name, i); ok {
			ps.TaskID = task.ID
			ps.TaskTitle = task.Title
			if len(todos) > 0 {
				ps.Status = aggregateTodoStatus(todos)
				ps.Outputs = s.aggregateStepOutputs(task, todos)
			} else {
				ps.Status = "pending"
			}
		}
		progress.Steps = append(progress.Steps, ps)
	}
	return progress, nil
}

// taskForPrimaryStepUnsafe returns the task owning the given step index of the
// named primary workflow (via WorkflowRef range), together with that step's
// todo when the PM has already produced a matching one (ordered alignment).
// The task is returned even when no todo matches yet (the step is still owned
// by this task). Caller must hold at least s.mu.RLock.
func (s *Store) taskForPrimaryStepUnsafe(projectID, workflowName string, stepIndex int) (*model.TaskDetail, []*model.Todo, bool) {
	type candidate struct {
		task  *model.TaskDetail
		todos []*model.Todo
	}
	// Ownership priority for a pipeline step, highest first:
	//  1. active task WITH matching todos (its live progress is the truth)
	//  2. cancelled task WITH matching todos (history must survive a cancel)
	//  3. active task WITHOUT todos (just planned/dispatched, shows pending)
	// so a freshly created planning task cannot blanket-hide completed steps.
	var bestActiveWith, bestCanceledWith, bestActiveAny *candidate
	for _, taskID := range s.projectTasks[projectID] {
		task, ok := s.tasks[taskID]
		if !ok {
			continue
		}
		ref := task.WorkflowRef
		if ref == nil || ref.WorkflowName != workflowName || stepIndex < ref.StepFrom || stepIndex > ref.StepTo {
			continue
		}
		// Map the absolute step index back to the trimmed snapshot (owned range)
		// and collect every todo bound to that step. A step may be split into
		// several same-agent todos (e.g. a preflight check plus the real
		// deliverable); the pipeline node must reflect all of them so its status
		// and outputs are aggregated rather than taken from only the first todo.
		ownedIdx := stepIndex - ref.StepFrom
		if ownedIdx < 0 || ownedIdx >= len(task.Workflow.Steps) {
			continue
		}
		copied := s.copyTaskWithArtifactsUnsafe(task)
		matched := alignTodosToStep(copied.Workflow.Steps, copied.Todos, ownedIdx, s.assigneeRoleUnsafe)
		switch {
		case task.Status == "canceled":
			if len(matched) > 0 && (bestCanceledWith == nil || task.UpdatedAt.After(bestCanceledWith.task.UpdatedAt)) {
				bestCanceledWith = &candidate{task: copied, todos: matched}
			}
		case len(matched) > 0:
			if bestActiveWith == nil || task.UpdatedAt.After(bestActiveWith.task.UpdatedAt) {
				bestActiveWith = &candidate{task: copied, todos: matched}
			}
		default:
			if bestActiveAny == nil || task.UpdatedAt.After(bestActiveAny.task.UpdatedAt) {
				bestActiveAny = &candidate{task: copied, todos: matched}
			}
		}
	}
	chosen := bestActiveWith
	if chosen == nil {
		chosen = bestCanceledWith
	}
	if chosen == nil {
		chosen = bestActiveAny
	}
	if chosen == nil {
		return nil, nil, false
	}
	// A task covers the step but has not produced a matching todo yet: still own
	// the node (shown as pending) so the pipeline reflects the latest progress.
	if len(chosen.todos) == 0 {
		return chosen.task, nil, true
	}
	return chosen.task, chosen.todos, true
}

// stepOutputsUnsafe resolves a todo's produced outputs to file metadata from
// the task's artifacts. Caller must hold at least s.mu.RLock.
func (s *Store) stepOutputsUnsafe(task *model.TaskDetail, todo *model.Todo) []model.WorkflowStepOutputRef {
	outputs := make([]model.WorkflowStepOutputRef, 0, len(todo.Outputs)+len(task.Artifacts))
	seen := make(map[string]bool)
	// Workflow nodes bind FINAL deliverables only: process artifacts
	// (中间稿/草稿) are excluded here — they remain visible in the task's
	// result view / file explorer, just not on the pipeline.
	// 1) Explicitly declared output slots bound via outputName. A binding that
	//    resolves to a known process artifact is dropped (legacy data).
	for _, out := range todo.Outputs {
		bound := -1
		for i := range task.Artifacts {
			if out.ArtifactID != "" && task.Artifacts[i].TransferID == out.ArtifactID {
				bound = i
				break
			}
		}
		if bound >= 0 && task.Artifacts[bound].Kind == model.ArtifactKindProcess {
			continue
		}
		ref := model.WorkflowStepOutputRef{
			OutputName: out.OutputName,
			ArtifactID: out.ArtifactID,
			FileID:     out.FileRef,
		}
		if bound >= 0 {
			ref.FileName = task.Artifacts[bound].FileName
			ref.MimeType = task.Artifacts[bound].MimeType
			ref.FileSize = task.Artifacts[bound].FileSize
		}
		outputs = append(outputs, ref)
		if ref.FileID != "" {
			seen[ref.FileID] = true
		}
	}
	// 2) Fallback: artifacts linked to this todo by TodoID but not yet recorded
	//    in todo.Outputs (agents often attach a deliverable without declaring an
	//    output name). This is what makes pipeline steps show their final
	//    artifact even when only the todo_id association is present.
	//    Workflow nodes bind FINAL deliverables only: process artifacts
	//    (中间稿/草稿) are excluded here — they remain visible in the task's
	//    result view / file explorer, just not on the pipeline.
	for i := range task.Artifacts {
		a := task.Artifacts[i]
		if a.TodoID != todo.ID {
			continue
		}
		// Only explicitly-classified deliverables may surface on the pipeline.
		// Legacy records (empty kind) and process artifacts stay off it.
		if a.Kind != model.ArtifactKindDeliverable {
			continue
		}
		fileID := a.ProjectFileID
		if fileID == "" {
			fileID = s.transferFileIndex[a.TransferID]
		}
		if fileID != "" && seen[fileID] {
			continue
		}
		ref := model.WorkflowStepOutputRef{
			OutputName: a.FileName,
			ArtifactID: a.TransferID,
			FileID:     fileID,
			FileName:   a.FileName,
			MimeType:   a.MimeType,
			FileSize:   a.FileSize,
		}
		outputs = append(outputs, ref)
		if fileID != "" {
			seen[fileID] = true
		}
	}
	return outputs
}

// aggregateTodoStatus collapses the statuses of several todos that all belong
// to the same pipeline step into a single representative status. A running or
// awaiting step dominates done; a failed step is surfaced; only when every
// todo is done is the step reported done.
func aggregateTodoStatus(todos []*model.Todo) string {
	anyDone, anyCanceled := false, false
	for _, td := range todos {
		switch td.Status {
		case "in_progress", "awaiting_review":
			return "in_progress"
		case "failed":
			return "failed"
		case "done":
			anyDone = true
		case "canceled":
			anyCanceled = true
		}
	}
	if anyDone {
		return "done"
	}
	if anyCanceled {
		return "canceled"
	}
	return "pending"
}

// aggregateStepOutputs merges the produced outputs of every todo bound to the
// same pipeline step, de-duplicating by file id so a split step shows all of
// its deliverables on the progress node.
func (s *Store) aggregateStepOutputs(task *model.TaskDetail, todos []*model.Todo) []model.WorkflowStepOutputRef {
	var out []model.WorkflowStepOutputRef
	seen := make(map[string]bool)
	for _, td := range todos {
		for _, o := range s.stepOutputsUnsafe(task, td) {
			key := o.FileID
			if key == "" {
				key = o.ArtifactID
			}
			if key != "" && seen[key] {
				continue
			}
			out = append(out, o)
			if key != "" {
				seen[key] = true
			}
		}
	}
	return out
}

// workflowStepStatus maps a todo's execution state to a pipeline step status.
func workflowStepStatus(todo *model.Todo) string {
	switch todo.Status {
	case "done":
		return "done"
	case "failed":
		return "failed"
	case "canceled":
		return "canceled"
	case "in_progress":
		return "in_progress"
	case "awaiting_review":
		return "awaiting_review"
	default:
		return "pending"
	}
}
