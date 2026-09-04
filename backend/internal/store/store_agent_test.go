package store

import (
	"math"
	"testing"
	"time"

	"trustmesh/backend/internal/model"
)

func TestGetAgentInsightsPM(t *testing.T) {
	s := New()
	now := time.Now().UTC()

	agent := &model.Agent{
		ID:     "agent-pm",
		UserID: "user-1",
		Role:   "pm",
	}
	s.agents[agent.ID] = agent

	s.projects["project-1"] = &model.Project{
		ID:     "project-1",
		UserID: "user-1",
		Name:   "项目 A",
	}

	s.tasks["task-done"] = &model.TaskDetail{
		ID:        "task-done",
		UserID:    "user-1",
		ProjectID: "project-1",
		Title:     "完成任务",
		Status:    "done",
		Priority:  "high",
		PMAgentID: agent.ID,
		CreatedAt: now.Add(-48 * time.Hour),
		UpdatedAt: now.Add(-6 * time.Hour),
	}
	s.tasks["task-pending"] = &model.TaskDetail{
		ID:        "task-pending",
		UserID:    "user-1",
		ProjectID: "project-1",
		Title:     "待处理任务",
		Status:    "pending",
		Priority:  "urgent",
		PMAgentID: agent.ID,
		CreatedAt: now.Add(-30 * time.Hour),
		UpdatedAt: now.Add(-30 * time.Hour),
	}
	s.tasks["task-running"] = &model.TaskDetail{
		ID:        "task-running",
		UserID:    "user-1",
		ProjectID: "project-1",
		Title:     "执行中任务",
		Status:    "in_progress",
		Priority:  "medium",
		PMAgentID: agent.ID,
		CreatedAt: now.Add(-2 * time.Hour),
		UpdatedAt: now.Add(-90 * time.Minute),
	}
	s.tasks["task-failed"] = &model.TaskDetail{
		ID:        "task-failed",
		UserID:    "user-1",
		ProjectID: "project-1",
		Title:     "失败任务",
		Status:    "failed",
		Priority:  "low",
		PMAgentID: agent.ID,
		CreatedAt: now.Add(-72 * time.Hour),
		UpdatedAt: now.Add(-12 * time.Hour),
	}

	insights, appErr := s.GetAgentInsights(Scope{UserID: "user-1"}, agent.ID)
	if appErr != nil {
		t.Fatalf("GetAgentInsights() error = %v", appErr)
	}

	if insights.Role != "pm" {
		t.Fatalf("Role = %q, want pm", insights.Role)
	}
	if insights.TotalItems != 4 {
		t.Fatalf("TotalItems = %d, want 4", insights.TotalItems)
	}
	if insights.ActiveItems != 1 {
		t.Fatalf("ActiveItems = %d, want 1", insights.ActiveItems)
	}
	if insights.PendingOver24h != 1 {
		t.Fatalf("PendingOver24h = %d, want 1", insights.PendingOver24h)
	}
	if insights.CompletionsLast7d != 1 {
		t.Fatalf("CompletionsLast7d = %d, want 1", insights.CompletionsLast7d)
	}
	if insights.FailuresLast7d != 1 {
		t.Fatalf("FailuresLast7d = %d, want 1", insights.FailuresLast7d)
	}
	if insights.OldestPendingMs == nil || *insights.OldestPendingMs < 24*60*60*1000 {
		t.Fatalf("OldestPendingMs = %v, want > 24h", insights.OldestPendingMs)
	}
	if insights.LongestInProgressMs == nil || *insights.LongestInProgressMs < 60*60*1000 {
		t.Fatalf("LongestInProgressMs = %v, want > 1h", insights.LongestInProgressMs)
	}
	if len(insights.ProjectContribution) != 1 {
		t.Fatalf("ProjectContribution len = %d, want 1", len(insights.ProjectContribution))
	}
	if insights.ProjectContribution[0].ProjectName != "项目 A" {
		t.Fatalf("ProjectName = %q, want 项目 A", insights.ProjectContribution[0].ProjectName)
	}
	if len(insights.PriorityBreakdown) != 4 {
		t.Fatalf("PriorityBreakdown len = %d, want 4", len(insights.PriorityBreakdown))
	}
	if len(insights.RiskItems) != 2 {
		t.Fatalf("RiskItems len = %d, want 2", len(insights.RiskItems))
	}
	if insights.RiskItems[0].Status != "pending" {
		t.Fatalf("RiskItems[0].Status = %q, want pending", insights.RiskItems[0].Status)
	}
	if insights.ResponseP50Ms != nil || insights.CompletionP50Ms != nil {
		t.Fatalf("PM insights should not include response/completion percentiles")
	}
}

func TestGetAgentInsightsExecutor(t *testing.T) {
	s := New()
	now := time.Now().UTC()

	agent := &model.Agent{
		ID:     "agent-dev",
		UserID: "user-1",
		Role:   "developer",
	}
	s.agents[agent.ID] = agent
	s.agents["other-agent"] = &model.Agent{
		ID:     "other-agent",
		UserID: "user-1",
		Role:   "developer",
	}

	s.projects["project-1"] = &model.Project{
		ID:     "project-1",
		UserID: "user-1",
		Name:   "项目 B",
	}

	doneStarted := now.Add(-110 * time.Minute)
	doneCompleted := now.Add(-80 * time.Minute)
	failedStarted := now.Add(-10 * time.Hour)
	failedAt := now.Add(-9 * time.Hour)
	runningStarted := now.Add(-90 * time.Minute)

	s.tasks["task-1"] = &model.TaskDetail{
		ID:        "task-1",
		UserID:    "user-1",
		ProjectID: "project-1",
		Title:     "开发任务",
		Status:    "in_progress",
		Priority:  "high",
		Todos: []model.Todo{
			{
				ID:          "todo-done",
				Title:       "完成 Todo",
				Status:      "done",
				Assignee:    model.TodoAssignee{AgentID: agent.ID},
				CreatedAt:   now.Add(-120 * time.Minute),
				StartedAt:   &doneStarted,
				CompletedAt: &doneCompleted,
			},
			{
				ID:        "todo-pending",
				Title:     "待处理 Todo",
				Status:    "pending",
				Assignee:  model.TodoAssignee{AgentID: agent.ID},
				CreatedAt: now.Add(-26 * time.Hour),
			},
			{
				ID:        "todo-running",
				Title:     "执行中 Todo",
				Status:    "in_progress",
				Assignee:  model.TodoAssignee{AgentID: agent.ID},
				CreatedAt: now.Add(-2 * time.Hour),
				StartedAt: &runningStarted,
			},
			{
				ID:        "todo-failed",
				Title:     "失败 Todo",
				Status:    "failed",
				Assignee:  model.TodoAssignee{AgentID: agent.ID},
				CreatedAt: now.Add(-11 * time.Hour),
				StartedAt: &failedStarted,
				FailedAt:  &failedAt,
			},
			{
				ID:        "todo-ignored",
				Title:     "其他人 Todo",
				Status:    "done",
				Assignee:  model.TodoAssignee{AgentID: "other-agent"},
				CreatedAt: now.Add(-3 * time.Hour),
			},
		},
	}

	insights, appErr := s.GetAgentInsights(Scope{UserID: "user-1"}, agent.ID)
	if appErr != nil {
		t.Fatalf("GetAgentInsights() error = %v", appErr)
	}

	if insights.Role != "developer" {
		t.Fatalf("Role = %q, want developer", insights.Role)
	}
	if insights.TotalItems != 4 {
		t.Fatalf("TotalItems = %d, want 4", insights.TotalItems)
	}
	if insights.ActiveItems != 1 {
		t.Fatalf("ActiveItems = %d, want 1", insights.ActiveItems)
	}
	if insights.PendingOver24h != 1 {
		t.Fatalf("PendingOver24h = %d, want 1", insights.PendingOver24h)
	}
	if insights.CompletionsLast7d != 1 {
		t.Fatalf("CompletionsLast7d = %d, want 1", insights.CompletionsLast7d)
	}
	if insights.FailuresLast7d != 1 {
		t.Fatalf("FailuresLast7d = %d, want 1", insights.FailuresLast7d)
	}
	assertApproxMs(t, "ResponseP50Ms", insights.ResponseP50Ms, 30*60*1000)
	assertApproxMs(t, "ResponseP90Ms", insights.ResponseP90Ms, 60*60*1000)
	assertApproxMs(t, "CompletionP50Ms", insights.CompletionP50Ms, 30*60*1000)
	assertApproxMs(t, "CompletionP90Ms", insights.CompletionP90Ms, 30*60*1000)
	if len(insights.RiskItems) != 2 {
		t.Fatalf("RiskItems len = %d, want 2", len(insights.RiskItems))
	}
	if insights.RiskItems[0].ID != "todo-pending" {
		t.Fatalf("RiskItems[0].ID = %q, want todo-pending", insights.RiskItems[0].ID)
	}
	if len(insights.ProjectContribution) != 1 {
		t.Fatalf("ProjectContribution len = %d, want 1", len(insights.ProjectContribution))
	}
	if insights.ProjectContribution[0].Done != 1 || insights.ProjectContribution[0].Failed != 1 {
		t.Fatalf("unexpected project contribution row: %+v", insights.ProjectContribution[0])
	}
}

func assertApproxMs(t *testing.T, label string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %v", label, want)
	}
	if math.Abs(*got-want) > 1 {
		t.Fatalf("%s = %v, want %v", label, *got, want)
	}
}
