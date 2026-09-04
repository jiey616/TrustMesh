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
