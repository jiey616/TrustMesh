package clawsynapse

import (
	"fmt"
	"strings"

	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/protocol"
)

// agentRoleResolver resolves the role/name/id of an agent by node id.
// Used to match plan todos against workflow steps.
type agentRoleResolver func(nodeID string) (role, name, agentID string, ok bool)

// validatePlanAgainstWorkflow checks the PM's planned todos against the task's
// workflow. It returns "" when the plan conforms, or a Chinese mismatch
// description (missing step / wrong order / role mismatch) for the PM to fix.
//
// This is the full-coverage variant: every workflow step must have a todo.
// See validatePlanAgainstWorkflowWithScope for partial-delivery plans.
func validatePlanAgainstWorkflow(wf *model.Workflow, todos []protocol.TaskCreateTodoPayload, roleOf agentRoleResolver) string {
	return validatePlanAgainstWorkflowWithScope(wf, todos, roleOf, nil)
}

// validatePlanAgainstWorkflowWithScope is validatePlanAgainstWorkflow with an
// optional delivery scope. When scope.UpToStep names a real workflow step, only
// the steps up to and including it are required — later steps may be omitted.
// This powers "user only wants delivery up to step X" (e.g. 只到资产图).
//
// Constraints (guards against PM abusing scope to bypass validation):
//   - UpToStep must exist in the workflow, otherwise the declaration is invalid
//   - only the TAIL may be truncated; skipping a middle step is still rejected
//     (the greedy sequential match below never allows out-of-order skips)
//
// Step binding: step.AgentID (exact agent) takes precedence; otherwise the
// step's Role is fuzzy-matched against the todo assignee's role or name.
//
// Algorithm (greedy sequential match, supports same-role consecutive steps):
//   - walk todos in plan order; a todo matching the CURRENT step's binding
//     consumes that step (cur++) and is its representative
//   - a todo matching a FUTURE step whose predecessors are still unmatched is
//     an order violation (the future role appeared too early)
//   - a todo matching no pending step is an extra todo (allowed: splits,
//     clarifications, checks)
//   - after the walk, any required step not consumed => missing step
func validatePlanAgainstWorkflowWithScope(wf *model.Workflow, todos []protocol.TaskCreateTodoPayload, roleOf agentRoleResolver, scope *model.TaskDeliverScope) string {
	if wf == nil || len(wf.Steps) == 0 {
		return ""
	}

	// Resolve the delivery scope: by default every step is required.
	limit := len(wf.Steps)
	if scope != nil {
		upTo := strings.TrimSpace(scope.UpToStep)
		if upTo != "" {
			idx := indexOfWorkflowStep(wf.Steps, upTo)
			if idx < 0 {
				return fmt.Sprintf("交付范围声明无效：工作流中不存在步骤「%s」", upTo)
			}
			// Only the tail may be dropped: steps [0, idx] remain required.
			limit = idx + 1
		}
	}

	// Resolve each todo's assignee: role \x00 name \x00 agentID ("" if unknown).
	roleOfTodo := make([]string, len(todos))
	for i, todo := range todos {
		if todo.AssigneeNodeID == "" {
			continue
		}
		if role, name, agentID, ok := roleOf(todo.AssigneeNodeID); ok {
			roleOfTodo[i] = role + "\x00" + name + "\x00" + agentID
		}
	}

	cur := 0 // index of the next workflow step to consume
	consumed := make([]bool, len(wf.Steps))

	for _, m := range roleOfTodo {
		if cur >= limit {
			break // remaining todos are extras (or beyond the declared scope)
		}
		parts := strings.SplitN(m, "\x00", 3)
		role, name, agentID := "", "", ""
		if len(parts) == 3 {
			role, name, agentID = parts[0], parts[1], parts[2]
		}

		// 1) Matches the current step's binding -> consume it.
		if stepMatches(wf.Steps[cur], role, name, agentID) {
			consumed[cur] = true
			cur++
			continue
		}

		// 2) Matches a FUTURE step while predecessors are unmatched -> order bug.
		future := -1
		for k := cur + 1; k < limit; k++ {
			if stepMatches(wf.Steps[k], role, name, agentID) {
				future = k
				break
			}
		}
		if future >= 0 {
			return fmt.Sprintf("工作流步骤顺序不符合：「%s」应先于「%s」", wf.Steps[cur].Name, wf.Steps[future].Name)
		}
		// 3) Otherwise: extra todo (split of a consumed step, clarification…). Skip.
	}

	// Missing steps?
	var missing []string
	for si, step := range wf.Steps {
		if si >= limit {
			break // steps after the declared delivery scope are not required
		}
		if !consumed[si] {
			missing = append(missing, fmt.Sprintf("「%s」（角色：%s）", step.Name, step.Role))
		}
	}
	if len(missing) > 0 {
		if limit < len(wf.Steps) {
			return "缺少工作流步骤对应的 todo（声明交付到「" + strings.TrimSpace(scope.UpToStep) + "」，该步及之前缺）：" + strings.Join(missing, "、")
		}
		return "缺少工作流步骤对应的 todo：" + strings.Join(missing, "、")
	}
	return ""
}

// indexOfWorkflowStep returns the index of the step with the given name, or -1.
func indexOfWorkflowStep(steps []model.WorkflowStep, name string) int {
	for i, s := range steps {
		if strings.TrimSpace(s.Name) == name {
			return i
		}
	}
	return -1
}

// stepMatches checks whether a todo's assignee (role/name/agentID) satisfies a
// step's binding. Exact agent id wins; otherwise fuzzy role/name match.
func stepMatches(step model.WorkflowStep, role, name, agentID string) bool {
	if step.AgentID != "" {
		return agentID != "" && step.AgentID == agentID
	}
	stepRole := strings.TrimSpace(step.Role)
	if stepRole == "" {
		return false
	}
	return roleMatch(role, stepRole) || roleMatch(name, stepRole)
}

// roleMatch does a fuzzy role comparison (exact or containment on either side).
func roleMatch(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	if strings.Contains(a, b) || strings.Contains(b, a) {
		return true
	}
	return false
}
