// Package metrics — protocol-envelope dual-read counters (T0.6a).
//
// Context: an inbound ClawSynapse webhook carries a two-layer envelope. The
// outer layer is the HTTP body decoded into protocol.WebhookPayload; the inner
// layer is WebhookPayload.Message, a string that may itself be a JSON
// protocol envelope ({"protocol":"clawsynapse/...","type":...}). Before T0.6a
// the platform had no visibility into how much traffic arrives in each shape,
// so a future tightening of the message parser (T0.6b+) would risk silently
// 400-rejecting legacy hand-rolled JSON — the exact failure mode behind the
// 2026-09-04 山雨 todo.complete incident (a string "result" was strict-decoded
// and rejected, the node retried, the task stalled).
//
// These counters are OBSERVE-ONLY: they classify, never reject. Declared in a
// dedicated file (not metrics.go) to keep the T0.8 shutdown-flush change and
// this one independent. They are deliberately NOT registered in
// KnownCounterNames()/AlertRules(): those lists are covered by
// TestAlertRulesCoverEveryDeclaredMetric, which requires every listed metric
// to have an alert rule, and these are diagnostic dual-read counters rather
// than page-worthy signals.
package metrics

const (
	// EnvelopeProtocolTotal counts inbound messages whose inner message is a
	// well-formed protocol envelope (valid JSON object carrying a
	// "protocol" field that contains "clawsynapse"). See
	// classifyEnvelopeShape / envShapeProtocol.
	EnvelopeProtocolTotal = "envelope_protocol_total"

	// EnvelopeLegacyJSONTotal counts valid JSON that carries NO protocol field
	// — the legacy hand-rolled payload shape (e.g. a bare
	// {"task_id":..,"todo_id":..,"result":..}). This is the shape T0.6a must
	// keep accepting; a nonzero count is the baseline a future stricter parser
	// has to preserve.
	EnvelopeLegacyJSONTotal = "envelope_legacy_json_total"

	// EnvelopePlainTextTotal counts non-JSON text messages (human/agent prose,
	// or anything not starting with '{'/'['). See envShapePlainText.
	EnvelopePlainTextTotal = "envelope_plain_text_total"

	// EnvelopeInvalidJSONTotal counts messages that look like JSON ('{'/'['
	// first) but fail to parse. These are already handled leniently downstream
	// (loose decode / repair); the counter just makes the volume visible.
	EnvelopeInvalidJSONTotal = "envelope_invalid_json_total"
)
