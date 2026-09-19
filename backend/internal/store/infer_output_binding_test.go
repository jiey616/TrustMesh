package store

import (
	"testing"

	"trustmesh/backend/internal/model"
)

// ─────────────────────────── 交付物绑定的两道闸门 ───────────────────────────
//
// Background: the manual BindStepOutput path has always refused names a step
// does not declare (OUTPUT_SLOT_NOT_DECLARED) and demoted displaced
// deliverables. The agent upload path trusted whatever name the agent sent and
// never demoted anything. 2026-09-19 画宗《双羊尊》 is what that drift costs: the
// screenwriter declared the single slot on 剧本创作 for all 24 process .md
// drafts, so every draft showed up as a deliverable.

const filingTestDocxMime = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

const filingTestSlotName = "剧名_剧本类型_版本_时间"

// filingTestSlot mirrors the 画宗 workflow's 剧本创作 step: exactly one slot.
func filingTestSlot() []model.StepOutput {
	return []model.StepOutput{{Name: filingTestSlotName, MimeType: "docx"}}
}

// TestInferOutputBindingDeclaredValid pins the happy path: a name the step
// declares, with a mime the slot accepts, binds as a declared deliverable.
func TestInferOutputBindingDeclaredValid(t *testing.T) {
	name, by, warn := inferOutputBinding(model.TaskArtifact{
		OutputName: filingTestSlotName,
		MimeType:   filingTestDocxMime,
	}, filingTestSlot())

	if name != filingTestSlotName || by != "declared" || warn {
		t.Fatalf("got name=%q by=%q warn=%v, want a clean declared binding", name, by, warn)
	}
}

// TestInferOutputBindingDeclaredSlotUnknown is the 画宗 guard: the agent named
// a slot this step does not declare (真实事故：`剧本解析` bound to a step whose
// slots are 角色/场景/道具/资产索引表). No downstream step can ever resolve that
// name, so it must not claim a slot.
func TestInferOutputBindingDeclaredSlotUnknown(t *testing.T) {
	name, by, warn := inferOutputBinding(model.TaskArtifact{
		OutputName: "剧本解析",
		MimeType:   filingTestDocxMime,
	}, filingTestSlot())

	if name != "" || by != "declared_slot_unknown" || !warn {
		t.Fatalf("got name=%q by=%q warn=%v, want the binding rejected", name, by, warn)
	}
}

// TestInferOutputBindingDeclaredMimeMismatch covers the 2026-09-04 shape that
// the mime guard used to only apply to the inferred branch: an .md screenplay
// claiming the docx deliverable slot.
func TestInferOutputBindingDeclaredMimeMismatch(t *testing.T) {
	name, by, warn := inferOutputBinding(model.TaskArtifact{
		OutputName: filingTestSlotName,
		MimeType:   "text/markdown",
	}, filingTestSlot())

	if name != "" || by != "declared_mime_mismatch" || !warn {
		t.Fatalf("got name=%q by=%q warn=%v, want a mime-mismatch rejection", name, by, warn)
	}
}

// TestInferOutputBindingUnknownMimeIsRejected pins that an upload whose mime
// could not be determined never claims a slot — mimeMatchesDeclared already
// treats empty as "no match", and the declared branch must honour that too.
func TestInferOutputBindingUnknownMimeIsRejected(t *testing.T) {
	name, by, _ := inferOutputBinding(model.TaskArtifact{
		OutputName: filingTestSlotName,
		MimeType:   "",
	}, filingTestSlot())

	if name != "" || by != "declared_mime_mismatch" {
		t.Fatalf("got name=%q by=%q, want an unknown mime to be rejected", name, by)
	}
}

// TestInferOutputBindingNoDeclaredSlotsKeepsFreeName keeps the legacy
// free-form behaviour for steps that declare nothing — there is nothing to
// validate against, and BindStepOutput applies the same rule.
func TestInferOutputBindingNoDeclaredSlotsKeepsFreeName(t *testing.T) {
	name, by, warn := inferOutputBinding(model.TaskArtifact{
		OutputName: "随便一个名字",
		MimeType:   "text/markdown",
	}, nil)

	if name != "随便一个名字" || by != "declared" || warn {
		t.Fatalf("got name=%q by=%q warn=%v, want the legacy free-form binding", name, by, warn)
	}
}

// TestInferOutputBindingDeclaredMultiSlotStillBinds is the payoff for
// validating instead of blanket-trusting: a multi-slot step (the 军旅 workflow
// declares two xlsx slots) can now be bound by name, which inference alone
// deliberately refuses to guess.
func TestInferOutputBindingDeclaredMultiSlotStillBinds(t *testing.T) {
	declared := []model.StepOutput{
		{Name: "剧名_分镜头脚本", MimeType: "xlsx"},
		{Name: "剧名_逐镜视频生成提示词", MimeType: "xlsx"},
	}
	xlsx := "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

	name, by, warn := inferOutputBinding(model.TaskArtifact{
		OutputName: "剧名_逐镜视频生成提示词",
		MimeType:   xlsx,
	}, declared)
	if name != "剧名_逐镜视频生成提示词" || by != "declared" || warn {
		t.Fatalf("got name=%q by=%q warn=%v, want a declared binding on a multi-slot step", name, by, warn)
	}

	// ...but an undeclared name on the same step is still refused.
	if name, by, warn := inferOutputBinding(model.TaskArtifact{
		OutputName: "剧名_分镜脚本",
		MimeType:   xlsx,
	}, declared); name != "" || by != "declared_slot_unknown" || !warn {
		t.Fatalf("got name=%q by=%q warn=%v, want the undeclared name refused", name, by, warn)
	}
}

// TestInferOutputBindingSlotNamePaddingIsTolerated: a template that pads its
// slot name must not silently reject a valid binding.
func TestInferOutputBindingSlotNamePaddingIsTolerated(t *testing.T) {
	name, by, warn := inferOutputBinding(model.TaskArtifact{
		OutputName: filingTestSlotName,
		MimeType:   filingTestDocxMime,
	}, []model.StepOutput{{Name: "  " + filingTestSlotName + " ", MimeType: "docx"}})

	if name != filingTestSlotName || by != "declared" || warn {
		t.Fatalf("got name=%q by=%q warn=%v, want the padded slot name to match", name, by, warn)
	}
}

// TestInferOutputBindingStrictGateOffRestoresLegacyTrust is the documented
// rollback: with the kill switch off the old trust-the-agent behaviour is back,
// so an operator facing a mis-declared template can restore service without a
// rebuild.
func TestInferOutputBindingStrictGateOffRestoresLegacyTrust(t *testing.T) {
	orig := declaredSlotStrict
	declaredSlotStrict = false
	t.Cleanup(func() { declaredSlotStrict = orig })

	if name, by, warn := inferOutputBinding(model.TaskArtifact{
		OutputName: "剧本解析",
		MimeType:   "text/markdown",
	}, filingTestSlot()); name != "剧本解析" || by != "declared" || warn {
		t.Fatalf("got name=%q by=%q warn=%v, want the legacy behaviour back", name, by, warn)
	}
}

// TestInferOutputBindingSingleSlotMimeMismatchStaysUnbound pins the inferred
// branch's pre-existing (unchanged) policy.
func TestInferOutputBindingSingleSlotMimeMismatchStaysUnbound(t *testing.T) {
	name, by, warn := inferOutputBinding(model.TaskArtifact{
		MimeType: "text/markdown",
	}, filingTestSlot())

	if name != "" || by != "" || !warn {
		t.Fatalf("got name=%q by=%q warn=%v, want an unbound warning", name, by, warn)
	}
}

// TestInferOutputBindingInferredSingleSlot covers the inferred happy path —
// the behaviour the 2026-09-02 fix introduced and which must survive this
// change (agents are never told the slot name, so their final deliverable
// arrives with no outputName at all).
func TestInferOutputBindingInferredSingleSlot(t *testing.T) {
	name, by, warn := inferOutputBinding(model.TaskArtifact{
		MimeType: "text/markdown",
	}, []model.StepOutput{{Name: "剧本正文", MimeType: "markdown"}})

	if name != "剧本正文" || by != "inferred" || warn {
		t.Fatalf("got name=%q by=%q warn=%v, want an inferred binding", name, by, warn)
	}
}
