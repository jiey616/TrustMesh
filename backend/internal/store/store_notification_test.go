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

func TestNotificationAgentStatusChanged(t *testing.T) {
	s := New()
	now := time.Now().UTC()

	// offline -> online 应生成"数字员工上线"
	content := "Agent 导演: offline -> online"
	event := &model.Event{
		ID:        "evt-status-1",
		UserID:    "user-1",
		EventType: "agent_status_changed",
		ActorType: "system",
		ActorID:   "agent-director",
		ActorName: "导演",
		Content:   &content,
		Metadata:  map[string]any{"prev_status": "offline", "new_status": "online"},
		CreatedAt: now,
	}
	s.maybeCreateNotificationUnsafe(event)

	ids := s.userNotifications["user-1"]
	if len(ids) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(ids))
	}
	n := s.notifications[ids[0]]
	if n.Title != "数字员工上线" {
		t.Errorf("title = %q, want 数字员工上线", n.Title)
	}
	if n.Category != "agent" {
		t.Errorf("category = %q, want agent", n.Category)
	}
	if n.ActorID != "agent-director" {
		t.Errorf("actor_id = %q, want agent-director", n.ActorID)
	}

	// online -> offline 应生成"数字员工离线"
	s2 := New()
	offlineContent := "Agent 导演: online -> offline"
	s2.maybeCreateNotificationUnsafe(&model.Event{
		ID:        "evt-status-2",
		UserID:    "user-2",
		EventType: "agent_status_changed",
		ActorType: "system",
		ActorID:   "agent-director",
		ActorName: "导演",
		Content:   &offlineContent,
		Metadata:  map[string]any{"prev_status": "online", "new_status": "offline"},
		CreatedAt: now,
	})
	ids2 := s2.userNotifications["user-2"]
	if len(ids2) != 1 {
		t.Fatalf("expected 1 notification (offline), got %d", len(ids2))
	}
	n2 := s2.notifications[ids2[0]]
	if n2.Title != "数字员工离线" {
		t.Errorf("title = %q, want 数字员工离线", n2.Title)
	}
}

func TestNotificationActionableTaskEvents(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name      string
		event     *model.Event
		wantTitle string
		wantBody  string
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
			name: "todo_rework_requested",
			event: &model.Event{
				ID: "e-w1", UserID: "user-1", EventType: "todo_rework_requested",
				ActorType: "system", ActorID: "reviewer", ActorName: "人工确认",
				Metadata:  map[string]any{"todo_title": "剧本初稿"},
				CreatedAt: now,
			},
			wantTitle: "产出被退回重做",
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
			if n.Category != "task" {
				t.Errorf("category = %q, want task", n.Category)
			}
			if n.Body == "" {
				t.Errorf("body should not be empty")
			}
		})
	}
}

func TestNotificationTaskCommentFromSelf(t *testing.T) {
	// 用户自己评论自己的任务不通知自己
	s := New()
	content := "我改了一下需求"
	s.maybeCreateNotificationUnsafe(&model.Event{
		ID: "e-c1", UserID: "user-1", EventType: "task_comment",
		ActorType: "user", ActorID: "user-1", ActorName: "我自己",
		Content:   &content,
		Metadata:  map[string]any{"task_title": "写剧本"},
		CreatedAt: time.Now().UTC(),
	})
	if len(s.userNotifications["user-1"]) != 0 {
		t.Errorf("self comment should not notify self, got %d notifications", len(s.userNotifications["user-1"]))
	}
}

// TestNotificationSourceEvent 钉死「事件 → SourceEvent」的映射。
//
// 为什么要专门测它：桌面端系统通知**按 SourceEvent 白名单**决定弹不弹（不再按中文标题），
// 所以这个字符串是**跨端的机器接口**：后端悄悄改名/改写法，客户端会静默失配（不报错、只是不弹），
// 没有任何编译期能拦。这里把 7 个「会弹」的键 + 几个「不该弹」的场景固化成契约。
// 注意它与 frontend-v2 的 `desktopNotifyFilter.test.ts` 是配对的：两边任何一侧改了都必须同时改。
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
		// —— 桌面端白名单里的 8 个来源键 ——
		{"任务完成", "task_status_changed", map[string]any{"to": "done"}, &done, "task_status_changed.done", true},
		{"任务失败", "task_status_changed", map[string]any{"to": "failed"}, &failed, "task_status_changed.failed", true},
		// 硬超时判失败：timeout_monitor 直接改 task.Status、**不**发 task_status_changed，
		// 因此这条键是「任务失败」在该路径下的唯一信号，必须列进桌面端白名单。
		{"硬超时失败", "todo_hard_deadline_failed", map[string]any{"todo_title": "剧本初稿"}, nil, "todo_hard_deadline_failed", true},
		{"成果待确认", "todo_awaiting_review", map[string]any{"todo_title": "分镜拆解"}, nil, "todo_awaiting_review", true},
		{"执行提问", "todo_ask_received", map[string]any{"question": "方向 A 还是 B？"}, nil, "todo_ask_received", true},
		{"方案确认", "task_plan_ready", map[string]any{"task_title": "写剧本"}, nil, "task_plan_ready", true},
		{"重做超限", "todo_rework_exhausted", map[string]any{"todo_title": "剧本初稿"}, nil, "todo_rework_exhausted", true},
		{"疑似卡死", "todo_remind_escalated", map[string]any{"todo_title": "剧本初稿"}, nil, "todo_remind_escalated", true},

		// —— 会生成通知但**不在**白名单：来源键同样必须可读，客户端才判得出「不弹」 ——
		{"任务取消", "task_status_changed", map[string]any{"to": "canceled"}, &canceled, "task_status_changed.canceled", true},
		{"Todo 失败", "todo_failed", map[string]any{"todo_title": "剧本初稿"}, nil, "todo_failed", true},
		{"派发失败", "todo_dispatch_failed", map[string]any{"todo_title": "剧本初稿"}, nil, "todo_dispatch_failed", true},
		{"预算耗尽", "todo_run_budget_exhausted", map[string]any{"todo_title": "剧本初稿"}, nil, "todo_run_budget_exhausted", true},
		{"数字员工上线", "agent_status_changed", map[string]any{"new_status": "online"}, nil, "agent_status_changed", true},
		{"入职申请", "join_request_received", map[string]any{"join_request_id": "jr-1"}, nil, "join_request_received", true},
		{"PM 回复", "planning_reply", map[string]any{}, nil, "planning_reply", true},

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
