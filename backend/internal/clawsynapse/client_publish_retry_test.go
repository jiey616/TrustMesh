package clawsynapse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestPublishWithRetryDoesNotRetryNodeRejection 验证节点明确拒绝（4xx）时不重试：
// 请求已经到达节点并被拒绝，重试没有意义且可能重复投递。
func TestPublishWithRetryDoesNotRetryNodeRejection(t *testing.T) {
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"code":"BAD_PAYLOAD"}`))
	}))
	defer ts.Close()

	c := NewClient(ts.URL, 2*time.Second)
	if c == nil {
		t.Fatalf("client must not be nil")
	}
	if _, err := c.PublishWithRetry(context.Background(), "node-1", "todo.assigned", map[string]any{"a": 1}, "sess-1", nil); err == nil {
		t.Fatalf("expected error from 4xx response")
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("4xx must not be retried, got %d attempts", got)
	}
}

// TestPublishWithRetryRetriesTransportError 验证传输层错误（请求从未到达节点）
// 会被有界重试：这是 2026-09-10 TD_04 停顿 56 分钟的直接修复点。
func TestPublishWithRetryRetriesTransportError(t *testing.T) {
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"data":{}}`))
	}))
	url := ts.URL
	ts.Close() // 关闭后连接被拒 → url.Error（传输层）

	c := NewClient(url, 1*time.Second)
	if c == nil {
		t.Fatalf("client must not be nil")
	}

	start := time.Now()
	_, err := c.PublishWithRetry(context.Background(), "node-1", "todo.assigned", map[string]any{"a": 1}, "sess-1", nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected transport error to surface after retries")
	}
	if !isTransportError(err) {
		t.Fatalf("expected a transport-classified error, got %v", err)
	}
	// 3 次尝试 + 退避 500ms + 1000ms ⇒ 至少 ~1.5s
	if elapsed < publishBaseBackoff {
		t.Fatalf("expected backoff between retries, elapsed=%v", elapsed)
	}
}

// TestPublishWithRetrySucceedsFirstTry 正常路径必须零重试。
func TestPublishWithRetrySucceedsFirstTry(t *testing.T) {
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"data":{"message_id":"m1"}}`))
	}))
	defer ts.Close()

	c := NewClient(ts.URL, 2*time.Second)
	res, err := c.PublishWithRetry(context.Background(), "node-1", "todo.assigned", map[string]any{"a": 1}, "sess-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatalf("expected result")
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("happy path must publish exactly once, got %d", got)
	}
}
