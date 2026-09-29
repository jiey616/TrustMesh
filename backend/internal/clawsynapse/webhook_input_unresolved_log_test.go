package clawsynapse

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"trustmesh/backend/internal/model"
)

// stepInputTask builds a 2-step task whose second step declares an input fed by
// the first step's "剧本正文" output ("prev" link, resolved within the task).
// bound controls whether the predecessor todo (t1) actually carries the output.
func stepInputTask(bound bool) *model.TaskDetail {
	wf := &model.Workflow{
		Name: "wf",
		Steps: []model.WorkflowStep{
			{Name: "s1", Role: "编剧", Outputs: []model.StepOutput{{Name: "剧本正文"}}},
			{Name: "s2", Role: "编剧", Inputs: []model.StepInput{
				{Name: "上一段剧本", Source: model.StepIOLink{Step: "prev", Output: "剧本正文"}},
			}},
		},
	}
	task := &model.TaskDetail{
		ID:       "task-1",
		Title:    "Test Task",
		Workflow: wf,
		Todos: []model.Todo{
			makeTodo("t1", 1, "done", "node-A", "编剧", model.TodoResult{}),
			makeTodo("t2", 2, "pending", "node-A", "编剧", model.TodoResult{}),
		},
	}
	if bound {
		task.Todos[0].Outputs = []model.TodoOutput{
			{OutputName: "剧本正文", ArtifactID: "tr-1", FileRef: "pf-1"},
		}
		task.Artifacts = []model.TaskArtifact{
			{TransferID: "tr-1", TodoID: "t1", FileName: "scene1.md", FileSize: 1234, MimeType: "text/markdown"},
		}
	}
	return task
}

// TestBuildTodoInputsLogsUnresolved pins the ONLY observable signal for a broken
// upstream link (2026-09-29 TD_02 incident): the platform API never returns
// `inputs`, so an unresolved ref is invisible to the user until the executing
// agent gives up and asks. The warning is what makes the break greppable at
// dispatch time — and, being the fingerprint of the defect, what a fix is
// verified against (the line must disappear once the source step resolves).
func TestBuildTodoInputsLogsUnresolved(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	h := NewWebhookHandler(WebhookDeps{Log: zap.New(core)})

	task := stepInputTask(false)
	inputs := h.BuildTodoInputs(task, &task.Todos[1])

	if len(inputs) != 1 {
		t.Fatalf("inputs count = %d, want 1", len(inputs))
	}
	if inputs[0].Resolved {
		t.Fatalf("expected the input to stay unresolved, got %+v", inputs[0])
	}
	entries := logs.FilterMessage("todo input unresolved").All()
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 unresolved-input warning, got %d (%v)", len(entries), logs.All())
	}
	ctx := entries[0].ContextMap()
	for key, want := range map[string]any{
		"task_id":       "task-1",
		"todo_id":       "t2",
		"input":         "上一段剧本",
		"source_step":   "prev",
		"source_output": "剧本正文",
	} {
		if got := ctx[key]; got != want {
			t.Errorf("warning field %s = %v, want %v", key, got, want)
		}
	}
}

// TestBuildTodoInputsNoWarningWhenResolved is the reverse guard: the warning
// must stay conditional on the input actually failing to resolve, otherwise
// every dispatch logs it and it stops being a usable fingerprint.
func TestBuildTodoInputsNoWarningWhenResolved(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	h := NewWebhookHandler(WebhookDeps{Log: zap.New(core)})

	task := stepInputTask(true)
	inputs := h.BuildTodoInputs(task, &task.Todos[1])

	if len(inputs) != 1 {
		t.Fatalf("inputs count = %d, want 1", len(inputs))
	}
	if !inputs[0].Resolved {
		t.Fatalf("expected the input to resolve, got %+v", inputs[0])
	}
	if got := logs.FilterMessage("todo input unresolved").Len(); got != 0 {
		t.Fatalf("must not warn when the input resolved; got %d (%v)", got, logs.All())
	}
}

// TestBuildTodoInputsNilLoggerDoesNotPanic keeps BuildTodoInputs safe for the
// zero-value handler used across this package's tests (and for any construction
// path that injects no logger): logging must never become a crash vector.
func TestBuildTodoInputsNilLoggerDoesNotPanic(t *testing.T) {
	task := stepInputTask(false)
	h := &WebhookHandler{}
	inputs := h.BuildTodoInputs(task, &task.Todos[1])
	if len(inputs) != 1 || inputs[0].Resolved {
		t.Fatalf("expected 1 unresolved input, got %+v", inputs)
	}
}
