package clawsynapse

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"trustmesh/backend/internal/metrics"
	"trustmesh/backend/internal/protocol"
	"trustmesh/backend/internal/store"
)

// T0.6a 协议信封「双读准备」测试。
//
// 目标：证明入站内层信封的分类是纯观测（不改行为、不拒绝），且关键的 legacy
// 路径——无 protocol 字段的手搓 JSON todo.complete——仍被接受并计数。这正是
// 2026-09-04 山雨事故（result 为字符串的 todo.complete 被严格解码 400 拒收 →
// channel 重试 → 任务卡死）必须长期守护的形状。

func TestClassifyEnvelopeShape(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want envelopeShape
	}{
		{
			name: "① 合法协议信封（task 类型）",
			raw:  `{"protocol":"clawsynapse/1.0","type":"todo.complete","task_id":"t1","todo_id":"TD_01","result":"done"}`,
			want: envShapeProtocol,
		},
		{
			name: "② 合法 JSON、无 protocol 字段（legacy 手搓）",
			raw:  `{"task_id":"t1","todo_id":"TD_01","result":"done"}`,
			want: envShapeLegacyJSON,
		},
		{
			name: "③ 非 JSON 文本",
			raw:  "你好，请查收交付文件",
			want: envShapePlainText,
		},
		{
			name: "④ 非法 JSON（以 { 开头但解析失败）",
			raw:  `{"task_id":}`,
			want: envShapeInvalidJSON,
		},
		{
			name: "⑤ 协议信封但 type 非任务类型——分类仍为 protocol，证明分类与拆包解耦",
			raw:  `{"protocol":"clawsynapse/1.0","type":"chat.message"}`,
			want: envShapeProtocol,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyEnvelopeShape(tc.raw); got != tc.want {
				t.Fatalf("classifyEnvelopeShape(%q) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}

// TestLegacyHandwrittenTodoCompleteStillAcceptedAndCounted 是 T0.6a 的核心回归：
// 一份「合法 JSON 但没有 protocol 字段」的手搓 todo.complete（result 为字符串）
// 必须仍能走通普通路径返回 200、把 todo 置 done，并被计入 legacy 计数器。
//
// 它同时锁死「分类 ≠ 拆包」：该消息分类为 envShapeLegacyJSON，不走
// unwrapTaskProtocolEnvelope，而是由 type=todo.complete 的正常分发处理。
func TestLegacyHandwrittenTodoCompleteStillAcceptedAndCounted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics.Reset()
	t.Cleanup(metrics.Reset)

	// 夹具镜像 webhook_ack_test.go：user / pm / dev / project / task(TD_01→dev)。
	s := store.New()
	user, appErr := s.CreateUser("pm-legacy@example.com", "PM User", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}
	pm, appErr := s.CreateAgent(store.Scope{UserID: user.ID}, "node-pm-legacy", "PM Agent", "pm", "pm", []string{"plan"})
	if appErr != nil {
		t.Fatalf("create pm: %v", appErr)
	}
	dev, appErr := s.CreateAgent(store.Scope{UserID: user.ID}, "node-dev-legacy", "Dev", "developer", "dev", nil)
	if appErr != nil {
		t.Fatalf("create dev: %v", appErr)
	}
	s.SyncAgentPresence([]store.AgentPresence{
		{NodeID: pm.NodeID, LastSeenAt: time.Now().UTC()},
		{NodeID: dev.NodeID, LastSeenAt: time.Now().UTC()},
	}, time.Now().UTC())
	proj, appErr := s.CreateProject(store.Scope{UserID: user.ID}, "legacy 协议回归", "demo", pm.ID)
	if appErr != nil {
		t.Fatalf("create project: %v", appErr)
	}
	task, appErr := s.CreateTaskByPMNodeWithMessageID(pm.NodeID, "", store.TaskCreateInput{
		ProjectID:   proj.ID,
		Title:       "legacy 信封回归",
		Description: "无 protocol 字段的手搓 todo.complete 必须继续被接受",
		Todos: []store.TaskCreateTodoInput{
			{ID: "TD_01", Order: 1, Title: "剧本创作", Description: "draft", AssigneeNodeID: dev.NodeID},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}

	h := NewWebhookHandler(s, nil, nil)

	// message = 无 protocol 字段的 legacy 手搓 JSON，且 result 是字符串。
	legacy := `{"task_id":"` + task.ID + `","todo_id":"TD_01","result":"简述：剧本初稿完成"}`
	payload := protocol.WebhookPayload{
		Type:       "todo.complete",
		From:       dev.NodeID,
		SessionKey: task.ID,
		Message:    legacy,
		Metadata:   map[string]any{},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhook/clawsynapse", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.HandleWebhook(c)

	if w.Code != http.StatusOK {
		t.Fatalf("legacy todo.complete rejected: status=%d body=%s", w.Code, w.Body.String())
	}

	got := s.GetTaskInternal(task.ID)
	if got == nil {
		t.Fatal("task disappeared after todo.complete")
	}
	status := ""
	for _, todo := range got.Todos {
		if todo.ID == "TD_01" {
			status = todo.Status
		}
	}
	if status != "done" {
		t.Fatalf("TD_01 status = %q, want done", status)
	}

	if n := metrics.Count(metrics.EnvelopeLegacyJSONTotal); n != 1 {
		t.Fatalf("EnvelopeLegacyJSONTotal = %d, want 1", n)
	}
	if n := metrics.Count(metrics.EnvelopeProtocolTotal); n != 0 {
		t.Fatalf("EnvelopeProtocolTotal = %d, want 0", n)
	}
}
