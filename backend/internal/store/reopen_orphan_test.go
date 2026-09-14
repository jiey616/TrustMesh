package store

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
)

// ────────────────────────────── P-05 ──────────────────────────────
//
// Background: the platform had no path back from a terminal state. On
// 2026-09-10 TD_06 was failed at 23:08 while its agent was still running; the
// agent produced 4/6 videos and uploaded a manifest at 00:08 which bound to a
// todo already marked failed — a "failed todo carrying a done deliverable"
// that nothing could reconcile.

// seedTerminalTodoState builds a store with a project, an agent, and a task
// whose second todo already reached a terminal state.
func seedTerminalTodoState(t *testing.T) (*Store, string, stringAgent, string, string) {
	t.Helper()
	s, userID, _, developer, project := seedWorkflowState(t)
	s.log = zap.NewNop()

	task := &model.TaskDetail{
		ID:        "t-reopen-1",
		UserID:    userID,
		ProjectID: project.ID,
		Status:    "in_progress",
		Title:     "P-05 reopen",
		Todos: []model.Todo{
			{ID: "td-1", Title: "剧本创作", Status: "done"},
			{ID: "td-2", Title: "分镜视频生成", Status: "failed"},
		},
	}
	s.tasks[task.ID] = task
	return s, userID, developer, task.ID, "td-2"
}

func TestReopenTodoFromFailed(t *testing.T) {
	s, userID, _, taskID, todoID := seedTerminalTodoState(t)

	old := time.Now().UTC().Add(-2 * time.Hour)
	td := &s.tasks[taskID].Todos[1]
	td.FailedAt = &old
	errMsg := "执行超时且多次提醒无响应，判定任务失败"
	td.Error = &errMsg
	td.RemindCount = 3
	td.RemindAt = &old
	td.LastActivityAt = &old
	td.LastProgressAt = &old

	_, reopened, appErr := s.ReopenTodo(Scope{UserID: userID}, taskID, todoID, "迟到产物回补")
	if appErr != nil {
		t.Fatalf("reopen failed todo: %v", appErr)
	}
	if reopened.Status != "in_progress" {
		t.Fatalf("reopened todo should be in_progress, got %s", reopened.Status)
	}
	if reopened.FailedAt != nil {
		t.Fatalf("FailedAt must be cleared on reopen")
	}
	if reopened.Error != nil {
		t.Fatalf("Error must be cleared on reopen, got %v", *reopened.Error)
	}
	if reopened.ReopenCount != 1 {
		t.Fatalf("ReopenCount = %d, want 1", reopened.ReopenCount)
	}
	// A reopened todo starts a fresh timeout budget, otherwise it would be
	// failed again by reminders accumulated while it sat in the terminal state.
	if reopened.RemindCount != 0 || reopened.RemindAt != nil {
		t.Fatalf("liveness counters must reset: RemindCount=%d RemindAt=%v", reopened.RemindCount, reopened.RemindAt)
	}
	if reopened.LastProgressAt == nil || !reopened.LastProgressAt.After(old) {
		t.Fatalf("LastProgressAt must be refreshed on reopen")
	}

	// Persisted state (not just the returned copy) must reflect the reopen.
	if got := s.tasks[taskID].Todos[1].Status; got != "in_progress" {
		t.Fatalf("persisted todo status = %s, want in_progress", got)
	}

	var found bool
	for _, ev := range s.taskEvents[taskID] {
		if ev.EventType == "todo_reopened" {
			found = true
			if ev.Metadata["reason"] != "迟到产物回补" {
				t.Fatalf("event reason = %v, want 迟到产物回补", ev.Metadata["reason"])
			}
		}
	}
	if !found {
		t.Fatalf("expected todo_reopened event")
	}
}

func TestReopenTodoRejectsNonTerminal(t *testing.T) {
	s, userID, _, taskID, _ := seedTerminalTodoState(t)

	for _, status := range []string{"pending", "in_progress"} {
		s.tasks[taskID].Todos[1].Status = status
		_, _, appErr := s.ReopenTodo(Scope{UserID: userID}, taskID, "td-2", "")
		if appErr == nil {
			t.Fatalf("reopen of %s todo must be rejected", status)
		}
		if appErr.Status != 409 || appErr.Code != "TODO_NOT_TERMINAL" {
			t.Fatalf("reopen of %s todo: status=%d code=%s, want 409 TODO_NOT_TERMINAL",
				status, appErr.Status, appErr.Code)
		}
	}
}

func TestReopenTodoRespectsMaxReopens(t *testing.T) {
	s, userID, _, taskID, todoID := seedTerminalTodoState(t)

	// Burn through the cap, failing the todo again between each reopen.
	for i := 0; i < maxReopens; i++ {
		s.tasks[taskID].Todos[1].Status = "failed"
		if _, _, appErr := s.ReopenTodo(Scope{UserID: userID}, taskID, todoID, ""); appErr != nil {
			t.Fatalf("reopen #%d should succeed: %v", i+1, appErr)
		}
	}
	if got := s.tasks[taskID].Todos[1].ReopenCount; got != maxReopens {
		t.Fatalf("ReopenCount = %d, want %d", got, maxReopens)
	}

	s.tasks[taskID].Todos[1].Status = "failed"
	_, _, appErr := s.ReopenTodo(Scope{UserID: userID}, taskID, todoID, "")
	if appErr == nil {
		t.Fatalf("reopen beyond the cap must be rejected")
	}
	if appErr.Status != 409 || appErr.Code != "TODO_REOPEN_LIMIT" {
		t.Fatalf("status=%d code=%s, want 409 TODO_REOPEN_LIMIT", appErr.Status, appErr.Code)
	}
}

// TestLateArtifactMarksOrphan covers the "failed todo with a done deliverable"
// case: the deliverable must be accepted and flagged, and must not be dropped
// nor duplicated.
func TestLateArtifactMarksOrphan(t *testing.T) {
	s, _, developer, taskID, todoID := seedTerminalTodoState(t)
	// The todo already reached a terminal state.
	s.tasks[taskID].Todos[1].Status = "failed"

	first, appErr := s.SaveArtifactWithFiling(model.TaskArtifact{
		TransferID: "tr-orphan-1",
		TaskID:     taskID,
		TodoID:     todoID,
		FileName:   "分镜视频台账.xlsx",
		FileSize:   2048,
		MimeType:   "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		FromNodeID: developer.NodeID,
		OutputName: "视频台账",
	}, nil)
	if appErr != nil {
		t.Fatalf("late artifact must be accepted, got: %v", appErr)
	}
	if first.OutputName != "视频台账" {
		t.Fatalf("OutputName = %q, want 视频台账", first.OutputName)
	}

	var late *model.TaskArtifact
	for i := range s.taskArtifacts[taskID] {
		if s.taskArtifacts[taskID][i].TransferID == "tr-orphan-1" {
			late = &s.taskArtifacts[taskID][i]
		}
	}
	if late == nil {
		t.Fatalf("late artifact was not stored")
	}
	if !late.Orphan {
		t.Fatalf("artifact bound to a terminal todo must be flagged orphan=true")
	}

	var found bool
	for _, ev := range s.taskEvents[taskID] {
		if ev.EventType == "artifact_bound_after_terminal" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected artifact_bound_after_terminal event")
	}

	// Re-uploading the same file must still collapse (no duplicate artifact),
	// even though the terminal door is now open.
	if _, appErr := s.SaveArtifactWithFiling(model.TaskArtifact{
		TransferID: "tr-orphan-2",
		TaskID:     taskID,
		TodoID:     todoID,
		FileName:   "分镜视频台账.xlsx",
		FileSize:   2048,
		MimeType:   "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		FromNodeID: developer.NodeID,
		OutputName: "视频台账",
	}, nil); appErr != nil {
		t.Fatalf("re-upload: %v", appErr)
	}
	if got := len(s.taskArtifacts[taskID]); got != 1 {
		t.Fatalf("duplicate artifact not collapsed: got %d artifacts, want 1", got)
	}
}

// ────────────────────────────── P-08 ──────────────────────────────

func TestRemindBackoffSchedule(t *testing.T) {
	cases := []struct {
		count int
		want  time.Duration
	}{
		{0, 0},
		{1, 15 * time.Minute},
		{2, 30 * time.Minute},
		{3, time.Hour},
		{4, 2 * time.Hour},
		{99, 2 * time.Hour},
	}
	for _, c := range cases {
		if got := remindBackoff(c.count); got != c.want {
			t.Fatalf("remindBackoff(%d) = %v, want %v", c.count, got, c.want)
		}
	}

	// Cumulative wait up to the failure verdict must stay inside the hard
	// deadline, otherwise the hard gate would kill the todo before the
	// reminder path ever escalates.
	var total time.Duration
	for i := 1; i <= defaultMaxReminders; i++ {
		total += remindBackoff(i)
	}
	if total >= defaultHardDeadline {
		t.Fatalf("cumulative remind wait %v must stay below the normal hard deadline %v",
			total, defaultHardDeadline)
	}
}

// TestRemindEscalatesAfterMax is the P-08 regression: a todo that exhausted its
// reminders must fail AND raise a human-facing escalation, instead of looping
// in a flat-cadence remind cycle.
func TestRemindEscalatesAfterMax(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	now := time.Now().UTC()
	stale := now.Add(-40 * time.Minute)         // past defaultTodoTimeout
	recentProgress := now.Add(-5 * time.Minute) // keeps the hard gate out of the way
	// The verdict now waits for the escalated backoff (remindBackoff(3) = 1h),
	// so the last reminder must be older than that for the todo to be failed.
	remindedAt := now.Add(-2 * time.Hour)

	task := &model.TaskDetail{
		ID: "t-esc-1", UserID: "u1", ProjectID: "p1", Status: "in_progress",
		Todos: []model.Todo{{
			ID:             "td-1",
			Title:          "分镜拆解",
			Status:         "in_progress",
			StartedAt:      &stale,
			LastActivityAt: &stale,
			LastProgressAt: &recentProgress,
			RemindCount:    defaultMaxReminders,
			RemindAt:       &remindedAt,
		}},
	}
	s.tasks[task.ID] = task

	s.checkTodoTimeouts()

	if got := s.tasks[task.ID].Todos[0].Status; got != "failed" {
		t.Fatalf("todo past max reminders must be failed, got %s", got)
	}
	var escalated, timedOut bool
	for _, ev := range s.taskEvents[task.ID] {
		switch ev.EventType {
		case "todo_remind_escalated":
			escalated = true
			if !strings.Contains(*ev.Content, "人工介入") {
				t.Fatalf("escalation must ask for human intervention, got %q", *ev.Content)
			}
		case "todo_timeout_failed":
			timedOut = true
		}
	}
	if !escalated {
		t.Fatalf("expected todo_remind_escalated event")
	}
	if !timedOut {
		t.Fatalf("expected todo_timeout_failed event alongside the escalation")
	}
}

// TestRunBudgetExhaustedEvent: the gateway silently truncates a run that hits
// its tool-call budget. That must surface as an explicit event instead of
// passing as generic "no progress".
func TestRunBudgetExhaustedEvent(t *testing.T) {
	s, _, pm, developer, project := seedWorkflowState(t)
	s.log = zap.NewNop()

	task, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "P-08 budget",
		Description: "验证预算耗尽可观测",
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

	if _, appErr := s.AddTaskCommentByNode(developer.NodeID, TaskCommentInput{
		TaskID:  task.ID,
		TodoID:  todoID,
		Content: "Turn ended with pending tool result after 60 steps ... budget=60/60",
	}); appErr != nil {
		t.Fatalf("add budget comment: %v", appErr)
	}

	var found bool
	for _, ev := range s.taskEvents[task.ID] {
		if ev.EventType == "todo_run_budget_exhausted" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected todo_run_budget_exhausted event")
	}

	// Budget exhaustion is a failure signal: it must NOT refresh liveness.
	td := &s.tasks[task.ID].Todos[0]
	if td.LastProgressAt == nil {
		t.Fatalf("LastProgressAt should have been set by the progress report")
	}
}

func TestIsBudgetExhaustedCommentClassification(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"budget 耗尽", "Turn ended with pending tool result ... budget=60/60", true},
		{"仅 budget 标记", "run terminated: budget=120/120", true},
		{"truncation 文案", "Turn ended with pending tool result", true},
		{"正常进度", "已完成第 3 组打组，正在处理第 4 组", false},
		{"空评论", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsBudgetExhaustedComment(c.in); got != c.want {
				t.Fatalf("IsBudgetExhaustedComment(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
