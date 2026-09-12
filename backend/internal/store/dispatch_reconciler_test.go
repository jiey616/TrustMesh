package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/metrics"
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

// --- T0.11: auto-advance telemetry and the agent compliance sentinel ---

func eventCount(s *Store, taskID, eventType string) int {
	var n int
	for _, ev := range s.taskEvents[taskID] {
		if ev.EventType == eventType {
			n++
		}
	}
	return n
}

// agentStepStallTask builds a task with `pendingSteps` auto-advanceable steps
// behind a completed predecessor, all assigned to the same agent. It is the
// shape of an agent that never reports: the platform has to walk the whole
// pipeline for it.
func agentStepStallTask(taskID string, pendingSteps int) *model.TaskDetail {
	now := time.Now().UTC()
	doneAt := now.Add(-5 * time.Minute)
	dispatchedAt := now.Add(-2 * time.Minute)

	assignee := model.TodoAssignee{AgentID: "a1", Name: "编剧", NodeID: "node-1"}
	task := &model.TaskDetail{
		ID: taskID, UserID: "u1", ProjectID: "p1", Status: "in_progress",
		Todos: []model.Todo{{
			ID: "td-1", Title: "剧本创作", Status: "done", CompletedAt: &doneAt,
			Assignee: assignee,
		}},
	}
	for i := 0; i < pendingSteps; i++ {
		task.Todos = append(task.Todos, model.Todo{
			ID: fmt.Sprintf("td-%d", i+2), Title: "步骤", Status: "pending",
			DispatchAttempts: 1, LastDispatchAt: &dispatchedAt,
			Assignee: assignee,
		})
	}
	return task
}

func TestAdvanceStalledPendingTodosEmitsAutoAdvanceTelemetry(t *testing.T) {
	metrics.Reset()
	t.Cleanup(metrics.Reset)

	s := New()
	s.log = zap.NewNop()

	task := agentStepStallTask("t-tel", 1)
	s.tasks[task.ID] = task
	s.SetDispatchHook(func(ctx context.Context, taskID, todoID string) {})

	s.advanceStalledPendingTodos()

	if got := metrics.Count(metrics.TodoAutoAdvancedTotal); got != 1 {
		t.Fatalf("TodoAutoAdvancedTotal = %d, want 1", got)
	}
	// One sample: the promoted step had a completed predecessor 5 minutes back.
	hist := metrics.Histogram(metrics.StepAdvanceDuration)
	if hist.Count != 1 {
		t.Fatalf("step_advance samples = %d, want 1", hist.Count)
	}
	if hist.SumMs < (4 * time.Minute).Milliseconds() {
		t.Fatalf("step_advance sum = %dms, want >= 4min (predecessor completed 5min ago)", hist.SumMs)
	}
	// A single advance is absorbed silently: no compliance sentinel.
	if got := metrics.Count(metrics.AgentStepStalledTotal); got != 0 {
		t.Fatalf("AgentStepStalledTotal = %d after one advance, want 0", got)
	}
	if n := eventCount(s, task.ID, "agent_step_stalled"); n != 0 {
		t.Fatalf("agent_step_stalled events = %d after one advance, want 0", n)
	}
}

func TestAdvanceStalledPendingTodosTripsComplianceSentinelOncePerStreak(t *testing.T) {
	metrics.Reset()
	t.Cleanup(metrics.Reset)

	s := New()
	s.log = zap.NewNop()

	task := agentStepStallTask("t-sentinel", 3)
	s.tasks[task.ID] = task
	s.SetDispatchHook(func(ctx context.Context, taskID, todoID string) {})

	// The agent hands each step back done, then goes silent on the next one —
	// i.e. it needs the platform to advance three steps in a row.
	for i := 1; i <= 3; i++ {
		if i > 1 {
			doneAt := time.Now().UTC()
			s.tasks[task.ID].Todos[i-1].Status = "done"
			s.tasks[task.ID].Todos[i-1].CompletedAt = &doneAt
		}
		s.advanceStalledPendingTodos()

		if got := s.tasks[task.ID].Todos[i].Status; got != "in_progress" {
			t.Fatalf("step %d status = %s, want in_progress", i, got)
		}
	}

	if got := metrics.Count(metrics.TodoAutoAdvancedTotal); got != 3 {
		t.Fatalf("TodoAutoAdvancedTotal = %d, want 3", got)
	}
	// Threshold is 2 consecutive advances, and it fires exactly once: the third
	// advance must not produce a second event, or an agent that stalls for a
	// long time would flood the timeline.
	if got := metrics.Count(metrics.AgentStepStalledTotal); got != 1 {
		t.Fatalf("AgentStepStalledTotal = %d, want exactly 1", got)
	}
	if n := eventCount(s, task.ID, "agent_step_stalled"); n != 1 {
		t.Fatalf("agent_step_stalled events = %d, want exactly 1", n)
	}
	if got := metrics.Histogram(metrics.StepAdvanceDuration).Count; got != 3 {
		t.Fatalf("step_advance samples = %d, want 3", got)
	}
}

func TestProgressReportClearsComplianceStreak(t *testing.T) {
	metrics.Reset()
	t.Cleanup(metrics.Reset)

	s, _, pm, developer, project := seedWorkflowState(t)
	s.log = zap.NewNop()

	task, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "T0.11 compliance streak",
		Description: "验证真实回报会清零合规哨兵计数",
		Todos: []TaskCreateTodoInput{
			{Title: "阶段一", Description: "d", AssigneeNodeID: developer.NodeID},
			{Title: "阶段二", Description: "d", AssigneeNodeID: developer.NodeID},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}

	// Stage the pipeline so step 2 is stalled and the platform must advance it.
	// The task itself must be in_progress: CreateTaskByPMNode leaves it pending
	// until the first step is actually dispatched.
	dispatchedAt := time.Now().UTC().Add(-2 * time.Minute)
	doneAt := time.Now().UTC().Add(-5 * time.Minute)
	s.mu.Lock()
	s.tasks[task.ID].Status = "in_progress"
	s.tasks[task.ID].Todos[0].Status = "done"
	s.tasks[task.ID].Todos[0].CompletedAt = &doneAt
	s.tasks[task.ID].Todos[1].DispatchAttempts = 1
	s.tasks[task.ID].Todos[1].LastDispatchAt = &dispatchedAt
	s.mu.Unlock()

	s.advanceStalledPendingTodos()

	s.mu.RLock()
	streak := s.autoAdvanceStreakUnsafe(task.ID, developer.ID)
	s.mu.RUnlock()
	if streak != 1 {
		t.Fatalf("streak after one auto-advance = %d, want 1", streak)
	}

	// The agent finally reports progress on the promoted step.
	if _, appErr := s.UpdateTodoProgressByNode(developer.NodeID, TodoProgressInput{
		TaskID:  task.ID,
		TodoID:  task.Todos[1].ID,
		Message: "分镜拆解进行中",
	}); appErr != nil {
		t.Fatalf("todo.progress: %v", appErr)
	}

	s.mu.RLock()
	streak = s.autoAdvanceStreakUnsafe(task.ID, developer.ID)
	s.mu.RUnlock()
	if streak != 0 {
		t.Fatalf("streak after a real progress report = %d, want 0 (sentinel must reset)", streak)
	}
}

func TestPruneAutoAdvanceStreaksDropsFinishedTasks(t *testing.T) {
	s := New()
	s.log = zap.NewNop()

	live := &model.TaskDetail{ID: "t-live", UserID: "u1", ProjectID: "p1", Status: "in_progress"}
	dead := &model.TaskDetail{ID: "t-dead", UserID: "u1", ProjectID: "p1", Status: "done"}
	s.tasks[live.ID] = live
	s.tasks[dead.ID] = dead

	s.mu.Lock()
	s.bumpAutoAdvanceStreakUnsafe(live.ID, "a1")
	s.bumpAutoAdvanceStreakUnsafe(dead.ID, "a1")
	s.pruneAutoAdvanceStreaksUnsafe()
	liveStreak := s.autoAdvanceStreakUnsafe(live.ID, "a1")
	deadStreak := s.autoAdvanceStreakUnsafe(dead.ID, "a1")
	s.mu.Unlock()

	if liveStreak != 1 {
		t.Fatalf("in_progress task streak = %d, want 1 (must survive pruning)", liveStreak)
	}
	if deadStreak != 0 {
		t.Fatalf("finished task streak = %d, want 0 (must be pruned)", deadStreak)
	}
}
