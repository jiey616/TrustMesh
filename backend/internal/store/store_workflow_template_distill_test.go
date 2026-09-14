package store

import (
	"net/http"
	"strings"
	"testing"

	"trustmesh/backend/internal/model"
)

// T1.9 从成功任务一键沉淀模板测试。

// seedDistillTask 直接写入一个任务（含 todos），避开完整 PM 建任务流程——
// 本测试关心的是沉淀映射规则，不是任务创建链路。
func seedDistillTask(t *testing.T, s *Store, userID, taskID, title string, todos []model.Todo) {
	t.Helper()
	s.mu.Lock()
	s.tasks[taskID] = &model.TaskDetail{
		ID:     taskID,
		UserID: userID,
		Title:  title,
		Status: "completed",
		Todos:  todos,
	}
	s.mu.Unlock()
}

func TestDistillWorkflowTemplateFromTaskHappyPath(t *testing.T) {
	s := New()
	user, appErr := s.CreateUser("distill@example.com", "U", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}
	writer, appErr := s.CreateAgent(Scope{UserID: user.ID}, "node-writer-1", "山雨", "developer", "编剧", nil)
	if appErr != nil {
		t.Fatalf("create writer: %v", appErr)
	}
	boarder, appErr := s.CreateAgent(Scope{UserID: user.ID}, "node-board-1", "阿板", "custom", "分镜", nil)
	if appErr != nil {
		t.Fatalf("create boarder: %v", appErr)
	}

	// 故意乱序写入，验证按 Order 重排。
	seedDistillTask(t, s, user.ID, "t-distill", "西游记后转延续动画制作", []model.Todo{
		{
			ID: "TD_02", Order: 2, Title: "分镜拆解", Status: "done", NeedReview: true,
			Assignee: model.TodoAssignee{AgentID: boarder.ID, Name: boarder.Name, NodeID: boarder.NodeID},
			Outputs:  []model.TodoOutput{{OutputName: "分镜表"}},
		},
		{
			ID: "TD_01", Order: 1, Title: "剧本创作", Status: "done",
			Assignee: model.TodoAssignee{AgentID: writer.ID, Name: writer.Name, NodeID: writer.NodeID},
		},
	})

	tpl, appErr := s.DistillWorkflowTemplateFromTask(Scope{UserID: user.ID}, "t-distill", "", "")
	if appErr != nil {
		t.Fatalf("distill: %v", appErr)
	}
	if tpl == nil {
		t.Fatal("distill returned nil template")
	}
	if tpl.Name != "西游记后转延续动画制作" {
		t.Errorf("name = %q, want task title (default)", tpl.Name)
	}
	if tpl.Version != 1 {
		t.Errorf("version = %d, want 1", tpl.Version)
	}
	if len(tpl.Steps) != 2 {
		t.Fatalf("steps len = %d, want 2", len(tpl.Steps))
	}

	// 按 Order 排序：剧本创作(TD_01) 先于 分镜拆解(TD_02)。
	if tpl.Steps[0].Name != "剧本创作" || tpl.Steps[1].Name != "分镜拆解" {
		t.Fatalf("step order = [%q, %q], want [剧本创作, 分镜拆解]", tpl.Steps[0].Name, tpl.Steps[1].Name)
	}
	// Role 来自指派 agent，跨项目可复用；AgentID 刻意不复制。
	if tpl.Steps[0].Role != "developer" {
		t.Errorf("step0 role = %q, want developer", tpl.Steps[0].Role)
	}
	if tpl.Steps[0].AgentID != "" {
		t.Errorf("step0 AgentID = %q, want empty (templates must stay portable)", tpl.Steps[0].AgentID)
	}
	if tpl.Steps[1].Role != "custom" {
		t.Errorf("step1 role = %q, want custom", tpl.Steps[1].Role)
	}
	if !tpl.Steps[1].NeedReview {
		t.Error("step1 need_review must be carried over")
	}
	if len(tpl.Steps[1].Outputs) != 1 || tpl.Steps[1].Outputs[0].Name != "分镜表" {
		t.Errorf("step1 outputs = %+v, want [分镜表]", tpl.Steps[1].Outputs)
	}

	// 沉淀出的模板必须可被列出（走 user 分区索引）。
	listed := s.ListWorkflowTemplates(Scope{UserID: user.ID})
	if len(listed) != 1 || listed[0].ID != tpl.ID {
		t.Fatalf("ListWorkflowTemplates = %+v, want the distilled template", listed)
	}
}

func TestDistillRejectsUnfinishedTask(t *testing.T) {
	s := New()
	user, _ := s.CreateUser("distill2@example.com", "U", "hash")
	seedDistillTask(t, s, user.ID, "t-partial", "半成品任务", []model.Todo{
		{ID: "TD_01", Order: 1, Title: "剧本创作", Status: "done"},
		{ID: "TD_02", Order: 2, Title: "分镜拆解", Status: "in_progress"},
	})

	_, appErr := s.DistillWorkflowTemplateFromTask(Scope{UserID: user.ID}, "t-partial", "", "")
	if appErr == nil {
		t.Fatal("distilling an unfinished task must fail")
	}
	if appErr.Status != http.StatusUnprocessableEntity || appErr.Code != "TASK_NOT_COMPLETED" {
		t.Fatalf("status=%d code=%s, want 422 TASK_NOT_COMPLETED", appErr.Status, appErr.Code)
	}
	if !strings.Contains(appErr.Message, "分镜拆解") {
		t.Errorf("message must name the unfinished step, got %q", appErr.Message)
	}
	// 失败时不得留下半个模板。
	if listed := s.ListWorkflowTemplates(Scope{UserID: user.ID}); len(listed) != 0 {
		t.Errorf("no template should be created on failure, got %d", len(listed))
	}
}

func TestDistillRejectsTaskWithoutTodos(t *testing.T) {
	s := New()
	user, _ := s.CreateUser("distill3@example.com", "U", "hash")
	seedDistillTask(t, s, user.ID, "t-empty", "空任务", nil)

	_, appErr := s.DistillWorkflowTemplateFromTask(Scope{UserID: user.ID}, "t-empty", "", "")
	if appErr == nil {
		t.Fatal("distilling a todo-less task must fail")
	}
}

func TestDistillDedupesStepNames(t *testing.T) {
	s := New()
	user, _ := s.CreateUser("distill4@example.com", "U", "hash")
	seedDistillTask(t, s, user.ID, "t-dup", "重名任务", []model.Todo{
		{ID: "TD_01", Order: 1, Title: "审核", Status: "done"},
		{ID: "TD_02", Order: 2, Title: "审核", Status: "done"},
	})

	tpl, appErr := s.DistillWorkflowTemplateFromTask(Scope{UserID: user.ID}, "t-dup", "", "")
	if appErr != nil {
		t.Fatalf("distill: %v", appErr)
	}
	if len(tpl.Steps) != 2 {
		t.Fatalf("steps len = %d, want 2", len(tpl.Steps))
	}
	if tpl.Steps[0].Name != "审核" || tpl.Steps[1].Name != "审核 (2)" {
		t.Fatalf("dedupe = [%q, %q], want [审核, 审核 (2)]", tpl.Steps[0].Name, tpl.Steps[1].Name)
	}
}

func TestDistillRespectsTaskVisibility(t *testing.T) {
	s := New()
	user, _ := s.CreateUser("distill5@example.com", "U", "hash")
	seedDistillTask(t, s, user.ID, "t-private", "私有任务", []model.Todo{
		{ID: "TD_01", Order: 1, Title: "剧本创作", Status: "done"},
	})

	if _, appErr := s.DistillWorkflowTemplateFromTask(Scope{UserID: "u-someone-else"}, "t-private", "", ""); appErr == nil {
		t.Fatal("a non-owner must not distill someone else's task")
	}
}
