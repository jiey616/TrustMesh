package store

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
)

// ────────────────────────────── P-02 ──────────────────────────────

func TestIsErrorCommentClassification(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"执行侧故障上报", "⚠️【执行侧故障上报】hermes run failed", true},
		{"gateway 错误", "hermes gateway connection refused", true},
		{"额度耗尽", "Billing or credits exhausted", true},
		{"超时", "context deadline exceeded", true},
		{"连接被拒", "dial tcp 10.0.0.1:443: connect: connection refused", true},
		{"进展汇报", "已完成第 3 组打组，正在处理第 4 组", false},
		{"普通进度", "进度：50%，预计还需 10 分钟", false},
		{"空评论", "", false},
		{"纯空白", "   ", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsErrorComment(c.in); got != c.want {
				t.Fatalf("IsErrorComment(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestErrorCommentDoesNotRefreshLiveness 是 P-02 的核心回归断言：故障上报型
// 评论不得续命，而进展型评论必须照旧续命（保住 persona 型执行者）。
func TestErrorCommentDoesNotRefreshLiveness(t *testing.T) {
	s, _, pm, developer, project := seedWorkflowState(t)
	s.log = zap.NewNop()

	task, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "P-02 liveness",
		Description: "验证错误评论不续命",
		Todos: []TaskCreateTodoInput{
			{Title: "阶段一", Description: "d", AssigneeNodeID: developer.NodeID},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}
	todoID := task.Todos[0].ID
	if _, appErr := s.UpdateTodoProgressByNode(developer.NodeID, TodoProgressInput{
		TaskID: task.ID, TodoID: todoID, Message: "开始执行",
	}); appErr != nil {
		t.Fatalf("start todo: %v", appErr)
	}

	// 人为制造「已提醒 2 次、40 分钟无活动」的状态
	old := time.Now().UTC().Add(-40 * time.Minute)
	td := &s.tasks[task.ID].Todos[0]
	if td.Status != "in_progress" {
		t.Fatalf("todo should be in_progress, got %s", td.Status)
	}
	td.LastActivityAt = &old
	td.LastProgressAt = &old
	td.RemindCount = 2
	td.RemindAt = &old

	// ---- 故障上报型评论：不得续命 ----
	if _, appErr := s.AddTaskCommentByNode(developer.NodeID, TaskCommentInput{
		TaskID: task.ID, TodoID: todoID,
		Content: "⚠️【执行侧故障上报】hermes run failed",
	}); appErr != nil {
		t.Fatalf("add error comment: %v", appErr)
	}
	td = &s.tasks[task.ID].Todos[0]
	if td.RemindCount != 2 {
		t.Fatalf("error comment must NOT reset RemindCount, got %d", td.RemindCount)
	}
	if td.LastActivityAt == nil || !td.LastActivityAt.Equal(old) {
		t.Fatalf("error comment must NOT refresh LastActivityAt")
	}
	if td.LastProgressAt == nil || !td.LastProgressAt.Equal(old) {
		t.Fatalf("error comment must NOT refresh LastProgressAt")
	}

	// ---- 进展型评论：照旧续命 ----
	if _, appErr := s.AddTaskCommentByNode(developer.NodeID, TaskCommentInput{
		TaskID: task.ID, TodoID: todoID,
		Content: "已完成第 3 组打组，正在处理第 4 组",
	}); appErr != nil {
		t.Fatalf("add progress comment: %v", appErr)
	}
	td = &s.tasks[task.ID].Todos[0]
	if td.RemindCount != 0 {
		t.Fatalf("progress comment must reset RemindCount, got %d", td.RemindCount)
	}
	if td.RemindAt != nil {
		t.Fatalf("progress comment must clear RemindAt")
	}
	if td.LastActivityAt == nil || !td.LastActivityAt.After(old) {
		t.Fatalf("progress comment must refresh LastActivityAt")
	}
	if td.LastProgressAt == nil || !td.LastProgressAt.After(old) {
		t.Fatalf("progress comment must refresh LastProgressAt")
	}
}

// ────────────────────────────── P-03 ──────────────────────────────

func TestHardDeadlineFailsUnproductiveTodo(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	now := time.Now().UTC()
	started := now.Add(-5 * time.Hour) // 超过普通 4h 阈值

	task := &model.TaskDetail{
		ID: "t-hd-1", UserID: "u1", ProjectID: "p1", Status: "in_progress",
		Todos: []model.Todo{{
			ID: "td-1", Title: "剧本创作", Status: "in_progress",
			StartedAt: &started,
			// 有活动（最近上报过），但没有任何有效进展
			LastActivityAt: &now,
			// LastProgressAt 为 nil → 回退 StartedAt
		}},
	}
	s.tasks[task.ID] = task

	s.checkTodoTimeouts()

	td := &s.tasks[task.ID].Todos[0]
	if td.Status != "failed" {
		t.Fatalf("unproductive todo past hard deadline must be failed, got %s", td.Status)
	}
	if task.Status != "failed" {
		t.Fatalf("task should aggregate to failed, got %s", task.Status)
	}
	if td.Error == nil || !strings.Contains(*td.Error, "无任何有效进展") {
		t.Fatalf("expected hard-deadline error message, got %v", td.Error)
	}
	var found bool
	for _, ev := range s.taskEvents[task.ID] {
		if ev.EventType == "todo_hard_deadline_failed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected todo_hard_deadline_failed event")
	}
}

func TestHardDeadlineSparesHeavyTodo(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	now := time.Now().UTC()
	started := now.Add(-5 * time.Hour) // 普通步骤会死，重活(8h)不该死

	task := &model.TaskDetail{
		ID: "t-hd-2", UserID: "u1", ProjectID: "p1", Status: "in_progress",
		Todos: []model.Todo{{
			ID: "td-1", Title: "分镜视频生成", Status: "in_progress",
			StartedAt:      &started,
			LastActivityAt: &started,
		}},
	}
	s.tasks[task.ID] = task

	s.checkTodoTimeouts()

	if s.tasks[task.ID].Todos[0].Status == "failed" {
		t.Fatalf("heavy (video) todo must NOT be killed by the 4h normal threshold")
	}
}

func TestHardDeadlineSparesTodoWithRecentProgress(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	now := time.Now().UTC()
	started := now.Add(-10 * time.Hour)
	progress := now.Add(-1 * time.Hour) // 1 小时前有真实进展

	task := &model.TaskDetail{
		ID: "t-hd-3", UserID: "u1", ProjectID: "p1", Status: "in_progress",
		Todos: []model.Todo{{
			ID: "td-1", Title: "剧本创作", Status: "in_progress",
			StartedAt:      &started,
			LastActivityAt: &progress,
			LastProgressAt: &progress,
		}},
	}
	s.tasks[task.ID] = task

	s.checkTodoTimeouts()

	if s.tasks[task.ID].Todos[0].Status == "failed" {
		t.Fatalf("todo with recent productive progress must NOT be killed")
	}
}

func TestHardDeadlineForThresholds(t *testing.T) {
	light := &model.Todo{Title: "剧本创作"}
	heavy := &model.Todo{Title: "分镜视频生成"}

	if got := hardDeadlineFor(light); got != defaultHardDeadline {
		t.Fatalf("light default = %v, want %v", got, defaultHardDeadline)
	}
	if got := hardDeadlineFor(heavy); got != defaultHardDeadlineHeavy {
		t.Fatalf("heavy default = %v, want %v", got, defaultHardDeadlineHeavy)
	}

	// 全局覆盖只影响普通步骤（保护重活不被误杀）
	t.Setenv("TODO_HARD_DEADLINE", "2h")
	if got := hardDeadlineFor(light); got != 2*time.Hour {
		t.Fatalf("light override = %v, want 2h", got)
	}
	if got := hardDeadlineFor(heavy); got != defaultHardDeadlineHeavy {
		t.Fatalf("global override must NOT shrink heavy deadline, got %v", got)
	}

	t.Setenv("TODO_HARD_DEADLINE_HEAVY", "12h")
	if got := hardDeadlineFor(heavy); got != 12*time.Hour {
		t.Fatalf("heavy override = %v, want 12h", got)
	}
}
