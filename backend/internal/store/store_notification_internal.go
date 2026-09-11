package store

import "trustmesh/backend/internal/model"

func (s *Store) maybeCreateNotificationUnsafe(event *model.Event) {
	var title, body, category, priority string
	switch event.EventType {
	case "task_status_changed":
		// 只通知终态（done / failed / canceled），中间状态不通知
		if to, ok := event.Metadata["to"].(string); ok {
			switch to {
			case "done":
				title = "任务已完成"
				body = stringOrDefault(event.Content, "任务完成")
				priority = "medium"
			case "failed":
				title = "任务执行失败"
				body = stringOrDefault(event.Content, "任务失败")
				priority = "high"
			case "canceled":
				title = "任务已取消"
				body = stringOrDefault(event.Content, "任务被取消")
				priority = "medium"
			default:
				return
			}
		} else {
			return
		}
		category = "task"
	case "todo_failed":
		title = "Todo 执行失败"
		body = stringOrDefault(event.Content, "Todo 失败")
		category = "todo"
		priority = "high"
	case "todo_hard_deadline_failed":
		// P-03: todo ran past the wall-clock hard deadline with no productive
		// progress. Distinct from todo_failed so the UI can tell "died" from
		// "busy but never produced anything".
		title = "任务长时间无有效进展"
		if t, ok := event.Metadata["todo_title"].(string); ok && t != "" {
			body = "「" + t + "」长时间无有效进展（无进度上报、无产物），已判定失败"
		} else {
			body = stringOrDefault(event.Content, "任务长时间无有效进展，已判定失败")
		}
		category = "todo"
		priority = "high"
	case "todo_remind_escalated":
		// P-08: reminded repeatedly with zero response - the run is wedged,
		// not slow. Escalated so a human looks instead of the platform
		// nudging a corpse.
		title = "任务疑似卡死，请人工介入"
		if t, ok := event.Metadata["todo_title"].(string); ok && t != "" {
			body = "「" + t + "」多次提醒无响应，已判定失败；疑似执行智能体静默卡死（预算耗尽 / 额度不足 / 进程异常）"
		} else {
			body = stringOrDefault(event.Content, "任务多次提醒无响应，请人工介入")
		}
		category = "todo"
		priority = "high"
	case "todo_run_budget_exhausted":
		// P-08: the run was truncated by its tool-call budget before it could
		// report completion. Distinct from a plain failure so the user knows to
		// split the step rather than wait.
		title = "执行预算耗尽"
		body = stringOrDefault(event.Content, "执行侧 run 预算耗尽，执行被静默截断")
		category = "todo"
		priority = "high"
	case "todo_reopened":
		// P-05: a terminal todo was brought back so late work could land.
		// Medium priority - it is a recovery, not a failure, but the user
		// should know the state changed underneath them.
		title = "Todo 已重新开启"
		if t, ok := event.Metadata["todo_title"].(string); ok && t != "" {
			body = "「" + t + "」已重新开启（原为终态），等待执行结果"
		} else {
			body = stringOrDefault(event.Content, "Todo 已重新开启")
		}
		category = "todo"
		priority = "medium"
	case "todo_dispatch_failed":
		// P-01: automatic sequential dispatch failed (all retries exhausted).
		// Surfaced because a silent dispatch failure used to stall the whole
		// pipeline invisibly.
		title = "任务派发失败"
		if t, ok := event.Metadata["todo_title"].(string); ok && t != "" {
			body = "「" + t + "」自动派发失败，系统将自动重试；若持续失败请手动派发"
		} else {
			body = stringOrDefault(event.Content, "任务派发失败")
		}
		category = "todo"
		priority = "high"
	case "planning_reply":
		title = "PM 回复"
		body = stringOrDefault(event.Content, "新的回复")
		category = "task"
		priority = "medium"
	case "join_request_received":
		// 数字员工入职申请：提示用户前往审批
		title = "数字员工入职申请"
		body = stringOrDefault(event.Content, "新的数字员工申请加入平台")
		category = "agent"
		priority = "high"
	case "agent_status_changed":
		// 数字员工上线 / 离线 / 状态变化
		newStatus, _ := event.Metadata["new_status"].(string)
		switch newStatus {
		case "offline":
			title = "数字员工离线"
			body = stringOrDefault(event.Content, "数字员工已离线")
		case "online":
			title = "数字员工上线"
			body = stringOrDefault(event.Content, "数字员工已上线")
		case "busy":
			title = "数字员工忙碌"
			body = stringOrDefault(event.Content, "数字员工正在执行任务")
		default:
			title = "数字员工状态变化"
			body = stringOrDefault(event.Content, "数字员工状态已变更")
		}
		category = "agent"
		priority = "low"
	case "task_plan_ready":
		// 任务规划完成，等待用户确认
		title = "任务规划完成"
		if t, ok := event.Metadata["task_title"].(string); ok && t != "" {
			body = "「" + t + "」规划已完成，请确认后开始执行"
		} else {
			body = stringOrDefault(event.Content, "任务规划已完成，请确认")
		}
		category = "task"
		priority = "high"
	case "todo_awaiting_review":
		// 产出提交，等待人工确认
		title = "待人工确认"
		if t, ok := event.Metadata["todo_title"].(string); ok && t != "" {
			body = "「" + t + "」产出已提交，等待人工确认"
		} else {
			body = stringOrDefault(event.Content, "有产出等待人工确认")
		}
		category = "task"
		priority = "high"
	case "todo_ask_received":
		// 数字员工向用户提问
		title = "数字员工提问"
		if q, ok := event.Metadata["question"].(string); ok && q != "" {
			body = "数字员工向你提问：" + q
		} else {
			body = stringOrDefault(event.Content, "数字员工需要你的回答")
		}
		category = "task"
		priority = "high"
	case "todo_rework_requested":
		// 产出被退回重做
		title = "产出被退回重做"
		if t, ok := event.Metadata["todo_title"].(string); ok && t != "" {
			body = "「" + t + "」产出未通过，已退回重做"
		} else {
			body = stringOrDefault(event.Content, "有产出被退回重做")
		}
		category = "task"
		priority = "medium"
	case "todo_rework_exhausted":
		// 重做次数耗尽，需要人工介入
		title = "产出重做超限"
		if t, ok := event.Metadata["todo_title"].(string); ok && t != "" {
			body = "「" + t + "」多次重做仍未通过，请人工介入"
		} else {
			body = stringOrDefault(event.Content, "产出多次重做仍未通过")
		}
		category = "task"
		priority = "high"
	case "todo_review_approved":
		// 产出通过人工审核
		title = "产出已通过审核"
		if t, ok := event.Metadata["todo_title"].(string); ok && t != "" {
			body = "「" + t + "」已通过人工确认"
		} else {
			body = stringOrDefault(event.Content, "产出已通过人工确认")
		}
		category = "task"
		priority = "low"
	case "task_comment":
		// 任务新评论（agent 评论时通知用户；用户自己评论不通知）
		title = "任务评论"
		body = stringOrDefault(event.Content, "任务有新评论")
		category = "task"
		priority = "low"
	default:
		return
	}

	// 用户自己触发的操作不通知自己
	if event.ActorType == "user" && event.ActorID == event.UserID {
		return
	}

	now := event.CreatedAt
	notification := &model.Notification{
		ID:        newID(),
		UserID:    event.UserID,
		OrgID:     s.personalOrgOfUnsafe(event.UserID),
		EventID:   event.ID,
		ProjectID: event.ProjectID,
		TaskID:    event.TaskID,
		ActorType: event.ActorType,
		ActorID:   event.ActorID,
		ActorName: event.ActorName,
		Title:     title,
		Body:      body,
		Category:  category,
		Priority:  priority,
		IsRead:    false,
		ReadAt:    nil,
		CreatedAt: now,
	}
	s.notifications[notification.ID] = notification
	s.userNotifications[event.UserID] = append(s.userNotifications[event.UserID], notification.ID)
	s.persistNotificationUnsafe(notification)
	s.publishUserEventUnsafe(event.UserID, "notification.created", map[string]any{
		"notification": *notification,
		"unread_count": unreadNotificationCountUnsafe(s.notifications, s.userNotifications[event.UserID]),
	}, now)
}

func stringOrDefault(s *string, def string) string {
	if s != nil && *s != "" {
		return *s
	}
	return def
}
