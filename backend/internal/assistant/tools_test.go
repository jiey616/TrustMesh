package assistant

import (
	"testing"

	"trustmesh/backend/internal/store"
)

// 🔴 回归护栏：「函数收了 Scope 参数」≠「已隔离」。
// ToolExecutor 曾在内部把参数降级成 store.Scope{UserID: …}，导致租户上下文
// （X-Org-Id）被静默丢弃。本测试锁死：org Scope 必须原样透传到 store 查询层。
func TestToolExecutorPassesOrgScopeThrough(t *testing.T) {
	s := store.New()

	ua, _ := s.CreateUser("a@example.com", "ua", "hash")
	orgA, _ := s.EnsurePersonalOrg(ua.ID, "ua")
	ub, _ := s.CreateUser("b@example.com", "ub", "hash")
	orgB, _ := s.EnsurePersonalOrg(ub.ID, "ub")

	scA := store.Scope{UserID: ua.ID, OrgID: orgA.ID}
	scB := store.Scope{UserID: ub.ID, OrgID: orgB.ID}

	// 各自 org 内建一个 PM agent + 一个项目（走公共 API，归属字段由写路径回填）
	pmA, err := s.CreateAgent(scA, "node-a", "PM-A", "pm", "pm", nil)
	if err != nil {
		t.Fatalf("create agent a: %v", err)
	}
	if _, err := s.CreateProject(scA, "A 的项目", "desc", pmA.ID); err != nil {
		t.Fatalf("create project a: %v", err)
	}
	pmB, err := s.CreateAgent(scB, "node-b", "PM-B", "pm", "pm", nil)
	if err != nil {
		t.Fatalf("create agent b: %v", err)
	}
	if _, err := s.CreateProject(scB, "B 的项目", "desc", pmB.ID); err != nil {
		t.Fatalf("create project b: %v", err)
	}

	e := NewToolExecutor(s, nil, nil)

	// user 维度（无租户头）：退回 user 维度，只看到自己的项目
	res, rerr := e.listProjects(store.Scope{UserID: ua.ID})
	if rerr != nil {
		t.Fatalf("listProjects user scope: %v", rerr)
	}
	if got := res.(map[string]any)["count"].(int); got != 1 {
		t.Fatalf("user scope count = %d, want 1", got)
	}

	// org 维度（带头）：orgA 视角绝不能看到 orgB 的项目（宁可漏、不可泄）
	res, rerr = e.listProjects(scA)
	if rerr != nil {
		t.Fatalf("listProjects org scope: %v", rerr)
	}
	if got := res.(map[string]any)["count"].(int); got != 1 {
		t.Fatalf("org scope count = %d, want 1 (must not leak other orgs)", got)
	}

	// getDashboardStats 同样要能带着 org Scope 跑通
	if _, rerr := e.getDashboardStats(scA); rerr != nil {
		t.Fatalf("getDashboardStats: %v", rerr)
	}
}
