package store

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/metrics"
)

// Dispatch reconciler configuration.
const (
	defaultDispatchReconcileInterval = 2 * time.Minute
	// dispatchReconcileGrace is how long a pending todo may sit before the
	// reconciler treats it as a failed dispatch. It guards against racing the
	// normal synchronous dispatch that follows task creation.
	dispatchReconcileGrace = 90 * time.Second
)

// StartDispatchReconciler periodically re-dispatches todos that should have
// been dispatched but were not. It is the safety net for every silent failure
// mode in dispatchNextTodo (transient publish errors, process restarts,
// network partitions): instead of enumerating causes, it simply asks
// "is there a dispatchable todo that never got dispatched?" and retries.
//
// Idempotency: RetryDispatch refuses non-pending todos and
// RecordSequentialTodoDispatch rejects non-pending ones too, so a duplicate
// attempt is a no-op. It reuses the same dispatchHook the timeout monitor uses
// (store must not import clawsynapse), which is invoked outside the store lock.
func (s *Store) StartDispatchReconciler(ctx context.Context) {
	interval := defaultDispatchReconcileInterval
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	if s.log != nil {
		s.log.Info("dispatch reconciler started", zap.Duration("interval", interval))
	}
	for {
		select {
		case <-ctx.Done():
			if s.log != nil {
				s.log.Info("dispatch reconciler stopped")
			}
			return
		case <-ticker.C:
			// T3.1：多实例下仅 leader 执行（自愈推进 + 重派有外部副作用，N 实例会 N 倍派发）。
			// 门禁关闭时恒 true，单实例行为不变。
			if !s.isLeaderForBackground() {
				continue
			}
			// Advance first: promoting a stalled pending todo into in_progress
			// makes the redispatch pass below skip it (NextDispatchableTodo
			// returns nil once the leading todo is in_progress), so the two
			// stages never fight over the same todo.
			s.advanceStalledPendingTodos()
			s.reconcilePendingDispatches()
		}
	}
}

// reconcilePendingDispatches scans for in_progress tasks whose next todo is
// still pending past the grace window and re-dispatches it.
func (s *Store) reconcilePendingDispatches() {
	dispatch := s.dispatchHook
	if dispatch == nil {
		return
	}

	now := time.Now().UTC()

	type candidate struct{ taskID, todoID string }
	var cands []candidate

	s.mu.RLock()
	for _, task := range s.tasks {
		if task.Status != "in_progress" {
			continue
		}
		todo := task.NextDispatchableTodo()
		if todo == nil || todo.Status != "pending" {
			continue
		}
		// Reference time: last dispatch attempt if any, else creation time.
		// A never-attempted todo can still be a silent failure (e.g. the
		// process restarted before the synchronous dispatch ran).
		ref := todo.LastDispatchAt
		if ref == nil {
			created := todo.CreatedAt
			if created.IsZero() {
				created = task.CreatedAt
			}
			ref = &created
		}
		if now.Sub(*ref) < dispatchReconcileGrace {
			continue
		}
		cands = append(cands, candidate{taskID: task.ID, todoID: todo.ID})
	}
	s.mu.RUnlock()

	for _, c := range cands {
		if s.log != nil {
			s.log.Warn("dispatch reconcile: re-dispatching stalled pending todo",
				zap.String("task_id", c.taskID), zap.String("todo_id", c.todoID))
		}
		dispatch(context.Background(), c.taskID, c.todoID)
	}
}

// advanceStalledPendingTodos is the second stage of the dispatch reconciler. It
// closes the "pipeline stuck at step N" failure mode (T0.7c).
//
// Root cause it addresses: CompleteTodoByNode only finalizes the current todo
// and never advances the next one, while checkTodoTimeouts only scans
// in_progress todos (timeout_monitor.go). A todo left pending after its
// predecessor completed is therefore invisible to the timeout monitor, the
// P-03 hard-deadline gate and human escalation. Whenever the assignee agent
// never reports — e.g. its skill context was pruned and it silently stopped —
// the step sits pending forever. Redispatching alone does not help: an
// unresponsive agent stays unresponsive, so the todo is retried every tick
// without ever escalating.
//
// This stage promotes such a stalled pending todo into in_progress so it
// enters the monitoring window, then re-dispatches it. Promotion is gated on
// all of: (a) the task being in_progress; (b) the next dispatchable todo being
// pending with no incomplete predecessor; (c) at least one real dispatch
// attempt already made and the grace window elapsed since that attempt — so it
// never races the synchronous dispatch that follows task creation.
//
// Idempotent: only pending todos are promoted, so once promoted (in_progress)
// later ticks skip it. State mutation happens under the write lock; the
// dispatch hook is invoked outside it, mirroring reconcilePendingDispatches.
//
// Telemetry (T0.11): every promotion is a step-advance latency sample and a
// TodoAutoAdvanced tick; a repeat offender on the same agent also trips the
// agent compliance sentinel.
func (s *Store) advanceStalledPendingTodos() {
	now := time.Now().UTC()

	type advancedTodo struct{ taskID, todoID string }
	var advanced []advancedTodo

	s.mu.Lock()
	s.pruneAutoAdvanceStreaksUnsafe()
	for _, task := range s.tasks {
		if task.Status != "in_progress" {
			continue
		}
		todo := task.NextDispatchableTodo()
		if todo == nil || todo.Status != "pending" {
			continue
		}
		idx := findTodoIndex(task, todo.ID)
		if idx < 0 || hasIncompletePredecessor(task, idx) {
			continue
		}
		// Only act after a real dispatch attempt has aged out. Guards against
		// racing the synchronous dispatch that follows task creation: a todo
		// that was never attempted (DispatchAttempts == 0) is the reconciler's
		// redispatch stage's job, not ours.
		if todo.DispatchAttempts < 1 || todo.LastDispatchAt == nil {
			continue
		}
		if now.Sub(*todo.LastDispatchAt) < dispatchReconcileGrace {
			continue
		}

		todo.Status = "in_progress"
		todo.StartedAt = &now
		todo.AssignedAt = &now
		// Baseline the hard-deadline clock at promotion time so the todo is not
		// judged dead the instant it enters the monitoring window.
		todo.LastProgressAt = &now
		todo.LastActivityAt = &now

		// T0.11②: this promotion IS the step-advance event, so measure the gap
		// from the predecessor's completion.
		observeStepAdvanceUnsafe(task, idx, now)
		metrics.Inc(metrics.TodoAutoAdvancedTotal)

		// T0.11③: once the same agent needs consecutive auto-advances it stops
		// being "a lost report" and becomes a compliance problem worth a human.
		if s.bumpAutoAdvanceStreakUnsafe(task.ID, todo.Assignee.AgentID) {
			metrics.Inc(metrics.AgentStepStalledTotal)
			stallMsg := fmt.Sprintf("执行智能体 %s 连续 %d 步未回报，均由平台自愈推进，疑似 skill 被裁剪或上下文丢失，需人工核查该 agent",
				todo.Assignee.Name, agentStallStreakThreshold)
			s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, todo.ID,
				"system", "dispatch-reconciler", "派发器", "agent_step_stalled", &stallMsg, map[string]any{
					"agent_id":   todo.Assignee.AgentID,
					"agent_name": todo.Assignee.Name,
					"streak":     agentStallStreakThreshold,
					"task_title": task.Title,
					"todo_id":    todo.ID,
					"todo_title": todo.Title,
				}, now)
		}

		msg := fmt.Sprintf("todo auto-advanced after %s without an agent report: %s", dispatchReconcileGrace, todo.Title)
		s.addEventUnsafe(task.UserID, task.ProjectID, task.ID, todo.ID, "system", "dispatch-reconciler", "派发器", "todo_auto_advanced", &msg, map[string]any{
			"todo_id":           todo.ID,
			"task_title":        task.Title,
			"todo_title":        todo.Title,
			"dispatch_attempts": todo.DispatchAttempts,
		}, now)
		s.publishTaskUnsafe(task.ID)
		if err := s.persistTaskBundleUnsafe(task.ID); err != nil && s.log != nil {
			s.log.Warn("dispatch reconcile: persist failed after auto-advance",
				zap.String("task_id", task.ID), zap.String("todo_id", todo.ID), zap.Error(err))
		}
		advanced = append(advanced, advancedTodo{task.ID, todo.ID})
	}
	s.mu.Unlock()

	if len(advanced) == 0 {
		return
	}
	dispatch := s.dispatchHook
	if dispatch == nil {
		return
	}
	for _, c := range advanced {
		if s.log != nil {
			s.log.Warn("dispatch reconcile: auto-advanced stalled pending todo",
				zap.String("task_id", c.taskID), zap.String("todo_id", c.todoID))
		}
		dispatch(context.Background(), c.taskID, c.todoID)
	}
}
