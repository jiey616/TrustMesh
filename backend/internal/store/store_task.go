package store

import (
	"sort"
	"strings"

	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

func (s *Store) ListTasks(sc Scope, projectID, status string) ([]model.TaskListItem, *transport.AppError) {
	defer s.coarseWithAggregates(AggProject, AggTask)()

	if _, err := s.projectForScopeUnsafe(sc, projectID); err != nil {
		return nil, err
	}
	if status != "" && !isValidTaskStatus(status) {
		return nil, transport.Validation("invalid status", map[string]any{"status": "must be planning/pending/in_progress/done/failed/canceled"})
	}

	ids := s.projectTasks[projectID]
	items := make([]model.TaskListItem, 0, len(ids))
	for _, id := range ids {
		task, ok := s.tasks[id]
		if !ok || !visibleToScope(sc, task.OrgID, task.UserID) {
			continue
		}
		if status != "" && task.Status != status {
			continue
		}
		items = append(items, toTaskListItem(*task))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items, nil
}

func (s *Store) GetTask(sc Scope, taskID string) (*model.TaskDetail, *transport.AppError) {
	// T2.5：读 s.tasks，并经由 copyTaskWithArtifactsUnsafe 读仍由 s.mu 守护的 s.taskArtifacts
	// → s.mu 外层 + AggTask 内层。
	defer s.coarseWithAggregates(AggTask)()
	task, ok := s.tasks[taskID]
	if !ok || !visibleToScope(sc, task.OrgID, task.UserID) {
		return nil, transport.NotFound("task not found")
	}
	return s.copyTaskWithArtifactsUnsafe(task), nil
}

// GetTaskInternal returns a task by ID without user authorization checks.
// Used by internal dispatch hooks (e.g. timeout redispatch) that already hold
// the task id from an in-memory scan.
func (s *Store) GetTaskInternal(taskID string) *model.TaskDetail {
	// T2.5：copyTaskWithArtifactsUnsafe 读仍由 s.mu 守护的 s.taskArtifacts，故用
	// coarseWithAggregates（s.mu 外层 + AggTask 内层）而非纯 lockTask()。
	defer s.coarseWithAggregates(AggTask)()
	task, ok := s.tasks[taskID]
	if !ok {
		return nil
	}
	return s.copyTaskWithArtifactsUnsafe(task)
}

// GetTaskByNodeID returns a task if the requesting agent (identified by nodeID)
// is a participant: either the PM agent or a todo assignee.
func (s *Store) GetTaskByNodeID(nodeID, taskID string) (*model.TaskDetail, *transport.AppError) {
	// T2.5：agentByNodeUnsafe 读仍由 s.mu 守护的 s.agents，且 copyTaskWithArtifactsUnsafe
	// 读 s.taskArtifacts → s.mu 外层 + AggTask 内层。
	defer s.coarseWithAggregates(AggTask)()

	agent, err := s.agentByNodeUnsafe(nodeID)
	if err != nil {
		return nil, err
	}
	// 节点路径没有 HTTP 租户上下文，用 agent 自身身份构造 Scope：
	// 同租户（或 agent/task 尚未回填 org 时的同 user）才放行。
	agentScope := Scope{UserID: agent.UserID, OrgID: agent.OrgID}
	task, ok := s.tasks[taskID]
	if !ok || !visibleToScope(agentScope, task.OrgID, task.UserID) {
		return nil, transport.NotFound("task not found")
	}
	if task.PMAgent.NodeID == nodeID {
		return s.copyTaskWithArtifactsUnsafe(task), nil
	}
	for i := range task.Todos {
		if task.Todos[i].Assignee.NodeID == nodeID {
			return s.copyTaskWithArtifactsUnsafe(task), nil
		}
	}
	return nil, transport.Forbidden("agent is not a participant of this task")
}

func (s *Store) ListTaskEvents(sc Scope, taskID string) ([]model.Event, *transport.AppError) {
	// T2.5：读 s.tasks 与仍由 s.mu 守护的 s.taskEvents → s.mu 外层 + AggTask 内层。
	defer s.coarseWithAggregates(AggTask)()
	task, ok := s.tasks[taskID]
	if !ok || !visibleToScope(sc, task.OrgID, task.UserID) {
		return nil, transport.NotFound("task not found")
	}
	events := s.taskEvents[taskID]
	cloned := make([]model.Event, len(events))
	copy(cloned, events)
	sort.Slice(cloned, func(i, j int) bool { return cloned[i].CreatedAt.Before(cloned[j].CreatedAt) })
	return cloned, nil
}

// ListUserEvents 返回活动流。
// 多租户：带租户上下文时返回该租户的共享活动流（跨成员，按 org 隔离）；
// 无租户上下文（个人空间）退回 user 维度，与改造前完全一致。
// orgEvents 按创建顺序追加，倒序遍历即时间倒序。
func (s *Store) ListUserEvents(sc Scope, limit int) []model.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	events := s.userEvents[sc.UserID]
	if sc.HasOrg() {
		events = s.orgEvents[sc.OrgID]
	}
	if limit <= 0 {
		limit = len(events)
	}
	result := make([]model.Event, 0, min(limit, len(events)))
	for i := len(events) - 1; i >= 0 && len(result) < limit; i-- {
		if events[i].EventType == "agent_status_changed" {
			continue
		}
		result = append(result, *events[i])
	}
	return result
}

func (s *Store) ListAgentEvents(sc Scope, agentID string, limit int) ([]model.Event, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	agent, ok := s.agents[agentID]
	if !ok || !visibleToScope(sc, agent.OrgID, agent.UserID) {
		return nil, transport.NotFound("agent not found")
	}
	events := s.agentEvents[agentID]
	if limit <= 0 || limit > len(events) {
		limit = len(events)
	}
	result := make([]model.Event, 0, limit)
	for i := len(events) - 1; i >= 0 && len(result) < limit; i-- {
		result = append(result, *events[i])
	}
	return result, nil
}

func (s *Store) ListRecentTasks(sc Scope, limit int) []model.TaskListItem {
	// T2.5：遍历 s.tasks（仅触纯函数 visibleToScope，无 s.mu 专属 map 读取）→ s.mu 外层 + AggTask 内层。
	defer s.coarseWithAggregates(AggTask)()

	items := make([]model.TaskListItem, 0)
	for _, t := range s.tasks {
		if visibleToScope(sc, t.OrgID, t.UserID) {
			items = append(items, toTaskListItem(*t))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	return items
}

func toTaskListItem(task model.TaskDetail) model.TaskListItem {
	completed := 0
	failed := 0
	for _, td := range task.Todos {
		if td.Status == "done" {
			completed++
		}
		if td.Status == "failed" {
			failed++
		}
	}
	return model.TaskListItem{
		ID:                 task.ID,
		ProjectID:          task.ProjectID,
		Title:              task.Title,
		Description:        task.Description,
		Status:             task.Status,
		Priority:           task.Priority,
		PMAgent:            task.PMAgent,
		TodoCount:          len(task.Todos),
		CompletedTodoCount: completed,
		FailedTodoCount:    failed,
		CreatedAt:          task.CreatedAt,
		UpdatedAt:          task.UpdatedAt,
	}
}

func toPMSummary(a *model.Agent) model.PMAgentSummary {
	return model.PMAgentSummary{ID: a.ID, Name: a.Name, NodeID: a.NodeID, Status: a.Status}
}

func isValidTaskStatus(status string) bool {
	switch status {
	case "planning", "pending", "in_progress", "done", "failed", "canceled":
		return true
	default:
		return false
	}
}

// SetTaskWorkflow attaches (or replaces) the workflow snapshot carried by a
// task. Tasks created before a project gained a primary workflow, and tasks
// assembled by operational scripts, have no workflow; attaching one is what
// makes step-level I/O declaration — and therefore deliverable binding —
// available to them.
func (s *Store) SetTaskWorkflow(taskID string, wf *model.Workflow) *transport.AppError {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return transport.Validation("invalid task", map[string]any{"task_id": "required"})
	}
	// T2.5：写 s.tasks 并落库（persistTaskUnsafe / publishTaskUnsafe）→ s.mu 外层 + AggTask 内层。
	defer s.coarseWithAggregates(AggTask)()
	task, ok := s.tasks[taskID]
	if !ok {
		return transport.NotFound("task not found")
	}
	task.Workflow = wf
	if err := s.persistTaskUnsafe(task); err != nil && s.log != nil {
		s.log.Warn("failed to persist task workflow",
			zap.String("task_id", taskID), zap.Error(err))
	}
	s.publishTaskUnsafe(taskID)
	return nil
}
