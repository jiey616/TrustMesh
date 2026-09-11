package store

import (
	"fmt"
	"context"
	"os"
	"strings"
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

// Hard-deadline gate (P-03). The reminder-counter path can be bypassed by
// various liveness semantics; this gate is a wall-clock backstop that fails a
// todo which has made NO productive progress for too long regardless of how
// much activity it reported in between.
//
// 🔴 Threshold is per-kind (v2): a fixed 4h would kill long video-generation
// steps (measured >4h in production). Normal steps get 4h, heavy steps 8h.
// Overridable via TODO_HARD_DEADLINE (normal) / TODO_HARD_DEADLINE_HEAVY
// (heavy). Must stay above the node-side adapter timeout (40m) and the longest
// real step, otherwise healthy tasks get killed.
const (
	defaultHardDeadline      = 4 * time.Hour
	defaultHardDeadlineHeavy = 8 * time.Hour
)

// defaultHeavyKeywords marks long-running steps when no explicit kind field
// exists on Todo. Matched case-insensitively against title+description.
// Override the whole list with TODO_HEAVY_KEYWORDS (comma-separated).
var defaultHeavyKeywords = []string{"视频", "video", "渲染", "render", "成片", "合成", "视频生成"}

func isHeavyTodo(todo *model.Todo) bool {
	if todo == nil {
		return false
	}
	hay := strings.ToLower(todo.Title + " " + todo.Description)
	if kws := strings.TrimSpace(os.Getenv("TODO_HEAVY_KEYWORDS")); kws != "" {
		for _, k := range strings.Split(kws, ",") {
			k = strings.ToLower(strings.TrimSpace(k))
			if k != "" && strings.Contains(hay, k) {
				return true
			}
		}
		return false
	}
	for _, k := range defaultHeavyKeywords {
		if strings.Contains(hay, strings.ToLower(k)) {
			return true
		}
	}
	return false
}

// hardDeadlineFor returns the "no productive progress" wall-clock limit for a
// todo. Heavy steps are evaluated first so a global TODO_HARD_DEADLINE cannot
// accidentally kill a long video step (the whole point of the per-kind split).
func hardDeadlineFor(todo *model.Todo) time.Duration {
	if isHeavyTodo(todo) {
		if v := strings.TrimSpace(os.Getenv("TODO_HARD_DEADLINE_HEAVY")); v != "" {
			if d, err := time.ParseDuration(v); err == nil && d > 0 {
				return d
			}
		}
		return defaultHardDeadlineHeavy
	}
	if v := strings.TrimSpace(os.Getenv("TODO_HARD_DEADLINE")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultHardDeadline
}


// remindBackoff returns how long to wait after `count` reminders have already
// been sent before sending the next one.
//
// Why escalating instead of the old flat defaultRemindInterval: a fixed
// 10-minute cadence bombarded a stalled agent 13 times across 8 hours
// (2026-09-10 TD_05) without ever escalating anywhere. It neither gave the
// agent time to finish a genuinely long step, nor surfaced the stall to a
// human. Escalating keeps the first nudge urgent while later ones stop
// spamming.
//
// The 2h cap is deliberate: cumulative wait (0 + 15m + 30m + 1h = 1h45m to
// reach the failure verdict) must stay well INSIDE hardDeadlineFor (4h
// normal / 8h heavy). If the backoff overshot the hard gate, the hard gate
// would fail the todo first and the reminder path would never escalate.
func remindBackoff(count int) time.Duration {
	switch {
	case count <= 0:
		return 0 // nothing sent yet: nudge as soon as the timeout elapses
	case count == 1:
		return 15 * time.Minute
	case count == 2:
		return 30 * time.Minute
	case count == 3:
		return 1 * time.Hour
	default:
		return 2 * time.Hour
	}
}

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
	hardFailedCount := 0
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

			// ── 硬闸（P-03）：长时间「有活动但无有效进展」直接判失败 ──
			// 计数器路径（RemindCount >= max）可能被各种续命语义绕过；
			// 这里用挂钟兜底：只要超过 hardDeadline 没有任何有效进展
			// （无 todo.progress、无产物、无有效评论），无论期间有多少
			// 活动一律判失败。放在最前面以真正独立于计数器。
			progressAt := todo.LastProgressAt
			if progressAt == nil {
				progressAt = todo.StartedAt
			}
			if progressAt != nil {
				if limit := hardDeadlineFor(todo); now.Sub(*progressAt) >= limit {
					failedAt := now
					todo.FailedAt = &failedAt
					todo.Status = "failed"
					errMsg := fmt.Sprintf("执行超过 %s 无任何有效进展（无进度上报、无产物），判定失败", limit)
					todo.Error = &errMsg
					failedCount++
					hardFailedCount++

					if s.log != nil {
						s.log.Warn("todo exceeded hard deadline without progress, marking failed",
							zap.String("task_id", task.ID),
							zap.String("todo_id", todo.ID),
							zap.Duration("since_progress", now.Sub(*progressAt)),
							zap.Duration("hard_deadline", limit),
						)
					}
					s.addEventUnsafe(
						task.UserID, task.ProjectID, task.ID, todo.ID,
						"system", "timeout-monitor", "超时监控",
						"todo_hard_deadline_failed", &errMsg,
						map[string]any{
							"since_progress": now.Sub(*progressAt).String(),
							"hard_deadline":  limit.String(),
							"todo_title":     todo.Title,
						}, now,
					)
					continue
				}
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
				if lastRemind == nil || !lastRemind.Add(remindBackoff(todo.RemindCount)).After(now) {
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
				// P-08: the reminder budget is spent. Escalate to a human-facing
				// alert instead of letting the todo sit in a remind loop: at this
				// point the agent has been nudged repeatedly with zero response,
				// which in practice means a silently wedged run (budget exhausted,
				// quota dead, or the process is gone) rather than a slow step.
				escalateMsg := "多次提醒无响应，疑似执行智能体静默卡死（run 预算耗尽 / 额度不足 / 进程异常），已停止提醒并判定失败，请人工介入"
				s.addEventUnsafe(
					task.UserID, task.ProjectID, task.ID, todo.ID,
					"system", "timeout-monitor", "超时监控",
					"todo_remind_escalated", &escalateMsg,
					map[string]any{
						"remind_count":  todo.RemindCount,
						"max_reminders": maxReminders,
						"todo_title":    todo.Title,
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
			if lastRemind != nil && lastRemind.Add(remindBackoff(todo.RemindCount)).After(now) {
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
		if timedOutCount > 0 || hardFailedCount > 0 {
			newStatus := aggregateTaskStatus(*task)
			if newStatus != task.Status {
				task.Status = newStatus
				task.UpdatedAt = now
			}
		}
	}

	s.mu.Unlock()

	if timedOutCount > 0 || hardFailedCount > 0 {
		s.log.Info("timeout monitor check completed",
			zap.Int("timed_out", timedOutCount),
			zap.Int("hard_deadline_failed", hardFailedCount),
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
