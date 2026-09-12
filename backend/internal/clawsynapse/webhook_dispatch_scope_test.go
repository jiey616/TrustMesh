package clawsynapse

import (
	"testing"

	"go.uber.org/zap"
	"trustmesh/backend/internal/metrics"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
)

// T0.3 acceptance: TestDispatchOrgMismatchWarnsNotBlocks.
//
// Phase 0 only makes the tenant trust assumption visible — it must NOT gate the
// dispatch (blocking would break every existing shared-agent dispatch, and
// enforcement is T1.1's job). So this test pins two things:
//
//  1. Hit detection: a task whose tenant differs from the assignee agent's
//     org_id increments dispatch_org_mismatch_total and logs a warning.
//  2. Non-blocking: observation mutates nothing — the todo stays pending and
//     dispatchable, and no dispatch failure is recorded.
//
// Cases that must NOT be flagged: a legacy task with no org_id (dispatch is
// governed by the author's user identity) and an unknown agent (a separate
// failure surfaced by the dispatch itself).
func TestDispatchOrgMismatchWarnsNotBlocks(t *testing.T) {
	metrics.Reset()
	t.Cleanup(metrics.Reset)

	s := store.New()
	orgA, appErr := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	if appErr != nil {
		t.Fatalf("create orgA: %v", appErr)
	}
	orgB, appErr := s.CreateOrganization("u9", "Globex", "globex", model.OrgKindEnterprise)
	if appErr != nil {
		t.Fatalf("create orgB: %v", appErr)
	}

	// Agent lives in orgA because create resolves the owner org from the scope.
	agent, appErr := s.CreateAgent(store.Scope{UserID: "u1", OrgID: orgA.ID}, "node-dev-001", "Dev", "developer", "dev agent", nil)
	if appErr != nil {
		t.Fatalf("create agent: %v", appErr)
	}
	if agent.OrgID != orgA.ID {
		t.Fatalf("precondition: agent.OrgID = %q, want %q", agent.OrgID, orgA.ID)
	}

	h := NewWebhookHandler(s, nil, zap.NewNop())

	todoFor := func(agentID, nodeID string) []model.Todo {
		return []model.Todo{{
			ID:       "td-1",
			Title:    "剧本创作",
			Status:   "pending",
			Assignee: model.TodoAssignee{AgentID: agentID, Name: "Dev", NodeID: nodeID},
		}}
	}

	t.Run("跨租户_命中计数且不阻断", func(t *testing.T) {
		before := metrics.Count(metrics.DispatchOrgMismatchTotal)

		task := &model.TaskDetail{
			ID: "t-mismatch", UserID: "u1", OrgID: orgB.ID, Status: "in_progress",
			Todos: todoFor(agent.ID, agent.NodeID),
		}

		agentOrg, mismatched := h.dispatchOrgMismatch(task, &task.Todos[0])
		if !mismatched {
			t.Fatalf("mismatched = false, want true (task org %q vs agent org %q)", task.OrgID, agentOrg)
		}
		if agentOrg != orgA.ID {
			t.Fatalf("agentOrg = %q, want %q", agentOrg, orgA.ID)
		}

		h.observeDispatchOrgMismatch(task, &task.Todos[0])
		if got := metrics.Count(metrics.DispatchOrgMismatchTotal) - before; got != 1 {
			t.Fatalf("dispatch_org_mismatch_total delta = %d, want 1", got)
		}

		// Non-blocking: observation left the todo untouched and dispatchable.
		td := &task.Todos[0]
		if td.Status != "pending" || td.DispatchAttempts != 0 || td.LastDispatchErr != nil {
			t.Fatalf("observation must not mutate the todo: status=%q attempts=%d err=%v",
				td.Status, td.DispatchAttempts, td.LastDispatchErr)
		}
		if !task.CanDispatchTodo(td.ID) {
			t.Fatalf("todo must remain dispatchable after observation")
		}
	})

	t.Run("同租户_未命中_不计数", func(t *testing.T) {
		before := metrics.Count(metrics.DispatchOrgMismatchTotal)

		task := &model.TaskDetail{
			ID: "t-match", UserID: "u1", OrgID: orgA.ID, Status: "in_progress",
			Todos: todoFor(agent.ID, agent.NodeID),
		}

		if _, mismatched := h.dispatchOrgMismatch(task, &task.Todos[0]); mismatched {
			t.Fatalf("mismatched = true, want false for same-tenant dispatch")
		}
		h.observeDispatchOrgMismatch(task, &task.Todos[0])
		if got := metrics.Count(metrics.DispatchOrgMismatchTotal) - before; got != 0 {
			t.Fatalf("counter delta = %d, want 0 for same-tenant dispatch", got)
		}
	})

	t.Run("存量空org任务_不算失配", func(t *testing.T) {
		before := metrics.Count(metrics.DispatchOrgMismatchTotal)

		task := &model.TaskDetail{
			ID: "t-legacy", UserID: "u1", Status: "in_progress",
			Todos: todoFor(agent.ID, agent.NodeID),
		}

		if _, mismatched := h.dispatchOrgMismatch(task, &task.Todos[0]); mismatched {
			t.Fatalf("mismatched = true, want false: a task without org_id is governed by user identity")
		}
		h.observeDispatchOrgMismatch(task, &task.Todos[0])
		if got := metrics.Count(metrics.DispatchOrgMismatchTotal) - before; got != 0 {
			t.Fatalf("counter delta = %d, want 0 for legacy empty-org task", got)
		}
	})

	t.Run("agent不存在_不算失配", func(t *testing.T) {
		before := metrics.Count(metrics.DispatchOrgMismatchTotal)

		task := &model.TaskDetail{
			ID: "t-unknown", UserID: "u1", OrgID: orgA.ID, Status: "in_progress",
			Todos: todoFor("ag-does-not-exist", "node-ghost"),
		}

		if _, mismatched := h.dispatchOrgMismatch(task, &task.Todos[0]); mismatched {
			t.Fatalf("mismatched = true, want false for unknown agent")
		}
		h.observeDispatchOrgMismatch(task, &task.Todos[0])
		if got := metrics.Count(metrics.DispatchOrgMismatchTotal) - before; got != 0 {
			t.Fatalf("counter delta = %d, want 0 for unknown agent", got)
		}
	})

	t.Run("nil输入不panic", func(t *testing.T) {
		if _, mismatched := h.dispatchOrgMismatch(nil, nil); mismatched {
			t.Fatalf("nil input must not report a mismatch")
		}
		h.observeDispatchOrgMismatch(nil, nil)
		empty := NewWebhookHandler(nil, nil, zap.NewNop())
		if _, mismatched := empty.dispatchOrgMismatch(&model.TaskDetail{ID: "t"}, &model.Todo{}); mismatched {
			t.Fatalf("handler without a store must not report a mismatch")
		}
	})
}
