package store

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
)

func TestCheckTodoTimeoutsRemindsThenFails(t *testing.T) {
	store := New()
	store.log = zap.NewNop()
	now := time.Now().UTC()

	userID, projectID := "u1", "p1"
	taskID := "task-1"
	old := now.Add(-defaultTodoTimeout - time.Minute)

	task := &model.TaskDetail{
		ID:        taskID,
		UserID:    userID,
		ProjectID: projectID,
		Status:    "in_progress",
		Todos: []model.Todo{
			{
				ID:             "todo-1",
				Status:         "in_progress",
				AssignedAt:     &old,
				LastActivityAt: &old,
				Assignee:       model.TodoAssignee{NodeID: "node-agent"},
			},
		},
	}
	store.tasks[taskID] = task

	var reminds []string
	store.SetRemindHook(func(ctx context.Context, tid, todoid string) {
		reminds = append(reminds, todoid)
	})

	// ---- first check: should send reminder #1, keep in_progress ----
	store.checkTodoTimeouts()
	if len(reminds) != 1 {
		t.Fatalf("expected 1 reminder, got %d", len(reminds))
	}
	if task.Todos[0].Status != "in_progress" {
		t.Fatalf("todo should stay in_progress after first reminder, got %s", task.Todos[0].Status)
	}
	if task.Todos[0].RemindCount != 1 {
		t.Fatalf("expected RemindCount=1, got %d", task.Todos[0].RemindCount)
	}

	// ---- simulate no response: push RemindAt back so interval elapses ----
	// then 2nd reminder
	// P-08: the remind cadence now escalates (15m / 30m / 1h / 2h) instead of
	// a flat defaultRemindInterval, so the elapsed window must be derived
	// from the current RemindCount rather than a fixed interval.
	elapseRemindBackoff := func() {
		past := now.Add(-remindBackoff(task.Todos[0].RemindCount) - time.Minute)
		task.Todos[0].RemindAt = &past
	}
	elapseRemindBackoff()
	store.checkTodoTimeouts()
	if len(reminds) != 2 {
		t.Fatalf("expected 2 reminders, got %d", len(reminds))
	}
	if task.Todos[0].RemindCount != 2 {
		t.Fatalf("expected RemindCount=2, got %d", task.Todos[0].RemindCount)
	}
	if task.Todos[0].Status != "in_progress" {
		t.Fatalf("todo should still be in_progress, got %s", task.Todos[0].Status)
	}

	// ---- 3rd reminder ----
	elapseRemindBackoff()
	store.checkTodoTimeouts()
	if len(reminds) != 3 {
		t.Fatalf("expected 3 reminders, got %d", len(reminds))
	}
	if task.Todos[0].RemindCount != 3 {
		t.Fatalf("expected RemindCount=3, got %d", task.Todos[0].RemindCount)
	}
	if task.Todos[0].Status != "in_progress" {
		t.Fatalf("todo should still be in_progress before failure window, got %s", task.Todos[0].Status)
	}

	// ---- after 3 reminders + one more interval with no response: fail ----
	elapseRemindBackoff()
	store.checkTodoTimeouts()
	if task.Todos[0].Status != "failed" {
		t.Fatalf("expected todo to be failed after 3 unanswered reminders, got %s", task.Todos[0].Status)
	}
	if task.Status != "failed" {
		t.Fatalf("expected task to be failed, got %s", task.Status)
	}
	if task.Todos[0].Error == nil || *task.Todos[0].Error == "" {
		t.Fatalf("expected error message on failed todo")
	}
}

func TestCheckTodoTimeoutsResetsOnActivity(t *testing.T) {
	store := New()
	store.log = zap.NewNop()
	now := time.Now().UTC()
	old := now.Add(-defaultTodoTimeout - time.Minute)

	taskID := "task-2"
	task := &model.TaskDetail{
		ID:        taskID,
		UserID:    "u1",
		ProjectID: "p1",
		Status:    "in_progress",
		Todos: []model.Todo{
			{
				ID:             "todo-1",
				Status:         "in_progress",
				AssignedAt:     &old,
				LastActivityAt: &old,
				Assignee:       model.TodoAssignee{NodeID: "node-agent"},
				RemindCount:    2,
				RemindAt:       &now,
			},
		},
	}
	store.tasks[taskID] = task

	// Agent reports progress → LastActivityAt updated & RemindCount reset
	active := time.Now().UTC()
	task.Todos[0].LastActivityAt = &active
	task.Todos[0].RemindCount = 0
	task.Todos[0].RemindAt = nil

	store.checkTodoTimeouts()
	if task.Todos[0].Status != "in_progress" {
		t.Fatalf("todo with fresh activity must stay in_progress, got %s", task.Todos[0].Status)
	}
	if task.Todos[0].RemindCount != 0 {
		t.Fatalf("RemindCount should stay reset, got %d", task.Todos[0].RemindCount)
	}
}
