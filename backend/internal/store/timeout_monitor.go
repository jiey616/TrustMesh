package store

import (
	"fmt"
	"context"
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
)

// Timeout monitor configuration.
const (
	timeoutCheckInterval     = 1 * time.Minute  // check every minute
	defaultTodoTimeout       = 30 * time.Minute // todo in_progress timeout
	defaultRemindInterval    = 10 * time.Minute // gap between timeout reminders
	defaultMaxReminders      = 3                // reminders with no response before failure
	defaultMeetingIdleTimeout = 30 * time.Minute // meeting idle (no new message) timeout
	defaultMaxRetries         = 3                // max retries before permanent failure
	defaultMaxReworks         = 3                // max reworks before permanent failure

	planningStallTimeout = 15 * time.Minute // planning 无进展（PM 欠回复）判定
	planningMaxReminders = 2                // planning 停滞最多自动催 PM 次数
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
			s.checkMeetingTimeouts()
			s.checkPlanningTimeouts()
		}
	}
}

// checkTodoTimeouts scans all tasks for in_progress todos that have timed out.
// Instead of auto-redispatching (which caused duplicate executions for
// long-running tasks), it sends a reminder to the assignee. If the assignee
// does not respond after defaultMaxReminders reminders, the todo is failed.
func (s *Store) checkTodoTimeouts() {
	s.mu.Lock()

	now := time.Now().UTC()
	cutoff := now.Add(-defaultTodoTimeout)
	timedOutCount := 0
	remindedCount := 0
	failedCount := 0
	type pendingRemind struct {
		taskID string
		todoID string
	}
	var reminds []pendingRemind

	for _, task := range s.tasks {
		if task.Status != "in_progress" && task.Status != "pending" {
			continue
		}

		for i := range task.Todos {
			todo := &task.Todos[i]
			if todo.Status != "in_progress" {
				continue
			}

			// Timeout is measured from the LAST activity (any todo.progress /
			// todo.complete / todo.fail) rather than AssignedAt. Long-running
			// tasks that keep reporting progress are therefore never spuriously
			// reset or reminded; only a todo that has been SILENT for the whole
			// timeout window is considered stuck.
			lastActivity := todo.LastActivityAt
			if lastActivity == nil {
				lastActivity = todo.AssignedAt
			}
			if lastActivity == nil || lastActivity.After(cutoff) {
				continue
			}

			timedOutCount++

			// Determine max reminders (default to 3 if not set)
			maxReminders := todo.MaxRetries
			if maxReminders == 0 {
				maxReminders = defaultMaxReminders
			}

			// If we already sent all reminders and the last one was sent at
			// least one remind-interval ago without any response: fail.
			if todo.RemindCount >= maxReminders {
				lastRemind := todo.RemindAt
				if lastRemind == nil || !lastRemind.Add(defaultRemindInterval).After(now) {
					failedAt := now
					todo.FailedAt = &failedAt
					todo.Status = "failed"
					errMsg := "执行超时且多次提醒无响应，判定任务失败"
					todo.Error = &errMsg
					failedCount++

					s.log.Warn("todo timed out, no response after reminders, marking failed",
						zap.String("task_id", task.ID),
						zap.String("todo_id", todo.ID),
						zap.Int("remind_count", todo.RemindCount),
					)

					s.addEventUnsafe(
						task.UserID, task.ProjectID, task.ID, todo.ID,
						"system", "timeout-monitor", "超时监控",
						"todo_timeout_failed", &errMsg,
						map[string]any{
							"remind_count": todo.RemindCount,
							"max_reminders": maxReminders,
						}, now,
					)
					continue
				}
				// RemindAt is still inside the interval window: wait for the
				// next check.
				continue
			}

			// Otherwise: send a reminder if enough time has passed since the
			// previous one.
			lastRemind := todo.RemindAt
			if lastRemind != nil && lastRemind.Add(defaultRemindInterval).After(now) {
				continue // reminder interval not yet elapsed
			}

			todo.RemindCount++
			todo.RemindAt = &now
			remindedCount++
			reminds = append(reminds, pendingRemind{taskID: task.ID, todoID: todo.ID})

			s.log.Warn("todo timed out, sending reminder",
				zap.String("task_id", task.ID),
				zap.String("todo_id", todo.ID),
				zap.Int("remind_count", todo.RemindCount),
				zap.Int("max_reminders", maxReminders),
			)

			errMsg := "执行超时，平台已提醒执行智能体（无响应将自动判失败）"
			s.addEventUnsafe(
				task.UserID, task.ProjectID, task.ID, todo.ID,
				"system", "timeout-monitor", "超时监控",
				"todo_timeout_remind", &errMsg,
				map[string]any{
					"remind_count":  todo.RemindCount,
					"max_reminders": maxReminders,
				}, now,
			)
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

	s.mu.Unlock()

	if timedOutCount > 0 {
		s.log.Info("timeout monitor check completed",
			zap.Int("timed_out", timedOutCount),
			zap.Int("reminded", remindedCount),
			zap.Int("failed", failedCount),
		)
	}

	// Send reminders outside the lock (the hook acquires the lock internally
	// via GetTaskInternal).
	for _, r := range reminds {
		if s.remindHook != nil {
			s.remindHook(context.Background(), r.taskID, r.todoID)
		}
	}
}

// checkPlanningTimeouts scans tasks stuck in planning with no finalized plan
// (todos == 0) where the PM owes a response — i.e. the LAST task message is a
// user message (or there are no messages at all). A task whose last message
// came from the PM is a clarification questionnaire waiting for the USER,
// which is normal idle and must NOT be nudged. Stalled tasks get up to
// planningMaxReminders automatic nudges (via planningStallHook, one per
// defaultRemindInterval); after the last one a system comment flags the task
// for human attention. We deliberately do NOT auto-fail planning tasks: a
// failed task here would most likely just be recreated by the user.
func (s *Store) checkPlanningTimeouts() {
	now := time.Now().UTC()
	cutoff := now.Add(-planningStallTimeout)

	var nudges []string
	var flagged []string

	s.mu.Lock()
	// Lazy cleanup: drop throttle state for tasks that left planning.
	for id := range s.planningStallCount {
		t, ok := s.tasks[id]
		if !ok || t.Status != "planning" {
			delete(s.planningStallCount, id)
			delete(s.planningStallLastRemind, id)
		}
	}
	for _, task := range s.tasks {
		if task.Status != "planning" || len(task.Todos) > 0 {
			continue
		}
		last := task.UpdatedAt
		if last.IsZero() {
			last = task.CreatedAt
		}
		if last.After(cutoff) {
			continue // recent activity, not stalled
		}
		// Trap guard: PM's last message is a reply/question → waiting for the
		// user to answer. That is normal idle; nudging the PM here would just
		// make it re-ask the same questionnaire.
		if n := len(task.Messages); n > 0 && task.Messages[n-1].Role == "pm_agent" {
			continue
		}
		count := s.planningStallCount[task.ID]
		if count >= planningMaxReminders {
			continue // already nudged max times and flagged
		}
		if lr, seen := s.planningStallLastRemind[task.ID]; seen && lr.Add(defaultRemindInterval).After(now) {
			continue // remind interval not yet elapsed
		}
		s.planningStallCount[task.ID] = count + 1
		s.planningStallLastRemind[task.ID] = now
		nudges = append(nudges, task.ID)
		if count+1 >= planningMaxReminders {
			flagged = append(flagged, task.ID)
		}
		if s.log != nil {
			s.log.Warn("planning stalled, sending nudge",
				zap.String("task_id", task.ID),
				zap.Int("nudge_count", count+1),
				zap.Int("max_reminders", planningMaxReminders),
			)
		}
	}
	s.mu.Unlock()

	for _, id := range nudges {
		if s.planningStallHook != nil {
			s.planningStallHook(context.Background(), id)
		}
	}
	for _, id := range flagged {
		comment := fmt.Sprintf("⚠️ 规划阶段停滞：PM 长时间未提交有效规划，平台已自动催办 %d 次仍无响应，已标记待人工介入。", planningMaxReminders)
		if _, err := s.AppendSystemTaskComment(id, comment); err != nil && s.log != nil {
			s.log.Warn("append planning-stall comment failed", zap.String("task_id", id), zap.Error(err))
		}
	}
}

// checkMeetingTimeouts force-concludes in_progress meetings that have been idle
// (no new meeting message) for longer than defaultMeetingIdleTimeout. This
// guarantees that every meeting eventually ends and produces minutes, even if
// the host agent (an external ClawSynapse node) never sends an explicit
// conclude/summary/minutes action. Minutes are auto-generated from the
// transcript when the host did not upload them.
//
// The store lock is released before calling GenerateMeetingMinutesFile /
// UpdateMeetingStatus / AddMeetingMessage (each locks internally) to avoid a
// re-entrant deadlock on the non-reentrant RWMutex.
func (s *Store) checkMeetingTimeouts() {
	now := time.Now().UTC()
	cutoff := now.Add(-defaultMeetingIdleTimeout)

	// Phase 1 (locked): collect IDs of in_progress meetings idle beyond cutoff.
	s.mu.RLock()
	candidates := make([]string, 0)
	for id, m := range s.meetings {
		if m.Status != model.MeetingInProgress {
			continue
		}
		ids := s.meetingMessageIndex[id]
		if len(ids) == 0 {
			// No messages at all: only conclude if idle since creation.
			if m.CreatedAt.After(cutoff) {
				continue
			}
		} else {
			last := s.meetingMessages[ids[len(ids)-1]]
			if last == nil || last.CreatedAt.After(cutoff) {
				continue
			}
		}
		candidates = append(candidates, id)
	}
	s.mu.RUnlock()

	if len(candidates) == 0 {
		return
	}

	// Phase 2 (unlocked): conclude + auto-generate minutes.
	concluded := 0
	minutesGenerated := 0
	for _, id := range candidates {
		meeting, appErr := s.GetMeeting(SystemScope(), id)
		if appErr != nil || meeting == nil {
			continue
		}
		if meeting.Status != model.MeetingInProgress {
			continue // changed concurrently
		}

		if meeting.MinutesFileID == "" {
			if fileID, err := s.GenerateMeetingMinutesFile(SystemScope(), meeting); err != nil {
				s.log.Warn("failed to auto-generate meeting minutes",
					zap.String("meeting_id", id), zap.Error(err))
			} else if fileID != "" {
				minutesGenerated++
				_, _ = s.AddMeetingMessage(SystemScope(), &model.MeetingMessage{
					MeetingID:  id,
					SenderType: "system",
					SenderID:   "system",
					SenderName: "系统",
					Content:    "📄 会议空闲超时，已自动结束并生成会议纪要（主持人未主动上传）。",
				})
			}
		}

		if err := s.UpdateMeetingStatus(SystemScope(), id, model.MeetingCompleted); err == nil {
			concluded++
			s.log.Warn("meeting idle timeout, auto-concluded",
				zap.String("meeting_id", id),
				zap.String("title", meeting.Title),
				zap.Bool("minutes_generated", meeting.MinutesFileID != ""),
			)
		}
	}

	if concluded > 0 {
		s.log.Info("meeting timeout monitor check completed",
			zap.Int("concluded", concluded),
			zap.Int("minutes_generated", minutesGenerated),
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
			lastActivity := todo.LastActivityAt
			if lastActivity == nil {
				lastActivity = todo.AssignedAt
			}
			if lastActivity != nil && lastActivity.Before(cutoff) {
				result = append(result, todo)
			}
		}
	}
	return result
}
