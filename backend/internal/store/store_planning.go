package store

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// ─── Planning task lifecycle ───
// CreateTaskPlanning → AppendTaskMessage / AppendPMTaskReply (multi-round) → FinalizePlanByPMNode

type TaskPlanReadyInput struct {
	TaskID      string
	Title       string
	Description string
	Todos       []TaskCreateTodoInput
	// DeliverScope 可选：声明本次交付到工作流的哪一步为止（部分交付）。
	DeliverScope *model.TaskDeliverScope
}

func (s *Store) CreateTaskPlanning(userID, projectID, content string) (*model.TaskDetail, *transport.AppError) {
	return s.CreateTaskPlanningWithFiles(userID, projectID, content, nil, nil, nil, 0, 0)
}

// CreateTaskPlanningWithFiles creates a planning-mode task with optional file attachments.
// When workflowIndex points at a project workflow, the snapshot is trimmed to the
// requested step range (stepFrom..stepTo) and a WorkflowRef is recorded, mirroring
// CreateTaskByUser. Otherwise the explicitly chosen (or default) workflow is used as-is.
func (s *Store) CreateTaskPlanningWithFiles(userID, projectID, content string, fileIDs []string, workflow *model.Workflow, workflowIndex *int, stepFrom, stepTo int) (*model.TaskDetail, *transport.AppError) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, transport.Validation("invalid content", map[string]any{"content": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	project, err := s.projectForScopeUnsafe(Scope{UserID: userID}, projectID)
	if err != nil {
		return nil, err
	}
	if project.Status == "archived" {
		return nil, transport.Conflict("PROJECT_ARCHIVED", "archived project cannot create tasks")
	}
	if err := s.validateProjectPMAgentOnlineUnsafe(project); err != nil {
		return nil, err
	}

	pmAgent, ok := s.agents[project.PMAgentID]
	if !ok {
		return nil, transport.Conflict("PROJECT_PM_AGENT_INVALID", "project bound PM agent is invalid")
	}

	now := time.Now().UTC()

	// Trim the workflow snapshot to the chosen step range when the task is created
	// against a slice of the project's primary workflow, so the PM only plans the
	// requested steps. Mirrors the trimming logic in CreateTaskByUser.
	var wfSnapshot *model.Workflow
	var wfRef *model.WorkflowRef
	if workflowIndex != nil && *workflowIndex >= 0 && *workflowIndex < len(project.Workflows) {
		pw := project.Workflows[*workflowIndex]
		from := stepFrom
		to := stepTo
		if from < 0 {
			from = 0
		}
		if to >= len(pw.Steps) {
			to = len(pw.Steps) - 1
		}
		if from > to {
			from, to = 0, len(pw.Steps)-1
		}
		wfSnapshot = &model.Workflow{
			Name:  pw.Name,
			Steps: append([]model.WorkflowStep(nil), pw.Steps[from:to+1]...),
		}
		wfRef = &model.WorkflowRef{
			WorkflowIndex: *workflowIndex,
			WorkflowName:  pw.Name,
			StepFrom:      from,
			StepTo:        to,
		}
	} else {
		wfSnapshot = workflowSnapshot(workflow)
	}

	msg := model.TaskMessage{ID: uuid.NewString(), Role: "user", Content: content, CreatedAt: now}

	task := &model.TaskDetail{
		ID:        newID(),
		UserID:    userID,
		OrgID:     s.personalOrgOfUnsafe(userID),
		ProjectID: project.ID,
		Title:     content,
		Status:    "planning",
		Priority:  "medium",
		// Workflow snapshot trimmed to the requested step range (or the
		// explicitly chosen workflow as-is), plus its WorkflowRef when bound.
		Workflow:    wfSnapshot,
		WorkflowRef: wfRef,
		PMAgentID:     pmAgent.ID,
		PMAgent:       toPMSummary(pmAgent),
		Messages:      []model.TaskMessage{msg},
		Todos:         []model.Todo{},
		AttachedFiles: s.resolveAttachedFilesUnsafe(fileIDs, project.ID),
		Result: model.TaskResult{
			Summary:     "",
			FinalOutput: "",
			Metadata:    map[string]any{},
		},
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.tasks[task.ID] = task
	s.projectTasks[task.ProjectID] = append(s.projectTasks[task.ProjectID], task.ID)

	taskTitle := task.Title
	s.addEventUnsafe(userID, project.ID, task.ID, "", "user", userID, "", "task_created", &taskTitle, map[string]any{
		"task_title": task.Title,
		"mode":       "planning",
	}, now)

	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)

	return s.copyTaskWithArtifactsUnsafe(task), nil
}

func (s *Store) AppendTaskMessage(userID, taskID, content string, uiResponse *model.UIResponse) (*model.TaskDetail, *transport.AppError) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, transport.Validation("invalid content", map[string]any{"content": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return nil, transport.NotFound("task not found")
	}
	if task.Status != "planning" {
		return nil, transport.Conflict("TASK_NOT_PLANNING", "task is not in planning status")
	}
	if appErr := s.ensureTaskProjectActiveUnsafe(task); appErr != nil {
		return nil, appErr
	}
	project, ok := s.projects[task.ProjectID]
	if !ok {
		return nil, transport.NotFound("project not found")
	}
	if err := s.validateProjectPMAgentOnlineUnsafe(project); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	msg := model.TaskMessage{ID: uuid.NewString(), Role: "user", Content: content, UIResponse: uiResponse, CreatedAt: now}
	task.Messages = append(task.Messages, msg)
	task.UpdatedAt = now
	task.Version++

	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)

	return s.copyTaskWithArtifactsUnsafe(task), nil
}

func (s *Store) AppendPMTaskReply(nodeID, taskID, content string, uiBlocks []model.UIBlock) (*model.TaskDetail, *transport.AppError) {
	taskID = strings.TrimSpace(taskID)
	content = strings.TrimSpace(content)
	// 剥离 PM 运行时尾部样板（"WAITING" 等）：这类纯协议噪音残留在消息尾部
	// 会原样展示给用户（"已发送澄清请求…\n\nWAITING"）。
	content = strings.TrimRight(content, " \t\n")
	for _, tail := range []string{"WAITING", "waiting"} {
		if strings.HasSuffix(content, tail) {
			content = strings.TrimSpace(strings.TrimSuffix(content, tail))
		}
	}
	if taskID == "" || content == "" {
		return nil, transport.Validation("invalid task.reply payload", map[string]any{"task_id": "required", "content": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	pmAgent, err := s.agentByNodeUnsafe(nodeID)
	if err != nil {
		return nil, err
	}
	if pmAgent.Role != "pm" {
		return nil, transport.Forbidden("only pm agent can reply to planning task")
	}

	task, ok := s.tasks[taskID]
	if !ok {
		return nil, transport.NotFound("task not found")
	}
	if task.Status != "planning" {
		return nil, transport.Conflict("TASK_NOT_PLANNING", "task is not in planning status")
	}

	project, ok := s.projects[task.ProjectID]
	if !ok {
		return nil, transport.NotFound("project not found")
	}
	if project.PMAgentID != pmAgent.ID {
		return nil, transport.Forbidden("pm agent is not bound to this project")
	}

	now := time.Now().UTC()
	s.markAgentSeenUnsafe(pmAgent.ID, now)
	msg := model.TaskMessage{ID: uuid.NewString(), Role: "pm_agent", Content: content, UIBlocks: uiBlocks, CreatedAt: now}
	task.Messages = append(task.Messages, msg)
	task.UpdatedAt = now
	task.Version++

	s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, "", "agent", pmAgent.ID, pmAgent.Name, "planning_reply", &content, map[string]any{
		"task_id":   task.ID,
		"ui_blocks": uiBlocks,
	}, now)

	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	if err := s.persistAgentGraphUnsafe(pmAgent.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)

	return s.copyTaskWithArtifactsUnsafe(task), nil
}

func (s *Store) FinalizePlanByPMNode(nodeID, messageID string, in TaskPlanReadyInput) (*model.TaskDetail, *transport.AppError) {
	in.TaskID = strings.TrimSpace(in.TaskID)
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	if in.TaskID == "" || in.Title == "" || in.Description == "" {
		return nil, transport.Validation("invalid task.plan_ready payload", map[string]any{
			"task_id":     "required",
			"title":       "required",
			"description": "required",
		})
	}
	if len(in.Todos) == 0 {
		return nil, transport.Validation("invalid task.plan_ready payload", map[string]any{"todos": "must not be empty"})
	}
	normalizedTodos, todoErr := normalizeTaskCreateTodos(in.Todos)
	if todoErr != nil {
		return nil, todoErr
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Idempotency: check processedMessages first
	if task, ok := s.findProcessedTaskUnsafe(processedMessageKey("task.plan_ready", nodeID, messageID)); ok {
		return task, nil
	}

	pmAgent, err := s.agentByNodeUnsafe(nodeID)
	if err != nil {
		return nil, err
	}
	if pmAgent.Role != "pm" {
		return nil, transport.Forbidden("only pm agent can finalize plan")
	}

	task, ok := s.tasks[in.TaskID]
	if !ok {
		return nil, transport.NotFound("task not found")
	}
	// Idempotency: if already finalized, return current state
	if task.Status != "planning" {
		return s.copyTaskWithArtifactsUnsafe(task), nil
	}

	project, ok := s.projects[task.ProjectID]
	if !ok {
		return nil, transport.NotFound("project not found")
	}
	if project.Status == "archived" {
		return nil, transport.Conflict("PROJECT_ARCHIVED", "archived project cannot finalize tasks")
	}
	if project.PMAgentID != pmAgent.ID {
		return nil, transport.Forbidden("pm agent is not bound to this project")
	}

	now := time.Now().UTC()
	s.markAgentSeenUnsafe(pmAgent.ID, now)

	// Build todos
	seenTodoIDs := make(map[string]struct{}, len(normalizedTodos))
	todos := make([]model.Todo, 0, len(normalizedTodos))
	todoRoles := make([]string, 0, len(normalizedTodos)) // assignee role per todo
	for i, todoIn := range normalizedTodos {
		title := strings.TrimSpace(todoIn.Title)
		desc := strings.TrimSpace(todoIn.Description)
		assigneeNode := strings.TrimSpace(todoIn.AssigneeNodeID)
		if title == "" || desc == "" || assigneeNode == "" {
			return nil, transport.Validation("invalid todo in task.plan_ready", map[string]any{"todo_index": i, "title": "required", "description": "required", "assignee_node_id": "required"})
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
			Result: model.TodoResult{
				Summary:  "",
				Output:   "",
				Metadata: map[string]any{},
			},
			CreatedAt: now,
		})
		todoRoles = append(todoRoles, assigneeAgent.Role)
	}

	// System-level review gates: inherit step.need_review from the workflow so
	// the plan is not dependent on the PM declaring need_review on each todo.
	if wf := task.Workflow; wf != nil {
		applyWorkflowReviewFlags(wf.Steps, &todos, todoRoles)
	}

	// Update task: planning → review (awaiting user approval)
	task.Title = in.Title
	task.Description = in.Description
	task.DeliverScope = in.DeliverScope
	task.Status = "review"
	task.Todos = todos
	task.UpdatedAt = now
	task.Version++

	taskTitle := task.Title
	s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, "", "agent", pmAgent.ID, pmAgent.Name, "task_plan_ready", &taskTitle, map[string]any{
		"task_title": task.Title,
		"todo_count": len(todos),
	}, now)

	s.rememberProcessedMessageUnsafe(processedMessageKey("task.plan_ready", nodeID, messageID), "task.plan_ready", task.ID)
	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	if err := s.persistAgentGraphUnsafe(pmAgent.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	if err := s.persistProcessedMessageUnsafe(processedMessageKey("task.plan_ready", nodeID, messageID)); err != nil {
		return nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)

	return s.copyTaskWithArtifactsUnsafe(task), nil
}

// ApprovePlan transitions a task from review → pending so todos can be dispatched.
func (s *Store) ApprovePlan(userID, taskID string) (*model.TaskDetail, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return nil, transport.NotFound("task not found")
	}
	if task.Status != "review" {
		return nil, transport.Conflict("TASK_NOT_IN_REVIEW", "task is not awaiting approval")
	}
	if appErr := s.ensureTaskProjectActiveUnsafe(task); appErr != nil {
		return nil, appErr
	}

	now := time.Now().UTC()
	task.Status = "pending"
	task.UpdatedAt = now
	task.Version++

	taskTitle := task.Title
	s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, "", "user", userID, "", "task_approved", &taskTitle, map[string]any{
		"task_title": task.Title,
	}, now)

	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)

	return s.copyTaskWithArtifactsUnsafe(task), nil
}

// RejectPlan transitions a task from review → planning and appends user feedback.
func (s *Store) RejectPlan(userID, taskID, feedback string) (*model.TaskDetail, *transport.AppError) {
	feedback = strings.TrimSpace(feedback)
	if feedback == "" {
		return nil, transport.Validation("invalid payload", map[string]any{"feedback": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return nil, transport.NotFound("task not found")
	}
	if task.Status != "review" {
		return nil, transport.Conflict("TASK_NOT_IN_REVIEW", "task is not awaiting approval")
	}
	if appErr := s.ensureTaskProjectActiveUnsafe(task); appErr != nil {
		return nil, appErr
	}
	project, ok := s.projects[task.ProjectID]
	if !ok {
		return nil, transport.NotFound("project not found")
	}
	if err := s.validateProjectPMAgentOnlineUnsafe(project); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	msg := model.TaskMessage{ID: uuid.NewString(), Role: "user", Content: feedback, CreatedAt: now}
	task.Messages = append(task.Messages, msg)
	task.Status = "planning"
	task.UpdatedAt = now
	task.Version++

	taskTitle := task.Title
	s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, "", "user", userID, "", "task_plan_rejected", &taskTitle, map[string]any{
		"task_title": task.Title,
		"feedback":   feedback,
	}, now)

	if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
		return nil, mongoWriteError(err)
	}
	s.publishTaskUnsafe(task.ID)

	return s.copyTaskWithArtifactsUnsafe(task), nil
}

// ClaimPlanRejectNotify 对「任务 + 规划被拒指纹」做进程内节流：同一指纹自动催 PM
// 的次数未超过 max 时，计数 +1 并返回 true（允许推送）；否则返回 false。
// fingerprint 用 mismatch 文本本身（同一原因重提只催有限次，换原因则重新计）。
func (s *Store) ClaimPlanRejectNotify(taskID, fingerprint string, max int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.planRejectNotify == nil {
		s.planRejectNotify = make(map[string]int)
	}
	key := taskID + "\x00" + fingerprint
	if s.planRejectNotify[key] >= max {
		return false
	}
	s.planRejectNotify[key]++
	return true
}

// GetTaskPMPublishTarget returns the PM agent node ID for a planning task.

func (s *Store) GetTaskPMPublishTarget(userID, taskID string) (string, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return "", transport.NotFound("task not found")
	}
	if task.PMAgent.NodeID == "" {
		return "", transport.Conflict("TASK_NO_PM", fmt.Sprintf("task %s has no PM agent", taskID))
	}
	return task.PMAgent.NodeID, nil
}
