package store

import (
	"context"
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
)

// Timeout monitor configuration.
const (
	timeoutCheckInterval  = 1 * time.Minute  // check every minute
	defaultTodoTimeout    = 30 * time.Minute // todo in_progress timeout
	defaultMaxRetries     = 3                // max retries before permanent failure
)

// StartTimeoutMonitor runs a background goroutine that periodically scans for
// in_progress todos that have exceeded the timeout threshold. Timed-out todos
// are either retried (if retry count < max) or marked as failed.
func (s *Store) StartTimeoutMonitor(ctx context.Context) {
	ticker := time.NewTicker(timeoutCheckInterval)
	defer ticker.Stop()

	s.log.Info("timeout monitor started",
		zap.Duration("check_interval", timeoutCheckInterval),
		zap.Duration("todo_timeout", defaultTodoTimeout),
	)

	for {
		select {
		case <-ctx.Done():
			s.log.Info("timeout monitor stopped")
			return
		case <-ticker.C:
			s.checkTodoTimeouts()
		}
	}
}

// checkTodoTimeouts scans all tasks for in_progress todos that have timed out.
func (s *Store) checkTodoTimeouts() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	cutoff := now.Add(-defaultTodoTimeout)
	timedOutCount := 0
	retriedCount := 0
	failedCount := 0

	for _, task := range s.tasks {
		if task.Status != "in_progress" && task.Status != "pending" {
			continue
		}

		for i := range task.Todos {
			todo := &task.Todos[i]
			if todo.Status != "in_progress" {
				continue
			}

			// Check if todo has timed out
			if todo.AssignedAt == nil || todo.AssignedAt.After(cutoff) {
				continue
			}

			timedOutCount++

			// Determine max retries (default to 3 if not set)
			maxRetries := todo.MaxRetries
			if maxRetries == 0 {
				maxRetries = defaultMaxRetries
			}

			if todo.RetryCount < maxRetries {
				// Retry: reset to pending so it can be redispatched
				todo.RetryCount++
				todo.Status = "pending"
				todo.AssignedAt = nil
				todo.StartedAt = nil
				retriedCount++

				s.log.Warn("todo timed out, retrying",
					zap.String("task_id", task.ID),
					zap.String("todo_id", todo.ID),
					zap.Int("retry_count", todo.RetryCount),
					zap.Int("max_retries", maxRetries),
				)

				// Add event for retry
				errMsg := "执行超时，自动重试"
				s.addEventUnsafe(
					task.UserID, task.ProjectID, task.ID, todo.ID,
					"system", "timeout-monitor", "超时监控",
					"todo_timeout_retry", &errMsg,
					map[string]any{
						"retry_count": todo.RetryCount,
						"max_retries": maxRetries,
					}, now,
				)
			} else {
				// Max retries exceeded: mark as failed
				failedAt := now
				todo.FailedAt = &failedAt
				todo.Status = "failed"
				errMsg := "执行超时，已达最大重试次数"
				todo.Error = &errMsg
				failedCount++

				s.log.Warn("todo timed out, max retries exceeded, marking as failed",
					zap.String("task_id", task.ID),
					zap.String("todo_id", todo.ID),
					zap.Int("retry_count", todo.RetryCount),
				)

				// Add event for failure
				s.addEventUnsafe(
					task.UserID, task.ProjectID, task.ID, todo.ID,
					"system", "timeout-monitor", "超时监控",
					"todo_timeout_failed", &errMsg,
					map[string]any{
						"retry_count": todo.RetryCount,
						"max_retries": maxRetries,
					}, now,
				)
			}
		}

		// Re-aggregate task status after timeout handling
		if timedOutCount > 0 {
			newStatus := aggregateTaskStatus(*task)
			if newStatus != task.Status {
				task.Status = newStatus
				task.UpdatedAt = now
			}
		}
	}

	if timedOutCount > 0 {
		s.log.Info("timeout monitor check completed",
			zap.Int("timed_out", timedOutCount),
			zap.Int("retried", retriedCount),
			zap.Int("failed", failedCount),
		)
	}
}

// SetTodoAssignedAt records the time when a todo was assigned/dispatched.
// This should be called when a todo transitions to in_progress.
func (s *Store) SetTodoAssignedAt(taskID, todoID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return
	}
	for i := range task.Todos {
		if task.Todos[i].ID == todoID {
			now := time.Now().UTC()
			task.Todos[i].AssignedAt = &now
			return
		}
	}
}

// GetTimedOutTodos returns a list of todos that are currently in_progress
// and have exceeded the timeout threshold. This is primarily for debugging/monitoring.
func (s *Store) GetTimedOutTodos() []model.Todo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().UTC().Add(-defaultTodoTimeout)
	result := make([]model.Todo, 0)

	for _, task := range s.tasks {
		for _, todo := range task.Todos {
			if todo.Status != "in_progress" {
				continue
			}
			if todo.AssignedAt != nil && todo.AssignedAt.Before(cutoff) {
				result = append(result, todo)
			}
		}
	}
	return result
}
