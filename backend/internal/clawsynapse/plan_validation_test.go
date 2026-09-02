package clawsynapse

import (
	"strings"
	"testing"

	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/protocol"
)

func roleResolver(rules map[string][3]string) agentRoleResolver {
	return func(nodeID string) (role, name, agentID string, ok bool) {
		r, found := rules[nodeID]
		if !found {
			return "", "", "", false
		}
		return r[0], r[1], r[2], true
	}
}

func mkWF(steps ...model.WorkflowStep) *model.Workflow {
	return &model.Workflow{Name: "test", Steps: steps}
}

// helper to build todos from plain node ids in order
func todosOf(nodes ...string) []protocol.TaskCreateTodoPayload {
	out := make([]protocol.TaskCreateTodoPayload, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, protocol.TaskCreateTodoPayload{ID: "t", AssigneeNodeID: n})
	}
	return out
}

func TestValidatePlanAgainstWorkflow(t *testing.T) {
	resolver := roleResolver(map[string][3]string{
		"n-writer":     {"编剧", "编剧小A", "ag-writer"},
		"n-director":   {"导演", "导演小B", "ag-director"},
		"n-tester":     {"测试", "测试小C", "ag-tester"},
		"n-developer1": {"developer", "导演小D", "ag-developer1"},
		"n-developer2": {"developer", "导演小E", "ag-developer2"},
	})

	steps := []model.WorkflowStep{
		{Name: "编剧产出剧本", Role: "编剧"},
		{Name: "导演拆解分镜", Role: "导演"},
		{Name: "测试质量验收", Role: "测试"},
	}

	// 用户实际场景：同角色三步（全部 role=developer）
	sameRoleSteps := []model.WorkflowStep{
		{Name: "剧本诊断（圆桌）", Role: "developer"},
		{Name: "编剧修改", Role: "developer"},
		{Name: "导演审核", Role: "developer", NeedReview: true},
	}

	// 具体数字员工绑定：三个步骤绑两个不同 agent
	boundSteps := []model.WorkflowStep{
		{Name: "圆桌", Role: "developer", AgentID: "ag-developer1"},
		{Name: "修改", Role: "developer", AgentID: "ag-developer2"},
		{Name: "审核", Role: "developer", AgentID: "ag-developer1"},
	}

	cases := []struct {
		name  string
		wf    *model.Workflow
		todos []protocol.TaskCreateTodoPayload
		want  string // "" = conform
	}{
		{"nil workflow passes", nil, todosOf("n-writer"), ""},
		{"empty workflow passes", &model.Workflow{Name: "x"}, todosOf("n-writer"), ""},
		{"conform in order", mkWF(steps...), todosOf("n-writer", "n-director", "n-tester"), ""},
		{"repeated role extra todo allowed", mkWF(steps...), todosOf("n-writer", "n-writer", "n-director", "n-tester"), ""},
		{"unknown-role extra todo allowed", mkWF(steps...), todosOf("n-writer", "n-ghost", "n-director", "n-tester"), ""},
		{"split step allowed", mkWF(steps...), todosOf("n-writer", "n-director", "n-director", "n-tester"), ""},
		{"missing step (future role ahead wins)", mkWF(steps...), todosOf("n-writer", "n-tester"), "步骤顺序不符合"},
		{"wrong order (future role ahead)", mkWF(steps...), todosOf("n-director", "n-writer", "n-tester"), "步骤顺序不符合"},
		{"same-role 3 steps conform", mkWF(sameRoleSteps...), todosOf("n-developer1", "n-developer1", "n-developer1"), ""},
		{"same-role 3 steps missing one", mkWF(sameRoleSteps...), todosOf("n-developer1", "n-developer1"), "缺少工作流步骤"},
		{"same-role with extra in middle", mkWF(sameRoleSteps...), todosOf("n-developer1", "n-developer1", "n-developer1", "n-developer1"), ""},
		{"bound agents in exact order", mkWF(boundSteps...), todosOf("n-developer1", "n-developer2", "n-developer1"), ""},
		{"bound agent wrong agent (order wins)", mkWF(boundSteps...), todosOf("n-developer1", "n-developer1", "n-developer1"), "步骤顺序不符合"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validatePlanAgainstWorkflow(tc.wf, tc.todos, resolver)
			if tc.want == "" {
				if got != "" {
					t.Fatalf("expected conform, got mismatch: %s", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("expected mismatch containing %q, got %q", tc.want, got)
			}
		})
	}
}
