package store

import (
	"testing"
	"time"

	"trustmesh/backend/internal/model"
)

func TestNotificationJoinRequestReceived(t *testing.T) {
	s := New()
	now := time.Now().UTC()

	content := "Agent「测试员」申请加入平台"
	event := &model.Event{
		ID:        "evt-join-1",
		UserID:    "user-1",
		EventType: "join_request_received",
		ActorType: "agent",
		ActorID:   "node-abc",
		ActorName: "测试员",
		Content:   &content,
		Metadata:  map[string]any{"join_request_id": "jr-1"},
		CreatedAt: now,
	}
	s.maybeCreateNotificationUnsafe(event)

	ids := s.userNotifications["user-1"]
	if len(ids) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(ids))
	}
	n := s.notifications[ids[0]]
	if n.Title != "数字员工入职申请" {
		t.Errorf("title = %q, want 数字员工入职申请", n.Title)
	}
	if n.Category != "agent" {
		t.Errorf("category = %q, want agent", n.Category)
	}
	if n.Priority != "high" {
		t.Errorf("priority = %q, want high", n.Priority)
	}
	if n.ActorID != "node-abc" {
		t.Errorf("actor_id = %q, want node-abc", n.ActorID)
	}
	if n.ActorName != "测试员" {
		t.Errorf("actor_name = %q, want 测试员", n.ActorName)
	}
	if n.Body != content {
		t.Errorf("body = %q, want %q", n.Body, content)
	}
}

// TestNotificationNoisyEventsAreNotNotified —— 通知收窄（2026-09-24 用户口径）。
//
// 为什么专门钉死它：这些事件**原先每条都生成通知**，结果是通知在广播"平台的机械动作"
// （执行侧进度、总控反复派发、自动重试、数字员工上下线、用户自己点的重开），
// 把真正待处理的那几条淹掉了。收窄后它们**仍然进任务事件流**（前端「执行过程」里看得到），
// 只是不再变成通知 —— 这条区分很关键，别日后有人"顺手"把它们加回来。
func TestNotificationNoisyEventsAreNotNotified(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name      string
		eventType string
		metadata  map[string]any
	}{
		{"Todo 失败（有任务级 task_status_changed.failed 兜底）", "todo_failed", map[string]any{"todo_title": "剧本初稿"}},
		{"派发失败（后台对账每 2min 自动补派）", "todo_dispatch_failed", map[string]any{"todo_title": "剧本初稿"}},
		{"执行预算耗尽（诊断信号，真卡死由 remind_escalated 兜底）", "todo_run_budget_exhausted", map[string]any{"todo_title": "剧本初稿"}},
		{"数字员工上线", "agent_status_changed", map[string]any{"new_status": "online"}},
		{"数字员工离线", "agent_status_changed", map[string]any{"new_status": "offline"}},
		{"数字员工忙碌", "agent_status_changed", map[string]any{"new_status": "busy"}},
		{"PM 纯文字回复（内容已在任务对话里，没挂澄清卡）", "planning_reply", map[string]any{"needs_user_input": false}},
		{"任务评论（任何 agent 评论都推太吵）", "task_comment", map[string]any{"task_title": "写剧本"}},
		{"Todo 重新开启（用户自己点的）", "todo_reopened", map[string]any{"todo_title": "剧本初稿"}},
		{"按用户要求重试（用户自己点的）", "todo_resumed", map[string]any{"todo_title": "剧本初稿", "reason": "重跑"}},
		{"产出被退回重做（重做超限时另有通知）", "todo_rework_requested", map[string]any{"todo_title": "剧本初稿"}},
		{"产出通过审核（好消息，不需要打断）", "todo_review_approved", map[string]any{"todo_title": "剧本初稿"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := New()
			content := "some content"
			s.maybeCreateNotificationUnsafe(&model.Event{
				ID: "e-" + tc.eventType, UserID: "user-1", EventType: tc.eventType,
				ActorType: "agent", ActorID: "a-1", ActorName: "数字员工",
				Content: &content, Metadata: tc.metadata, CreatedAt: now,
			})
			if got := len(s.userNotifications["user-1"]); got != 0 {
				t.Fatalf("收窄后该事件不应生成通知，实得 %d 条", got)
			}
		})
	}
}

// TestNotificationActionableTaskEvents —— 保留清单里每个事件都要有可读的中文标题与正文。
func TestNotificationActionableTaskEvents(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name      string
		event     *model.Event
		wantTitle string
	}{
		{
			name: "task_plan_ready",
			event: &model.Event{
				ID: "e-p1", UserID: "user-1", EventType: "task_plan_ready",
				ActorType: "agent", ActorID: "pm-1", ActorName: "PM 数字员工",
				Metadata:  map[string]any{"task_title": "写剧本"},
				CreatedAt: now,
			},
			wantTitle: "任务规划完成",
		},
		{
			name: "todo_awaiting_review",
			event: &model.Event{
				ID: "e-r1", UserID: "user-1", EventType: "todo_awaiting_review",
				ActorType: "agent", ActorID: "a-1", ActorName: "执行数字员工",
				Metadata:  map[string]any{"todo_title": "分镜拆解"},
				CreatedAt: now,
			},
			wantTitle: "待人工确认",
		},
		{
			name: "todo_ask_received",
			event: &model.Event{
				ID: "e-q1", UserID: "user-1", EventType: "todo_ask_received",
				ActorType: "agent", ActorID: "a-1", ActorName: "执行数字员工",
				Metadata:  map[string]any{"question": "需要确认方向"},
				CreatedAt: now,
			},
			wantTitle: "数字员工提问",
		},
		{
			name: "todo_hard_deadline_failed",
			event: &model.Event{
				ID: "e-h1", UserID: "user-1", EventType: "todo_hard_deadline_failed",
				ActorType: "system", ActorID: "timeout-monitor", ActorName: "平台",
				Metadata:  map[string]any{"todo_title": "剧本初稿"},
				CreatedAt: now,
			},
			wantTitle: "任务长时间无有效进展",
		},
		{
			name: "todo_remind_escalated",
			event: &model.Event{
				ID: "e-s1", UserID: "user-1", EventType: "todo_remind_escalated",
				ActorType: "system", ActorID: "timeout-monitor", ActorName: "平台",
				Metadata:  map[string]any{"todo_title": "剧本初稿"},
				CreatedAt: now,
			},
			wantTitle: "任务疑似卡死，请人工介入",
		},
		{
			name: "todo_rework_exhausted",
			event: &model.Event{
				ID: "e-x1", UserID: "user-1", EventType: "todo_rework_exhausted",
				ActorType: "system", ActorID: "reviewer", ActorName: "人工确认",
				Metadata:  map[string]any{"todo_title": "剧本初稿"},
				CreatedAt: now,
			},
			wantTitle: "产出重做超限",
		},
		{
			name: "planning_reply（挂了澄清卡）",
			event: &model.Event{
				ID: "e-pc1", UserID: "user-1", EventType: "planning_reply",
				ActorType: "agent", ActorID: "pm-1", ActorName: "PM 数字员工",
				Metadata:  map[string]any{"task_title": "写剧本", "needs_user_input": true},
				CreatedAt: now,
			},
			wantTitle: "规划待澄清",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := New()
			s.maybeCreateNotificationUnsafe(tc.event)
			ids := s.userNotifications["user-1"]
			if len(ids) != 1 {
				t.Fatalf("expected 1 notification, got %d", len(ids))
			}
			n := s.notifications[ids[0]]
			if n.Title != tc.wantTitle {
				t.Errorf("title = %q, want %q", n.Title, tc.wantTitle)
			}
			if n.Category == "" {
				t.Errorf("category should not be empty")
			}
			if n.Body == "" {
				t.Errorf("body should not be empty")
			}
		})
	}
}

// TestNotificationSourceEvent 钉死「事件 → SourceEvent」的映射。
//
// 为什么要专门测它：桌面端系统通知与收件箱**按 SourceEvent 白名单**决定显不显示（不再按中文标题），
// 所以这个字符串是**跨端的机器接口**：后端悄悄改名/改写法，客户端会静默失配（不报错、只是不显示），
// 没有任何编译期能拦。这里把「会生成通知」的键 + 「收窄掉/不该生成」的场景固化成契约。
//
// ⚠️ 与前端 `frontend-v2/src/lib/notifications.ts` 的 `DESKTOP_NOTIFY_SOURCES` 是**配对契约**，
// 前端还有一条测试直接读本源码断言白名单键存在 —— 改动必须两侧同时做。
func TestNotificationSourceEvent(t *testing.T) {
	now := time.Now().UTC()
	done := "任务「写剧本」已完成"
	failed := "任务「写剧本」失败"
	canceled := "任务「写剧本」已取消"

	cases := []struct {
		name       string
		eventType  string
		metadata   map[string]any
		content    *string
		wantSource string
		wantNotify bool
	}{
		// —— 保留清单：会生成通知，且 SourceEvent 必须可读 ——
		{"任务完成", "task_status_changed", map[string]any{"to": "done"}, &done, "task_status_changed.done", true},
		{"任务失败", "task_status_changed", map[string]any{"to": "failed"}, &failed, "task_status_changed.failed", true},
		{"任务取消", "task_status_changed", map[string]any{"to": "canceled"}, &canceled, "task_status_changed.canceled", true},
		// 硬超时判失败：timeout_monitor 直接改 task.Status、**不**发 task_status_changed，
		// 因此这条键是「任务失败」在该路径下的唯一信号，必须保留。
		{"硬超时失败", "todo_hard_deadline_failed", map[string]any{"todo_title": "剧本初稿"}, nil, "todo_hard_deadline_failed", true},
		{"疑似卡死", "todo_remind_escalated", map[string]any{"todo_title": "剧本初稿"}, nil, "todo_remind_escalated", true},
		{"重做超限", "todo_rework_exhausted", map[string]any{"todo_title": "剧本初稿"}, nil, "todo_rework_exhausted", true},
		{"成果待确认", "todo_awaiting_review", map[string]any{"todo_title": "分镜拆解"}, nil, "todo_awaiting_review", true},
		{"执行提问", "todo_ask_received", map[string]any{"question": "方向 A 还是 B？"}, nil, "todo_ask_received", true},
		{"方案确认", "task_plan_ready", map[string]any{"task_title": "写剧本"}, nil, "task_plan_ready", true},
		{"入职申请", "join_request_received", map[string]any{"join_request_id": "jr-1"}, nil, "join_request_received", true},
		// 规划待澄清：同一事件类型下的**条件通知** —— 挂了澄清卡才通知，来源键带 `needs_input` 后缀。
		// 用后缀而不是另造一个键名，是因为前端契约测试要求「白名单键的基类型必须能挂到后端
		// switch 的既有 case 上」（`desktop-notification-contract.test.ts`），另造键名会失配。
		{"规划待澄清", "planning_reply", map[string]any{"task_title": "写剧本", "needs_user_input": true}, nil, "planning_reply.needs_input", true},

		// —— 收窄掉的机械噪声：压根不该生成通知（连 SourceEvent 都不该有）——
		{"Todo 失败已收窄", "todo_failed", map[string]any{"todo_title": "剧本初稿"}, nil, "", false},
		{"派发失败已收窄", "todo_dispatch_failed", map[string]any{"todo_title": "剧本初稿"}, nil, "", false},
		{"预算耗尽已收窄", "todo_run_budget_exhausted", map[string]any{"todo_title": "剧本初稿"}, nil, "", false},
		{"数字员工上下线已收窄", "agent_status_changed", map[string]any{"new_status": "online"}, nil, "", false},
		{"PM 纯文字回复已收窄", "planning_reply", map[string]any{"needs_user_input": false}, nil, "", false},
		{"PM 回复缺 needs_user_input 也不通知", "planning_reply", map[string]any{}, nil, "", false},
		{"任务评论已收窄", "task_comment", map[string]any{}, nil, "", false},
		{"重新开启已收窄", "todo_reopened", map[string]any{"todo_title": "剧本初稿"}, nil, "", false},
		{"按需重试已收窄", "todo_resumed", map[string]any{"todo_title": "剧本初稿"}, nil, "", false},
		{"退回重做已收窄", "todo_rework_requested", map[string]any{"todo_title": "剧本初稿"}, nil, "", false},
		{"审核通过已收窄", "todo_review_approved", map[string]any{"todo_title": "剧本初稿"}, nil, "", false},

		// —— 压根不生成通知的情况 ——
		{"中间态不通知", "task_status_changed", map[string]any{"to": "running"}, nil, "", false},
		{"缺 to 不通知", "task_status_changed", map[string]any{}, nil, "", false},
		{"未映射事件不通知", "totally_unknown_event", map[string]any{}, nil, "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := New()
			s.maybeCreateNotificationUnsafe(&model.Event{
				ID: "e-" + tc.name, UserID: "user-1", EventType: tc.eventType,
				ActorType: "agent", ActorID: "a-1", ActorName: "数字员工",
				Content: tc.content, Metadata: tc.metadata, CreatedAt: now,
			})
			ids := s.userNotifications["user-1"]
			if !tc.wantNotify {
				if len(ids) != 0 {
					t.Fatalf("不该生成通知，实得 %d 条", len(ids))
				}
				return
			}
			if len(ids) != 1 {
				t.Fatalf("expected 1 notification, got %d", len(ids))
			}
			n := s.notifications[ids[0]]
			if n.SourceEvent != tc.wantSource {
				t.Errorf("source_event = %q, want %q", n.SourceEvent, tc.wantSource)
			}
			if n.SourceEvent == "" {
				t.Errorf("生成的通知必须带 SourceEvent（客户端靠它判类）")
			}
		})
	}
}
