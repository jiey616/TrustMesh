package clawsynapse

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"trustmesh/backend/internal/metrics"
	"trustmesh/backend/internal/protocol"
)

// T1.6 强制协议 schema 测试。
//
// 目标：证明「结构化 task/todo 入站消息必须携带 protocol 信封」这一强校验
//   1. 只在结构化任务类型上生效（自由文本通道永不误伤）；
//   2. 默认（gate off）完全等同 T0.6a 双读——legacy 手搓 JSON 仍被接受；
//   3. gate on 时 legacy / 纯文本 / 非法 JSON 一律 422 且「不 fallback」；
//   4. 信封内部 result 为字符串的宽容（FlexibleTodoResult）不受门禁影响。

// withStrictProtocolGate 临时翻转包级开关，并在测试结束还原。
func withStrictProtocolGate(t *testing.T, on bool) {
	t.Helper()
	orig := strictProtocolSchemaGate
	strictProtocolSchemaGate = on
	t.Cleanup(func() { strictProtocolSchemaGate = orig })
}

// TestRequiresProtocolEnvelopeScope 锁死门禁的作用域：只覆盖结构化 task/todo
// 类型；chat/meeting/knowledge/散文回复/故障上报一律不覆盖。
func TestRequiresProtocolEnvelopeScope(t *testing.T) {
	required := []string{
		"task.create", "task.plan_ready",
		"task.todo_add", "task.todo_modify", "task.comment",
		"task.context.query",
		"todo.progress", "todo.complete", "todo.fail", "todo.ask", "todo.review",
	}
	for _, typ := range required {
		if !requiresProtocolEnvelope(typ) {
			t.Errorf("requiresProtocolEnvelope(%q) = false, want true", typ)
		}
		// 前后空白不应影响判定（分发前会用 TrimSpace 归一）。
		if !requiresProtocolEnvelope("  " + typ + " ") {
			t.Errorf("requiresProtocolEnvelope(%q) = false, want true (whitespace)", typ)
		}
	}

	freeText := []string{
		"chat.message", "chat.response",
		"meeting.chat", "meeting.control", "meeting.message",
		"meeting.response", "meeting.ack", "meeting.error",
		"knowledge.query",
		// persona 执行者的整段工作汇报 / 执行侧故障上报：人读文本，强加信封只会误伤。
		"task.response", "todo.response", "todo.error", "task.error",
		// task.reply：handleTaskReply 对畸形 JSON 有 loose-decode 容错，门禁会把它变成硬 422。
		"task.reply",
		// 🔴 transfer.received：平台/CLI 运行时产出的裸 JSON，纳入即「gate 一开所有
		// 交付物上传被拒」——见 requiresProtocolEnvelope 注释与下方端到端回归。
		"transfer.received",
		"", "unknown.thing",
	}
	for _, typ := range freeText {
		if requiresProtocolEnvelope(typ) {
			t.Errorf("requiresProtocolEnvelope(%q) = true, want false", typ)
		}
	}
}

// TestEnvelopeShapeName 覆盖四个形状都有可读标签（用于 422 文案可操作性）。
func TestEnvelopeShapeName(t *testing.T) {
	cases := map[envelopeShape]string{
		envShapeProtocol:    "protocol 信封",
		envShapeLegacyJSON:  "无 protocol 字段的 legacy JSON",
		envShapeInvalidJSON: "非法 JSON",
		envShapePlainText:   "非 JSON 纯文本",
	}
	for shape, want := range cases {
		if got := envelopeShapeName(shape); got != want {
			t.Fatalf("envelopeShapeName(%d) = %q, want %q", shape, got, want)
		}
	}
}

// TestStrictProtocolGateMatrix 是 T1.6 核心验收矩阵：
//
//	gate on  + legacy/纯文本/非法JSON（结构化类型）→ 422 且 todo 不变；
//	gate on  + 合法协议信封                        → 200 且 todo done；
//	gate off + legacy                              → 200（双读回退，T0.6a 行为）。
func TestStrictProtocolGateMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const legacyFmt = `{"task_id":"%s","todo_id":"TD_01","result":"简述：剧本初稿完成"}`

	cases := []struct {
		name         string
		gateOn       bool
		message      func(taskID string) string
		wantStatus   int
		wantDone     bool
		wantRejected int64
		wantWarned   int64
	}{
		{
			name:         "gate开启_legacy手搓JSON_硬拒",
			gateOn:       true,
			message:      func(id string) string { return fmt.Sprintf(legacyFmt, id) },
			wantStatus:   http.StatusUnprocessableEntity,
			wantDone:     false,
			wantRejected: 1,
			wantWarned:   0,
		},
		{
			name:   "gate开启_合法协议信封_放行",
			gateOn: true,
			message: func(id string) string {
				return `{"protocol":"clawsynapse/1.0","type":"todo.complete","task_id":"` + id + `","todo_id":"TD_01","result":{"summary":"完成剧本初稿，产出 script_v1.md"}}`
			},
			wantStatus:   http.StatusOK,
			wantDone:     true,
			wantRejected: 0,
			wantWarned:   0,
		},
		{
			name:   "gate开启_信封内result为字符串_宽容仍生效",
			gateOn: true,
			message: func(id string) string {
				// 2026-09-04 山雨事故的正面回归：信封合法 + result 为字符串，
				// FlexibleTodoResult 必须仍接受，不得 422。
				return `{"protocol":"clawsynapse/1.0","type":"todo.complete","task_id":"` + id + `","todo_id":"TD_01","result":"简述：剧本初稿完成"}`
			},
			wantStatus:   http.StatusOK,
			wantDone:     true,
			wantRejected: 0,
			wantWarned:   0,
		},
		{
			name:         "gate开启_纯文本_硬拒",
			gateOn:       true,
			message:      func(string) string { return "你好，请查收交付文件" },
			wantStatus:   http.StatusUnprocessableEntity,
			wantDone:     false,
			wantRejected: 1,
			wantWarned:   0,
		},
		{
			name:         "gate开启_非法JSON_硬拒",
			gateOn:       true,
			message:      func(string) string { return `{"task_id":}` },
			wantStatus:   http.StatusUnprocessableEntity,
			wantDone:     false,
			wantRejected: 1,
			wantWarned:   0,
		},
		{
			name:         "gate关闭_legacy手搓JSON_仍放行并告警计数",
			gateOn:       false,
			message:      func(id string) string { return fmt.Sprintf(legacyFmt, id) },
			wantStatus:   http.StatusOK,
			wantDone:     true,
			wantRejected: 0,
			wantWarned:   1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			metrics.Reset()
			t.Cleanup(metrics.Reset)
			withStrictProtocolGate(t, tc.gateOn)

			s, _, taskID, devNode := newTransferTestFixture(t)
			h := NewWebhookHandler(WebhookDeps{Store: s})

			w := postWebhookJSON(t, h, protocol.WebhookPayload{
				Type:       "todo.complete",
				From:       devNode,
				SessionKey: taskID,
				Message:    tc.message(taskID),
				Metadata:   map[string]any{},
			})

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tc.wantStatus, w.Body.String())
			}
			if tc.wantStatus == http.StatusUnprocessableEntity {
				body := w.Body.String()
				if !strings.Contains(body, "PROTOCOL_ENVELOPE_REQUIRED") {
					t.Fatalf("body missing code PROTOCOL_ENVELOPE_REQUIRED: %s", body)
				}
				if !strings.Contains(body, "protocol") {
					t.Fatalf("rejection message not actionable (no protocol hint): %s", body)
				}
			}
			if got := todoStatusOf(t, taskID, "TD_01", h); (got == "done") != tc.wantDone {
				t.Fatalf("TD_01 status = %q, wantDone=%v", got, tc.wantDone)
			}
			if n := metrics.Count(metrics.ProtocolSchemaRejectedTotal); n != tc.wantRejected {
				t.Fatalf("ProtocolSchemaRejectedTotal = %d, want %d", n, tc.wantRejected)
			}
			if n := metrics.Count(metrics.ProtocolSchemaWarnedTotal); n != tc.wantWarned {
				t.Fatalf("ProtocolSchemaWarnedTotal = %d, want %d", n, tc.wantWarned)
			}
		})
	}
}

// TestStrictProtocolGateDoesNotBlockTransferReceived 是 P1 回归：transfer.received
// 的消息体由平台/CLI 运行时产出，**恒为裸 JSON**（不是 agent 手搓的协议信封）。
// 若把它纳入 requiresProtocolEnvelope，gate 一开就会 422 掉**所有**交付物上传。
//
// 必须走 HandleWebhook（门禁所在处），故 NodeID 留空以跳过本地节点校验。
func TestStrictProtocolGateDoesNotBlockTransferReceived(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics.Reset()
	t.Cleanup(metrics.Reset)
	withStrictProtocolGate(t, true)

	s, _, taskID, devNode := newTransferTestFixture(t)
	h := NewWebhookHandler(WebhookDeps{Store: s})

	body, err := json.Marshal(map[string]any{
		"transferId": "tid-protocol-gate-regression",
		"fileName":   "login-guide.md",
		"fileSize":   321,
		"localPath":  "/var/lib/trustmesh-transfers/tid-protocol-gate-regression-login-guide.md",
		"mimeType":   "text/markdown",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	w := postWebhookJSON(t, h, protocol.WebhookPayload{
		Type:     "transfer.received",
		From:     devNode,
		Message:  string(body),
		Metadata: map[string]any{"taskId": taskID, "todoId": "TD_01"},
	})

	if w.Code != http.StatusOK {
		t.Fatalf("transfer.received must survive the protocol gate: status=%d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "PROTOCOL_ENVELOPE_REQUIRED") {
		t.Fatalf("transfer.received wrongly protocol-gated: %s", w.Body.String())
	}
	if n := len(s.GetArtifactsByTaskID(taskID)); n != 1 {
		t.Fatalf("artifact must still be filed, got %d", n)
	}
	if n := metrics.Count(metrics.ProtocolSchemaRejectedTotal); n != 0 {
		t.Fatalf("ProtocolSchemaRejectedTotal = %d, want 0 for transfer.received", n)
	}
}

// TestStrictProtocolGateDoesNotBlockMalformedTaskReply 是 P2 回归：PM 常在
// task.reply 的 content 里带未转义引号（如引用 "need_review"），handleTaskReply
// 对此有专门的 loose-decode 容错。门禁不得把这层容错变成硬 422。
func TestStrictProtocolGateDoesNotBlockMalformedTaskReply(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics.Reset()
	t.Cleanup(metrics.Reset)
	withStrictProtocolGate(t, true)

	s, _, taskID, devNode := newTransferTestFixture(t)
	h := NewWebhookHandler(WebhookDeps{Store: s})

	// 未转义的双引号 → 非法 JSON（正是 handleTaskReply 容错要救的形状）。
	malformed := `{"task_id":"` + taskID + `","content":"请确认 "need_review" 字段是否开启"}`

	w := postWebhookJSON(t, h, protocol.WebhookPayload{
		Type:       "task.reply",
		From:       devNode,
		SessionKey: taskID,
		Message:    malformed,
		Metadata:   map[string]any{},
	})

	if strings.Contains(w.Body.String(), "PROTOCOL_ENVELOPE_REQUIRED") {
		t.Fatalf("task.reply must not be protocol-gated: %s", w.Body.String())
	}
	if n := metrics.Count(metrics.ProtocolSchemaRejectedTotal); n != 0 {
		t.Fatalf("ProtocolSchemaRejectedTotal = %d, want 0 for task.reply", n)
	}
}

// TestStrictProtocolGateIgnoresFreeTextChannel 证明自由文本通道在 gate on 时
// 不受协议门禁影响：chat.message 即使收到纯文本，也绝不能被打上
// PROTOCOL_ENVELOPE_REQUIRED（它走的是 chat 路径，不是任务协议路径）。
func TestStrictProtocolGateIgnoresFreeTextChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics.Reset()
	t.Cleanup(metrics.Reset)
	withStrictProtocolGate(t, true)

	s, _, _, devNode := newTransferTestFixture(t)
	h := NewWebhookHandler(WebhookDeps{Store: s})

	w := postWebhookJSON(t, h, protocol.WebhookPayload{
		Type:       "chat.message",
		From:       devNode,
		SessionKey: "no-such-session",
		Message:    "这是人读的自由文本，不该被协议门禁拦截",
		Metadata:   map[string]any{},
	})

	if w.Code == http.StatusUnprocessableEntity {
		t.Fatalf("free-text chat.message must not be protocol-rejected; body=%s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "PROTOCOL_ENVELOPE_REQUIRED") {
		t.Fatalf("chat.message wrongly reported as protocol-envelope violation: %s", w.Body.String())
	}
	if n := metrics.Count(metrics.ProtocolSchemaRejectedTotal); n != 0 {
		t.Fatalf("ProtocolSchemaRejectedTotal = %d, want 0 for free-text channel", n)
	}
	if n := metrics.Count(metrics.EnvelopePlainTextTotal); n != 1 {
		t.Fatalf("EnvelopePlainTextTotal = %d, want 1 (T0.6a counting must still run)", n)
	}
}

// TestStrictProtocolGateDefaultOff 断言默认值就是 OFF（canary 回退语义）：
// 只要没有显式设 TRUSTMESH_STRICT_PROTOCOL_SCHEMA_GATE=true，双读模式必须成立。
// 该断言同时守护「一键回退」——包级 var 的零值行为即双读。
func TestStrictProtocolGateDefaultOff(t *testing.T) {
	// 注意：本用例不翻转 gate，而是验证“未开启时”的行为契约。
	// 若运行环境显式设了 env，则跳过，避免误报。
	if strictProtocolSchemaGate {
		t.Skip("TRUSTMESH_STRICT_PROTOCOL_SCHEMA_GATE=true in env; default-off contract not observable")
	}

	gin.SetMode(gin.TestMode)
	metrics.Reset()
	t.Cleanup(metrics.Reset)

	s, _, taskID, devNode := newTransferTestFixture(t)
	h := NewWebhookHandler(WebhookDeps{Store: s})

	w := postWebhookJSON(t, h, protocol.WebhookPayload{
		Type:       "todo.complete",
		From:       devNode,
		SessionKey: taskID,
		Message:    fmt.Sprintf(`{"task_id":"%s","todo_id":"TD_01","result":"双读模式默认放行"}`, taskID),
		Metadata:   map[string]any{},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("default (gate off) must accept legacy todo.complete: status=%d body=%s", w.Code, w.Body.String())
	}
	if n := metrics.Count(metrics.ProtocolSchemaRejectedTotal); n != 0 {
		t.Fatalf("ProtocolSchemaRejectedTotal = %d, want 0 with gate off", n)
	}
	if n := metrics.Count(metrics.ProtocolSchemaWarnedTotal); n != 1 {
		t.Fatalf("ProtocolSchemaWarnedTotal = %d, want 1 with gate off", n)
	}
}
