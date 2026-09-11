package store

import "strings"

// errorCommentPatterns matches agent comments that report a FAILURE rather
// than progress. Such comments must NOT reset the timeout counter: a crashing
// agent that emits an error report every 30 minutes would otherwise keep
// itself alive forever (2026-09-10: TD_05 stalled 8h across 14 reminders,
// every one of them with remind_count=1 because each error report reset it).
//
// This list is deliberately conservative: anything that does not match is
// treated as progress, so persona-style agents that report via task.comment
// keep working exactly as before.
var errorCommentPatterns = []string{
	"【执行侧故障上报】",
	"【故障上报】",
	"hermes run failed",
	"hermes gateway",
	"adapter error",
	"Billing or credits exhausted",
	"pre-consumed quota failed",
	"Context length exceeded",
	"Operation interrupted",
	"context deadline exceeded",
	"dial tcp",
	"connect: connection refused",
	"401 Unauthorized",
	"context 被模型服务拒绝",
	"任务上下文被模型服务拒绝",
	// P-08: gateway truncation notice, e.g. "Turn ended with pending tool
	// result ... budget=60/60". The run is cut off BEFORE todo.complete, so
	// the todo merely looks slow and gets reminded for hours.
	"budget=",
	"Turn ended with pending tool result",
}

// IsErrorComment reports whether an agent comment is a failure report rather
// than a progress signal.
//
// NOTE: pattern matching is a stopgap. Once the node side emits a dedicated
// todo.error message type (node-side N-07), callers should branch on the
// message type first and keep this as a fallback only.
func IsErrorComment(content string) bool {
	c := strings.TrimSpace(content)
	if c == "" {
		return false
	}
	for _, p := range errorCommentPatterns {
		if strings.Contains(c, p) {
			return true
		}
	}
	return false
}

// budgetExhaustedPatterns identifies the gateway's silent truncation notice.
// Kept separate from errorCommentPatterns so the platform can raise a
// dedicated signal instead of only withholding liveness credit.
var budgetExhaustedPatterns = []string{
	"budget=",
	"Turn ended with pending tool result",
}

// IsBudgetExhaustedComment reports whether an agent comment shows the run hit
// its tool-call budget and was truncated before it could report completion.
//
// The platform never sees a run-level event for this: the gateway simply ends
// the turn mid-flight. All that surfaces is this text inside a comment, so
// recognising it is the only way to tell "the run died" apart from "the step
// is just slow" - without it the todo is reminded for hours on a run that
// can no longer make progress (2026-09-10 TD_05: budget gone 23:09, todo only
// completed 06:56).
//
// NOTE: the budget value lives on the node/hermes side. The platform only
// detects and reports it; raising the limit is a node-side decision (N-10).
func IsBudgetExhaustedComment(content string) bool {
	c := strings.TrimSpace(content)
	if c == "" {
		return false
	}
	for _, p := range budgetExhaustedPatterns {
		if strings.Contains(c, p) {
			return true
		}
	}
	return false
}
