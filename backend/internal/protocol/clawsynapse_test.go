package protocol

import (
	"encoding/json"
	"testing"
)

// Regression 2026-09-04: the 山雨 screenwriter node sent a correct
// todo.complete whose "result" was a plain string. The strict object decode
// 400-rejected the message ("invalid todo.complete message") and stalled the
// task. FlexibleTodoResult must accept both shapes.
func TestTodoCompletePayloadResultAsString(t *testing.T) {
	raw := `{"task_id":"t1","todo_id":"TD_01","result":"【终局交付】完成剧本"}`
	var p TodoCompletePayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("string result decode failed: %v", err)
	}
	if p.Result.Summary != "【终局交付】完成剧本" {
		t.Fatalf("summary = %q", p.Result.Summary)
	}
}

func TestTodoCompletePayloadResultAsObject(t *testing.T) {
	raw := `{"task_id":"t1","todo_id":"TD_01","result":{"summary":"s","output":"o"}}`
	var p TodoCompletePayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("object result decode failed: %v", err)
	}
	if p.Result.Summary != "s" || p.Result.Output != "o" {
		t.Fatalf("result = %+v", p.Result)
	}
}

func TestTodoCompletePayloadResultInvalidStillFails(t *testing.T) {
	raw := `{"task_id":"t1","todo_id":"TD_01","result":42}`
	var p TodoCompletePayload
	if err := json.Unmarshal([]byte(raw), &p); err == nil {
		t.Fatal("numeric result should not decode")
	}
}
