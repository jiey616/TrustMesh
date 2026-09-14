package clawsynapse

import (
	"testing"

	"trustmesh/backend/internal/model"
)

func makeTodo(id string, order int, status, nodeID, name string, result model.TodoResult) model.Todo {
	return model.Todo{
		ID:          id,
		Order:       order,
		Title:       "Todo " + id,
		Description: "Description " + id,
		Status:      status,
		Assignee:    model.TodoAssignee{AgentID: "agent-" + nodeID, Name: name, NodeID: nodeID},
		Result:      result,
	}
}

func makeTask(todos []model.Todo) *model.TaskDetail {
	return &model.TaskDetail{
		ID:          "task-1",
		Title:       "Test Task",
		Description: "Test task description",
		Todos:       todos,
		PMAgent:     model.PMAgentSummary{NodeID: "pm-node"},
	}
}

func TestAgentHasPriorTodoInTask(t *testing.T) {
	tests := []struct {
		name     string
		todos    []model.Todo
		current  string // todo ID
		expected bool
	}{
		{
			name: "first todo for agent",
			todos: []model.Todo{
				makeTodo("t1", 1, "pending", "node-A", "AgentA", model.TodoResult{}),
			},
			current:  "t1",
			expected: false,
		},
		{
			name: "agent has prior completed todo",
			todos: []model.Todo{
				makeTodo("t1", 1, "done", "node-A", "AgentA", model.TodoResult{Summary: "done"}),
				makeTodo("t2", 2, "done", "node-B", "AgentB", model.TodoResult{Summary: "done"}),
				makeTodo("t3", 3, "pending", "node-A", "AgentA", model.TodoResult{}),
			},
			current:  "t3",
			expected: true,
		},
		{
			name: "agent has prior failed todo",
			todos: []model.Todo{
				makeTodo("t1", 1, "failed", "node-A", "AgentA", model.TodoResult{}),
				makeTodo("t2", 2, "pending", "node-A", "AgentA", model.TodoResult{}),
			},
			current:  "t2",
			expected: true,
		},
		{
			name: "different agent has prior todo",
			todos: []model.Todo{
				makeTodo("t1", 1, "done", "node-B", "AgentB", model.TodoResult{Summary: "done"}),
				makeTodo("t2", 2, "pending", "node-A", "AgentA", model.TodoResult{}),
			},
			current:  "t2",
			expected: false,
		},
		{
			name: "agent has later todo only",
			todos: []model.Todo{
				makeTodo("t1", 1, "pending", "node-A", "AgentA", model.TodoResult{}),
				makeTodo("t2", 2, "done", "node-A", "AgentA", model.TodoResult{Summary: "done"}),
			},
			current:  "t1",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := makeTask(tt.todos)
			var current *model.Todo
			for i := range task.Todos {
				if task.Todos[i].ID == tt.current {
					current = &task.Todos[i]
					break
				}
			}
			if current == nil {
				t.Fatalf("current todo %q not found", tt.current)
			}
			got := agentHasPriorTodoInTask(task, current)
			if got != tt.expected {
				t.Errorf("agentHasPriorTodoInTask() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBuildTaskContext(t *testing.T) {
	task := makeTask([]model.Todo{
		makeTodo("t1", 1, "done", "node-A", "AgentA", model.TodoResult{}),
		makeTodo("t2", 2, "pending", "node-B", "AgentB", model.TodoResult{}),
		makeTodo("t3", 3, "pending", "node-A", "AgentA", model.TodoResult{}),
	})

	ctx := buildTaskContext(task, "t2")

	if ctx.Title != "Test Task" {
		t.Errorf("title = %q, want %q", ctx.Title, "Test Task")
	}
	if len(ctx.Todos) != 3 {
		t.Fatalf("todos count = %d, want 3", len(ctx.Todos))
	}
	if !ctx.Todos[1].IsCurrent {
		t.Error("todos[1] should be current")
	}
	if ctx.Todos[0].IsCurrent || ctx.Todos[2].IsCurrent {
		t.Error("only todos[1] should be current")
	}
}

func TestBuildAllPriorResults(t *testing.T) {
	task := makeTask([]model.Todo{
		makeTodo("t1", 1, "done", "node-A", "AgentA", model.TodoResult{
			Summary: "Result 1",
			Output:  "Output 1",
		}),
		makeTodo("t2", 2, "done", "node-B", "AgentB", model.TodoResult{
			Summary: "Result 2",
			Output:  "Output 2",
		}),
		makeTodo("t3", 3, "pending", "node-C", "AgentC", model.TodoResult{}),
	})

	current := &task.Todos[2] // t3
	results := buildAllPriorResults(&WebhookHandler{}, task, current)

	if len(results) != 2 {
		t.Fatalf("results count = %d, want 2", len(results))
	}
	if results[0].TodoID != "t1" || results[0].Summary != "Result 1" {
		t.Errorf("results[0] = %+v", results[0])
	}
	if results[1].TodoID != "t2" || results[1].Summary != "Result 2" {
		t.Errorf("results[1] = %+v", results[1])
	}
}

func TestBuildCrossAgentPriorResults(t *testing.T) {
	task := makeTask([]model.Todo{
		makeTodo("t1", 1, "done", "node-A", "AgentA", model.TodoResult{
			Summary: "Result A1",
			Output:  "Output A1",
		}),
		makeTodo("t2", 2, "done", "node-B", "AgentB", model.TodoResult{
			Summary: "Result B",
			Output:  "Output B",
		}),
		makeTodo("t3", 3, "pending", "node-A", "AgentA", model.TodoResult{}),
	})

	current := &task.Todos[2] // t3 assigned to node-A
	results := buildCrossAgentPriorResults(&WebhookHandler{}, task, current)

	// Should only include t2 (node-B), not t1 (node-A, same agent)
	if len(results) != 1 {
		t.Fatalf("results count = %d, want 1", len(results))
	}
	if results[0].TodoID != "t2" {
		t.Errorf("expected t2, got %s", results[0].TodoID)
	}
	if results[0].Summary != "Result B" {
		t.Errorf("summary = %q, want %q", results[0].Summary, "Result B")
	}
}

func TestBuildTodoAssignedPayload_FirstTimeAgent(t *testing.T) {
	task := makeTask([]model.Todo{
		makeTodo("t1", 1, "done", "node-A", "AgentA", model.TodoResult{Summary: "Done A"}),
		makeTodo("t2", 2, "pending", "node-B", "AgentB", model.TodoResult{}),
	})

	h := &WebhookHandler{}
	payload := h.buildTodoAssignedPayload(task, &task.Todos[1])

	if payload.TaskContext == nil {
		t.Fatal("expected task_context for first-time agent")
	}
	if payload.TaskContext.Title != "Test Task" {
		t.Errorf("task_context.title = %q", payload.TaskContext.Title)
	}
	if len(payload.PriorResults) != 1 || payload.PriorResults[0].TodoID != "t1" {
		t.Errorf("expected prior result for t1, got %+v", payload.PriorResults)
	}
}

func TestBuildTodoAssignedPayload_ReturningAgent(t *testing.T) {
	task := makeTask([]model.Todo{
		makeTodo("t1", 1, "done", "node-A", "AgentA", model.TodoResult{Summary: "Done A1"}),
		makeTodo("t2", 2, "done", "node-B", "AgentB", model.TodoResult{Summary: "Done B"}),
		makeTodo("t3", 3, "pending", "node-A", "AgentA", model.TodoResult{}),
	})

	h := &WebhookHandler{}
	payload := h.buildTodoAssignedPayload(task, &task.Todos[2])

	// Returning agent: no task_context (already in session)
	if payload.TaskContext != nil {
		t.Error("expected nil task_context for returning agent")
	}
	// Only cross-agent results (t2 from node-B), not t1 (own work)
	if len(payload.PriorResults) != 1 {
		t.Fatalf("expected 1 cross-agent result, got %d", len(payload.PriorResults))
	}
	if payload.PriorResults[0].TodoID != "t2" {
		t.Errorf("expected t2, got %s", payload.PriorResults[0].TodoID)
	}
}

func TestBuildTodoAssignedPayload_NoPriorResults(t *testing.T) {
	task := makeTask([]model.Todo{
		makeTodo("t1", 1, "pending", "node-A", "AgentA", model.TodoResult{}),
	})

	h := &WebhookHandler{}
	payload := h.buildTodoAssignedPayload(task, &task.Todos[0])

	if payload.TaskContext == nil {
		t.Fatal("expected task_context for first todo")
	}
	if len(payload.PriorResults) != 0 {
		t.Errorf("expected no prior results, got %d", len(payload.PriorResults))
	}
}

func TestBuildTodoInputsResolvesPrevOutput(t *testing.T) {
	// Step 2 declares an input linked to the previous step's output "剧本正文".
	wf := &model.Workflow{
		Name: "微短剧生产",
		Steps: []model.WorkflowStep{
			{Name: "编剧-第1段", Role: "编剧", Outputs: []model.StepOutput{{Name: "剧本正文", MimeType: "text/markdown"}}},
			{Name: "编剧-第2段", Role: "编剧", Inputs: []model.StepInput{
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
	// t1 produced the "剧本正文" output, backed by an artifact.
	task.Todos[0].Outputs = []model.TodoOutput{
		{OutputName: "剧本正文", ArtifactID: "tr-1", FileRef: "pf-1"},
	}
	task.Artifacts = []model.TaskArtifact{
		{TransferID: "tr-1", TodoID: "t1", FileName: "scene1.md", FileSize: 1234, MimeType: "text/markdown"},
	}

	h := &WebhookHandler{}
	inputs := h.BuildTodoInputs(task, &task.Todos[1])

	if len(inputs) != 1 {
		t.Fatalf("inputs count = %d, want 1", len(inputs))
	}
	in := inputs[0]
	if in.Name != "上一段剧本" {
		t.Errorf("name = %q, want %q", in.Name, "上一段剧本")
	}
	if !in.Resolved {
		t.Error("expected input to be resolved (Resolved=true)")
	}
	if in.SourceStep != "prev" {
		t.Errorf("source_step = %q, want %q", in.SourceStep, "prev")
	}
	if in.OutputName != "剧本正文" {
		t.Errorf("output_name = %q, want %q", in.OutputName, "剧本正文")
	}
	if in.SourceTodoID != "t1" {
		t.Errorf("source_todo_id = %q, want %q", in.SourceTodoID, "t1")
	}
	if in.FileName != "scene1.md" {
		t.Errorf("file_name = %q, want %q", in.FileName, "scene1.md")
	}
	if in.ArtifactID != "tr-1" {
		t.Errorf("artifact_id = %q, want %q", in.ArtifactID, "tr-1")
	}
}

func TestBuildTodoInputsUnresolvedWhenNoUpstreamOutput(t *testing.T) {
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
	// t1 produced NO matching output → input surfaced but unresolved.

	h := &WebhookHandler{}
	inputs := h.BuildTodoInputs(task, &task.Todos[1])

	if len(inputs) != 1 {
		t.Fatalf("inputs count = %d, want 1", len(inputs))
	}
	if inputs[0].Resolved {
		t.Error("expected unresolved (Resolved=false) when upstream output missing")
	}
	if inputs[0].Name != "上一段剧本" {
		t.Errorf("name = %q", inputs[0].Name)
	}
}

// TestBuildTodoInputsResolvesDeliverableArtifact covers the realistic case where
// an upstream agent uploaded its deliverable WITHOUT a todo.Outputs binding but
// WITH a declared outputName (SaveArtifact classifies it kind=deliverable).
// todo.Outputs stays empty, but the artifact is linked to the todo by TodoID.
// The resolver falls back to deliverable-kind artifacts only — legacy records
// without a kind and process artifacts must never resolve.
func TestBuildTodoInputsResolvesDeliverableArtifact(t *testing.T) {
	wf := &model.Workflow{
		Name: "画宗AIGC无人工厂产线工作流",
		Steps: []model.WorkflowStep{
			{
				Name:    "剧本创作",
				Role:    "developer",
				Outputs: []model.StepOutput{{Name: "剧本文件", MimeType: "text/markdown"}},
			},
			{
				Name: "分镜拆解",
				Role: "developer",
				Inputs: []model.StepInput{
					{Name: "剧本文件", Source: model.StepIOLink{Step: "剧本创作", Output: "剧本文件"}},
				},
			},
		},
	}
	task := &model.TaskDetail{
		ID:        "task-1",
		Title:     "Test Task",
		UserID:    "u1",
		ProjectID: "p1",
		Workflow:  wf,
		Todos: []model.Todo{
			makeTodo("t1", 1, "done", "node-A", "developer", model.TodoResult{}),
			makeTodo("t2", 2, "pending", "node-A", "developer", model.TodoResult{}),
		},
	}
	// t1 produced NO todo.Outputs, but a deliverable-kind artifact is linked
	// only by TodoID and carries the declared output name.
	task.Artifacts = []model.TaskArtifact{
		{TransferID: "tr-script", TodoID: "t1", FileName: "mini-script.md", FileSize: 1754, MimeType: "text/markdown", ProjectFileID: "pf-script", OutputName: "剧本文件", Kind: model.ArtifactKindDeliverable},
	}

	h := &WebhookHandler{}
	inputs := h.BuildTodoInputs(task, &task.Todos[1])

	if len(inputs) != 1 {
		t.Fatalf("inputs count = %d, want 1", len(inputs))
	}
	in := inputs[0]
	if !in.Resolved {
		t.Fatal("expected input resolved via deliverable artifact fallback (Resolved=true)")
	}
	if in.SourceStep != "剧本创作" {
		t.Errorf("source_step = %q, want %q", in.SourceStep, "剧本创作")
	}
	if in.SourceTodoID != "t1" {
		t.Errorf("source_todo_id = %q, want %q", in.SourceTodoID, "t1")
	}
	if in.ArtifactID != "tr-script" {
		t.Errorf("artifact_id = %q, want %q", in.ArtifactID, "tr-script")
	}
	if in.FileRef != "pf-script" {
		t.Errorf("file_ref = %q, want %q", in.FileRef, "pf-script")
	}
	if in.FileName != "mini-script.md" {
		t.Errorf("file_name = %q, want %q", in.FileName, "mini-script.md")
	}
}

// TestBuildTodoInputsSkipsNonDeliverableArtifacts pins the strict-matching
// contract: a legacy artifact without a kind and an explicit process artifact
// must both stay unresolved (Resolved:false expectation surfaced) — the
// 2026-09-01/02 incidents fed drafts (01-intake.md) to downstream steps.
func TestBuildTodoInputsSkipsNonDeliverableArtifacts(t *testing.T) {
	wf := &model.Workflow{
		Name: "画宗AIGC无人工厂产线工作流",
		Steps: []model.WorkflowStep{
			{
				Name:    "剧本创作",
				Role:    "developer",
				Outputs: []model.StepOutput{{Name: "剧本文件", MimeType: "text/markdown"}},
			},
			{
				Name: "分镜拆解",
				Role: "developer",
				Inputs: []model.StepInput{
					{Name: "剧本文件", Source: model.StepIOLink{Step: "剧本创作", Output: "剧本文件"}},
				},
			},
		},
	}
	task := &model.TaskDetail{
		ID:        "task-1",
		Title:     "Test Task",
		UserID:    "u1",
		ProjectID: "p1",
		Workflow:  wf,
		Todos: []model.Todo{
			makeTodo("t1", 1, "done", "node-A", "developer", model.TodoResult{}),
			makeTodo("t2", 2, "pending", "node-A", "developer", model.TodoResult{}),
		},
	}
	// Legacy record (no kind) + explicit process artifact: neither may resolve.
	task.Artifacts = []model.TaskArtifact{
		{TransferID: "tr-legacy", TodoID: "t1", FileName: "legacy-draft.md", MimeType: "text/markdown", ProjectFileID: "pf-legacy"},
		{TransferID: "tr-process", TodoID: "t1", FileName: "01-intake.md", MimeType: "text/markdown", ProjectFileID: "pf-intake", Kind: model.ArtifactKindProcess},
	}

	h := &WebhookHandler{}
	inputs := h.BuildTodoInputs(task, &task.Todos[1])

	if len(inputs) != 1 {
		t.Fatalf("inputs count = %d, want 1", len(inputs))
	}
	if inputs[0].Resolved {
		t.Fatalf("non-deliverable artifacts must not resolve: %+v", inputs[0])
	}
}

func TestBuildPriorResult(t *testing.T) {
	todo := &model.Todo{
		ID:     "t1",
		Title:  "Design API",
		Status: "done",
		Result: model.TodoResult{
			Summary: "API designed",
			Output:  "Detailed design doc",
		},
	}
	task := &model.TaskDetail{}

	h := &WebhookHandler{}
	r := h.buildPriorResult(task, todo)

	if r.Summary != "API designed" || r.Output != "Detailed design doc" {
		t.Errorf("unexpected result: %+v", r)
	}
	if r.TodoID != "t1" || r.Title != "Design API" {
		t.Errorf("unexpected identity: %+v", r)
	}
}
