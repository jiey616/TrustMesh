package clawsynapse

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"trustmesh/backend/internal/protocol"
	"trustmesh/backend/internal/store"
)

// TestIsSilentACKProtocolReceipts guards the protocol-receipt taxonomy: the
// PM Agent acknowledges every publish with a bare "ACK <message-type>" line
// (plus arbitrary trailing decoration). Planning and dispatch receipts such as
// "ACK task.plan_ready" / "ACK todo.assigned" were NOT covered by the old
// regex and leaked into the task transcript as fake user-visible messages.
func TestIsSilentACKProtocolReceipts(t *testing.T) {
	silent := []string{
		// planning / dispatch receipts (the incident this guards against)
		"ACK task.plan_ready",
		"ACK task.plan_ready\n\n等待平台校验规划。",
		"ACK todo.assigned",
		"ACK task.assigned",
		"ACK task.todo_add",
		"ACK todo.status_changed - TD_01 -> in_progress（第 4 阶段 Structure 派工中）",
		"WAITING",
		// pre-existing receipts must stay silent (regression guard)
		"ACK task.reply",
		"ACK task.reply WAITING",
		"ACK chat.message",
		"ACK meeting.chat — 已记录导演数字员工就绪。",
	}
	for _, msg := range silent {
		if !isSilentACK(msg) {
			t.Errorf("expected silent ACK: %q", msg)
		}
	}

	real := []string{
		"已完成第5步视听蓝图，产出文件见交付列表。",
		"规划已提交，共3个步骤，等待平台校验。",
	}
	for _, msg := range real {
		if isSilentACK(msg) {
			t.Errorf("expected real content, got silent ACK: %q", msg)
		}
	}
}

// TestStripLeadingExplicitAckKeepsRealContent makes sure the widened receipt
// taxonomy does not swallow persona-style "ACK first, content after" replies.
func TestStripLeadingExplicitAckKeepsRealContent(t *testing.T) {
	msg := "ACK task.comment\n\n已完成第5步视听蓝图，产出文件见交付列表。"
	got := stripLeadingExplicitAck(msg)
	if got != "已完成第5步视听蓝图，产出文件见交付列表。" {
		t.Fatalf("real content lost after ACK strip: %q", got)
	}
	// A receipt for a newly covered type with only boilerplate after it
	// collapses to empty (pure receipt).
	if got := stripLeadingExplicitAck("ACK task.plan_ready\n\nWAITING"); got != "" {
		t.Fatalf("pure receipt should collapse to empty, got %q", got)
	}
}

// TestTaskPlanReadyBadPayloadAppendsSystemComment covers the "@file literal"
// incident: a task.plan_ready whose message body is not JSON is rejected with
// 400 BAD_PAYLOAD. The rejection used to be fully invisible in the platform
// while the PM still reported success to the user. It must now surface as a
// system comment on the task timeline.
func TestTaskPlanReadyBadPayloadAppendsSystemComment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := store.New()
	user, appErr := s.CreateUser("pm-badpayload@example.com", "PM User", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}
	pm, appErr := s.CreateAgent(store.Scope{UserID: user.ID}, "node-pm-bad", "PM Agent", "pm", "pm", []string{"plan"})
	if appErr != nil {
		t.Fatalf("create pm: %v", appErr)
	}
	dev, appErr := s.CreateAgent(store.Scope{UserID: user.ID}, "node-dev-bad", "Dev", "developer", "dev", nil)
	if appErr != nil {
		t.Fatalf("create dev: %v", appErr)
	}
	s.SyncAgentPresence([]store.AgentPresence{
		{NodeID: pm.NodeID, LastSeenAt: time.Now().UTC()},
		{NodeID: dev.NodeID, LastSeenAt: time.Now().UTC()},
	}, time.Now().UTC())
	proj, appErr := s.CreateProject(store.Scope{UserID: user.ID}, "军旅影视制作", "demo", pm.ID)
	if appErr != nil {
		t.Fatalf("create project: %v", appErr)
	}
	task, appErr := s.CreateTaskByPMNodeWithMessageID(pm.NodeID, "", store.TaskCreateInput{
		ProjectID:   proj.ID,
		Title:       "军旅题材微电影制作",
		Description: "e2e bad-payload visibility",
		Todos: []store.TaskCreateTodoInput{
			{ID: "TD_01", Order: 1, Title: "剧本创作", Description: "draft", AssigneeNodeID: dev.NodeID},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}

	h := NewWebhookHandler(s, nil, nil)

	// Session key carries the task id (platform convention); the body is the
	// literal @file reference the PM emitted in the incident.
	payload := protocol.WebhookPayload{
		NodeID:     pm.NodeID,
		Type:       "task.plan_ready",
		From:       pm.NodeID,
		SessionKey: task.ID,
		Message:    "@/tmp/payload_plan.json",
		Metadata:   map[string]any{},
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/webhook/clawsynapse", nil)
	h.handleTaskPlanReady(c, payload)

	if w.Code != 400 {
		t.Fatalf("expected 400 BAD_PAYLOAD, got %d", w.Code)
	}
	comments, appErr := s.ListTaskComments(store.Scope{UserID: user.ID}, task.ID)
	if appErr != nil {
		t.Fatalf("list comments: %v", appErr)
	}
	found := false
	for _, cm := range comments {
		if strings.Contains(cm.Content, "PM 规划提交被拒") && strings.Contains(cm.Content, "@/tmp/payload_plan.json") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected bad-payload system comment on task timeline, got %d comment(s)", len(comments))
	}
}
