package store

import (
	"sort"
	"strings"
	"time"

	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

type UpdateProjectInput struct {
	Name        *string
	Description *string
	PMAgentID   *string
	// Workflows replaces the project's workflow list when non-nil. Pass an
	// empty slice to clear all workflows.
	Workflows []model.Workflow
	// PrimaryWorkflowIndex marks which workflow is the project's 总流程.
	// nil keeps the current value; -1 clears it.
	PrimaryWorkflowIndex *int
	// PrimaryWorkflowID is the ID-based primary workflow reference (new
	// style). When set together with PrimaryWorkflowIndex, it takes effect.
	PrimaryWorkflowID *string
}

func (s *Store) buildProjectViewUnsafe(project *model.Project) *model.Project {
	clone := copyProject(project)
	clone.TaskSummary = s.aggregateProjectTaskSummaryUnsafe(project)
	return clone
}

func (s *Store) aggregateProjectTaskSummaryUnsafe(project *model.Project) model.ProjectTaskSummary {
	summary := model.ProjectTaskSummary{
		WorkStatus: "empty",
	}

	ids := s.projectTasks[project.ID]
	for _, id := range ids {
		task, ok := s.tasks[id]
		if !ok || !taskVisibleInProjectUnsafe(task, project) {
			continue
		}

		summary.TaskTotal++
		switch task.Status {
		case "pending":
			summary.PendingCount++
		case "in_progress":
			summary.InProgressCount++
		case "done":
			summary.DoneCount++
		case "failed":
			summary.FailedCount++
		case "canceled":
			summary.CanceledCount++
		}

		if summary.LatestTaskAt == nil || task.UpdatedAt.After(*summary.LatestTaskAt) {
			at := task.UpdatedAt
			summary.LatestTaskAt = &at
		}
	}

	switch {
	case project.Status == "archived":
		summary.WorkStatus = "archived"
	case summary.TaskTotal == 0:
		summary.WorkStatus = "empty"
	case summary.InProgressCount > 0:
		summary.WorkStatus = "running"
	case summary.FailedCount > 0:
		summary.WorkStatus = "attention"
	case summary.PendingCount > 0:
		summary.WorkStatus = "queued"
	default:
		summary.WorkStatus = "idle"
	}

	return summary
}

func (s *Store) CreateProject(sc Scope, name, description, pmAgentID string) (*model.Project, *transport.AppError) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" || description == "" || strings.TrimSpace(pmAgentID) == "" {
		return nil, transport.Validation("invalid project payload", map[string]any{
			"name":        "required",
			"description": "required",
			"pm_agent_id": "required",
		})
	}

	// T2.5：创建项目写 s.projects，并调 pmAgentForScopeUnsafe / resolveOwnerOrgUnsafe
	// 读仍由 s.mu 守护的 agents / orgs，故用 coarseWithAggregates（s.mu 外层 + AggProject 内层）。
	defer s.coarseWithAggregates(AggProject)()

	pm, err := s.pmAgentForScopeUnsafe(sc, pmAgentID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	project := &model.Project{
		ID:          newID(),
		UserID:      sc.UserID,
		OrgID:       s.resolveOwnerOrgUnsafe(sc),
		Name:        name,
		Description: description,
		Status:      "active",
		PMAgentID:   pmAgentID,
		PMAgent:     toPMSummary(pm),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.projects[project.ID] = project
	if appErr := s.persistProjectUnsafe(project); appErr != nil {
		// 幽灵项目修复（T2.3）：Mongo 写失败必须把刚插入的内存对象移除，
		// 否则 ListProjects/GetProject 能列出一个 Mongo 没有的项目，重启即消失。
		delete(s.projects, project.ID)
		return nil, appErr
	}
	return s.buildProjectViewUnsafe(project), nil
}

func (s *Store) ListProjects(sc Scope) []model.Project {
	// T2.5：读 s.projects 与 projectVisible（读 s.projectMembers，仍由 s.mu 守护），
	// 并经由 buildProjectViewUnsafe 读 s.tasks / s.projectTasks → AggProject + AggTask。
	defer s.coarseWithAggregates(AggProject, AggTask)()

	items := make([]model.Project, 0)
	for _, p := range s.projects {
		if s.projectVisible(sc, p) {
			items = append(items, *s.buildProjectViewUnsafe(p))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items
}

func (s *Store) GetProject(sc Scope, projectID string) (*model.Project, *transport.AppError) {
	// T2.5：读 s.projects + projectVisible + buildProjectViewUnsafe（触 s.tasks）→ AggProject + AggTask。
	defer s.coarseWithAggregates(AggProject, AggTask)()
	p, ok := s.projects[projectID]
	if !ok || !s.projectVisible(sc, p) {
		return nil, transport.NotFound("project not found")
	}
	return s.buildProjectViewUnsafe(p), nil
}

func (s *Store) UpdateProject(sc Scope, projectID string, in UpdateProjectInput) (*model.Project, *transport.AppError) {
	// T2.5：projectVisible / pmAgentForScopeUnsafe（读 agents）+ buildProjectViewUnsafe（读 tasks）
	// + mutateProjectUnsafe（写 s.projects）→ AggProject + AggTask。
	defer s.coarseWithAggregates(AggProject, AggTask)()
	p, ok := s.projects[projectID]
	if !ok || !s.projectVisible(sc, p) {
		return nil, transport.NotFound("project not found")
	}
	// T2.3：把校验 + 变更整体放进 mutateProjectUnsafe —— 任一步返回错误都会用快照还原，
	// 持久化失败同样回滚，保证「内存零副作用」。注意 fn 内必须直接改入参 p（快照回滚依赖）。
	appErr := s.mutateProjectUnsafe(projectID, func(p *model.Project) *transport.AppError {
		if in.Name != nil {
			name := strings.TrimSpace(*in.Name)
			if name == "" {
				return transport.Validation("invalid name", map[string]any{"name": "cannot be empty"})
			}
			p.Name = name
		}
		if in.Description != nil {
			desc := strings.TrimSpace(*in.Description)
			if desc == "" {
				return transport.Validation("invalid description", map[string]any{"description": "cannot be empty"})
			}
			p.Description = desc
		}
		if in.Workflows != nil {
			cloned := make([]model.Workflow, 0, len(in.Workflows))
			for _, wf := range in.Workflows {
				if c := wf.Clone(); c != nil {
					// 输入引用校验：拦截悬空的 source.step（指向不存在的步骤），
					// 避免派发时上游交付物解析失败（2026-09-07 资产制作事故）。
					if err := workflowStepsInputErr(c.Name, c.Steps); err != nil {
						return err
					}
					if c.ID == "" {
						c.ID = "wf_" + newID() // 存量迁移：老工作流没有 ID，保存时补上
					}
					// 保护继承快照：前端保存时通常不带 template_snapshot/template_version，
					// 若这是已存在的继承工作流，保留原有的快照与版本，避免三路合并 base 丢失。
					if old := findWorkflowByID(p.Workflows, c.ID); old != nil &&
						old.ParentTemplateID != "" && c.ParentTemplateID == old.ParentTemplateID {
						if len(c.TemplateSnapshot) == 0 {
							c.TemplateSnapshot = cloneSteps(old.TemplateSnapshot)
						}
						if c.TemplateVersion == 0 {
							c.TemplateVersion = old.TemplateVersion
						}
					}
					cloned = append(cloned, *c)
				}
			}
			p.Workflows = cloned
		}
		if in.PrimaryWorkflowIndex != nil {
			idx := *in.PrimaryWorkflowIndex
			if idx < -1 || idx >= len(p.Workflows) {
				return transport.Validation("invalid primary_workflow_index", map[string]any{
					"primary_workflow_index": idx,
					"max":                    len(p.Workflows) - 1,
				})
			}
			p.PrimaryWorkflowIndex = idx
			if idx >= 0 && p.Workflows[idx].ID != "" {
				p.PrimaryWorkflowID = p.Workflows[idx].ID
			} else if idx < 0 {
				p.PrimaryWorkflowID = ""
			}
		}
		if in.PrimaryWorkflowID != nil {
			id := strings.TrimSpace(*in.PrimaryWorkflowID)
			if id == "" {
				p.PrimaryWorkflowID = ""
				p.PrimaryWorkflowIndex = -1
			} else {
				idx := findWorkflowIndexByID(p.Workflows, id)
				if idx < 0 {
					return transport.Validation("invalid primary_workflow_id", map[string]any{"primary_workflow_id": id})
				}
				p.PrimaryWorkflowID = id
				p.PrimaryWorkflowIndex = idx
			}
		}
		if in.PMAgentID != nil {
			agentID := strings.TrimSpace(*in.PMAgentID)
			if agentID == "" {
				return transport.Validation("invalid pm_agent_id", map[string]any{"pm_agent_id": "cannot be empty"})
			}
			pm, err := s.pmAgentForScopeUnsafe(sc, agentID)
			if err != nil {
				return err
			}
			p.PMAgentID = agentID
			p.PMAgent = toPMSummary(pm)
		}

		// 若主流程引用的工作流已被删除（或越界），自动清空 primary 标记，避免脏数据。
		if p.PrimaryWorkflowID != "" && findWorkflowByID(p.Workflows, p.PrimaryWorkflowID) == nil {
			p.PrimaryWorkflowID = ""
			p.PrimaryWorkflowIndex = -1
		} else if p.PrimaryWorkflowIndex < -1 || p.PrimaryWorkflowIndex >= len(p.Workflows) {
			p.PrimaryWorkflowID = ""
			p.PrimaryWorkflowIndex = -1
		}
		p.UpdatedAt = time.Now().UTC()
		return nil
	})
	if appErr != nil {
		return nil, appErr
	}
	return s.buildProjectViewUnsafe(s.projects[projectID]), nil
}

func (s *Store) ArchiveProject(sc Scope, projectID string) (*model.Project, *transport.AppError) {
	// T2.5：projectVisible + resetArchivedProjectTasksUnsafe（读/写 s.tasks、persistTaskBundleUnsafe）
	// + persistAgentGraphUnsafe（读 agents）→ AggProject + AggTask。
	defer s.coarseWithAggregates(AggProject, AggTask)()
	p, ok := s.projects[projectID]
	if !ok || !s.projectVisible(sc, p) {
		return nil, transport.NotFound("project not found")
	}

	now := time.Now().UTC()
	affectedAgents, err := s.resetArchivedProjectTasksUnsafe(p, now)
	if err != nil {
		return nil, mongoWriteError(err)
	}

	// T2.3：只为「项目自身」这一步加版本化 + 失败回滚；上一步的任务重置保持既有语义，
	// 不引入跨聚合事务（项目步骤失败不影响已落库的任务重置）。
	if appErr := s.mutateProjectUnsafe(projectID, func(p *model.Project) *transport.AppError {
		p.Status = "archived"
		p.UpdatedAt = now
		return nil
	}); appErr != nil {
		return nil, appErr
	}

	for agentID := range affectedAgents {
		s.refreshAgentExecutionStatusUnsafe(agentID, now)
		if err := s.persistAgentGraphUnsafe(agentID); err != nil {
			return nil, mongoWriteError(err)
		}
	}
	return s.buildProjectViewUnsafe(p), nil
}

func (s *Store) resetArchivedProjectTasksUnsafe(project *model.Project, now time.Time) (map[string]struct{}, error) {
	affectedAgents := make(map[string]struct{})

	for _, taskID := range s.projectTasks[project.ID] {
		task, ok := s.tasks[taskID]
		if !ok || !taskVisibleInProjectUnsafe(task, project) {
			continue
		}

		taskChanged := false
		for i := range task.Todos {
			todo := &task.Todos[i]
			if todo.Status != "in_progress" {
				continue
			}

			todo.Status = "pending"
			todo.StartedAt = nil
			todo.CompletedAt = nil
			todo.FailedAt = nil
			todo.Error = nil
			affectedAgents[todo.Assignee.AgentID] = struct{}{}
			taskChanged = true
		}

		if task.Status == "in_progress" {
			task.Status = "pending"
			taskChanged = true
		}

		if !taskChanged {
			continue
		}

		task.Result = aggregateTaskResult(task.Todos, task.Status)
		task.UpdatedAt = now

		if err := s.persistTaskBundleUnsafe(task.ID); err != nil {
			return nil, err
		}
		s.publishTaskUnsafe(task.ID)
	}

	return affectedAgents, nil
}

func (s *Store) GetProjectPMNode(sc Scope, projectID string) (string, *transport.AppError) {
	// T2.5：projectForScopeUnsafe 含 projectVisible（读 s.projectMembers，仍由 s.mu 守护）
	// + 读 s.agents → s.mu 外层 + AggProject 内层。
	defer s.coarseWithAggregates(AggProject)()

	project, err := s.projectForScopeUnsafe(sc, projectID)
	if err != nil {
		return "", err
	}
	agent, ok := s.agents[project.PMAgentID]
	if !ok {
		return "", transport.Conflict("PROJECT_PM_AGENT_INVALID", "project bound PM agent is invalid")
	}
	return agent.NodeID, nil
}

func (s *Store) CheckTaskProjectActive(taskID string) *transport.AppError {
	defer s.lockAggregates(AggProject, AggTask)()

	task, ok := s.tasks[taskID]
	if !ok {
		return transport.NotFound("task not found")
	}
	return s.ensureTaskProjectActiveUnsafe(task)
}

func (s *Store) projectForScopeUnsafe(sc Scope, projectID string) (*model.Project, *transport.AppError) {
	p, ok := s.projects[projectID]
	if !ok || !s.projectVisible(sc, p) {
		return nil, transport.NotFound("project not found")
	}
	return p, nil
}

func (s *Store) ensureTaskProjectActiveUnsafe(task *model.TaskDetail) *transport.AppError {
	project, ok := s.projects[task.ProjectID]
	if !ok {
		return transport.NotFound("project not found")
	}
	if project.Status == "archived" {
		return transport.Conflict("PROJECT_ARCHIVED", "archived project cannot mutate tasks")
	}
	return nil
}

func (s *Store) pmAgentForScopeUnsafe(sc Scope, agentID string) (*model.Agent, *transport.AppError) {
	a, ok := s.agents[agentID]
	if !ok || !visibleToScope(sc, a.OrgID, a.UserID) || a.Role != "pm" || a.Archived {
		return nil, transport.Conflict("PROJECT_PM_AGENT_INVALID", "pm_agent_id must reference a PM agent of current user")
	}
	return a, nil
}

func (s *Store) validateProjectPMAgentOnlineUnsafe(project *model.Project) *transport.AppError {
	a, ok := s.agents[project.PMAgentID]
	if !ok || a.Role != "pm" {
		return transport.Conflict("PROJECT_PM_AGENT_INVALID", "project bound PM agent is invalid")
	}
	if a.Status == "offline" {
		return transport.Conflict("PM_AGENT_OFFLINE", "project pm agent is offline")
	}
	return nil
}
