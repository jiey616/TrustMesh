package clawsynapse

import (
	"strings"
	"testing"

	"trustmesh/backend/internal/protocol"
)

// Regression 2026-09-04: after a 400 on todo.complete, the writer node
// retried by publishing the full task-protocol envelope as chat.message with
// a random session key. The chat lookup 404'd ("agent chat not found") and
// the todo stalled. unwrapTaskProtocolEnvelope must detect and retag these.
func TestUnwrapTaskProtocolEnvelope(t *testing.T) {
	envelope := `{"protocol": "clawsynapse/1.0", "type": "todo.complete", "from": "n1-a", "to": "n1-b", "session": "8dd663f4-208c-4276-ada7-f6900e47e0d7", "task_id": "t1", "todo_id": "TD_01", "result": "done"}`
	webhook := protocol.WebhookPayload{Type: "chat.message", SessionKey: "8dd663f4", Message: envelope}

	got, ok := unwrapTaskProtocolEnvelope(webhook)
	if !ok {
		t.Fatal("envelope not detected")
	}
	if got.Type != "todo.complete" {
		t.Fatalf("type = %q", got.Type)
	}
}

// The 山雨 orchestrator (measured 2026-09-04 06:26) builds its envelope in
// python with the task payload nested under a "body" key and publishes via
// `clawsynapse publish --message <envelope>` (no --type flag), so the wire
// type is chat.message. The body must be swapped in as the real payload.
func TestUnwrapTaskProtocolEnvelopeNestedBody(t *testing.T) {
	envelope := `{"protocol": "clawsynapse/1.0", "type": "todo.complete", "from": "n1-a", "to": "n1-b", "session": "t1", "body": {"task_id": "t1", "todo_id": "TD_01", "status": "completed", "result": "done"}}`
	webhook := protocol.WebhookPayload{Type: "chat.message", SessionKey: "random-uuid", Message: envelope}

	got, ok := unwrapTaskProtocolEnvelope(webhook)
	if !ok {
		t.Fatal("nested-body envelope not detected")
	}
	if got.Type != "todo.complete" {
		t.Fatalf("type = %q", got.Type)
	}
	if !strings.Contains(got.Message, `"task_id"`) || strings.Contains(got.Message, `"body"`) {
		t.Fatalf("message not swapped to body: %s", got.Message)
	}

	// A string body must not be swapped in as the payload.
	envelope2 := `{"protocol": "clawsynapse/1.0", "type": "todo.complete", "body": "just text"}`
	got2, ok := unwrapTaskProtocolEnvelope(protocol.WebhookPayload{Message: envelope2})
	if !ok {
		t.Fatal("nested-body envelope (string) not detected")
	}
	if got2.Message != envelope2 {
		t.Fatalf("string body should stay untouched, got %s", got2.Message)
	}
}

func TestUnwrapTaskProtocolEnvelopeIgnoresPlainChat(t *testing.T) {
	cases := []string{
		"你好，请查收交付文件",
		`{"task_id":"t1","comment":"no protocol field"}`,
		`{"protocol":"clawsynapse/1.0","type":"chat.message","content":"hi"}`,
	}
	for _, msg := range cases {
		webhook := protocol.WebhookPayload{Type: "chat.message", Message: msg}
		if _, ok := unwrapTaskProtocolEnvelope(webhook); ok {
			t.Fatalf("unexpected unwrap for %q", msg)
		}
	}
}
