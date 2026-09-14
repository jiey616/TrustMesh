package clawsynapse

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"trustmesh/backend/internal/metrics"
	"trustmesh/backend/internal/protocol"
	"trustmesh/backend/internal/store"
)

// 独立验证（QA · software-qa-engineer-2）——不是复跑工程师的用例，而是反证：
//   - A. T0.12「拒绝时零副作用」+ hasProduce 边界漏洞（纯空白）
//   - B. T0.6a 计数正确性（尤其「不重复计数」）
// 复用同包既有夹具：newTransferTestFixture / postWebhookJSON / todoStatusOf /
// withStrictProduceGate / postTransfer。
//
// 说明：本文件不修改任何生产源码；所有断言基于对当前实现的实测。

func countCommentsContaining(t *testing.T, s *store.Store, userID, taskID, needle string) int {
	t.Helper()
	comments, appErr := s.ListTaskComments(store.Scope{UserID: userID}, taskID)
	if appErr != nil {
		t.Fatalf("list comments: %v", appErr)
	}
	n := 0
	for _, c := range comments {
		if strings.Contains(c.Content, needle) {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// A. T0.12 —— 拒绝时零副作用 + 边界反证
// ---------------------------------------------------------------------------

// TestTodoCompleteRejectionHasNoSideEffectsExceptOneComment 证明：一个无产出
// 声明的 todo.complete 被 422 拒绝后，具体到集合/条目：
//   - todo TD_01 状态保持原值（pending，未变 done）；
//   - 该任务的 artifact 数量不变（0）；
//   - 任务评论集合**恰好 +1**，且该新增条目就是「缺少产出声明」拒收系统评论。
//
// 比「零新增记录」更精确：拒收**应当**留一条可见的系统评论（可观测性要求），
// 但除此之外不得有任何状态漂移。
func TestTodoCompleteRejectionHasNoSideEffectsExceptOneComment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics.Reset()
	t.Cleanup(metrics.Reset)
	withStrictProduceGate(t, true)

	s, userID, taskID, devNode := newTransferTestFixture(t)
	h := NewWebhookHandler(WebhookDeps{Store: s})

	beforeStatus := todoStatusOf(t, taskID, "TD_01", h)
	if beforeStatus != "pending" {
		t.Fatalf("precondition: TD_01 status = %q, want pending", beforeStatus)
	}
	beforeArtifacts := len(s.GetArtifactsByTaskID(taskID))
	beforeComments, appErr := s.ListTaskComments(store.Scope{UserID: userID}, taskID)
	if appErr != nil {
		t.Fatalf("list comments (before): %v", appErr)
	}
	beforeRejectComments := countCommentsContaining(t, s, userID, taskID, "缺少产出声明")

	w := postWebhookJSON(t, h, protocol.WebhookPayload{
		Type:       "todo.complete",
		From:       devNode,
		SessionKey: taskID,
		Message:    `{"task_id":"` + taskID + `","todo_id":"TD_01"}`,
		Metadata:   map[string]any{},
	})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body=%s", w.Code, w.Body.String())
	}

	// (1) todo 状态不变，尤其不是 done。
	afterStatus := todoStatusOf(t, taskID, "TD_01", h)
	if afterStatus != beforeStatus || afterStatus == "done" {
		t.Fatalf("todo status drifted on rejection: before=%q after=%q", beforeStatus, afterStatus)
	}
	// (2) artifact 数量不变。
	if afterArtifacts := len(s.GetArtifactsByTaskID(taskID)); afterArtifacts != beforeArtifacts {
		t.Fatalf("artifact count changed on rejection: before=%d after=%d", beforeArtifacts, afterArtifacts)
	}
	// (3) 评论集合恰好 +1。
	afterComments, appErr := s.ListTaskComments(store.Scope{UserID: userID}, taskID)
	if appErr != nil {
		t.Fatalf("list comments (after): %v", appErr)
	}
	if len(afterComments) != len(beforeComments)+1 {
		t.Fatalf("comment delta = %d, want exactly 1 (before=%d after=%d)",
			len(afterComments)-len(beforeComments), len(beforeComments), len(afterComments))
	}
	// 且新增的那一条就是拒收系统评论。
	if got := countCommentsContaining(t, s, userID, taskID, "缺少产出声明"); got != beforeRejectComments+1 {
		t.Fatalf("rejection-comment delta = %d, want exactly 1", got-beforeRejectComments)
	}
}

// TestProduceGateBoundaryCharacterization 逐档实测 hasProduce 的判定边界。
//
// 重点（team-lead 指定）：hasProduce 对 summary/output 做 TrimSpace 后判定，
// 「纯空白」summary/output（空格 / 制表符 / 换行，含经 content backfill 回填
// 的空白）现在一律被当作「无产出」而拒绝（HTTP 422）。这是 T0.12a 对 F-01 的修复：
// 修复前纯空白被 `!= ""` 判为「有产出」→ 放行，是真实的设计漏洞。
//
// 本用例断言的是**修复后的预期行为**（regression guard）。
func TestProduceGateBoundaryCharacterization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withStrictProduceGate(t, true)

	type tc struct {
		name    string
		message func(taskID string) string
		want    int
	}
	cases := []tc{
		{
			name:    "① result 字段完全缺失",
			message: func(id string) string { return `{"task_id":"` + id + `","todo_id":"TD_01"}` },
			want:    http.StatusUnprocessableEntity,
		},
		{
			name:    "② result 为空对象 {}",
			message: func(id string) string { return `{"task_id":"` + id + `","todo_id":"TD_01","result":{}}` },
			want:    http.StatusUnprocessableEntity,
		},
		{
			// T0.12a 修复：纯空白 summary 经 TrimSpace 判为「无产出」→ 拒。
			name:    "③ result.summary 纯空白（修复后拒）",
			message: func(id string) string { return `{"task_id":"` + id + `","todo_id":"TD_01","result":{"summary":"   "}}` },
			want:    http.StatusUnprocessableEntity,
		},
		{
			// T0.12a 修复：制表符/换行同样 TrimSpace 后为空 → 拒。
			name:    "④ result.output 纯制表符/换行（修复后拒）",
			message: func(id string) string { return `{"task_id":"` + id + `","todo_id":"TD_01","result":{"output":"\t\n"}}` },
			want:    http.StatusUnprocessableEntity,
		},
		{
			// T0.12a 修复：content 纯空白 → summary 被回填为空白 → TrimSpace 后为空 → 拒。
			name:    "⑤ content 纯空白 result 空（修复后拒）",
			message: func(id string) string { return `{"task_id":"` + id + `","todo_id":"TD_01","content":"   "}` },
			want:    http.StatusUnprocessableEntity,
		},
		{
			name:    "⑥ result 为 legacy 字符串（2026-09-04 回归保护）",
			message: func(id string) string { return `{"task_id":"` + id + `","todo_id":"TD_01","result":"定稿完成"}` },
			want:    http.StatusOK,
		},
		{
			name: "⑦ content 有值 result 空（backfill 正常路径）",
			message: func(id string) string {
				return `{"task_id":"` + id + `","todo_id":"TD_01","content":"完成第3步分镜"}`
			},
			want: http.StatusOK,
		},
		{
			name: "⑧ result.metadata 非空对象",
			message: func(id string) string {
				return `{"task_id":"` + id + `","todo_id":"TD_01","result":{"metadata":{"k":"v"}}}`
			},
			want: http.StatusOK,
		},
		{
			name:    "⑨ result.metadata 空对象（不算产出）",
			message: func(id string) string { return `{"task_id":"` + id + `","todo_id":"TD_01","result":{"metadata":{}}}` },
			want:    http.StatusUnprocessableEntity,
		},
		{
			name: "⑩ result.action_items 非空",
			message: func(id string) string {
				return `{"task_id":"` + id + `","todo_id":"TD_01","result":{"action_items":[{"title":"跟进"}]}}`
			},
			want: http.StatusOK,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			metrics.Reset()
			t.Cleanup(metrics.Reset)
			s, _, taskID, devNode := newTransferTestFixture(t)
			h := NewWebhookHandler(WebhookDeps{Store: s})

			w := postWebhookJSON(t, h, protocol.WebhookPayload{
				Type:       "todo.complete",
				From:       devNode,
				SessionKey: taskID,
				Message:    c.message(taskID),
				Metadata:   map[string]any{},
			})
			if w.Code != c.want {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, c.want, w.Body.String())
			}
			if c.want == http.StatusOK {
				t.Logf("ACCEPTED (有产出): %s", c.name)
			}
		})
	}
}

// TestProduceGateSoftModeCounters 独立验证 strictProduceGate=false 的软模式：
// 无产出 → 200、todo done、AgentProduceWarnedTotal=1 且 AgentProduceRejectedTotal=0。
func TestProduceGateSoftModeCounters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics.Reset()
	t.Cleanup(metrics.Reset)
	withStrictProduceGate(t, false)

	s, _, taskID, devNode := newTransferTestFixture(t)
	h := NewWebhookHandler(WebhookDeps{Store: s})

	w := postWebhookJSON(t, h, protocol.WebhookPayload{
		Type:       "todo.complete",
		From:       devNode,
		SessionKey: taskID,
		Message:    `{"task_id":"` + taskID + `","todo_id":"TD_01"}`,
		Metadata:   map[string]any{},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("soft mode status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if st := todoStatusOf(t, taskID, "TD_01", h); st != "done" {
		t.Fatalf("soft mode TD_01 = %q, want done", st)
	}
	if n := metrics.Count(metrics.AgentProduceWarnedTotal); n != 1 {
		t.Fatalf("AgentProduceWarnedTotal = %d, want 1", n)
	}
	if n := metrics.Count(metrics.AgentProduceRejectedTotal); n != 0 {
		t.Fatalf("AgentProduceRejectedTotal = %d, want 0 (soft mode must not reject)", n)
	}
}

// TestTransferReceivedWhitespaceFileNameRejected 反证 transfer.received 的
// fileName 校验确实用 strings.TrimSpace：纯空白（空格 / 制表符 / 换行）一律 422
// ARTIFACT_FILENAME_REQUIRED，且**不产生任何 artifact 记录**（写入发生在校验之后）。
func TestTransferReceivedWhitespaceFileNameRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for i, blank := range []string{"   ", "\t", "\n", " \t\n "} {
		blank := blank
		t.Run("blank-"+string(rune('a'+i)), func(t *testing.T) {
			metrics.Reset()
			t.Cleanup(metrics.Reset)

			s, _, taskID, devNode := newTransferTestFixture(t)
			h := NewWebhookHandler(WebhookDeps{Store: s})
			before := len(s.GetArtifactsByTaskID(taskID))

			w := postTransfer(t, h, devNode, taskID, "TD_01",
				"tid-ws-filename-000"+string(rune('1'+i)), blank, "text/markdown", nil)

			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422; body=%s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "ARTIFACT_FILENAME_REQUIRED") {
				t.Fatalf("body missing ARTIFACT_FILENAME_REQUIRED: %s", w.Body.String())
			}
			if after := len(s.GetArtifactsByTaskID(taskID)); after != before {
				t.Fatalf("artifact count changed: before=%d after=%d", before, after)
			}
			if n := metrics.Count(metrics.AgentProduceRejectedTotal); n != 1 {
				t.Fatalf("AgentProduceRejectedTotal = %d, want 1", n)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// B. T0.6a —— 计数正确性（重点：不重复计数）
// ---------------------------------------------------------------------------

// TestEnvelopeProtocolCountedExactlyOnceOnReroute 证明：一个协议信封形态、且会
// 被 unwrapTaskProtocolEnvelope reroute 到真实 handler 的 chat.message，计数
// **恰好 +1**（不是 +2）——即计数发生在 reroute 之前的单点，reroute 本身不再计数。
//
// 两种信封形状都覆盖：payload 平铺在顶层，以及 payload 嵌在 "body" 内。
func TestEnvelopeProtocolCountedExactlyOnceOnReroute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withStrictProduceGate(t, true)

	shapes := []struct {
		name    string
		message func(taskID string) string
	}{
		{
			name: "顶层平铺 payload",
			message: func(id string) string {
				return `{"protocol":"clawsynapse/1.0","type":"todo.complete","task_id":"` + id +
					`","todo_id":"TD_01","result":{"summary":"enveloped done"}}`
			},
		},
		{
			name: "body 内嵌 payload（山雨形状）",
			message: func(id string) string {
				return `{"protocol":"clawsynapse/1.0","type":"todo.complete","body":{"task_id":"` + id +
					`","todo_id":"TD_01","result":{"summary":"body nested done"}}}`
			},
		},
	}

	for _, sp := range shapes {
		sp := sp
		t.Run(sp.name, func(t *testing.T) {
			metrics.Reset()
			t.Cleanup(metrics.Reset)
			s, _, taskID, devNode := newTransferTestFixture(t)
			h := NewWebhookHandler(WebhookDeps{Store: s})

			w := postWebhookJSON(t, h, protocol.WebhookPayload{
				Type:       "chat.message",
				From:       devNode,
				SessionKey: taskID,
				Message:    sp.message(taskID),
				Metadata:   map[string]any{},
			})

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
			}
			if got := metrics.Count(metrics.EnvelopeProtocolTotal); got != 1 {
				t.Fatalf("EnvelopeProtocolTotal = %d, want exactly 1 (no double count)", got)
			}
			if got := metrics.Count(metrics.EnvelopeLegacyJSONTotal); got != 0 {
				t.Fatalf("EnvelopeLegacyJSONTotal = %d, want 0", got)
			}
			if st := todoStatusOf(t, taskID, "TD_01", h); st != "done" {
				t.Fatalf("rerouted todo status = %q, want done (T0.6a × T0.12 不冲突)", st)
			}
		})
	}
}

// TestEnvelopeLegacyJSONCountedOnceAndTodoCompleted：legacy 手搓 JSON（无 protocol）
// 的 todo.complete → EnvelopeLegacyJSONTotal 恰好 1、EnvelopeProtocolTotal 0、
// HTTP 200 且 todo 变 done。
func TestEnvelopeLegacyJSONCountedOnceAndTodoCompleted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics.Reset()
	t.Cleanup(metrics.Reset)
	withStrictProduceGate(t, true)

	s, _, taskID, devNode := newTransferTestFixture(t)
	h := NewWebhookHandler(WebhookDeps{Store: s})

	legacy := `{"task_id":"` + taskID + `","todo_id":"TD_01","result":{"summary":"legacy ok"}}`
	w := postWebhookJSON(t, h, protocol.WebhookPayload{
		Type:       "todo.complete",
		From:       devNode,
		SessionKey: taskID,
		Message:    legacy,
		Metadata:   map[string]any{},
	})

	if w.Code != http.StatusOK {
		t.Fatalf("legacy todo.complete status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := metrics.Count(metrics.EnvelopeLegacyJSONTotal); got != 1 {
		t.Fatalf("EnvelopeLegacyJSONTotal = %d, want 1", got)
	}
	if got := metrics.Count(metrics.EnvelopeProtocolTotal); got != 0 {
		t.Fatalf("EnvelopeProtocolTotal = %d, want 0", got)
	}
	if st := todoStatusOf(t, taskID, "TD_01", h); st != "done" {
		t.Fatalf("TD_01 status = %q, want done", st)
	}
}

// TestEnvelopePlainTextCountedAndAccepted：纯文本 task.comment → 200 且计入
// EnvelopePlainTextTotal（分类纯观测，不拒绝真实评论）。
func TestEnvelopePlainTextCountedAndAccepted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics.Reset()
	t.Cleanup(metrics.Reset)

	s, _, taskID, devNode := newTransferTestFixture(t)
	h := NewWebhookHandler(WebhookDeps{Store: s})

	w := postWebhookJSON(t, h, protocol.WebhookPayload{
		Type:       "task.comment",
		From:       devNode,
		SessionKey: taskID,
		Message:    "这是一条纯文本评论，不是 JSON",
		Metadata:   map[string]any{},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("plain-text comment status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := metrics.Count(metrics.EnvelopePlainTextTotal); got != 1 {
		t.Fatalf("EnvelopePlainTextTotal = %d, want 1", got)
	}
}

// TestEnvelopeInvalidJSONCountedWithoutNewRejection：以 '{'/'[' 开头但非法 JSON
// 的消息 → 计入 EnvelopeInvalidJSONTotal，且不引入任何**新增拒绝**（走既有行为，
// 这里用 meeting.error 类型验证：其既有行为恒为 200 ignored）。
func TestEnvelopeInvalidJSONCountedWithoutNewRejection(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for i, bad := range []string{`{"task_id":`, `[1,2`} {
		bad := bad
		t.Run("bad-"+string(rune('a'+i)), func(t *testing.T) {
			metrics.Reset()
			t.Cleanup(metrics.Reset)
			s, _, taskID, devNode := newTransferTestFixture(t)
			h := NewWebhookHandler(WebhookDeps{Store: s})

			// meeting.error 的既有行为：200 ignored，不受内层信封形状影响。
			w := postWebhookJSON(t, h, protocol.WebhookPayload{
				Type:       "meeting.error",
				From:       devNode,
				SessionKey: taskID,
				Message:    bad,
				Metadata:   map[string]any{},
			})
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (既有行为不得新增拒绝); body=%s", w.Code, w.Body.String())
			}
			if got := metrics.Count(metrics.EnvelopeInvalidJSONTotal); got != 1 {
				t.Fatalf("EnvelopeInvalidJSONTotal = %d, want 1", got)
			}
			if got := metrics.Count(metrics.AgentProduceRejectedTotal); got != 0 {
				t.Fatalf("AgentProduceRejectedTotal = %d, want 0 (分类不得触发拒绝)", got)
			}
		})
	}
}
