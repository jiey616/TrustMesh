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
