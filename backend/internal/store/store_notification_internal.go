package store

import "trustmesh/backend/internal/model"

func (s *Store) maybeCreateNotificationUnsafe(event *model.Event) {
	var title, body, category, priority string
	// terminalStatus 非空表示这是 task_status_changed 的某个终态（done/failed/canceled），
	// 只用于拼 SourceEvent —— 客户端靠它区分「完成」与「失败/取消」。
	var terminalStatus string
	// sourceEventSuffix 非空表示要给 SourceEvent 加后缀，用于区分**同一事件类型下的不同场景**
	// （目前只有 planning_reply：纯文字回复不通知，挂了澄清卡的才通知）。
	var sourceEventSuffix string
	// 🔴 收窄原则（2026-09-24 用户口径）：通知的**唯一职责是"要人做决定"**。
	//    平台的机械动作（执行侧进度、总控反复派发、自动重试、agent 上下线、状态的中间态）
	//    不改变"用户需要做什么"，只会淹掉真正待处理的那几条 ⇒ **一律不生成通知**。
	//
	//    因此刻意**不处理**下面这些事件（它们仍会进任务事件流，只是不再变成通知）：
	//      todo_progress / todo_failed / todo_dispatch_failed / todo_run_budget_exhausted
	//      todo_reopened / todo_resumed / todo_rework_requested / todo_review_approved
	//      task_comment / agent_status_changed
	//    ⚠️ 其中 todo_dispatch_failed 曾被刻意"可见"（P-01：静默派发失败会卡住整条流水线），
	//       现在由后台对账每 2min 自动补派 + 任务级 task_status_changed.failed 兜底，故收窄。
	//
	//    ⚠️ planning_reply 是**条件通知**（唯一的例外）：PM 的纯文字回复（"收到，我来分析"）
	//       内容已在任务对话里，不打扰；但**挂了 ui_blocks 澄清卡**时属于"等你填表"，
	//       必须通知，否则用户离开页面后就不知道 PM 在等自己。判据是事件 metadata 的
	//       `needs_user_input`（机器可读标量，别拿 `ui_blocks` 判 —— 它的类型在
	//       内存态与 Mongo 回读态之间会退化），来源键用 `planning_reply.needs_input`。
	//
	// 🔴 红线：**绝不能砍"需要人介入"的失败信号**。硬超时那条路径（timeout_monitor）
	//    跳过 updateTaskStatusUnsafe、不发 task_status_changed，所以 todo_hard_deadline_failed
	//    是「任务失败」在该路径下的**唯一**信号，必须保留。
	//
	//    保留清单（同时是桌面端弹横幅的白名单来源）：
	//      task_status_changed(done/failed/canceled)、todo_hard_deadline_failed、
	//      todo_remind_escalated、todo_rework_exhausted、todo_awaiting_review、
	//      todo_ask_received、task_plan_ready、join_request_received、
	//      planning_reply.needs_input（规划待澄清）
	switch event.EventType {
	case "task_status_changed":
		// 只通知终态（done / failed / canceled），中间状态不通知
		to, ok := event.Metadata["to"].(string)
		if !ok {
			return
		}
		terminalStatus = to
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
		category = "task"
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
	case "join_request_received":
		// 数字员工入职申请：提示用户前往审批
		title = "数字员工入职申请"
		body = stringOrDefault(event.Content, "新的数字员工申请加入平台")
		category = "agent"
		priority = "high"
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
	case "planning_reply":
		// 规划阶段的 PM 回复：**只在挂了澄清卡（ui_blocks）时才通知**。
		// 纯文字回复（"收到，我来分析"）内容已在任务对话里，通知只会是噪声 —— 正是
		// 2026-09-24 收窄时砍掉的那批。但「PM 抛卡等你填」是"要人做决定"，必须通知。
		// 判据用 metadata 的 needs_user_input（见 store_planning.go 同名字段的说明）。
		if needsInput, _ := event.Metadata["needs_user_input"].(bool); !needsInput {
			return
		}
		title = "规划待澄清"
		if t, ok := event.Metadata["task_title"].(string); ok && t != "" {
			body = "「" + t + "」PM 在规划阶段有问题等你回答"
		} else {
			body = stringOrDefault(event.Content, "PM 在规划阶段有问题等你回答")
		}
		category = "task"
		priority = "high"
		sourceEventSuffix = "needs_input"
	default:
		return
	}

	// 用户自己触发的操作不通知自己
	if event.ActorType == "user" && event.ActorID == event.UserID {
		return
	}

	// 机器可读来源键：客户端据此判类，不依赖中文标题。
	// 上面 switch 里每个 case 都与事件类型一一对应，只有两处需要加后缀：
	//   - task_status_changed 需要带上终态（它的三个终态共用一个事件类型，不带就分不出
	//     「完成」与「失败/取消」）；
	//   - planning_reply 需要带上 needs_input（同一事件类型下"纯闲聊"不通知、"等你填"才通知，
	//     后端 switch 的基类型对不上具体场景，客户端靠后缀区分）。
	sourceEvent := event.EventType
	switch {
	case terminalStatus != "":
		sourceEvent = event.EventType + "." + terminalStatus
	case sourceEventSuffix != "":
		sourceEvent = event.EventType + "." + sourceEventSuffix
	}

	now := event.CreatedAt
	notification := &model.Notification{
		ID:          newID(),
		UserID:      event.UserID,
		OrgID:       s.personalOrgOfUnsafe(event.UserID),
		EventID:     event.ID,
		ProjectID:   event.ProjectID,
		TaskID:      event.TaskID,
		ActorType:   event.ActorType,
		ActorID:     event.ActorID,
		ActorName:   event.ActorName,
		Title:       title,
		Body:        body,
		Category:    category,
		Priority:    priority,
		SourceEvent: sourceEvent,
		IsRead:      false,
		ReadAt:      nil,
		CreatedAt:   now,
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
