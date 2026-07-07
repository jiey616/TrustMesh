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
}

type TodoFailInput struct {
	TaskID string
	TodoID string
	Error  string
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
		ID:          newID(),
		UserID:      project.UserID,
		ProjectID:   project.ID,
		Title:       in.Title,
		Description: in.Description,
		Status:      "pending",
		Priority:    "medium",
		PMAgentID:   pmAgent.ID,
		PMAgent:     toPMSummary(pmAgent),
		Todos:       todos,

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

	task := &model.TaskDetail{
		ID:          newID(),
		UserID:      userID,
		ProjectID:   project.ID,
		Title:       in.Title,
		Description: in.Description,
		Status:      "pending",
		Priority:    priority,
		PMAgentID:   "",
		PMAgent:     model.PMAgentSummary{},
		Todos:       []model.Todo{todo},
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

func (s *Store) CompleteTodoByNode(nodeID string, in TodoCompleteInput) (*model.TaskDetail, *transport.AppError) {
	return s.CompleteTodoByNodeWithMessageID(nodeID, "", in)
}

func (s *Store) CompleteTodoByNodeWithMessageID(nodeID, messageID string, in TodoCompleteInput) (*model.TaskDetail, *transport.AppError) {
	in.TaskID = strings.TrimSpace(in.TaskID)
	in.TodoID = strings.TrimSpace(in.TodoID)
	if in.TaskID == "" || in.TodoID == "" {
		return nil, transport.Validation("invalid todo.complete payload", map[string]any{"task_id": "required", "todo_id": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if task, ok := s.findProcessedTaskUnsafe(processedMessageKey("todo.complete", nodeID, messageID)); ok {
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
	todo.Status = "done"
	todo.CompletedAt = &now
	todo.FailedAt = nil
	todo.CanceledAt = nil
	todo.Error = nil
	todo.CancelReason = nil
	todo.Result = model.TodoResult{
		Summary:  strings.TrimSpace(in.Result.Summary),
		Output:   strings.TrimSpace(in.Result.Output),
		Metadata: copyMap(in.Result.Metadata),
	}
	completed := fmt.Sprintf("todo completed: %s", todo.Title)
	s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, todo.ID, "agent", agent.ID, agent.Name, "todo_completed", &completed, map[string]any{"todo_id": todo.ID, "task_title": task.Title, "todo_title": todo.Title}, now)

	s.updateTaskStatusUnsafe(task, now)
	s.rememberProcessedMessageUnsafe(processedMessageKey("todo.complete", nodeID, messageID), "todo.complete", task.ID)
	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	s.refreshAgentExecutionStatusUnsafe(agent.ID, now)
	if err := s.persistAgentGraphUnsafe(agent.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	if err := s.persistProcessedMessageUnsafe(processedMessageKey("todo.complete", nodeID, messageID)); err != nil {
		return nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)
	return s.copyTaskWithArtifactsUnsafe(task), nil
}

func (s *Store) FailTodoByNode(nodeID string, in TodoFailInput) (*model.TaskDetail, *transport.AppError) {
	return s.FailTodoByNodeWithMessageID(nodeID, "", in)
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
	if err := s.persistCommentUnsafe(comment); err != nil {
		return nil, mongoWriteError(err)
	}
	_ = s.persistTaskEventsUnsafe(task.ID)
	s.publishTaskUnsafe(task.ID)
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
	for _, todo := range task.Todos {
		switch todo.Status {
		case "failed":
			return "failed"
		case "done":
			hasWork = true
		case "in_progress":
			allDone = false
			hasWork = true
		case "canceled":
			allDone = false
		default:
			allDone = false
		}
	}
	if allDone {
		return "done"
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
		"todo_id":   todo.ID,
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
