package store

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
)

func TestRecordSequentialDispatchFailurePersists(t *testing.T) {
	s := New()
	s.log = zap.NewNop()

	task := &model.TaskDetail{
		ID: "t-d1", UserID: "u1", ProjectID: "p1", Status: "in_progress",
		Todos: []model.Todo{{
			ID: "td-1", Title: "剧本创作", Status: "pending",
			Assignee: model.TodoAssignee{AgentID: "a1", Name: "编剧", NodeID: "node-1"},
		}},
	}
	s.tasks[task.ID] = task

	if appErr := s.RecordSequentialDispatchFailure(task.ID, "td-1", "publish request failed: connection refused"); appErr != nil {
		t.Fatalf("record dispatch failure: %v", appErr)
	}

	td := &s.tasks[task.ID].Todos[0]
	if td.DispatchAttempts != 1 {
		t.Fatalf("DispatchAttempts = %d, want 1", td.DispatchAttempts)
	}
	if td.LastDispatchAt == nil {
		t.Fatalf("LastDispatchAt must be set")
	}
	if td.LastDispatchErr == nil || *td.LastDispatchErr == "" {
		t.Fatalf("LastDispatchErr must be set")
	}

	var found bool
	for _, ev := range s.taskEvents[task.ID] {
		if ev.EventType == "todo_dispatch_failed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected todo_dispatch_failed event to be persisted")
	}
}

func TestRecordSequentialTodoDispatchClearsFailureTrace(t *testing.T) {
	s, _, pm, developer, project := seedWorkflowState(t)
	s.log = zap.NewNop()

	task, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "P-01 dispatch trace",
		Description: "验证成功派发会清空失败痕迹",
		Todos: []TaskCreateTodoInput{
			{Title: "阶段一", Description: "d", AssigneeNodeID: developer.NodeID},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}

	todoID := task.Todos[0].ID
	msg := "boom"
	td := &s.tasks[task.ID].Todos[0]
	td.DispatchAttempts = 2
	td.LastDispatchErr = &msg

	if _, appErr := s.RecordSequentialTodoDispatch(task.ID, todoID); appErr != nil {
		t.Fatalf("record dispatch: %v", appErr)
	}

	td = &s.tasks[task.ID].Todos[0]
	if td.Status != "in_progress" {
		t.Fatalf("status = %s, want in_progress", td.Status)
	}
	if td.LastDispatchErr != nil {
		t.Fatalf("successful dispatch must clear LastDispatchErr")
	}
	if td.DispatchAttempts != 3 {
		t.Fatalf("DispatchAttempts = %d, want 3", td.DispatchAttempts)
	}
	if td.LastDispatchAt == nil {
		t.Fatalf("LastDispatchAt must be set on success")
	}
}

func TestDispatchReconcilerRepublishesStalledPendingTodo(t *testing.T) {
	s := New()
	s.log = zap.NewNop()

	old := time.Now().UTC().Add(-10 * time.Minute)
	task := &model.TaskDetail{
		ID: "t-r1", UserID: "u1", ProjectID: "p1", Status: "in_progress",
		Todos: []model.Todo{{
			ID: "td-1", Title: "剧本创作", Status: "pending", CreatedAt: old,
			Assignee: model.TodoAssignee{AgentID: "a1", Name: "编剧", NodeID: "node-1"},
		}},
	}
	s.tasks[task.ID] = task

	var calls []string
	s.SetDispatchHook(func(ctx context.Context, taskID, todoID string) {
		calls = append(calls, taskID+"/"+todoID)
	})

	s.reconcilePendingDispatches()

	if len(calls) != 1 || calls[0] != "t-r1/td-1" {
		t.Fatalf("expected one reconcile dispatch for t-r1/td-1, got %v", calls)
	}
}

func TestDispatchReconcilerSkipsFreshAndBlockedTodos(t *testing.T) {
	s := New()
	s.log = zap.NewNop()

	now := time.Now().UTC()

	// 刚创建的 pending todo：位于宽限期内，不应补派（避免与正常派发抢跑）
	fresh := &model.TaskDetail{
		ID: "t-r2", UserID: "u1", ProjectID: "p1", Status: "in_progress",
		Todos: []model.Todo{{
			ID: "td-1", Title: "剧本创作", Status: "pending", CreatedAt: now,
			Assignee: model.TodoAssignee{AgentID: "a1", NodeID: "node-1"},
		}},
	}
	s.tasks[fresh.ID] = fresh

	// 已完成的任务：不参与对账
	done := &model.TaskDetail{
		ID: "t-r3", UserID: "u1", ProjectID: "p1", Status: "done",
		Todos: []model.Todo{{
			ID: "td-1", Title: "剧本创作", Status: "pending",
			CreatedAt: now.Add(-1 * time.Hour),
			Assignee:  model.TodoAssignee{AgentID: "a1", NodeID: "node-1"},
		}},
	}
	s.tasks[done.ID] = done

	var calls []string
	s.SetDispatchHook(func(ctx context.Context, taskID, todoID string) {
		calls = append(calls, taskID+"/"+todoID)
	})

	s.reconcilePendingDispatches()

	if len(calls) != 0 {
		t.Fatalf("reconciler must not dispatch fresh or finished tasks, got %v", calls)
	}
}

func TestDispatchReconcilerSkipsInProgressTodo(t *testing.T) {
	s := New()
	s.log = zap.NewNop()

	old := time.Now().UTC().Add(-1 * time.Hour)
	task := &model.TaskDetail{
		ID: "t-r4", UserID: "u1", ProjectID: "p1", Status: "in_progress",
		Todos: []model.Todo{{
			ID: "td-1", Title: "剧本创作", Status: "in_progress", CreatedAt: old,
			Assignee: model.TodoAssignee{AgentID: "a1", NodeID: "node-1"},
		}},
	}
	s.tasks[task.ID] = task

	var calls []string
	s.SetDispatchHook(func(ctx context.Context, taskID, todoID string) {
		calls = append(calls, taskID+"/"+todoID)
	})

	s.reconcilePendingDispatches()

	if len(calls) != 0 {
		t.Fatalf("reconciler must not re-dispatch an already in_progress todo, got %v", calls)
	}
}

// --- T0.7c: stalled pending todos must be promoted into the monitoring window ---

// stalledTodoTask builds an in_progress task whose single todo is pending and
// already carries a dispatch attempt, which is the exact shape left behind when
// an assignee agent never reports back.
func stalledTodoTask(taskID, todoID string, attempts int, lastDispatch *time.Time) *model.TaskDetail {
	return &model.TaskDetail{
		ID: taskID, UserID: "u1", ProjectID: "p1", Status: "in_progress",
		Todos: []model.Todo{{
			ID: todoID, Title: "剧本创作", Status: "pending",
			DispatchAttempts: attempts, LastDispatchAt: lastDispatch,
			Assignee: model.TodoAssignee{AgentID: "a1", Name: "编剧", NodeID: "node-1"},
		}},
	}
}

func todoAutoAdvancedCount(s *Store, taskID string) int {
	var n int
	for _, ev := range s.taskEvents[taskID] {
		if ev.EventType == "todo_auto_advanced" {
			n++
		}
	}
	return n
}

func TestAdvanceStalledPendingTodosPromotesAndDispatches(t *testing.T) {
	s := New()
	s.log = zap.NewNop()

	old := time.Now().UTC().Add(-10 * time.Minute)
	task := stalledTodoTask("t-a1", "td-1", 1, &old)
	s.tasks[task.ID] = task

	var calls []string
	s.SetDispatchHook(func(ctx context.Context, taskID, todoID string) {
		calls = append(calls, taskID+"/"+todoID)
	})

	s.advanceStalledPendingTodos()

	td := &s.tasks[task.ID].Todos[0]
	if td.Status != "in_progress" {
		t.Fatalf("status = %s, want in_progress", td.Status)
	}
	if td.StartedAt == nil || td.AssignedAt == nil || td.LastProgressAt == nil || td.LastActivityAt == nil {
		t.Fatalf("promotion must baseline StartedAt/AssignedAt/LastProgressAt/LastActivityAt")
	}
	if len(calls) != 1 || calls[0] != "t-a1/td-1" {
		t.Fatalf("expected one dispatch for t-a1/td-1, got %v", calls)
	}
	if n := todoAutoAdvancedCount(s, task.ID); n != 1 {
		t.Fatalf("todo_auto_advanced events = %d, want 1", n)
	}
}

func TestAdvanceStalledPendingTodosSkipsWithinGrace(t *testing.T) {
	s := New()
	s.log = zap.NewNop()

	// Dispatched 10s ago: still inside the 90s grace window, so a slow agent
	// must not be treated as stalled.
	fresh := time.Now().UTC().Add(-10 * time.Second)
	task := stalledTodoTask("t-a2", "td-1", 1, &fresh)
	s.tasks[task.ID] = task

	var calls []string
	s.SetDispatchHook(func(ctx context.Context, taskID, todoID string) {
		calls = append(calls, taskID+"/"+todoID)
	})

	s.advanceStalledPendingTodos()

	if got := s.tasks[task.ID].Todos[0].Status; got != "pending" {
		t.Fatalf("status = %s, want pending (grace window not elapsed)", got)
	}
	if len(calls) != 0 {
		t.Fatalf("must not advance within grace, got %v", calls)
	}
}

func TestAdvanceStalledPendingTodosSkipsNeverAttempted(t *testing.T) {
	s := New()
	s.log = zap.NewNop()

	// No dispatch was ever recorded (e.g. process restarted before the
	// synchronous dispatch ran). Retrying is reconcilePendingDispatches' job;
	// promoting here would mask a dispatch that never happened at all.
	task := stalledTodoTask("t-a3", "td-1", 0, nil)
	task.Todos[0].CreatedAt = time.Now().UTC().Add(-1 * time.Hour)
	s.tasks[task.ID] = task

	var calls []string
	s.SetDispatchHook(func(ctx context.Context, taskID, todoID string) {
		calls = append(calls, taskID+"/"+todoID)
	})

	s.advanceStalledPendingTodos()

	if got := s.tasks[task.ID].Todos[0].Status; got != "pending" {
		t.Fatalf("status = %s, want pending (never-attempted todo is the redispatch stage's job)", got)
	}
	if len(calls) != 0 {
		t.Fatalf("must not advance a never-attempted todo, got %v", calls)
	}
}

func TestAdvanceStalledPendingTodosIsIdempotent(t *testing.T) {
	s := New()
	s.log = zap.NewNop()

	old := time.Now().UTC().Add(-10 * time.Minute)
	task := stalledTodoTask("t-a4", "td-1", 1, &old)
	s.tasks[task.ID] = task

	var calls []string
	s.SetDispatchHook(func(ctx context.Context, taskID, todoID string) {
		calls = append(calls, taskID+"/"+todoID)
	})

	s.advanceStalledPendingTodos()
	s.advanceStalledPendingTodos()

	if len(calls) != 1 {
		t.Fatalf("second pass must not re-advance, got %v", calls)
	}
	if n := todoAutoAdvancedCount(s, task.ID); n != 1 {
		t.Fatalf("todo_auto_advanced events = %d, want exactly 1", n)
	}
}

func TestAdvanceStalledPendingTodosDoesNotTouchRunningOrFinishedTasks(t *testing.T) {
	s := New()
	s.log = zap.NewNop()

	old := time.Now().UTC().Add(-10 * time.Minute)

	// Leading todo already in_progress: nothing pending to advance.
	running := stalledTodoTask("t-a5", "td-1", 1, &old)
	running.Todos[0].Status = "in_progress"
	s.tasks[running.ID] = running

	// Finished task: out of scope entirely.
	finished := stalledTodoTask("t-a6", "td-1", 2, &old)
	finished.Status = "done"
	s.tasks[finished.ID] = finished

	// A done-but-awaiting-review todo blocks the pipeline: NextDispatchableTodo
	// returns nil so the later pending todo must stay pending.
	reviewing := &model.TaskDetail{
		ID: "t-a7", UserID: "u1", ProjectID: "p1", Status: "in_progress",
		Todos: []model.Todo{
			{ID: "td-1", Title: "剧本创作", Status: "done", ReviewStatus: model.ReviewPending},
			{ID: "td-2", Title: "分镜拆解", Status: "pending", DispatchAttempts: 1, LastDispatchAt: &old},
		},
	}
	s.tasks[reviewing.ID] = reviewing

	var calls []string
	s.SetDispatchHook(func(ctx context.Context, taskID, todoID string) {
		calls = append(calls, taskID+"/"+todoID)
	})

	s.advanceStalledPendingTodos()

	if len(calls) != 0 {
		t.Fatalf("must not advance in these shapes, got %v", calls)
	}
	if got := s.tasks[running.ID].Todos[0].Status; got != "in_progress" {
		t.Fatalf("running todo status = %s, want untouched in_progress", got)
	}
	if got := s.tasks[reviewing.ID].Todos[1].Status; got != "pending" {
		t.Fatalf("review-blocked todo status = %s, want pending", got)
	}
}

func TestAdvanceStalledPendingTodosWithoutDispatchHookIsSafe(t *testing.T) {
	s := New()
	s.log = zap.NewNop()

	old := time.Now().UTC().Add(-10 * time.Minute)
	task := stalledTodoTask("t-a8", "td-1", 1, &old)
	s.tasks[task.ID] = task

	// No dispatch hook wired: the state transition must still land, and the
	// missing hook must not panic.
	s.advanceStalledPendingTodos()

	if got := s.tasks[task.ID].Todos[0].Status; got != "in_progress" {
		t.Fatalf("status = %s, want in_progress even without a dispatch hook", got)
	}
	if n := todoAutoAdvancedCount(s, task.ID); n != 1 {
		t.Fatalf("todo_auto_advanced events = %d, want 1", n)
	}
}
