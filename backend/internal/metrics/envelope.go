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

	// AgentProduceRejectedTotal counts produces hard-rejected at the two
	// compliance chokepoints (T0.12): a todo.complete carrying no produce
	// declaration, or a transfer.received carrying an empty fileName.
	AgentProduceRejectedTotal = "agent_produce_rejected_total"

	// AgentProduceWarnedTotal counts produces that failed the same compliance
	// check but were let through because the strict gate is off. Lets a deploy
	// measure the blast radius of the strict gate before enabling it.
	AgentProduceWarnedTotal = "agent_produce_warned_total"

	// ProtocolSchemaRejectedTotal counts inbound task/todo messages hard-rejected
	// because they did not carry a protocol envelope while the strict gate is on
	// (T1.6). Scope is the structured task/todo types only (see
	// requiresProtocolEnvelope); free-text channels (chat./meeting./prose
	// responses/errors) are never counted here.
	ProtocolSchemaRejectedTotal = "protocol_schema_rejected_total"

	// ProtocolSchemaWarnedTotal counts the same shape mismatch but let through
	// because the strict gate is off — the "blast radius" meter a deploy watches
	// before flipping TRUSTMESH_STRICT_PROTOCOL_SCHEMA_GATE to true. It mirrors
	// AgentProduceWarnedTotal and, like the T0.6a counters, is observe-only.
	ProtocolSchemaWarnedTotal = "protocol_schema_warned_total"

	// DeliverableQualityRejectedTotal counts uploaded deliverables hard-rejected
	// by the judgeable quality gate (T1.10): a zero-byte file, or a file whose
	// type cannot be identified. Only objective defects count — no subjective
	// judgement is encoded here.
	DeliverableQualityRejectedTotal = "deliverable_quality_rejected_total"

	// DeliverableQualityWarnedTotal counts the same defects let through (marked
	// on the task timeline + ops feed) because the strict gate is off. Observe
	// first, then flip TRUSTMESH_STRICT_DELIVERABLE_QUALITY_GATE.
	DeliverableQualityWarnedTotal = "deliverable_quality_warned_total"
)
