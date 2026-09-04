package store

import (
	"testing"

	"trustmesh/backend/internal/model"
)

func roleOfDummy(model.Todo) string { return "developer" }

// Same agent owning two steps must not let the first step's done todo light
// up the second (not-yet-started) step. Measured 2026-09-04: 分镜拆解's done
// todo marked the 分镜分组 node done because both steps share one agent.
func TestAlignTodosToStepSameAgentMultipleSteps(t *testing.T) {
	steps := []model.WorkflowStep{
		{Name: "分镜拆解", AgentID: "ag-x"},
		{Name: "分镜分组", AgentID: "ag-x"},
	}
	todos := []model.Todo{
		{ID: "TD_02", Title: "分镜拆解", Assignee: model.TodoAssignee{AgentID: "ag-x", NodeID: "n1-a"}},
	}

	got := alignTodosToStep(steps, todos, 0, roleOfDummy)
	if len(got) != 1 || got[0].ID != "TD_02" {
		t.Fatalf("step 分镜拆解 should claim TD_02, got %v", got)
	}
	if got := alignTodosToStep(steps, todos, 1, roleOfDummy); len(got) != 0 {
		t.Fatalf("step 分镜分组 must not be claimed by TD_02, got %v", got)
	}
}

// A step split into several same-agent todos: the sibling without a title
// match stays on the step its sibling named, never leaking to the agent's
// other steps.
func TestAlignTodosToStepSplitTodosStayTogether(t *testing.T) {
	steps := []model.WorkflowStep{
		{Name: "分镜拆解", AgentID: "ag-x"},
		{Name: "分镜分组", AgentID: "ag-x"},
	}
	todos := []model.Todo{
		{ID: "TD_02", Title: "分镜拆解", Assignee: model.TodoAssignee{AgentID: "ag-x"}},
		{ID: "TD_02b", Title: "上传前自检", Assignee: model.TodoAssignee{AgentID: "ag-x"}},
	}

	if got := alignTodosToStep(steps, todos, 0, roleOfDummy); len(got) != 2 {
		t.Fatalf("both sibling todos should stay on the named step, got %v", got)
	}
	if got := alignTodosToStep(steps, todos, 1, roleOfDummy); len(got) != 0 {
		t.Fatalf("unnamed sibling must not leak to the other step, got %v", got)
	}
}

// No title signal anywhere: keep the legacy all-matches behavior so legacy
// data never matches less than before.
func TestAlignTodosToStepAmbiguousKeepsLegacy(t *testing.T) {
	steps := []model.WorkflowStep{
		{Name: "分镜拆解", AgentID: "ag-x"},
		{Name: "分镜分组", AgentID: "ag-x"},
	}
	todos := []model.Todo{
		{ID: "TD_02", Title: "随便写的标题", Assignee: model.TodoAssignee{AgentID: "ag-x"}},
	}

	for idx := range steps {
		if got := alignTodosToStep(steps, todos, idx, roleOfDummy); len(got) != 1 {
			t.Fatalf("legacy ambiguity should match every step, step %d got %v", idx, got)
		}
	}
}
