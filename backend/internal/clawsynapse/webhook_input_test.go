package clawsynapse

import (
	"testing"
	"time"

	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
)

// TestResolvePrevStepName verifies that a single-step task's "prev" source
// link resolves to the name of the workflow step immediately before it
// (e.g. step3「剧本解析」→「分镜拆解」), and that the first step / bad index
// yields no predecessor. This backs the cross-task resolution of "prev"
// inputs that previously stayed Resolved=false.
func TestResolvePrevStepName(t *testing.T) {
	s := store.New()
	user, appErr := s.CreateUser("user@example.com", "User", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}
	pm, appErr := s.CreateAgent(store.Scope{UserID: user.ID}, "node-pm-001", "PM Agent", "pm", "pm", []string{"plan"})
	if appErr != nil {
		t.Fatalf("create pm: %v", appErr)
	}
	dev, appErr := s.CreateAgent(store.Scope{UserID: user.ID}, "node-dev-001", "Dev", "developer", "dev", nil)
	if appErr != nil {
		t.Fatalf("create dev: %v", appErr)
	}
	s.SyncAgentPresence([]store.AgentPresence{
		{NodeID: pm.NodeID, LastSeenAt: time.Now().UTC()},
		{NodeID: dev.NodeID, LastSeenAt: time.Now().UTC()},
	}, time.Now().UTC())
	proj, appErr := s.CreateProject(store.Scope{UserID: user.ID}, "TrustMesh MVP", "demo", pm.ID)
	if appErr != nil {
		t.Fatalf("create project: %v", appErr)
	}

	// Mirror the project's real pipeline: workflow 0 with 9 steps, where step3
	// (剧本解析) declares a "prev" input pointing at step2 (分镜拆解).
	wf := model.Workflow{
		Name: "画宗AIGC无人工厂产线工作流",
		Steps: []model.WorkflowStep{
			{Name: "剧本创作", Role: "developer", AgentID: dev.ID},
			{Name: "剧本诊断", Role: "developer", AgentID: dev.ID},
			{Name: "分镜拆解", Role: "developer", AgentID: dev.ID},
			{Name: "剧本解析", Role: "developer", AgentID: dev.ID},
		},
	}
	idx := 0
	if _, appErr := s.UpdateProject(store.Scope{UserID: user.ID}, proj.ID, store.UpdateProjectInput{
		Workflows:            []model.Workflow{wf},
		PrimaryWorkflowIndex: &idx,
	}); appErr != nil {
		t.Fatalf("update project workflows: %v", appErr)
	}

	h := NewWebhookHandler(WebhookDeps{Store: s})
	base := model.TaskDetail{
		UserID:    user.ID,
		ProjectID: proj.ID,
		WorkflowRef: &model.WorkflowRef{
			WorkflowIndex: 0,
			WorkflowName:  "画宗AIGC无人工厂产线工作流",
		},
	}

	t.Run("prev of step3 resolves to step2 分镜拆解", func(t *testing.T) {
		task := base
		task.WorkflowRef.StepFrom, task.WorkflowRef.StepTo = 3, 3
		if got := h.resolvePrevStepName(&task); got != "分镜拆解" {
			t.Fatalf("want 分镜拆解, got %q", got)
		}
	})

	t.Run("first step has no prev", func(t *testing.T) {
		task := base
		task.WorkflowRef.StepFrom, task.WorkflowRef.StepTo = 0, 0
		if got := h.resolvePrevStepName(&task); got != "" {
			t.Fatalf("want empty for first step, got %q", got)
		}
	})

	t.Run("nil workflow ref yields empty", func(t *testing.T) {
		task := model.TaskDetail{UserID: user.ID, ProjectID: proj.ID}
		if got := h.resolvePrevStepName(&task); got != "" {
			t.Fatalf("want empty for nil workflow ref, got %q", got)
		}
	})

	t.Run("out-of-range workflow index yields empty", func(t *testing.T) {
		task := base
		task.WorkflowRef.WorkflowIndex = 99
		if got := h.resolvePrevStepName(&task); got != "" {
			t.Fatalf("want empty for bad index, got %q", got)
		}
	})
}
