package store

import (
	"context"
	"time"

	"go.uber.org/zap"
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
