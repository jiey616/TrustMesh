package clawsynapse

import (
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
)

// inputStateTask builds a 2-step in-task pipeline whose second step declares one
// input fed by the previous step's "剧本正文" output.
//
// upstreamStatus and bound are separate knobs on purpose — they are what the
// taxonomy hinges on. A live predecessor that has not uploaded yet is a
// different situation (wait) from a finished predecessor that never will
// (intervene), and the two differ only in Status.
func inputStateTask(upstreamStatus string, bound bool) *model.TaskDetail {
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
			makeTodo("t1", 1, upstreamStatus, "node-A", "编剧", model.TodoResult{}),
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

// missingSourceStepTask declares an input whose source step exists nowhere in
// this task: the task owns two steps named s1/s2, the input asks for a step
// named "分镜分组", and there is no WorkflowRef, so the cross-task branch cannot
// even be attempted. No holder can ever appear — this is the "nobody ever ran
// it" half of InputStateMissing.
func missingSourceStepTask() *model.TaskDetail {
	wf := &model.Workflow{
		Name: "画宗AIGC无人工厂产线工作流",
		Steps: []model.WorkflowStep{
			{Name: "s1", Role: "编剧", Outputs: []model.StepOutput{{Name: "剧本正文"}}},
			{Name: "s2", Role: "编剧", Inputs: []model.StepInput{
				{Name: "分组视频生成提示词", Source: model.StepIOLink{Step: "分镜分组", Output: "分组视频生成提示词"}},
			}},
		},
	}
	return &model.TaskDetail{
		ID:       "task-1",
		Title:    "Test Task",
		Workflow: wf,
		Todos: []model.Todo{
			makeTodo("t1", 1, "done", "node-A", "编剧", model.TodoResult{}),
			makeTodo("t2", 2, "pending", "node-A", "编剧", model.TodoResult{}),
		},
	}
}

// inputRefsStore builds the store side of the same fixture: a task that exists
// with todos t1/t2, which is all RecordTodoInputRefs looks up. Without it the
// write is skipped entirely (the handler is nil-store-safe for the many tests
// that build a zero-value handler), so the state would never become observable.
func inputRefsStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	s := store.New()
	user, appErr := s.CreateUser("inputrefs@example.com", "Input Refs", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}
	dev, appErr := s.CreateAgent(store.Scope{UserID: user.ID}, "node-inputrefs-dev", "编剧", "developer", "writer", nil)
	if appErr != nil {
		t.Fatalf("create dev: %v", appErr)
	}
	pm, appErr := s.CreateAgent(store.Scope{UserID: user.ID}, "node-inputrefs-pm", "PM", "pm", "pm", nil)
	if appErr != nil {
		t.Fatalf("create pm: %v", appErr)
	}
	s.SyncAgentPresence([]store.AgentPresence{
		{NodeID: dev.NodeID, LastSeenAt: time.Now().UTC()},
		{NodeID: pm.NodeID, LastSeenAt: time.Now().UTC()},
	}, time.Now().UTC())
	proj, appErr := s.CreateProject(store.Scope{UserID: user.ID}, "输入位可见化", "demo", pm.ID)
	if appErr != nil {
		t.Fatalf("create project: %v", appErr)
	}
	task, appErr := s.CreateTaskByPMNodeWithMessageID(pm.NodeID, "", store.TaskCreateInput{
		ProjectID:   proj.ID,
		Title:       "输入位状态",
		Description: "input state taxonomy",
		Todos: []store.TaskCreateTodoInput{
			{ID: "t1", Order: 1, Title: "s1", Description: "d1", AssigneeNodeID: dev.NodeID},
			{ID: "t2", Order: 2, Title: "s2", Description: "d2", AssigneeNodeID: dev.NodeID},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}
	return s, task.ID
}

// storedTodoInputs reads the platform-observed snapshot back off the stored
// todo. This is the ONLY place TodoInput.State is observable: the payload ref
// the agent receives stays a boolean on purpose (an executing agent needs to
// know whether it has the file, not why not), so asserting through the store is
// both the taxonomy test and the persistence test.
func storedTodoInputs(t *testing.T, s *store.Store, taskID, todoID string) []model.TodoInput {
	t.Helper()
	task := s.GetTaskInternal(taskID)
	if task == nil {
		t.Fatalf("task %s not found in store", taskID)
	}
	for i := range task.Todos {
		if task.Todos[i].ID == todoID {
			return task.Todos[i].Inputs
		}
	}
	t.Fatalf("todo %s not found in store", todoID)
	return nil
}

// TestBuildTodoInputsStateTaxonomy is the contract test for TodoInput.State at
// its single production site. The three states carry opposite remedies, so
// collapsing any two of them makes the field useless: resolved needs nothing,
// pending needs patience, missing needs a human.
//
// The last case is the one that matters most and the one a naive
// resolved/unresolved implementation gets wrong: a predecessor that has already
// ENDED without producing the file. The 2026-09-29 incident was exactly this
// shape, and the whole reason the downstream agent burned a todo.ask was that
// nothing told anyone the file would never arrive.
func TestBuildTodoInputsStateTaxonomy(t *testing.T) {
	cases := []struct {
		name         string
		task         func() *model.TaskDetail
		wantState    string
		wantResolved bool
		wantName     string
		wantSource   string
		wantOutput   string
	}{
		{
			name:         "resolved when the source output is bound",
			task:         func() *model.TaskDetail { return inputStateTask("done", true) },
			wantState:    model.InputStateResolved,
			wantResolved: true,
			wantName:     "上一段剧本",
			wantSource:   "prev",
			wantOutput:   "剧本正文",
		},
		{
			name:         "pending when a live holder has not produced the output yet",
			task:         func() *model.TaskDetail { return inputStateTask("in_progress", false) },
			wantState:    model.InputStatePending,
			wantResolved: false,
			wantName:     "上一段剧本",
			wantSource:   "prev",
			wantOutput:   "剧本正文",
		},
		{
			name:         "missing when a terminal holder never produced the output",
			task:         func() *model.TaskDetail { return inputStateTask("canceled", false) },
			wantState:    model.InputStateMissing,
			wantResolved: false,
			wantName:     "上一段剧本",
			wantSource:   "prev",
			wantOutput:   "剧本正文",
		},
		{
			name:         "missing when no task holds the source step at all",
			task:         missingSourceStepTask,
			wantState:    model.InputStateMissing,
			wantResolved: false,
			wantName:     "分组视频生成提示词",
			wantSource:   "分镜分组",
			wantOutput:   "分组视频生成提示词",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, taskID := inputRefsStore(t)
			h := NewWebhookHandler(WebhookDeps{Store: s})

			task := tc.task()
			task.ID = taskID // the recorder looks the todo up by (taskID, todoID)
			refs := h.BuildTodoInputs(task, &task.Todos[1])

			if len(refs) != 1 {
				t.Fatalf("refs count = %d, want 1", len(refs))
			}
			if refs[0].Resolved != tc.wantResolved {
				t.Errorf("payload resolved = %v, want %v", refs[0].Resolved, tc.wantResolved)
			}
			if refs[0].Name != tc.wantName {
				t.Errorf("payload name = %q, want %q", refs[0].Name, tc.wantName)
			}

			inputs := storedTodoInputs(t, s, taskID, "t2")
			if len(inputs) != 1 {
				t.Fatalf("stored inputs count = %d, want 1", len(inputs))
			}
			got := inputs[0]
			if got.State != tc.wantState {
				t.Errorf("state = %q, want %q", got.State, tc.wantState)
			}
			if got.Name != tc.wantName {
				t.Errorf("name = %q, want %q", got.Name, tc.wantName)
			}
			if got.SourceStep != tc.wantSource {
				t.Errorf("source_step = %q, want %q", got.SourceStep, tc.wantSource)
			}
			if got.OutputName != tc.wantOutput {
				t.Errorf("output_name = %q, want %q", got.OutputName, tc.wantOutput)
			}
			if got.CheckedAt == nil {
				t.Error("checked_at must be stamped so the UI can say how stale the observation is")
			}
		})
	}
}

// TestBuildTodoInputsRecordsResolvedFileDetail pins that a RESOLVED input is
// recorded with the file it resolved to. A row that only says "resolved" would
// leave the user unable to tell which revision of the file the agent got.
func TestBuildTodoInputsRecordsResolvedFileDetail(t *testing.T) {
	s, taskID := inputRefsStore(t)
	h := NewWebhookHandler(WebhookDeps{Store: s})

	task := inputStateTask("done", true)
	task.ID = taskID
	h.BuildTodoInputs(task, &task.Todos[1])

	inputs := storedTodoInputs(t, s, taskID, "t2")
	if len(inputs) != 1 {
		t.Fatalf("stored inputs count = %d, want 1", len(inputs))
	}
	if inputs[0].FileName != "scene1.md" || inputs[0].FileSize != 1234 {
		t.Fatalf("resolved input must carry the file it found, got %+v", inputs[0])
	}
}

// TestBuildTodoInputsNoInputsWritesNothing is the no-fabrication guard: a step
// that declares no inputs must not gain an empty snapshot, otherwise the task
// page cannot distinguish "this step has no inputs" from "its inputs were
// checked and none could be built".
func TestBuildTodoInputsNoInputsWritesNothing(t *testing.T) {
	s, taskID := inputRefsStore(t)
	h := NewWebhookHandler(WebhookDeps{Store: s})

	task := inputStateTask("done", true)
	task.ID = taskID
	task.Workflow.Steps[1].Inputs = nil

	if refs := h.BuildTodoInputs(task, &task.Todos[1]); len(refs) != 0 {
		t.Fatalf("expected no refs for a step without inputs, got %+v", refs)
	}
	if got := storedTodoInputs(t, s, taskID, "t2"); len(got) != 0 {
		t.Fatalf("expected nothing to be recorded, got %+v", got)
	}
}

// TestBuildTodoInputsWarningCarriesState pins the operator-facing half of the
// taxonomy. Grepping `todo input unresolved` tells you something broke; the
// state tells you whether waiting or a human is the remedy. Without it every
// unresolved line looks the same and the log is not actionable.
func TestBuildTodoInputsWarningCarriesState(t *testing.T) {
	cases := []struct {
		name string
		task *model.TaskDetail
		want string
	}{
		{"live holder", inputStateTask("in_progress", false), model.InputStatePending},
		{"terminal holder", inputStateTask("failed", false), model.InputStateMissing},
		{"no holder", missingSourceStepTask(), model.InputStateMissing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			core, logs := observer.New(zap.WarnLevel)
			h := NewWebhookHandler(WebhookDeps{Log: zap.New(core)})

			h.BuildTodoInputs(tc.task, &tc.task.Todos[1])

			entries := logs.FilterMessage("todo input unresolved").All()
			if len(entries) != 1 {
				t.Fatalf("expected exactly 1 unresolved-input warning, got %d (%v)", len(entries), logs.All())
			}
			if got := entries[0].ContextMap()["state"]; got != tc.want {
				t.Errorf("warning state = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestBuildTodoInputsTerminalPredecessorStaysUnresolved is the regression guard
// for the incident shape: a terminal predecessor must still not resolve (the
// fix is about labelling, never about widening what counts as an input), and it
// must not silently borrow the predecessor's OTHER outputs.
func TestBuildTodoInputsTerminalPredecessorStaysUnresolved(t *testing.T) {
	s, taskID := inputRefsStore(t)
	h := NewWebhookHandler(WebhookDeps{Store: s})

	task := inputStateTask("canceled", false)
	task.ID = taskID
	// The terminal predecessor did produce something — just not the file this
	// input declares. Strict name matching must not fall back to it.
	task.Todos[0].Outputs = []model.TodoOutput{
		{OutputName: "别的产物", ArtifactID: "tr-other", FileRef: "pf-other"},
	}
	task.Artifacts = []model.TaskArtifact{
		{TransferID: "tr-other", TodoID: "t1", FileName: "other.md", FileSize: 10, MimeType: "text/markdown"},
	}

	refs := h.BuildTodoInputs(task, &task.Todos[1])
	if len(refs) != 1 {
		t.Fatalf("refs count = %d, want 1", len(refs))
	}
	if refs[0].Resolved {
		t.Fatalf("a mismatched output name must never resolve, got %+v", refs[0])
	}
	if refs[0].OutputName != "剧本正文" {
		t.Errorf("output_name = %q, want the DECLARED name 剧本正文", refs[0].OutputName)
	}
}
