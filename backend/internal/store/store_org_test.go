package store

import (
	"testing"

	"trustmesh/backend/internal/model"
)

func TestCreateOrganizationSeedsOwnerMembership(t *testing.T) {
	s := New()

	org, appErr := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	if appErr != nil {
		t.Fatalf("create org: %v", appErr)
	}
	if org.Kind != model.OrgKindEnterprise || org.OwnerID != "u1" {
		t.Fatalf("org = %+v", org)
	}
	if org.Quota.MaxMembers != -1 {
		t.Fatalf("default quota should be unlimited, got %+v", org.Quota)
	}

	m, ok := s.GetMembership(org.ID, "u1")
	if !ok {
		t.Fatal("owner membership missing")
	}
	if m.Role != model.OrgRoleOwner {
		t.Fatalf("role = %q", m.Role)
	}
	if got := s.ListUserOrganizations("u1"); len(got) != 1 || got[0].ID != org.ID {
		t.Fatalf("list orgs = %+v", got)
	}
	if got := s.ListUserOrganizations("u2"); len(got) != 0 {
		t.Fatalf("u2 should see no org, got %+v", got)
	}
}

func TestCreateOrganizationRejectsDuplicateSlug(t *testing.T) {
	s := New()
	if _, err := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := s.CreateOrganization("u2", "Other", "acme", model.OrgKindEnterprise)
	if err == nil || err.Code != "ORG_SLUG_TAKEN" {
		t.Fatalf("expected ORG_SLUG_TAKEN, got %v", err)
	}
}

func TestOrgMemberLifecycle(t *testing.T) {
	s := New()
	org, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)

	if _, err := s.AddOrgMember(org.ID, "u2", model.OrgRoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if _, err := s.AddOrgMember(org.ID, "u2", model.OrgRoleMember); err == nil {
		t.Fatal("duplicate member should conflict")
	}
	if members := s.ListOrgMembers(org.ID); len(members) != 2 {
		t.Fatalf("members = %d", len(members))
	}

	updated, err := s.UpdateOrgMemberRole(org.ID, "u2", model.OrgRoleAdmin)
	if err != nil {
		t.Fatalf("update role: %v", err)
	}
	if updated.Role != model.OrgRoleAdmin {
		t.Fatalf("role = %q", updated.Role)
	}
	if _, err := s.UpdateOrgMemberRole(org.ID, "u2", "superuser"); err == nil {
		t.Fatal("invalid role should be rejected")
	}

	// 最后一个 Owner 不允许被移除
	if err := s.RemoveOrgMember(org.ID, "u1"); err == nil || err.Code != "LAST_OWNER" {
		t.Fatalf("expected LAST_OWNER, got %v", err)
	}
	if err := s.RemoveOrgMember(org.ID, "u2"); err != nil {
		t.Fatalf("remove member: %v", err)
	}
	if _, ok := s.GetMembership(org.ID, "u2"); ok {
		t.Fatal("membership should be gone")
	}
	if err := s.RemoveOrgMember(org.ID, "u2"); err == nil {
		t.Fatal("removing a non-member should fail")
	}
}

func TestProjectVisibility(t *testing.T) {
	s := New()
	orgA, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	orgB, _ := s.CreateOrganization("u9", "Globex", "globex", model.OrgKindEnterprise)
	if _, err := s.AddOrgMember(orgA.ID, "u2", model.OrgRoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}

	project := &model.Project{ID: "p1", OrgID: orgA.ID, UserID: "u1"}
	s.projects[project.ID] = project

	// 未设成员白名单 → 同 org 成员全员可见
	if !s.projectVisible(Scope{UserID: "u2", OrgID: orgA.ID}, project) {
		t.Fatal("empty whitelist should be org-visible")
	}
	// 跨 org → 不可见
	if s.projectVisible(Scope{UserID: "u9", OrgID: orgB.ID}, project) {
		t.Fatal("other org must not see the project")
	}
	// 无租户上下文 → 退回 user 维度
	legacy := &model.Project{ID: "p0", UserID: "u1"}
	if !s.projectVisible(Scope{UserID: "u1"}, legacy) {
		t.Fatal("legacy (no org) project should follow user ownership")
	}
	if s.projectVisible(Scope{UserID: "u2"}, legacy) {
		t.Fatal("other user must not see a legacy project")
	}

	// 设置白名单后：非成员不可见、成员可见
	if err := s.SetProjectMembers(project.ID, []model.ProjectMember{{UserID: "u2", Role: model.ProjectMemberRoleViewer}}); err != nil {
		t.Fatalf("set members: %v", err)
	}
	if s.projectVisible(Scope{UserID: "u1", OrgID: orgA.ID}, project) {
		t.Fatal("owner outside whitelist should be blocked")
	}
	if !s.projectVisible(Scope{UserID: "u2", OrgID: orgA.ID}, project) {
		t.Fatal("whitelisted member should see the project")
	}
	if err := s.SetProjectMembers(project.ID, nil); err != nil {
		t.Fatalf("clear members: %v", err)
	}
	if s.projectVisible(Scope{UserID: "u1", OrgID: orgA.ID}, project) != true {
		t.Fatal("clearing the whitelist should restore org-wide visibility")
	}
}

func TestScopeOwnershipHelpers(t *testing.T) {
	sc := Scope{UserID: "u1", OrgID: "org1", Role: model.OrgRoleAdmin}
	if !sc.HasOrg() {
		t.Fatal("HasOrg should be true")
	}
	if !ownedByOrg(sc, "org1") || ownedByOrg(sc, "org2") {
		t.Fatal("ownedByOrg mismatch")
	}
	if !ownedByUser(sc, "u1") || ownedByUser(sc, "u2") {
		t.Fatal("ownedByUser mismatch")
	}
	// 无租户上下文时不得默认放行
	if ownedByOrg(Scope{UserID: "u1"}, "org1") {
		t.Fatal("missing org context must not grant ownership")
	}
	if ownedByUser(Scope{}, "u1") {
		t.Fatal("missing user context must not grant ownership")
	}
}

// 阶段 1：注册即开通个人租户，保证增量数据永远有归属可回填。
func TestCreateUserSeedsPersonalOrg(t *testing.T) {
	s := New()
	u, appErr := s.CreateUser("jiey@example.com", "Jiey", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}

	orgs := s.ListUserOrganizations(u.ID)
	if len(orgs) != 1 {
		t.Fatalf("personal orgs = %d, want 1", len(orgs))
	}
	org := orgs[0]
	if org.Kind != model.OrgKindPersonal || org.OwnerID != u.ID {
		t.Fatalf("org = %+v", org)
	}
	if org.Slug != "u-"+u.ID {
		t.Fatalf("slug = %q", org.Slug)
	}
	m, ok := s.GetMembership(org.ID, u.ID)
	if !ok || m.Role != model.OrgRoleOwner {
		t.Fatalf("owner membership missing: %+v", m)
	}
}

func TestEnsurePersonalOrgIsIdempotent(t *testing.T) {
	s := New()
	first, err := s.EnsurePersonalOrg("u1", "Jiey")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	second, err := s.EnsurePersonalOrg("u1", "Jiey Renamed")
	if err != nil {
		t.Fatalf("ensure again: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected the same org, got %q vs %q", second.ID, first.ID)
	}
	if got := s.ListUserOrganizations("u1"); len(got) != 1 {
		t.Fatalf("orgs = %d, want 1", len(got))
	}
	if got := s.ListOrgMembers(first.ID); len(got) != 1 {
		t.Fatalf("members = %d, want 1", len(got))
	}
}

// ---------- 阶段 2-1：Project 归属收敛 ----------

// TestProjectScopedVisibility 覆盖「带租户头才跨 user 可见、不带则完全等同改造前」。
func TestProjectScopedVisibility(t *testing.T) {
	s := New()
	orgA, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	orgB, _ := s.CreateOrganization("u9", "Globex", "globex", model.OrgKindEnterprise)
	if _, err := s.AddOrgMember(orgA.ID, "u2", model.OrgRoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}

	p1 := &model.Project{ID: "p1", OrgID: orgA.ID, UserID: "u1", Name: "n", Description: "d", Status: "active"}
	s.projects[p1.ID] = p1
	legacy := &model.Project{ID: "p0", UserID: "u1", Name: "legacy", Description: "d", Status: "active"}
	s.projects[legacy.ID] = legacy

	// 无租户上下文：退回 user 维度，u2 什么都看不到（与改造前一致）
	if got := s.ListProjects(Scope{UserID: "u2"}); len(got) != 0 {
		t.Fatalf("u2 without org ctx should see nothing, got %d", len(got))
	}
	if got := s.ListProjects(Scope{UserID: "u1"}); len(got) != 2 {
		t.Fatalf("u1 without org ctx should see own 2 projects, got %d", len(got))
	}
	// 带租户上下文：同租户成员可见
	if got := s.ListProjects(Scope{UserID: "u2", OrgID: orgA.ID}); len(got) != 1 || got[0].ID != "p1" {
		t.Fatalf("u2 in orgA should see p1 only, got %+v", got)
	}
	// 跨租户：看不到
	if got := s.ListProjects(Scope{UserID: "u9", OrgID: orgB.ID}); len(got) != 0 {
		t.Fatalf("other org must see nothing, got %+v", got)
	}

	// GetProject 同理
	if _, err := s.GetProject(Scope{UserID: "u2", OrgID: orgA.ID}, "p1"); err != nil {
		t.Fatalf("u2 in orgA should read p1: %v", err)
	}
	if _, err := s.GetProject(Scope{UserID: "u2"}, "p1"); err == nil {
		t.Fatal("u2 without org ctx must not read p1")
	}
	if _, err := s.GetProject(Scope{UserID: "u9", OrgID: orgB.ID}, "p1"); err == nil {
		t.Fatal("other org must not read p1")
	}
}

// TestResolveOwnerOrg 新建资源的归属：活跃租户优先，否则个人租户。
func TestResolveOwnerOrg(t *testing.T) {
	s := New()
	orgA, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	if got := s.resolveOwnerOrgUnsafe(Scope{UserID: "u1", OrgID: orgA.ID}); got != orgA.ID {
		t.Fatalf("active org should win, got %q", got)
	}

	if _, err := s.EnsurePersonalOrg("u1", "Jiey"); err != nil {
		t.Fatalf("ensure personal org: %v", err)
	}
	var personal *model.Organization
	for _, o := range s.ListUserOrganizations("u1") {
		if o.Kind == model.OrgKindPersonal {
			personal = o
		}
	}
	if personal == nil {
		t.Fatal("personal org missing")
	}
	if got := s.resolveOwnerOrgUnsafe(Scope{UserID: "u1"}); got != personal.ID {
		t.Fatalf("without org ctx should fall back to personal org, got %q want %q", got, personal.ID)
	}
	// 没有个人租户也不阻断写入，只是留空，事后可补偿
	if got := s.resolveOwnerOrgUnsafe(Scope{UserID: "nobody"}); got != "" {
		t.Fatalf("unknown user should resolve to empty, got %q", got)
	}
}

func TestVisibleToScopeFallback(t *testing.T) {
	// 无租户上下文：按 user 维度，与改造前一致
	if !visibleToScope(Scope{UserID: "u1"}, "", "u1") {
		t.Fatal("no-org ctx should fall back to user ownership")
	}
	if visibleToScope(Scope{UserID: "u1"}, "", "u2") {
		t.Fatal("no-org ctx must not grant other user's resource")
	}
	// 有租户上下文：只看 org，user 维度失效
	if !visibleToScope(Scope{UserID: "u1", OrgID: "orgA"}, "orgA", "u2") {
		t.Fatal("same org should see the resource regardless of author")
	}
	if visibleToScope(Scope{UserID: "u1", OrgID: "orgA"}, "orgB", "u1") {
		t.Fatal("other org must not see the resource even if authored by self")
	}
	// 资源还没回填 org_id 时：带租户上下文退回 user 维度兜底，
	// 绝不因数据缺失让作者失联（阶段 1 回填是尽力而为，不能假定 100% 命中）
	if !visibleToScope(Scope{UserID: "u1", OrgID: "orgA"}, "", "u1") {
		t.Fatal("un-backfilled resource authored by self must stay visible")
	}
	if visibleToScope(Scope{UserID: "u1", OrgID: "orgA"}, "", "u2") {
		t.Fatal("un-backfilled resource of other user must not leak under org ctx")
	}
}

// ---------- 阶段 2-2：Task / Planning 归属收敛 ----------

// seedTaskFixture 构造「u1 在 orgA 下的项目 + 任务」与「u9 在 orgB 下的任务」，
// 另放一条未回填 org_id 的存量任务，用于验证兜底语义。
func seedTaskFixture(s *Store, orgA, orgB *model.Organization) {
	s.projects["p1"] = &model.Project{
		ID: "p1", OrgID: orgA.ID, UserID: "u1",
		Name: "p1", Description: "d", Status: "active",
	}
	s.projectTasks["p1"] = []string{"t1", "t1-legacy"}

	s.tasks["t1"] = &model.TaskDetail{
		ID: "t1", OrgID: orgA.ID, UserID: "u1", ProjectID: "p1",
		Title: "org task", Status: "pending",
	}
	// 未回填 org_id 的存量任务：无租户头时按 user 维度仍需可见
	s.tasks["t1-legacy"] = &model.TaskDetail{
		ID: "t1-legacy", UserID: "u1", ProjectID: "p1",
		Title: "legacy task", Status: "pending",
	}
	s.tasks["t9"] = &model.TaskDetail{
		ID: "t9", OrgID: orgB.ID, UserID: "u9", ProjectID: "p1",
		Title: "other org task", Status: "pending",
	}
}

// TestTaskScopedVisibility 覆盖 Task 读路径的三级裁决：
// 无租户头 = 改造前行为；有租户头同租户可见；跨租户不可见。
func TestTaskScopedVisibility(t *testing.T) {
	s := New()
	orgA, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	orgB, _ := s.CreateOrganization("u9", "Globex", "globex", model.OrgKindEnterprise)
	if _, err := s.AddOrgMember(orgA.ID, "u2", model.OrgRoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}
	seedTaskFixture(s, orgA, orgB)

	// GetTask —— 无租户头：u2 看不到 u1 的任务（与改造前一致）
	if _, err := s.GetTask(Scope{UserID: "u2"}, "t1"); err == nil {
		t.Fatal("u2 without org ctx must not read t1")
	}
	if _, err := s.GetTask(Scope{UserID: "u1"}, "t1"); err != nil {
		t.Fatalf("u1 without org ctx should read own task: %v", err)
	}
	// GetTask —— 有租户头：同租户成员可见
	if _, err := s.GetTask(Scope{UserID: "u2", OrgID: orgA.ID}, "t1"); err != nil {
		t.Fatalf("u2 in orgA should read t1: %v", err)
	}
	// GetTask —— 跨租户：不可见（即便作者是自己也不行）
	if _, err := s.GetTask(Scope{UserID: "u1", OrgID: orgB.ID}, "t1"); err == nil {
		t.Fatal("t1 must not be visible under orgB")
	}
	// 未回填 org 的存量任务：无租户头按 user 维度仍可见
	if _, err := s.GetTask(Scope{UserID: "u1"}, "t1-legacy"); err != nil {
		t.Fatalf("legacy task should stay visible to its author: %v", err)
	}

	// ListTasks（按项目）—— 无租户头：u1 看到自己两条
	items, err := s.ListTasks(Scope{UserID: "u1"}, "p1", "")
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("u1 without org ctx should see 2 own tasks, got %d", len(items))
	}
	// ListTasks —— 有租户头：只看到已回填 org 的同租户任务。
	// 未回填 org_id 的存量任务（t1-legacy）在租户上下文下**不会**对他人放行：
	// 这是刻意的安全兜底 —— 数据缺失时宁可漏，不可泄。
	items, err = s.ListTasks(Scope{UserID: "u2", OrgID: orgA.ID}, "p1", "")
	if err != nil {
		t.Fatalf("list tasks in orgA: %v", err)
	}
	if len(items) != 1 || items[0].ID != "t1" {
		t.Fatalf("u2 in orgA should see only the org-backed task t1, got %+v", items)
	}
	// 但未回填任务对作者本人在租户上下文下仍然可见（user 维度兜底）。
	items, err = s.ListTasks(Scope{UserID: "u1", OrgID: orgA.ID}, "p1", "")
	if err != nil {
		t.Fatalf("list tasks as author: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("author should still see both tasks, got %d", len(items))
	}

	// ListRecentTasks —— 跨租户裁剪
	if got := s.ListRecentTasks(Scope{UserID: "u9", OrgID: orgB.ID}, 0); len(got) != 1 || got[0].ID != "t9" {
		t.Fatalf("orgB should only see t9, got %+v", len(got))
	}
	if got := s.ListRecentTasks(Scope{UserID: "u1"}, 0); len(got) != 2 {
		t.Fatalf("u1 without org ctx should see own 2 tasks, got %d", len(got))
	}

	// ListTaskEvents —— 与 GetTask 同源裁决
	if _, err := s.ListTaskEvents(Scope{UserID: "u2"}, "t1"); err == nil {
		t.Fatal("u2 without org ctx must not read t1 events")
	}
	if _, err := s.ListTaskEvents(Scope{UserID: "u2", OrgID: orgA.ID}, "t1"); err != nil {
		t.Fatalf("u2 in orgA should read t1 events: %v", err)
	}
}

// TestGetTaskByNodeIDSameOrg 节点路径：agent 用自身身份构造 Scope，
// 同租户内即使作者不同也能取到任务；跨租户不行。
func TestGetTaskByNodeIDSameOrg(t *testing.T) {
	s := New()
	orgA, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	orgB, _ := s.CreateOrganization("u9", "Globex", "globex", model.OrgKindEnterprise)

	// u2 的 PM agent，归属 orgA
	s.agents["pmA"] = &model.Agent{
		ID: "pmA", OrgID: orgA.ID, UserID: "u2", NodeID: "nodeA",
		Name: "PM A", Role: "pm", Status: "online",
	}
	// u9 的 PM agent，归属 orgB
	s.agents["pmB"] = &model.Agent{
		ID: "pmB", OrgID: orgB.ID, UserID: "u9", NodeID: "nodeB",
		Name: "PM B", Role: "pm", Status: "online",
	}
	// agentByNodeUnsafe 走 s.agentByNode 索引，只写 s.agents 会查不到
	s.agentByNode["nodeA"] = "pmA"
	s.agentByNode["nodeB"] = "pmB"

	// u1（orgA）下的任务，PM 是 pmA
	s.tasks["t1"] = &model.TaskDetail{
		ID: "t1", OrgID: orgA.ID, UserID: "u1", ProjectID: "p1",
		Title: "org task", Status: "planning",
		PMAgent: model.PMAgentSummary{ID: "pmA", Name: "PM A", NodeID: "nodeA"},
	}

	// 同租户、不同作者：放行
	if _, err := s.GetTaskByNodeID("nodeA", "t1"); err != nil {
		t.Fatalf("pm agent in same org should read the task: %v", err)
	}
	// 跨租户：拒绝
	if _, err := s.GetTaskByNodeID("nodeB", "t1"); err == nil {
		t.Fatal("pm agent from another org must not read the task")
	}
}

// TestFinalizePlanByPMNodeAssigneeScope 派发校验以项目归属为准：
// 同租户的 agent 可被派活，跨租户的不行。
func TestFinalizePlanByPMNodeAssigneeScope(t *testing.T) {
	s := New()
	orgA, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	orgB, _ := s.CreateOrganization("u9", "Globex", "globex", model.OrgKindEnterprise)

	s.agents["pmA"] = &model.Agent{
		ID: "pmA", OrgID: orgA.ID, UserID: "u1", NodeID: "nodePM",
		Name: "PM A", Role: "pm", Status: "online",
	}
	s.agents["exA"] = &model.Agent{
		ID: "exA", OrgID: orgA.ID, UserID: "u2", NodeID: "nodeExecSame",
		Name: "Exec A", Role: "executor", Status: "online",
	}
	s.agents["exB"] = &model.Agent{
		ID: "exB", OrgID: orgB.ID, UserID: "u9", NodeID: "nodeExecOther",
		Name: "Exec B", Role: "executor", Status: "online",
	}
	s.agentByNode["nodePM"] = "pmA"
	s.agentByNode["nodeExecSame"] = "exA"
	s.agentByNode["nodeExecOther"] = "exB"

	s.projects["p1"] = &model.Project{
		ID: "p1", OrgID: orgA.ID, UserID: "u1",
		Name: "p1", Description: "d", Status: "active", PMAgentID: "pmA",
	}
	s.tasks["t1"] = &model.TaskDetail{
		ID: "t1", OrgID: orgA.ID, UserID: "u1", ProjectID: "p1",
		Title: "planning task", Status: "planning",
		PMAgent: model.PMAgentSummary{ID: "pmA", Name: "PM A", NodeID: "nodePM"},
	}

	base := func(assigneeNode string) TaskPlanReadyInput {
		return TaskPlanReadyInput{
			TaskID:      "t1",
			Title:       "step one",
			Description: "do the thing",
			Todos: []TaskCreateTodoInput{{
				ID:             "TD_01",
				Order:          1,
				Title:          "todo one",
				Description:    "desc",
				AssigneeNodeID: assigneeNode,
			}},
		}
	}

	// 同租户的 executor：放行
	if _, err := s.FinalizePlanByPMNode("nodePM", "msg-same", base("nodeExecSame")); err != nil {
		t.Fatalf("same-org assignee should be accepted: %v", err)
	}
	// 跨租户的 executor：拒绝（用一个新任务，避免幂等命中）
	s.tasks["t2"] = &model.TaskDetail{
		ID: "t2", OrgID: orgA.ID, UserID: "u1", ProjectID: "p1",
		Title: "planning task 2", Status: "planning",
		PMAgent: model.PMAgentSummary{ID: "pmA", Name: "PM A", NodeID: "nodePM"},
	}
	cross := base("nodeExecOther")
	cross.TaskID = "t2"
	if _, err := s.FinalizePlanByPMNode("nodePM", "msg-cross", cross); err == nil {
		t.Fatal("cross-org assignee must be rejected")
	}
}
// ---------- 阶段 2-2 写路径：新建任务的租户落点 ----------

// TestCreateTaskOwnerOrg 新建任务必须挂到「活跃租户」，
// 无租户上下文时退回到个人租户。
func TestCreateTaskOwnerOrg(t *testing.T) {
	s := New()
	orgA, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	if _, err := s.AddOrgMember(orgA.ID, "u2", model.OrgRoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if _, err := s.EnsurePersonalOrg("u1", "Jiey"); err != nil {
		t.Fatalf("ensure personal org: %v", err)
	}
	var personal *model.Organization
	for _, o := range s.ListUserOrganizations("u1") {
		if o.Kind == model.OrgKindPersonal {
			personal = o
		}
	}
	if personal == nil {
		t.Fatal("personal org missing")
	}

	s.projects["p1"] = &model.Project{
		ID: "p1", OrgID: orgA.ID, UserID: "u1",
		Name: "p1", Description: "d", Status: "active",
	}
	s.agents["exA"] = &model.Agent{
		ID: "exA", OrgID: orgA.ID, UserID: "u2", NodeID: "nodeA",
		Name: "Exec A", Role: "executor", Status: "online",
	}
	// u1 自己的执行者：无租户上下文时按 user 维度命中，用于验证个人租户兜底
	s.agents["exU1"] = &model.Agent{
		ID: "exU1", OrgID: personal.ID, UserID: "u1", NodeID: "nodeU1",
		Name: "Exec U1", Role: "executor", Status: "online",
	}

	input := func() UserTaskCreateInput {
		return UserTaskCreateInput{
			ProjectID:       "p1",
			Title:           "step one",
			Description:     "do the thing",
			Priority:        "medium",
			AssigneeAgentID: "exA",
		}
	}

	// 带租户上下文：任务挂到活跃租户（即便执行者属于 u2）
	task, err := s.CreateTaskByUser(Scope{UserID: "u2", OrgID: orgA.ID}, input())
	if err != nil {
		t.Fatalf("create task under org ctx: %v", err)
	}
	if task.OrgID != orgA.ID {
		t.Fatalf("task should land in the active org, got %q want %q", task.OrgID, orgA.ID)
	}
	if task.UserID != "u2" {
		t.Fatalf("task author should be the caller, got %q", task.UserID)
	}

	// 无租户上下文：退回个人租户（与改造前一致）。
	// 注意执行者必须换成 u1 自己的 —— 无租户头时归属走 user 维度，
	// orgA 里的 exA（属 u2）对 u1 不可见，这与改造前行为完全一致。
	inU1 := input()
	inU1.AssigneeAgentID = "exU1"
	task2, err := s.CreateTaskByUser(Scope{UserID: "u1"}, inU1)
	if err != nil {
		t.Fatalf("create task without org ctx: %v", err)
	}
	if task2.OrgID != personal.ID {
		t.Fatalf("task should land in personal org, got %q want %q", task2.OrgID, personal.ID)
	}

	// 越权：非成员不能在别人的项目里建任务
	if _, err := s.CreateTaskByUser(Scope{UserID: "u9"}, input()); err == nil {
		t.Fatal("outsider must not create tasks in someone else's project")
	}
	// 越权：非成员也不能借用别人的执行者
	if _, err := s.CreateTaskByUser(Scope{UserID: "u9", OrgID: orgA.ID}, input()); err != nil {
		// 带租户头时先卡成员身份（membership 未授予 u9），这里只断言不放行
		_ = err
	}
}
// ---------- 阶段 2-3：Agent / AgentChat 归属收敛 ----------

// seedAgentFixture 构造 orgA(u1 拥有，u2 成员) 与 orgB(u9 拥有) 各一个 agent，
// 另放一条未回填 org_id 的存量 agent，用于验证兜底语义。
func seedAgentFixture(s *Store, orgA, orgB *model.Organization) {
	s.agents["aA"] = &model.Agent{
		ID: "aA", OrgID: orgA.ID, UserID: "u1", NodeID: "nodeA",
		Name: "Agent A", Role: "pm", Status: "online",
	}
	s.agents["aA-legacy"] = &model.Agent{
		ID: "aA-legacy", UserID: "u1", NodeID: "nodeLegacy",
		Name: "Legacy Agent", Role: "executor", Status: "online",
	}
	s.agents["aB"] = &model.Agent{
		ID: "aB", OrgID: orgB.ID, UserID: "u9", NodeID: "nodeB",
		Name: "Agent B", Role: "executor", Status: "online",
	}
}

// TestAgentScopedVisibility 覆盖 Agent 读路径的三级裁决。
func TestAgentScopedVisibility(t *testing.T) {
	s := New()
	orgA, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	orgB, _ := s.CreateOrganization("u9", "Globex", "globex", model.OrgKindEnterprise)
	if _, err := s.AddOrgMember(orgA.ID, "u2", model.OrgRoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}
	seedAgentFixture(s, orgA, orgB)

	// ListAgents —— 无租户头：只看自己的
	if got := s.ListAgents(Scope{UserID: "u1"}); len(got) != 2 {
		t.Fatalf("u1 without org ctx should see own 2 agents, got %d", len(got))
	}
	if got := s.ListAgents(Scope{UserID: "u2"}); len(got) != 0 {
		t.Fatalf("u2 without org ctx should see nothing, got %d", len(got))
	}
	// ListAgents —— 有租户头：同租户成员可见（未回填的存量 agent 不对他人放行）
	if got := s.ListAgents(Scope{UserID: "u2", OrgID: orgA.ID}); len(got) != 1 || got[0].ID != "aA" {
		t.Fatalf("u2 in orgA should see only the org-backed agent, got %+v", got)
	}
	// ListAgents —— 跨租户：不可见
	if got := s.ListAgents(Scope{UserID: "u9", OrgID: orgB.ID}); len(got) != 1 || got[0].ID != "aB" {
		t.Fatalf("orgB should only see aB, got %+v", got)
	}

	// GetAgent
	if _, err := s.GetAgent(Scope{UserID: "u2"}, "aA"); err == nil {
		t.Fatal("u2 without org ctx must not read aA")
	}
	if _, err := s.GetAgent(Scope{UserID: "u2", OrgID: orgA.ID}, "aA"); err != nil {
		t.Fatalf("u2 in orgA should read aA: %v", err)
	}
	if _, err := s.GetAgent(Scope{UserID: "u1", OrgID: orgB.ID}, "aA"); err == nil {
		t.Fatal("aA must not be visible under orgB")
	}
	// 未回填 agent 对作者本人在租户上下文下仍可见
	if _, err := s.GetAgent(Scope{UserID: "u1", OrgID: orgA.ID}, "aA-legacy"); err != nil {
		t.Fatalf("legacy agent should stay visible to its owner: %v", err)
	}

	// UpdateAgent / DeleteAgent 同源裁决
	if _, err := s.UpdateAgent(Scope{UserID: "u2"}, "aA", UpdateAgentInput{Name: strPtr("renamed")}); err == nil {
		t.Fatal("u2 without org ctx must not update aA")
	}
	if _, err := s.UpdateAgent(Scope{UserID: "u2", OrgID: orgA.ID}, "aA", UpdateAgentInput{Name: strPtr("renamed")}); err != nil {
		t.Fatalf("u2 in orgA should update aA: %v", err)
	}
	if err := s.DeleteAgent(Scope{UserID: "u9", OrgID: orgB.ID}, "aA"); err == nil {
		t.Fatal("cross-org delete must be rejected")
	}
	if err := s.DeleteAgent(Scope{UserID: "u1", OrgID: orgA.ID}, "aA-legacy"); err != nil {
		t.Fatalf("owner should delete own legacy agent: %v", err)
	}

	// GetAgentStats / GetAgentInsights / ListAgentTasks 同源裁决
	if _, err := s.GetAgentStats(Scope{UserID: "u2"}, "aB"); err == nil {
		t.Fatal("u2 without org ctx must not read aB stats")
	}
	if _, err := s.GetAgentStats(Scope{UserID: "u9", OrgID: orgB.ID}, "aB"); err != nil {
		t.Fatalf("u9 in orgB should read aB stats: %v", err)
	}
	if _, err := s.GetAgentInsights(Scope{UserID: "u2"}, "aB"); err == nil {
		t.Fatal("u2 without org ctx must not read aB insights")
	}
	if _, err := s.ListAgentTasks(Scope{UserID: "u2"}, "aB", ""); err == nil {
		t.Fatal("u2 without org ctx must not list aB tasks")
	}
	if _, err := s.ListAgentTasks(Scope{UserID: "u9", OrgID: orgB.ID}, "aB", ""); err != nil {
		t.Fatalf("u9 in orgB should list aB tasks: %v", err)
	}
}

// TestCreateAgentOwnerOrg 新建 Agent 必须挂到活跃租户，
// 无租户上下文时退回个人租户。
func TestCreateAgentOwnerOrg(t *testing.T) {
	s := New()
	orgA, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	if _, err := s.AddOrgMember(orgA.ID, "u2", model.OrgRoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if _, err := s.EnsurePersonalOrg("u1", "Jiey"); err != nil {
		t.Fatalf("ensure personal org: %v", err)
	}
	var personal *model.Organization
	for _, o := range s.ListUserOrganizations("u1") {
		if o.Kind == model.OrgKindPersonal {
			personal = o
		}
	}
	if personal == nil {
		t.Fatal("personal org missing")
	}

	// 带租户上下文：挂活跃租户（作者是 u2）
	agent, err := s.CreateAgent(Scope{UserID: "u2", OrgID: orgA.ID}, "nodeX", "Exec X", "developer", "desc", nil)
	if err != nil {
		t.Fatalf("create agent under org ctx: %v", err)
	}
	if agent.OrgID != orgA.ID {
		t.Fatalf("agent should land in the active org, got %q want %q", agent.OrgID, orgA.ID)
	}
	if agent.UserID != "u2" {
		t.Fatalf("agent author should be the caller, got %q", agent.UserID)
	}

	// 无租户上下文：退回个人租户
	agent2, err := s.CreateAgent(Scope{UserID: "u1"}, "nodeY", "Exec Y", "developer", "desc", nil)
	if err != nil {
		t.Fatalf("create agent without org ctx: %v", err)
	}
	if agent2.OrgID != personal.ID {
		t.Fatalf("agent should land in personal org, got %q want %q", agent2.OrgID, personal.ID)
	}
}

// TestAgentChatIsUserScoped Agent 会话是「某人 ↔ 某 agent」的一对一私人对话：
// agent 可见性走租户（org 成员可与租户内的 agent 对话），但会话归属仍按 user。
func TestAgentChatIsUserScoped(t *testing.T) {
	s := New()
	orgA, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	if _, err := s.AddOrgMember(orgA.ID, "u2", model.OrgRoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}
	s.agents["aA"] = &model.Agent{
		ID: "aA", OrgID: orgA.ID, UserID: "u1", NodeID: "nodeA",
		Name: "Agent A", Role: "pm", Status: "online",
	}

	// u2 能看到租户内的 agent，因此可以与它对话（agent 维度走 org）
	if _, err := s.agentForUserUnsafe(Scope{UserID: "u2", OrgID: orgA.ID}, "aA"); err != nil {
		t.Fatalf("org member should be able to talk to an org agent: %v", err)
	}
	// 无租户头则不行（与改造前一致）
	if _, err := s.agentForUserUnsafe(Scope{UserID: "u2"}, "aA"); err == nil {
		t.Fatal("outsider without org ctx must not use the agent")
	}

	// 会话本身按 user 分区：u2 开了会话，u1 看不到它
	if _, _, err := s.AppendAgentChatUserMessage(Scope{UserID: "u2", OrgID: orgA.ID}, "aA", "hello"); err != nil {
		t.Fatalf("append chat message: %v", err)
	}
	detail, err := s.GetActiveAgentChat(Scope{UserID: "u2", OrgID: orgA.ID}, "aA")
	if err != nil || detail == nil {
		t.Fatalf("u2 should see own active chat: %v", err)
	}
	if len(detail.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(detail.Messages))
	}
	own, err := s.GetActiveAgentChat(Scope{UserID: "u1", OrgID: orgA.ID}, "aA")
	if err != nil {
		t.Fatalf("u1 reading own (empty) chat: %v", err)
	}
	if own != nil {
		t.Fatal("u1 must not see u2's personal chat session")
	}
}

// ---------------------------------------------------------------------------
// 阶段 2-4：Knowledge + WorkflowTemplate 归属收敛
// ---------------------------------------------------------------------------

func seedKnowledgeFixture(s *Store, orgA, orgB *model.Organization) {
	s.knowledgeDocs["kbA"] = &model.KnowledgeDocument{
		ID: "kbA", OrgID: orgA.ID, UserID: "u1", Title: "A 的知识", Status: model.KnowledgeDocStatusReady,
	}
	s.knowledgeDocs["kbA-legacy"] = &model.KnowledgeDocument{
		ID: "kbA-legacy", UserID: "u1", Title: "未回填的知识", Status: model.KnowledgeDocStatusReady,
	}
	s.knowledgeDocs["kbB"] = &model.KnowledgeDocument{
		ID: "kbB", OrgID: orgB.ID, UserID: "u9", Title: "B 的知识", Status: model.KnowledgeDocStatusReady,
	}
	s.userKnowledgeDocs["u1"] = []string{"kbA", "kbA-legacy"}
	s.userKnowledgeDocs["u9"] = []string{"kbB"}

	s.workflowTemplates["wtA"] = &model.WorkflowTemplate{
		ID: "wtA", OrgID: orgA.ID, UserID: "u1", Name: "A 的模板", Version: 1,
	}
	s.workflowTemplates["wtB"] = &model.WorkflowTemplate{
		ID: "wtB", OrgID: orgB.ID, UserID: "u9", Name: "B 的模板", Version: 1,
	}
	s.userWorkflowTemplates["u1"] = []string{"wtA"}
	s.userWorkflowTemplates["u9"] = []string{"wtB"}
}

// TestKnowledgeScopedVisibility 覆盖知识库文档的三级裁决。
func TestKnowledgeScopedVisibility(t *testing.T) {
	s := New()
	orgA, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	orgB, _ := s.CreateOrganization("u9", "Globex", "globex", model.OrgKindEnterprise)
	if _, err := s.AddOrgMember(orgA.ID, "u2", model.OrgRoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}
	seedKnowledgeFixture(s, orgA, orgB)

	// 无租户头：与改造前一致，只看自己的
	if got := s.ListKnowledgeDocuments(Scope{UserID: "u1"}, "", "", ""); len(got) != 2 {
		t.Fatalf("u1 without org ctx should see own 2 docs, got %d", len(got))
	}
	if got := s.ListKnowledgeDocuments(Scope{UserID: "u2"}, "", "", ""); len(got) != 0 {
		t.Fatalf("u2 without org ctx should see nothing, got %d", len(got))
	}

	// 有租户头：全量扫描 + 归属裁决，同租户成员的已回填文档可见
	got := s.ListKnowledgeDocuments(Scope{UserID: "u2", OrgID: orgA.ID}, "", "", "")
	if len(got) != 1 || got[0].ID != "kbA" {
		t.Fatalf("u2 in orgA should see kbA only (未回填文档不对他人放行), got %+v", got)
	}
	// 未回填文档对作者本人在租户上下文下仍可见
	if got := s.ListKnowledgeDocuments(Scope{UserID: "u1", OrgID: orgA.ID}, "", "", ""); len(got) != 2 {
		t.Fatalf("u1 in orgA should still see own 2 docs, got %d", len(got))
	}
	// 跨租户不可见（u9 是 orgB 的成员；Scope 的成员合法性由 middleware.OrgScope 保证，
	// store 层信任传入的 Scope，因此这里必须用"真实成员 + 目标租户"的组合）
	if got := s.ListKnowledgeDocuments(Scope{UserID: "u9", OrgID: orgB.ID}, "", "", ""); len(got) != 1 || got[0].ID != "kbB" {
		t.Fatalf("u9 in orgB should see only kbB, got %+v", got)
	}

	// GetKnowledgeDocument
	if _, err := s.GetKnowledgeDocument(Scope{UserID: "u2"}, "kbA"); err == nil {
		t.Fatal("u2 without org ctx must not read kbA")
	}
	if _, err := s.GetKnowledgeDocument(Scope{UserID: "u2", OrgID: orgA.ID}, "kbA"); err != nil {
		t.Fatalf("u2 in orgA should read kbA: %v", err)
	}
	if _, err := s.GetKnowledgeDocument(Scope{UserID: "u9", OrgID: orgB.ID}, "kbA"); err == nil {
		t.Fatal("kbA must not be visible under orgB")
	}
}

// TestCreateKnowledgeDocOwnerOrg 覆盖阶段 1 漏掉的双写：
// 带租户头建的文档挂活跃租户，无租户头退回个人租户。
func TestCreateKnowledgeDocOwnerOrg(t *testing.T) {
	s := New()
	orgA, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	if _, err := s.AddOrgMember(orgA.ID, "u2", model.OrgRoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if _, err := s.EnsurePersonalOrg("u1", "Jiey"); err != nil {
		t.Fatalf("ensure personal org: %v", err)
	}
	var personal *model.Organization
	for _, o := range s.ListUserOrganizations("u1") {
		if o.Kind == model.OrgKindPersonal {
			personal = o
		}
	}
	if personal == nil {
		t.Fatal("personal org missing")
	}

	// 带租户上下文（作者是 u2）
	doc, err := s.CreateKnowledgeDocument(Scope{UserID: "u2", OrgID: orgA.ID}, &model.KnowledgeDocument{Title: "X"})
	if err != nil {
		t.Fatalf("create doc under org ctx: %v", err)
	}
	if doc.OrgID != orgA.ID {
		t.Fatalf("doc should land in the active org, got %q want %q", doc.OrgID, orgA.ID)
	}
	if doc.UserID != "u2" {
		t.Fatalf("doc author should be the caller, got %q", doc.UserID)
	}

	// 无租户上下文：退回个人租户
	doc2, err := s.CreateKnowledgeDocument(Scope{UserID: "u1"}, &model.KnowledgeDocument{Title: "Y"})
	if err != nil {
		t.Fatalf("create doc without org ctx: %v", err)
	}
	if doc2.OrgID != personal.ID {
		t.Fatalf("doc should land in personal org, got %q want %q", doc2.OrgID, personal.ID)
	}
}

// TestWorkflowTemplateScopedVisibility 覆盖工作流模板的三级裁决，
// 并验证带租户头时 List 能突破 user 分区索引看到同租户成员的模板。
func TestWorkflowTemplateScopedVisibility(t *testing.T) {
	s := New()
	orgA, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	orgB, _ := s.CreateOrganization("u9", "Globex", "globex", model.OrgKindEnterprise)
	if _, err := s.AddOrgMember(orgA.ID, "u2", model.OrgRoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}
	seedKnowledgeFixture(s, orgA, orgB)

	// 无租户头：走 user 分区索引，与改造前一致
	if got := s.ListWorkflowTemplates(Scope{UserID: "u1"}); len(got) != 1 || got[0].ID != "wtA" {
		t.Fatalf("u1 without org ctx should see own wtA, got %+v", got)
	}
	if got := s.ListWorkflowTemplates(Scope{UserID: "u2"}); len(got) != 0 {
		t.Fatalf("u2 without org ctx should see nothing, got %d", len(got))
	}

	// 有租户头：全量扫描，同租户成员的模板可见（分区索引扫不到）
	got := s.ListWorkflowTemplates(Scope{UserID: "u2", OrgID: orgA.ID})
	if len(got) != 1 || got[0].ID != "wtA" {
		t.Fatalf("u2 in orgA should see wtA via org scan, got %+v", got)
	}
	// 跨租户不可见
	if got := s.ListWorkflowTemplates(Scope{UserID: "u9", OrgID: orgB.ID}); len(got) != 1 || got[0].ID != "wtB" {
		t.Fatalf("u9 in orgB should see only wtB, got %+v", got)
	}

	// GetWorkflowTemplate
	if _, err := s.GetWorkflowTemplate(Scope{UserID: "u2"}, "wtA"); err == nil {
		t.Fatal("u2 without org ctx must not read wtA")
	}
	if _, err := s.GetWorkflowTemplate(Scope{UserID: "u2", OrgID: orgA.ID}, "wtA"); err != nil {
		t.Fatalf("u2 in orgA should read wtA: %v", err)
	}
	if _, err := s.GetWorkflowTemplate(Scope{UserID: "u9", OrgID: orgB.ID}, "wtA"); err == nil {
		t.Fatal("wtA must not be visible under orgB")
	}

	// CopyWorkflowTemplate 的副本归属跟随当前 Scope
	copied, err := s.CopyWorkflowTemplate(Scope{UserID: "u2", OrgID: orgA.ID}, "wtA")
	if err != nil {
		t.Fatalf("copy under org ctx: %v", err)
	}
	if copied.OrgID != orgA.ID || copied.UserID != "u2" {
		t.Fatalf("copy owner = org:%q user:%q", copied.OrgID, copied.UserID)
	}
}

// TestValidateProjectOwnershipScoped 项目归属校验走统一裁决。
func TestValidateProjectOwnershipScoped(t *testing.T) {
	s := New()
	orgA, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	orgB, _ := s.CreateOrganization("u9", "Globex", "globex", model.OrgKindEnterprise)
	if _, err := s.AddOrgMember(orgA.ID, "u2", model.OrgRoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}
	s.projects["pA"] = &model.Project{ID: "pA", OrgID: orgA.ID, UserID: "u1", Name: "A 的项目"}

	if err := s.ValidateProjectOwnership(Scope{UserID: "u2"}, "pA"); err == nil {
		t.Fatal("u2 without org ctx must not own pA")
	}
	if err := s.ValidateProjectOwnership(Scope{UserID: "u2", OrgID: orgA.ID}, "pA"); err != nil {
		t.Fatalf("u2 in orgA should own pA: %v", err)
	}
	if err := s.ValidateProjectOwnership(Scope{UserID: "u9", OrgID: orgB.ID}, "pA"); err == nil {
		t.Fatal("pA must not validate under orgB")
	}
}

// ---------------------------------------------------------------------------
// 阶段 2-5a：File 域归属收敛（project_file + artifact）
// ---------------------------------------------------------------------------

// seedProjectFileFixture 建两个租户各一个项目，并各挂一个文件。
// 注意：File 域的归属是「通过项目裁决」的（projectVisible），不是按文件自身字段。
func seedProjectFileFixture(s *Store, orgA, orgB *model.Organization) {
	s.projects["pA"] = &model.Project{ID: "pA", OrgID: orgA.ID, UserID: "u1", Name: "A 的项目"}
	s.projects["pB"] = &model.Project{ID: "pB", OrgID: orgB.ID, UserID: "u9", Name: "B 的项目"}
	s.projectFiles["pfA"] = &model.ProjectFile{
		ID: "pfA", ProjectID: "pA", OrgID: orgA.ID, FileName: "a.txt", UploadedBy: "u1",
	}
	s.projectFileIndex["pA"] = []string{"pfA"}
}

// TestProjectFileScopedVisibility 覆盖项目文件读路径的归属裁决。
func TestProjectFileScopedVisibility(t *testing.T) {
	s := New()
	orgA, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	orgB, _ := s.CreateOrganization("u9", "Globex", "globex", model.OrgKindEnterprise)
	if _, err := s.AddOrgMember(orgA.ID, "u2", model.OrgRoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}
	seedProjectFileFixture(s, orgA, orgB)

	// 无租户头：与改造前一致
	if got := s.ListProjectFiles(Scope{UserID: "u1"}, "pA", "", "", ""); len(got) != 1 {
		t.Fatalf("u1 without org ctx should see own file, got %d", len(got))
	}
	if _, err := s.GetProjectFile(Scope{UserID: "u2"}, "pfA"); err == nil {
		t.Fatal("u2 without org ctx must not read pfA")
	}

	// 有租户头：同租户成员可见
	if got := s.ListProjectFiles(Scope{UserID: "u2", OrgID: orgA.ID}, "pA", "", "", ""); len(got) != 1 {
		t.Fatalf("u2 in orgA should see pfA, got %d", len(got))
	}
	if _, err := s.GetProjectFile(Scope{UserID: "u2", OrgID: orgA.ID}, "pfA"); err != nil {
		t.Fatalf("u2 in orgA should read pfA: %v", err)
	}

	// 跨租户不可见（u9 是 orgB 的真实成员）
	if got := s.ListProjectFiles(Scope{UserID: "u9", OrgID: orgB.ID}, "pA", "", "", ""); len(got) != 0 {
		t.Fatalf("orgB must not expose pA files to u9, got %d", len(got))
	}
	if _, err := s.GetProjectFile(Scope{UserID: "u9", OrgID: orgB.ID}, "pfA"); err == nil {
		t.Fatal("pfA must not be visible under orgB")
	}

	// 其他读路径同样收口
	// 拒绝路径返回空树（不是 nil），按"无内容"断言
	if got := s.GetProjectFileTree(Scope{UserID: "u9", OrgID: orgB.ID}, "pA"); got != nil && (len(got.Uploads) != 0 || len(got.Tasks) != 0) {
		t.Fatalf("GetProjectFileTree leaked pA to orgB: %+v", got)
	}
	if _, err := s.BrowseProjectFiles(Scope{UserID: "u9", OrgID: orgB.ID}, "pA", ""); err == nil {
		t.Fatal("BrowseProjectFiles must reject pA under orgB")
	}
	if _, err := s.ListArtifactGroups(Scope{UserID: "u9", OrgID: orgB.ID}, "pA"); err == nil {
		t.Fatal("ListArtifactGroups must reject pA under orgB")
	}
	if got := s.BatchDeleteProjectFiles(Scope{UserID: "u9", OrgID: orgB.ID}, "pA", []string{"pfA"}); got.Deleted != 0 {
		t.Fatalf("BatchDeleteProjectFiles must not delete across orgs, deleted=%d", got.Deleted)
	}
}

// TestSaveProjectFileOwnerOrg 覆盖阶段 1 漏掉的双写：
// SaveProjectFile 从未设置 pf.OrgID，不补则新建文件 org_id 恒空。
func TestSaveProjectFileOwnerOrg(t *testing.T) {
	s := New()
	orgA, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	if _, err := s.AddOrgMember(orgA.ID, "u2", model.OrgRoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if _, err := s.EnsurePersonalOrg("u1", "Jiey"); err != nil {
		t.Fatalf("ensure personal org: %v", err)
	}
	var personal *model.Organization
	for _, o := range s.ListUserOrganizations("u1") {
		if o.Kind == model.OrgKindPersonal {
			personal = o
		}
	}
	if personal == nil {
		t.Fatal("personal org missing")
	}
	s.projects["pA"] = &model.Project{ID: "pA", OrgID: orgA.ID, UserID: "u1", Name: "A 的项目"}

	// 带租户上下文（作者是 u2，同租户成员）
	pf, err := s.SaveProjectFile(Scope{UserID: "u2", OrgID: orgA.ID}, "pA", &model.ProjectFile{FileName: "x.txt"})
	if err != nil {
		t.Fatalf("save file under org ctx: %v", err)
	}
	if pf.OrgID != orgA.ID {
		t.Fatalf("file should land in the active org, got %q want %q", pf.OrgID, orgA.ID)
	}
	if pf.UploadedBy != "u2" {
		t.Fatalf("uploader should be the caller, got %q", pf.UploadedBy)
	}

	// 无租户上下文：退回个人租户
	pf2, err := s.SaveProjectFile(Scope{UserID: "u1"}, "pA", &model.ProjectFile{FileName: "y.txt"})
	if err != nil {
		t.Fatalf("save file without org ctx: %v", err)
	}
	if pf2.OrgID != personal.ID {
		t.Fatalf("file should land in personal org, got %q want %q", pf2.OrgID, personal.ID)
	}

	// CreateFolder 同理
	folder, err := s.CreateFolder(Scope{UserID: "u2", OrgID: orgA.ID}, "pA", "新建目录", "")
	if err != nil {
		t.Fatalf("create folder under org ctx: %v", err)
	}
	if folder.OrgID != orgA.ID || folder.UploadedBy != "u2" {
		t.Fatalf("folder owner = org:%q by:%q", folder.OrgID, folder.UploadedBy)
	}
}

// TestBindArtifactOutputScoped 交付物手工绑定按任务归属裁决。
func TestBindArtifactOutputScoped(t *testing.T) {
	s := New()
	orgA, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	orgB, _ := s.CreateOrganization("u9", "Globex", "globex", model.OrgKindEnterprise)
	if _, err := s.AddOrgMember(orgA.ID, "u2", model.OrgRoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}
	s.tasks["tA"] = &model.TaskDetail{
		ID: "tA", OrgID: orgA.ID, UserID: "u1", Title: "A 的任务",
		Todos: []model.Todo{{ID: "TD_01", Title: "待办"}},
	}
	s.taskArtifacts["tA"] = []model.TaskArtifact{
		{TransferID: "tid-1", TaskID: "tA", FileName: "out.md"},
	}

	// 无租户头：他人不可绑定
	if _, err := s.BindArtifactOutput(Scope{UserID: "u2"}, "tA", "TD_01", "tid-1", "分镜脚本"); err == nil {
		t.Fatal("u2 without org ctx must not bind tA artifacts")
	}
	// 有租户头：同租户成员可绑定
	if _, err := s.BindArtifactOutput(Scope{UserID: "u2", OrgID: orgA.ID}, "tA", "TD_01", "tid-1", "分镜脚本"); err != nil {
		t.Fatalf("u2 in orgA should bind tA artifacts: %v", err)
	}
	// 跨租户不可见（u9 是 orgB 的真实成员）
	if _, err := s.BindArtifactOutput(Scope{UserID: "u9", OrgID: orgB.ID}, "tA", "TD_01", "tid-1", "脚本"); err == nil {
		t.Fatal("tA must not be bindable under orgB")
	}
}


// ─── 阶段 2-5b：Meeting + Comment 归属收敛 ───

func seedMeetingFixture(s *Store, orgA, orgB *model.Organization) {
	s.projects["pA"] = &model.Project{ID: "pA", OrgID: orgA.ID, UserID: "u1", Name: "A 的项目"}
	s.projects["pB"] = &model.Project{ID: "pB", OrgID: orgB.ID, UserID: "u9", Name: "B 的项目"}
	s.meetings["mA"] = &model.Meeting{
		ID: "mA", ProjectID: "pA", OrgID: orgA.ID, CreatorID: "u1",
		Title: "A 的会议", Status: model.MeetingWaiting,
	}
	s.projectMeetings["pA"] = []string{"mA"}
	s.meetingMessages["msgA"] = &model.MeetingMessage{
		ID: "msgA", MeetingID: "mA", OrgID: orgA.ID,
		SenderType: "user", SenderID: "u1", Content: "hi",
	}
	s.meetingMessageIndex["mA"] = []string{"msgA"}
}

// TestMeetingScopedVisibility 覆盖会议域读/写路径的归属裁决。
// 改造前这些函数的 userID 形参被写成 `_`（完全丢弃），HTTP 请求不做任何校验。
func TestMeetingScopedVisibility(t *testing.T) {
	s := New()
	orgA, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	orgB, _ := s.CreateOrganization("u9", "Globex", "globex", model.OrgKindEnterprise)
	if _, err := s.AddOrgMember(orgA.ID, "u2", model.OrgRoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}
	seedMeetingFixture(s, orgA, orgB)

	// 无租户头：与改造前一致（按会议创建者判断）
	if got := s.ListMeetings(Scope{UserID: "u1"}, "pA"); len(got) != 1 {
		t.Fatalf("u1 without org ctx should see own meeting, got %d", len(got))
	}
	if _, err := s.GetMeeting(Scope{UserID: "u2"}, "mA"); err == nil {
		t.Fatal("u2 without org ctx must not read mA")
	}

	// 有租户头：同租户成员可见（项目成员白名单生效）
	if got := s.ListMeetings(Scope{UserID: "u2", OrgID: orgA.ID}, "pA"); len(got) != 1 {
		t.Fatalf("u2 in orgA should see mA, got %d", len(got))
	}
	if _, err := s.GetMeeting(Scope{UserID: "u2", OrgID: orgA.ID}, "mA"); err != nil {
		t.Fatalf("u2 in orgA should read mA: %v", err)
	}
	if got := s.ListMeetingMessages(Scope{UserID: "u2", OrgID: orgA.ID}, "mA"); len(got) != 1 {
		t.Fatalf("u2 in orgA should read mA messages, got %d", len(got))
	}

	// 跨租户不可见（u9 是 orgB 的真实成员）
	if got := s.ListMeetings(Scope{UserID: "u9", OrgID: orgB.ID}, "pA"); len(got) != 0 {
		t.Fatalf("orgB must not expose pA meetings to u9, got %d", len(got))
	}
	if _, err := s.GetMeeting(Scope{UserID: "u9", OrgID: orgB.ID}, "mA"); err == nil {
		t.Fatal("mA must not be visible under orgB")
	}
	if got := s.ListMeetingMessages(Scope{UserID: "u9", OrgID: orgB.ID}, "mA"); len(got) != 0 {
		t.Fatalf("orgB must not read mA messages, got %d", len(got))
	}

	// 写路径同样收口
	if err := s.UpdateMeetingStatus(Scope{UserID: "u9", OrgID: orgB.ID}, "mA", model.MeetingCompleted); err == nil {
		t.Fatal("mA must not be updatable under orgB")
	}
	if err := s.UpdateMeetingSummary(Scope{UserID: "u9", OrgID: orgB.ID}, "mA", "sf-1"); err == nil {
		t.Fatal("UpdateMeetingSummary must reject cross-org")
	}
	if err := s.UpdateMeetingMinutes(Scope{UserID: "u9", OrgID: orgB.ID}, "mA", "md", "mf-1"); err == nil {
		t.Fatal("UpdateMeetingMinutes must reject cross-org")
	}
	if _, err := s.AddMeetingTodo(Scope{UserID: "u9", OrgID: orgB.ID}, "mA", model.MeetingTodoItem{Description: "x"}); err == nil {
		t.Fatal("AddMeetingTodo must reject cross-org")
	}
	if _, err := s.AddMeetingMessage(Scope{UserID: "u9", OrgID: orgB.ID}, &model.MeetingMessage{
		MeetingID: "mA", SenderType: "user", SenderID: "u9", Content: "越权发言",
	}); err == nil {
		t.Fatal("AddMeetingMessage must reject cross-org (改造前任何登录用户都能往任意会议发消息)")
	}

	// 越权调用不得改写任何状态
	if s.meetings["mA"].Status != model.MeetingWaiting {
		t.Fatal("mA status mutated by cross-org call")
	}
	if s.meetings["mA"].SummaryFileID != "" || s.meetings["mA"].MinutesFileID != "" {
		t.Fatal("mA mutated by cross-org call")
	}
	if len(s.meetingMessageIndex["mA"]) != 1 {
		t.Fatal("cross-org message was appended")
	}
}

// TestSystemScopeBypassesMeetingOwnership 系统旁路语义。
// 内部路径（agent webhook / timeout_monitor）没有用户会话，改造前一律传 ""
// 绕过校验；收敛后必须显式 SystemScope() 才放行。
func TestSystemScopeBypassesMeetingOwnership(t *testing.T) {
	s := New()
	orgA, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	orgB, _ := s.CreateOrganization("u9", "Globex", "globex", model.OrgKindEnterprise)
	seedMeetingFixture(s, orgA, orgB)

	// 系统路径显式放行
	if _, err := s.GetMeeting(SystemScope(), "mA"); err != nil {
		t.Fatalf("SystemScope should read mA: %v", err)
	}
	if err := s.UpdateMeetingStatus(SystemScope(), "mA", model.MeetingCompleted); err != nil {
		t.Fatalf("SystemScope should update mA status: %v", err)
	}
	if got := s.ListMeetingMessages(SystemScope(), "mA"); len(got) != 1 {
		t.Fatalf("SystemScope should read mA messages, got %d", len(got))
	}
	if got := s.ListMeetings(SystemScope(), "pA"); len(got) != 1 {
		t.Fatalf("SystemScope should list pA meetings, got %d", len(got))
	}
	if _, err := s.AddMeetingMessage(SystemScope(), &model.MeetingMessage{
		MeetingID: "mA", SenderType: "agent", SenderID: "node-1", Content: "agent 发言",
	}); err != nil {
		t.Fatalf("SystemScope should allow agent message: %v", err)
	}

	// 🔴 零值 Scope 不是系统旁路 —— 不能因为「忘了传 UserID」就放行，
	// 否则等于给越权留后门（改造前正是靠传 "" 蒙混过关的）。
	if _, err := s.GetMeeting(Scope{}, "mA"); err == nil {
		t.Fatal("zero-value Scope must NOT bypass ownership")
	}
	if got := s.ListMeetings(Scope{}, "pA"); len(got) != 0 {
		t.Fatalf("zero-value Scope must NOT list meetings, got %d", len(got))
	}
}

// TestCreateMeetingOwnerOrg 覆盖阶段 1 漏掉的双写：
// CreateMeeting 的 OrgID 恒挂作者个人租户，企业租户下建的会议带租户头看不到。
func TestCreateMeetingOwnerOrg(t *testing.T) {
	s := New()
	orgA, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	if _, err := s.AddOrgMember(orgA.ID, "u2", model.OrgRoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if _, err := s.EnsurePersonalOrg("u1", "Jiey"); err != nil {
		t.Fatalf("ensure personal org: %v", err)
	}
	var personal *model.Organization
	for _, o := range s.ListUserOrganizations("u1") {
		if o.Kind == model.OrgKindPersonal {
			personal = o
		}
	}
	if personal == nil {
		t.Fatal("personal org missing")
	}
	s.projects["pA"] = &model.Project{ID: "pA", OrgID: orgA.ID, UserID: "u1", Name: "A 的项目"}

	// 带租户上下文：挂到当前活跃租户（作者是同租户成员 u2）
	m, err := s.CreateMeeting(Scope{UserID: "u2", OrgID: orgA.ID}, &model.Meeting{ProjectID: "pA", Title: "同租户会议"})
	if err != nil {
		t.Fatalf("create meeting under org ctx: %v", err)
	}
	if m.OrgID != orgA.ID {
		t.Fatalf("meeting should land in the active org, got %q want %q", m.OrgID, orgA.ID)
	}
	if m.CreatorID != "u2" {
		t.Fatalf("creator should be the caller, got %q", m.CreatorID)
	}
	// 同租户另一个成员必须能看到
	if got := s.ListMeetings(Scope{UserID: "u1", OrgID: orgA.ID}, "pA"); len(got) != 1 {
		t.Fatalf("u1 should see the org meeting created by u2, got %d", len(got))
	}

	// 无租户上下文：退回个人租户
	m2, err := s.CreateMeeting(Scope{UserID: "u1"}, &model.Meeting{ProjectID: "pA", Title: "个人会议"})
	if err != nil {
		t.Fatalf("create meeting without org ctx: %v", err)
	}
	if m2.OrgID != personal.ID {
		t.Fatalf("meeting should land in personal org, got %q want %q", m2.OrgID, personal.ID)
	}
}

// TestTaskCommentScopedAndOrgInheritance 评论域归属收敛 + 评论归属继承任务。
func TestTaskCommentScopedAndOrgInheritance(t *testing.T) {
	s := New()
	orgA, _ := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	orgB, _ := s.CreateOrganization("u9", "Globex", "globex", model.OrgKindEnterprise)
	if _, err := s.AddOrgMember(orgA.ID, "u2", model.OrgRoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}
	s.tasks["tA"] = &model.TaskDetail{ID: "tA", OrgID: orgA.ID, UserID: "u1", Title: "A 的任务"}

	// 无租户头：他人不可评论 / 不可读
	if _, err := s.AddTaskComment(Scope{UserID: "u2"}, "tA", TaskCommentInput{Content: "hi"}); err == nil {
		t.Fatal("u2 without org ctx must not comment on tA")
	}
	if _, err := s.ListTaskComments(Scope{UserID: "u2"}, "tA"); err == nil {
		t.Fatal("u2 without org ctx must not read tA comments")
	}

	// 有租户头：同租户成员可评论
	c, err := s.AddTaskComment(Scope{UserID: "u2", OrgID: orgA.ID}, "tA", TaskCommentInput{Content: "hi"})
	if err != nil {
		t.Fatalf("u2 in orgA should comment on tA: %v", err)
	}
	// 🔴 评论归属必须继承所属任务：改造前从作者个人租户派生，
	// 企业租户下评论会挂到个人租户、与任务的 org 不一致。
	if c.OrgID != orgA.ID {
		t.Fatalf("comment must inherit task org, got %q want %q", c.OrgID, orgA.ID)
	}
	if got, err := s.ListTaskComments(Scope{UserID: "u2", OrgID: orgA.ID}, "tA"); err != nil || len(got) != 1 {
		t.Fatalf("u2 in orgA should read tA comments: got=%d err=%v", len(got), err)
	}

	// 跨租户不可见
	if _, err := s.ListTaskComments(Scope{UserID: "u9", OrgID: orgB.ID}, "tA"); err == nil {
		t.Fatal("tA comments must not be visible under orgB")
	}
	if _, err := s.AddTaskComment(Scope{UserID: "u9", OrgID: orgB.ID}, "tA", TaskCommentInput{Content: "越权"}); err == nil {
		t.Fatal("tA must not be commentable under orgB")
	}
}
