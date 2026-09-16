package store

import (
	"encoding/json"
	"testing"
	"time"

	"go.uber.org/zap"

	"trustmesh/backend/internal/model"
)

// ─── T3.1 W2：SSE 广播的纯内存侧不变量（不依赖 Mongo）───

func newBroadcastStore() *Store {
	s := New()
	s.log = zap.NewNop()
	s.sseInstanceID = "instance-test"
	return s
}

// 广播关闭（默认）→ outboxCh 为 nil → enqueue 返回 false，绝不 panic。
// 这是「默认关闭 = 行为与改造前逐字节一致」的核心护栏。
func TestSSEEnqueueDisabledIsNoop(t *testing.T) {
	s := newBroadcastStore()
	if s.outboxCh != nil {
		t.Fatalf("outboxCh must be nil when broadcast disabled")
	}
	if s.SSEBroadcastArmed() {
		t.Fatalf("must not be armed when disabled")
	}
	ok := s.enqueueSSEEventUnsafe(model.UserStreamEvent{Type: "task.updated"}, "u-1")
	if ok {
		t.Fatalf("enqueue must report not-enqueued when broadcast off")
	}
}

// 正常入队 → 返回 true，且缓冲里能取到该事件（origin/user/payload 完整）。
func TestSSEEnqueueDeliversToOutbox(t *testing.T) {
	s := newBroadcastStore()
	s.outboxCh = make(chan sseUserEventDoc, sseOutboxCapacity)

	at := time.Now().UTC().Truncate(time.Millisecond)
	ok := s.enqueueSSEEventUnsafe(model.UserStreamEvent{
		ID: "evt-1", Type: "task.updated", OccurredAt: at,
		Payload: map[string]any{"task_id": "t-1"},
	}, "u-42")
	if !ok {
		t.Fatalf("enqueue must succeed when outbox has room")
	}

	select {
	case doc := <-s.outboxCh:
		if doc.UserID != "u-42" {
			t.Fatalf("user_id = %q, want u-42", doc.UserID)
		}
		if doc.OriginID != "instance-test" {
			t.Fatalf("origin_id = %q, want instance-test", doc.OriginID)
		}
		if doc.CreatedAt.IsZero() {
			t.Fatalf("created_at must be set (TTL depends on it)")
		}
		var back model.UserStreamEvent
		if err := json.Unmarshal([]byte(doc.EventJSON), &back); err != nil {
			t.Fatalf("stored event_json must be valid JSON: %v", err)
		}
		if back.ID != "evt-1" || back.Type != "task.updated" {
			t.Fatalf("round-trip lost identity: %+v", back)
		}
		if back.Payload["task_id"] != "t-1" {
			t.Fatalf("round-trip lost payload: %+v", back.Payload)
		}
	default:
		t.Fatalf("event must be buffered in outbox")
	}
}

// outbox 满 → 丢弃并计数，**绝不阻塞**（持锁路径的硬约束）。
func TestSSEEnqueueDropsWhenFullWithoutBlocking(t *testing.T) {
	s := newBroadcastStore()
	s.outboxCh = make(chan sseUserEventDoc, 2) // 故意小容量

	ev := model.UserStreamEvent{ID: "x", Type: "task.updated", OccurredAt: time.Now().UTC()}
	if !s.enqueueSSEEventUnsafe(ev, "u-1") {
		t.Fatalf("first enqueue must succeed")
	}
	if !s.enqueueSSEEventUnsafe(ev, "u-1") {
		t.Fatalf("second enqueue must succeed")
	}

	done := make(chan bool, 1)
	go func() { done <- s.enqueueSSEEventUnsafe(ev, "u-1") }()
	select {
	case ok := <-done:
		if ok {
			t.Fatalf("third enqueue must report drop when outbox is full")
		}
	case <-time.After(time.Second):
		t.Fatalf("enqueue BLOCKED on a full outbox — must never happen on the locked path")
	}
	if got := s.sseDropped.Load(); got != 1 {
		t.Fatalf("dropped counter = %d, want 1", got)
	}
}

// userID 为空 → publishUserEventUnsafe 提前返回，不入 outbox（沿用既有短路语义）。
func TestSSEPublishSkipsEmptyUser(t *testing.T) {
	s := newBroadcastStore()
	s.outboxCh = make(chan sseUserEventDoc, sseOutboxCapacity)
	s.publishUserEventUnsafe("", "task.updated", map[string]any{"k": "v"}, time.Now().UTC())
	select {
	case <-s.outboxCh:
		t.Fatalf("empty user must not enqueue a broadcast event")
	default:
	}
}

// 载荷里内嵌 struct（真实调用点：store_events.go 把 model.Event / model.Agent 塞进 payload）。
// 必须验证「本地直投」与「跨实例经 outbox 回灌」两条路径交给前端的字段名与值一致
// —— 这正是我们选择存 JSON 字符串、而非原生 BSON 文档的理由。
//
// 注：只断言语义等价，不断言字节相等。direct 路径里嵌套值是 struct（encoding/json 按字段
// 声明序输出），回灌路径里是 map（按 key 字母序输出），顺序必然不同；前端 JSON.parse 不关心
// 顺序，关心的是**键名**（json tag）与值。
func TestSSEBroadcastPreservesJSONTagKeysAcrossInstances(t *testing.T) {
	s := newBroadcastStore()
	s.outboxCh = make(chan sseUserEventDoc, sseOutboxCapacity)

	at := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	payload := map[string]any{
		"task_id": "t-1",
		// model.Event 的 ID 字段是 json:"id" / bson:"_id" —— 若走原生 BSON 往返，
		// 解码成 map[string]any 后键会退回 "_id"，前端按 "id" 取值即断裂。
		"event": model.Event{ID: "e-1", EventType: "todo_done", TaskID: "t-1", Content: strPtr("步骤完成")},
	}

	// 走真实咽喉函数：本地投递 + 入 outbox（而不是手工 marshal 后塞 channel）。
	s.publishUserEventUnsafe("u-1", "task.event.created", payload, at)
	doc := <-s.outboxCh
	if doc.UserID != "u-1" {
		t.Fatalf("doc.UserID = %q, want u-1", doc.UserID)
	}
	var viaBus model.UserStreamEvent
	if err := json.Unmarshal([]byte(doc.EventJSON), &viaBus); err != nil {
		t.Fatalf("unmarshal bus copy: %v", err)
	}

	if viaBus.Type != "task.event.created" {
		t.Fatalf("top-level type = %q, want task.event.created", viaBus.Type)
	}
	if viaBus.ID == "" {
		t.Fatalf("event must carry a generated id")
	}
	if !viaBus.OccurredAt.Equal(at.UTC()) {
		t.Fatalf("occurred_at = %v, want %v", viaBus.OccurredAt, at)
	}
	if viaBus.Payload["task_id"] != "t-1" {
		t.Fatalf("payload.task_id = %v, want t-1", viaBus.Payload["task_id"])
	}

	nested, ok := viaBus.Payload["event"].(map[string]any)
	if !ok {
		t.Fatalf("nested event must decode to an object, got %T", viaBus.Payload["event"])
	}
	// 键名必须是 json tag（"id"），不能是 bson tag（"_id"）。
	if nested["id"] != "e-1" {
		t.Fatalf(`nested event must keep json tag key "id" (not bson "_id"); got keys: %v`, keysOf(nested))
	}
	if _, leaked := nested["_id"]; leaked {
		t.Fatalf(`bson tag key "_id" must not leak into the broadcast payload`)
	}
	if nested["event_type"] != "todo_done" {
		t.Fatalf("nested event_type = %v, want todo_done", nested["event_type"])
	}
	content, ok := nested["content"].(string)
	if !ok || content != "步骤完成" {
		t.Fatalf("nested content = %v, want 步骤完成", nested["content"])
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// deliverUserEvent 把远端事件投给本实例订阅者（tailer 的出口）。
func TestDeliverUserEventReachesLocalSubscriber(t *testing.T) {
	s := New()
	s.log = zap.NewNop()

	ch, unsub := s.SubscribeUser("u-1")
	defer unsub()

	s.deliverUserEvent("u-1", model.UserStreamEvent{
		ID: "remote-1", Type: "notification.created", OccurredAt: time.Now().UTC(),
		Payload: map[string]any{"a": "b"},
	})

	select {
	case got := <-ch:
		if got.ID != "remote-1" {
			t.Fatalf("delivered event id = %q, want remote-1", got.ID)
		}
	case <-time.After(time.Second):
		t.Fatalf("remote event must reach the local subscriber")
	}
}

// deliverUserEvent 对空 userID / 无订阅者用户都必须安全无操作（tailer 会大量遇到）。
func TestDeliverUserEventIgnoresEmptyUser(t *testing.T) {
	s := New()
	s.log = zap.NewNop()
	s.deliverUserEvent("", model.UserStreamEvent{ID: "x"})
	s.deliverUserEvent("nobody", model.UserStreamEvent{ID: "y"})
}

// marshal 失败（不可序列化的 payload，如 chan/func）→ EventJSON 为空串，
// writer 跳过落库，业务写路径不受影响。
func TestSSEMarshalFailureYieldsEmptyDoc(t *testing.T) {
	s := newBroadcastStore()
	s.outboxCh = make(chan sseUserEventDoc, sseOutboxCapacity)

	s.publishUserEventUnsafe("u-1", "task.updated", map[string]any{"bad": make(chan int)}, time.Now().UTC())

	doc := <-s.outboxCh
	if doc.EventJSON != "" {
		t.Fatalf("unmarshalable payload must yield empty EventJSON, got %q", doc.EventJSON)
	}
}
