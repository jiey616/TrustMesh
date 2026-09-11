package store

import (
	"testing"

	"trustmesh/backend/internal/model"
)

// T0.0 测试前置：锁住 agentCanWriteTaskUnsafe（scope.go:141）的裁决语义。
//
// 该函数的回归面覆盖 8 个调用点：
//   - workflow.go:684 / 769 / 911 / 1155 / 1235 / 2189
//     （todo.progress / todo.complete / todo.fail / todo.ask / task.comment / 重试）
//   - store_artifact.go:217 / 487（产物归档）
//
// T0.2 要修改它的 3 选一规则并把「同租户可指派」前置到创建期，因此必须先有
// 基线断言：跨租户不可写这条底线在任何改动后都不能被放开。

// TestAgentCanWriteTaskUnsafeRegression 是跨租户「不可写」基线。
// 这些断言针对**现有行为**，现在就应当通过；T0.2 若放开创建期指派，
// 也不得让下面的 false 用例翻成 true（否则就是越权）。
func TestAgentCanWriteTaskUnsafeRegression(t *testing.T) {
	s := New()

	orgA, appErr := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	if appErr != nil {
		t.Fatalf("create orgA: %v", appErr)
	}
	if _, appErr := s.CreateOrganization("u9", "Globex", "globex", model.OrgKindEnterprise); appErr != nil {
		t.Fatalf("create orgB: %v", appErr)
	}
	// u2 是 orgA 成员：其 agent 属于「同租户共享 agent」
	if _, appErr := s.AddOrgMember(orgA.ID, "u2", model.OrgRoleMember); appErr != nil {
		t.Fatalf("add member: %v", appErr)
	}

	cases := []struct {
		name  string
		task  *model.TaskDetail
		agent *model.Agent
		want  bool
	}{
		{
			name:  "同用户_存量行为放行",
			task:  &model.TaskDetail{ID: "t-1", UserID: "u1"},
			agent: &model.Agent{ID: "a-1", UserID: "u1"},
			want:  true,
		},
		{
			name:  "跨用户且任务未回填org_拒绝_宁可漏不可泄",
			task:  &model.TaskDetail{ID: "t-2", UserID: "u1"},
			agent: &model.Agent{ID: "a-2", UserID: "u2"},
			want:  false,
		},
		{
			name:  "agent显式归属任务租户_放行",
			task:  &model.TaskDetail{ID: "t-3", UserID: "u1", OrgID: orgA.ID},
			agent: &model.Agent{ID: "a-3", UserID: "u2", OrgID: orgA.ID},
			want:  true,
		},
		{
			name:  "agent属主是任务租户成员_同租户跨用户协作放行",
			task:  &model.TaskDetail{ID: "t-4", UserID: "u1", OrgID: orgA.ID},
			agent: &model.Agent{ID: "a-4", UserID: "u2"},
			want:  true,
		},
		{
			name:  "agent属主非该租户成员_拒绝",
			task:  &model.TaskDetail{ID: "t-5", UserID: "u1", OrgID: orgA.ID},
			agent: &model.Agent{ID: "a-5", UserID: "u9"},
			want:  false,
		},
		{
			name:  "nil任务_拒绝",
			task:  nil,
			agent: &model.Agent{ID: "a-6", UserID: "u1"},
			want:  false,
		},
		{
			name:  "nil_agent_拒绝",
			task:  &model.TaskDetail{ID: "t-7", UserID: "u1"},
			agent: nil,
			want:  false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s.mu.Lock()
			got := s.agentCanWriteTaskUnsafe(c.task, c.agent)
			s.mu.Unlock()
			if got != c.want {
				t.Fatalf("agentCanWriteTaskUnsafe = %v, want %v", got, c.want)
			}
		})
	}
}

// TestAgentCanAssignableToProjectUnsafe 覆盖 T0.2 新增的**创建期**指派裁决
// （scope.go:203），与回写期 agentCanWriteTaskUnsafe 成对：
//
//	创建期可指派 + 回写期可写 = 共享 agent 回报才不会被 404 吞掉
//	（2026-09-11 实锤：山雨账号建任务，编剧全部回报被 webhook 404 吞掉）。
//
// 跨租户仍必须拒绝 —— 这是 T0.2「放宽创建期」时不得越过的底线。
func TestAgentCanAssignableToProjectUnsafe(t *testing.T) {
	s := New()

	orgA, appErr := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	if appErr != nil {
		t.Fatalf("create orgA: %v", appErr)
	}
	// u2 是 orgA 成员：其 agent 属于「同租户共享 agent」
	if _, appErr := s.AddOrgMember(orgA.ID, "u2", model.OrgRoleMember); appErr != nil {
		t.Fatalf("add member: %v", appErr)
	}

	cases := []struct {
		name    string
		agent   *model.Agent
		project *model.Project
		want    bool
	}{
		{
			name:    "同用户_放行",
			agent:   &model.Agent{ID: "a-1", UserID: "u1"},
			project: &model.Project{ID: "p-1", UserID: "u1"},
			want:    true,
		},
		{
			name:    "跨用户且项目未回填org_拒绝_存量个人项目严格",
			agent:   &model.Agent{ID: "a-2", UserID: "u2"},
			project: &model.Project{ID: "p-2", UserID: "u1"},
			want:    false,
		},
		{
			name:    "agent显式归属项目租户_放行",
			agent:   &model.Agent{ID: "a-3", UserID: "u2", OrgID: orgA.ID},
			project: &model.Project{ID: "p-3", UserID: "u1", OrgID: orgA.ID},
			want:    true,
		},
		{
			name:    "agent属主是项目租户成员_同租户共享agent可指派",
			agent:   &model.Agent{ID: "a-4", UserID: "u2"},
			project: &model.Project{ID: "p-4", UserID: "u1", OrgID: orgA.ID},
			want:    true,
		},
		{
			name:    "agent属主非该租户成员_拒绝",
			agent:   &model.Agent{ID: "a-5", UserID: "u9"},
			project: &model.Project{ID: "p-5", UserID: "u1", OrgID: orgA.ID},
			want:    false,
		},
		{
			name:    "nil_agent_拒绝",
			agent:   nil,
			project: &model.Project{ID: "p-6", UserID: "u1"},
			want:    false,
		},
		{
			name:    "nil_project_拒绝",
			agent:   &model.Agent{ID: "a-7", UserID: "u1"},
			project: nil,
			want:    false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s.mu.Lock()
			got := s.agentCanAssignableToProjectUnsafe(c.agent, c.project)
			s.mu.Unlock()
			if got != c.want {
				t.Fatalf("agentCanAssignableToProjectUnsafe = %v, want %v", got, c.want)
			}
		})
	}
}
