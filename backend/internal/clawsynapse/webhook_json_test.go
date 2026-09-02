package clawsynapse

import "testing"

func TestDecodeWebhookMessageTolerant(t *testing.T) {
	type payload struct {
		TaskID   string `json:"task_id"`
		Question string `json:"question"`
	}

	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "valid json passes through",
			raw:  `{"task_id":"t1","question":"hello"}`,
			want: "hello",
		},
		{
			name: "bare newline inside string is repaired",
			raw:  "{\"task_id\":\"t1\",\"question\":\"line1\nline2\"}",
			want: "line1\nline2",
		},
		{
			name: "bare tab inside string is repaired",
			raw:  "{\"task_id\":\"t1\",\"question\":\"a\tb\"}",
			want: "a\tb",
		},
		{
			name: "bare content quote inside string is repaired",
			raw:  `{"task_id":"t1","question":"他说"你好""}`,
			want: `他说"你好"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var p payload
			if err := decodeWebhookMessage(tc.raw, &p); err != nil {
				t.Fatalf("decode failed: %v", err)
			}
			if p.Question != tc.want {
				t.Fatalf("question = %q, want %q", p.Question, tc.want)
			}
			if p.TaskID != "t1" {
				t.Fatalf("task_id = %q, want t1", p.TaskID)
			}
		})
	}
}

func TestFixLLMJSONUnchangedForValidJSON(t *testing.T) {
	valid := `{"task_id":"t1","question":"hello\nworld","options":["A","B"]}`
	if got := fixLLMJSON(valid); got != valid {
		t.Fatalf("valid JSON should be unchanged, got %q", got)
	}
}
