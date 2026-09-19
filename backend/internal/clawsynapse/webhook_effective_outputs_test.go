package clawsynapse

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
)

// ────────── 下游输入解析：显式绑定也要过 kind 闸门（与 pipeline 同口径） ──────────
//
// effectiveTodoOutputs used to return todo.Outputs verbatim whenever the slice
// was non-empty, while stepOutputsUnsafe (the pipeline view) filtered the same
// rows by kind. A binding left pointing at a process artifact — legacy data, or
// a slot that has since been rebound — therefore still fed the next step,
// which is the 2026-09-01/02 wrong-input incident all over again.

func newOutputsTestHandler() *WebhookHandler {
	return NewWebhookHandler(WebhookDeps{Store: store.New()})
}

// TestEffectiveTodoOutputsDropsStaleBindingAndFallsBack: a binding whose
// artifact is a process file must be filtered out, and the todo's real
// deliverable must still be found by the artifact fallback rather than
// reporting the step as producing nothing.
func TestEffectiveTodoOutputsDropsStaleBindingAndFallsBack(t *testing.T) {
	h := newOutputsTestHandler()
	task := &model.TaskDetail{
		ID: "t1",
		Artifacts: []model.TaskArtifact{
			{TransferID: "tr-draft", TodoID: "td-1", Kind: model.ArtifactKindProcess,
				FileName: "01-intake.md", MimeType: "text/markdown"},
			{TransferID: "tr-final", TodoID: "td-1", Kind: model.ArtifactKindDeliverable,
				OutputName: "剧本正文", FileName: "剧本_v1.md", MimeType: "text/markdown"},
		},
		Todos: []model.Todo{{
			ID: "td-1",
			Outputs: []model.TodoOutput{
				// Stale row: the slot points at a file that ended up classified
				// as a process artifact.
				{OutputName: "剧本正文", ArtifactID: "tr-draft", FileRef: "pf-draft"},
			},
		}},
	}

	outs := h.effectiveTodoOutputs(task, &task.Todos[0])
	if len(outs) != 1 || outs[0].ArtifactID != "tr-final" {
		t.Fatalf("outputs = %+v, want only the deliverable artifact", outs)
	}
}

// TestEffectiveTodoOutputsKeepsValidBindings: the healthy path is unchanged —
// bindings that resolve to a deliverable are handed over verbatim.
func TestEffectiveTodoOutputsKeepsValidBindings(t *testing.T) {
	h := newOutputsTestHandler()
	task := &model.TaskDetail{
		ID: "t1",
		Artifacts: []model.TaskArtifact{
			{TransferID: "tr-final", TodoID: "td-1", Kind: model.ArtifactKindDeliverable,
				FileName: "剧本_v1.md", MimeType: "text/markdown"},
			{TransferID: "tr-extra", TodoID: "td-1", Kind: model.ArtifactKindDeliverable,
				FileName: "审计报告.md", MimeType: "text/markdown"},
		},
		Todos: []model.Todo{{
			ID: "td-1",
			Outputs: []model.TodoOutput{
				{OutputName: "剧本正文", ArtifactID: "tr-final", FileRef: "pf-final"},
			},
		}},
	}

	outs := h.effectiveTodoOutputs(task, &task.Todos[0])
	if len(outs) != 1 || outs[0].OutputName != "剧本正文" || outs[0].FileID != "pf-final" {
		t.Fatalf("outputs = %+v, want the declared binding only (no artifact-appended extras)", outs)
	}
}

// TestEffectiveTodoOutputsKeepsUnresolvableBinding: a historical row whose
// artifact can no longer be found is surfaced as-is instead of being silently
// dropped — losing it would make the miss invisible to the caller.
func TestEffectiveTodoOutputsKeepsUnresolvableBinding(t *testing.T) {
	h := newOutputsTestHandler()
	task := &model.TaskDetail{
		ID: "t1",
		Todos: []model.Todo{{
			ID: "td-1",
			Outputs: []model.TodoOutput{
				{OutputName: "剧本正文", ArtifactID: "tr-ghost", FileRef: "pf-ghost"},
			},
		}},
	}

	outs := h.effectiveTodoOutputs(task, &task.Todos[0])
	if len(outs) != 1 || outs[0].ArtifactID != "tr-ghost" {
		t.Fatalf("outputs = %+v, want the unresolvable binding preserved", outs)
	}
}

// TestEffectiveTodoOutputsKeepsLegacyUnclassifiedBinding pins the regression
// caught by TestBuildTodoInputsResolvesPrevOutput: bindings written before the
// kind field existed carry an empty kind, and they still resolve correctly.
// Only an explicit *process* classification may disqualify a binding — the
// same rule stepOutputsUnsafe applies.
func TestEffectiveTodoOutputsKeepsLegacyUnclassifiedBinding(t *testing.T) {
	h := newOutputsTestHandler()
	task := &model.TaskDetail{
		ID: "t1",
		Artifacts: []model.TaskArtifact{
			{TransferID: "tr-legacy", TodoID: "td-1", FileName: "scene1.md", MimeType: "text/markdown"},
		},
		Todos: []model.Todo{{
			ID: "td-1",
			Outputs: []model.TodoOutput{
				{OutputName: "剧本正文", ArtifactID: "tr-legacy", FileRef: "pf-legacy"},
			},
		}},
	}

	outs := h.effectiveTodoOutputs(task, &task.Todos[0])
	if len(outs) != 1 || outs[0].ArtifactID != "tr-legacy" {
		t.Fatalf("outputs = %+v, want the legacy binding preserved", outs)
	}
}

// TestEffectiveTodoOutputsNilTaskAndEmptyTodo covers the two degenerate inputs.
func TestEffectiveTodoOutputsNilTaskAndEmptyTodo(t *testing.T) {
	h := newOutputsTestHandler()
	if got := h.effectiveTodoOutputs(nil, &model.Todo{ID: "td-1"}); got != nil {
		t.Fatalf("nil task = %+v, want nil", got)
	}
	task := &model.TaskDetail{ID: "t1", Todos: []model.Todo{{ID: "td-1"}}}
	if got := h.effectiveTodoOutputs(task, &task.Todos[0]); got != nil {
		t.Fatalf("todo with no outputs = %+v, want nil", got)
	}
}

// TestTransferRejectsUndeclaredOutputName is the end-to-end version of the new
// gate: an agent that names a slot the step does not declare ends up with a
// process artifact, a response that says why, and one actionable warning
// comment — instead of a silently misfiled deliverable.
func TestTransferRejectsUndeclaredOutputName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, userID, taskID, devNode := newTransferWorkflowFixture(t, []model.StepOutput{
		{Name: "剧名_剧本类型_版本_时间", MimeType: "docx"},
	})
	h := NewWebhookHandler(WebhookDeps{Store: s})

	w := postTransfer(t, h, devNode, taskID, "TD_01",
		"tid-badslot-000001", "剧本解析_v03.md", "text/markdown",
		map[string]any{"outputName": "剧本解析"})

	if w.Code != 200 {
		t.Fatalf("expected 200 (filed as a process file), got %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"bound":false`) {
		t.Fatalf("expected bound:false, got %s", body)
	}
	if !strings.Contains(body, `"bound_by":"declared_slot_unknown"`) {
		t.Fatalf("expected bound_by declared_slot_unknown, got %s", body)
	}

	artifacts := s.GetArtifactsByTaskID(taskID)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	if artifacts[0].Kind != model.ArtifactKindProcess || artifacts[0].OutputName != "" {
		t.Fatalf("artifact = kind %q output %q, want process with no slot",
			artifacts[0].Kind, artifacts[0].OutputName)
	}
	if got := len(s.GetTaskInternal(taskID).Todos[0].Outputs); got != 0 {
		t.Fatalf("todo.Outputs = %d entries, want none", got)
	}

	// The warning must name the real cause so the agent can fix it — the old
	// copy claimed "未携带 outputName", which was actively wrong here.
	comments, appErr := s.ListTaskComments(store.Scope{UserID: userID}, taskID)
	if appErr != nil {
		t.Fatalf("list comments: %v", appErr)
	}
	var warned bool
	for _, cm := range comments {
		if strings.Contains(cm.Content, "剧本解析_v03.md") &&
			strings.Contains(cm.Content, "不在本步骤声明的输出位里") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("expected an actionable unbound warning naming the undeclared slot, comments=%+v", comments)
	}
}

// TestTransferRejectsDeclaredNameWithWrongMime is the sibling arm: the name is
// declared, but the artifact is not what the slot describes.
func TestTransferRejectsDeclaredNameWithWrongMime(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, _, taskID, devNode := newTransferWorkflowFixture(t, []model.StepOutput{
		{Name: "剧名_剧本类型_版本_时间", MimeType: "docx"},
	})
	h := NewWebhookHandler(WebhookDeps{Store: s})

	w := postTransfer(t, h, devNode, taskID, "TD_01",
		"tid-mismatch-00001", "生死靶心_微电影_剧本_v1.md", "text/markdown",
		map[string]any{"outputName": "剧名_剧本类型_版本_时间"})

	if w.Code != 200 {
		t.Fatalf("expected 200 (filed as a process file), got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"bound_by":"declared_mime_mismatch"`) {
		t.Fatalf("expected bound_by declared_mime_mismatch, got %s", w.Body.String())
	}
	artifacts := s.GetArtifactsByTaskID(taskID)
	if len(artifacts) != 1 || artifacts[0].Kind != model.ArtifactKindProcess {
		t.Fatalf("artifacts = %+v, want one process artifact", artifacts)
	}
}
