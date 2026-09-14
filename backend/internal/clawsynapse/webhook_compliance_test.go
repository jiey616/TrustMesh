package clawsynapse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"trustmesh/backend/internal/metrics"
	"trustmesh/backend/internal/protocol"
)

// T0.12 Agent 合规「平台侧 interim」测试。
//
// 校验面刻意收窄到两个咽喉入口：todo.complete（产出声明非空）与
// transfer.received（fileName 非空）。其余入口不校验，本文件不覆盖它们。

// withStrictProduceGate 临时翻转包级开关，并在测试结束还原。
func withStrictProduceGate(t *testing.T, on bool) {
	t.Helper()
	orig := strictProduceGate
	strictProduceGate = on
	t.Cleanup(func() { strictProduceGate = orig })
}

// postWebhookJSON 走完整 HandleWebhook 路径投递一个 payload。
func postWebhookJSON(t *testing.T, h *WebhookHandler, payload protocol.WebhookPayload) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhook/clawsynapse", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.HandleWebhook(c)
	return w
}

// todoStatusOf 返回任务里指定 todo 的状态（不存在则返回空串）。
func todoStatusOf(t *testing.T, taskID, todoID string, h *WebhookHandler) string {
	t.Helper()
	task := h.store.GetTaskInternal(taskID)
	if task == nil {
		t.Fatalf("task %s not found", taskID)
	}
	for _, td := range task.Todos {
		if td.ID == todoID {
			return td.Status
		}
	}
	return ""
}

// TestTodoCompleteMissingResultRejected：strictProduceGate=true 且 todo.complete
// 无产出声明 → 硬拒 422 TODO_RESULT_EMPTY，文案可操作，todo 状态不变，计数 +1。
func TestTodoCompleteMissingResultRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics.Reset()
	t.Cleanup(metrics.Reset)
	withStrictProduceGate(t, true)

	s, _, taskID, devNode := newTransferTestFixture(t)
	h := NewWebhookHandler(WebhookDeps{Store: s})

	w := postWebhookJSON(t, h, protocol.WebhookPayload{
		Type:       "todo.complete",
		From:       devNode,
		SessionKey: taskID,
		// 没有 result、没有 content → 无产出声明。
		Message:  `{"task_id":"` + taskID + `","todo_id":"TD_01"}`,
		Metadata: map[string]any{},
	})

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "TODO_RESULT_EMPTY") {
		t.Fatalf("body missing code TODO_RESULT_EMPTY: %s", body)
	}
	if !strings.Contains(body, "缺少产出声明") || !strings.Contains(body, "summary") {
		t.Fatalf("rejection message not actionable: %s", body)
	}
	if got := todoStatusOf(t, taskID, "TD_01", h); got == "done" {
		t.Fatalf("todo must stay unfinished on rejection, got %q", got)
	}
	if n := metrics.Count(metrics.AgentProduceRejectedTotal); n != 1 {
		t.Fatalf("AgentProduceRejectedTotal = %d, want 1", n)
	}
}

// TestTodoCompleteWithResultAccepted：带 result.summary → 200，todo done。
func TestTodoCompleteWithResultAccepted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics.Reset()
	t.Cleanup(metrics.Reset)
	withStrictProduceGate(t, true)

	s, _, taskID, devNode := newTransferTestFixture(t)
	h := NewWebhookHandler(WebhookDeps{Store: s})

	w := postWebhookJSON(t, h, protocol.WebhookPayload{
		Type:       "todo.complete",
		From:       devNode,
		SessionKey: taskID,
		Message:    `{"task_id":"` + taskID + `","todo_id":"TD_01","result":{"summary":"完成剧本初稿，产出 script_v1.md"}}`,
		Metadata:   map[string]any{},
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := todoStatusOf(t, taskID, "TD_01", h); got != "done" {
		t.Fatalf("TD_01 status = %q, want done", got)
	}
	if n := metrics.Count(metrics.AgentProduceRejectedTotal); n != 0 {
		t.Fatalf("AgentProduceRejectedTotal = %d, want 0", n)
	}
}

// TestTodoCompleteLegacyStringResultAccepted：result 为字符串仍合法
// （FlexibleTodoResult → {Summary: text}），不得回归 2026-09-04 山雨事故。
func TestTodoCompleteLegacyStringResultAccepted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics.Reset()
	t.Cleanup(metrics.Reset)
	withStrictProduceGate(t, true)

	s, _, taskID, devNode := newTransferTestFixture(t)
	h := NewWebhookHandler(WebhookDeps{Store: s})

	w := postWebhookJSON(t, h, protocol.WebhookPayload{
		Type:       "todo.complete",
		From:       devNode,
		SessionKey: taskID,
		Message:    `{"task_id":"` + taskID + `","todo_id":"TD_01","result":"简述：定稿完成"}`,
		Metadata:   map[string]any{},
	})

	if w.Code != http.StatusOK {
		t.Fatalf("legacy string result rejected: status=%d body=%s", w.Code, w.Body.String())
	}
	if got := todoStatusOf(t, taskID, "TD_01", h); got != "done" {
		t.Fatalf("TD_01 status = %q, want done", got)
	}
}

// TestTodoCompleteMissingResultWarnsWhenGateOff：strictProduceGate=false 时，
// 无产出声明（缺 result，或 summary/output/content 纯空白——T0.12a 的语义缺口）
// 仍放行 200，todo done，但 AgentProduceWarnedTotal +1 且 AgentProduceRejectedTotal 不变。
func TestTodoCompleteMissingResultWarnsWhenGateOff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withStrictProduceGate(t, false)

	cases := []struct {
		name    string
		message string
	}{
		{"缺 result", `{"task_id":"%s","todo_id":"TD_01"}`},
		{"summary 纯空格", `{"task_id":"%s","todo_id":"TD_01","result":{"summary":"   "}}`},
		{"output 制表符换行", `{"task_id":"%s","todo_id":"TD_01","result":{"output":"\t\n"}}`},
		{"content 纯空格 backfill", `{"task_id":"%s","todo_id":"TD_01","content":"   "}`},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			metrics.Reset()
			t.Cleanup(metrics.Reset)

			s, _, taskID, devNode := newTransferTestFixture(t)
			h := NewWebhookHandler(WebhookDeps{Store: s})

			w := postWebhookJSON(t, h, protocol.WebhookPayload{
				Type:       "todo.complete",
				From:       devNode,
				SessionKey: taskID,
				Message:    fmt.Sprintf(c.message, taskID),
				Metadata:   map[string]any{},
			})

			if w.Code != http.StatusOK {
				t.Fatalf("soft mode must accept, status=%d body=%s", w.Code, w.Body.String())
			}
			if got := todoStatusOf(t, taskID, "TD_01", h); got != "done" {
				t.Fatalf("TD_01 status = %q, want done (soft mode lets it through)", got)
			}
			if n := metrics.Count(metrics.AgentProduceWarnedTotal); n != 1 {
				t.Fatalf("AgentProduceWarnedTotal = %d, want 1", n)
			}
			if n := metrics.Count(metrics.AgentProduceRejectedTotal); n != 0 {
				t.Fatalf("AgentProduceRejectedTotal = %d, want 0 (soft mode must not reject)", n)
			}
		})
	}
}

// TestTransferReceivedMissingFileNameRejected：transfer.received 有 transferId 与
// metadata.taskId 但 fileName 为空 → 422 ARTIFACT_FILENAME_REQUIRED，且不产生任何
// artifact 记录，计数 +1。
func TestTransferReceivedMissingFileNameRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics.Reset()
	t.Cleanup(metrics.Reset)

	s, _, taskID, devNode := newTransferTestFixture(t)
	h := NewWebhookHandler(WebhookDeps{Store: s})

	body, err := json.Marshal(map[string]any{
		"transferId": "tid-empty-filename-0001",
		"fileName":   "",
		"fileSize":   1234,
		"localPath":  "/var/lib/trustmesh-transfers/tid-empty-filename-0001",
		"mimeType":   "text/markdown",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhook/clawsynapse", nil)
	h.handleTransferReceived(c, protocol.WebhookPayload{
		NodeID:   devNode,
		Type:     "transfer.received",
		From:     devNode,
		Message:  string(body),
		Metadata: map[string]any{"taskId": taskID, "todoId": "TD_01"},
	})

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "ARTIFACT_FILENAME_REQUIRED") {
		t.Fatalf("body missing code ARTIFACT_FILENAME_REQUIRED: %s", w.Body.String())
	}
	if artifacts := s.GetArtifactsByTaskID(taskID); len(artifacts) != 0 {
		t.Fatalf("rejected upload must not create artifacts, got %d", len(artifacts))
	}
	if n := metrics.Count(metrics.AgentProduceRejectedTotal); n != 1 {
		t.Fatalf("AgentProduceRejectedTotal = %d, want 1", n)
	}
}

// TestTodoCompleteWhitespaceProduceRejected（T0.12a / F-01 回归）：
// 纯空白的产出声明（summary 空白、output 制表符/换行、或 content 空白经
// backfill 回填到 summary）在 strictProduceGate=true 下一律 422 拒绝，
// 不得被 `!= ""` 当成「有产出」而放行。todo 保持未完成、计数 +1。
func TestTodoCompleteWhitespaceProduceRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withStrictProduceGate(t, true)

	cases := []struct {
		name    string
		message string
	}{
		{"summary 纯空格", `{"task_id":"%s","todo_id":"TD_01","result":{"summary":"   "}}`},
		{"output 制表符换行", `{"task_id":"%s","todo_id":"TD_01","result":{"output":"\t\n"}}`},
		{"content 纯空格 backfill", `{"task_id":"%s","todo_id":"TD_01","content":"   "}`},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			metrics.Reset()
			t.Cleanup(metrics.Reset)

			s, _, taskID, devNode := newTransferTestFixture(t)
			h := NewWebhookHandler(WebhookDeps{Store: s})

			w := postWebhookJSON(t, h, protocol.WebhookPayload{
				Type:       "todo.complete",
				From:       devNode,
				SessionKey: taskID,
				Message:    fmt.Sprintf(c.message, taskID),
				Metadata:   map[string]any{},
			})

			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422 (whitespace must not bypass gate); body=%s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "TODO_RESULT_EMPTY") {
				t.Fatalf("body missing TODO_RESULT_EMPTY: %s", w.Body.String())
			}
			if got := todoStatusOf(t, taskID, "TD_01", h); got == "done" {
				t.Fatalf("todo must stay unfinished on whitespace rejection, got %q", got)
			}
			if n := metrics.Count(metrics.AgentProduceRejectedTotal); n != 1 {
				t.Fatalf("AgentProduceRejectedTotal = %d, want 1", n)
			}
		})
	}
}
