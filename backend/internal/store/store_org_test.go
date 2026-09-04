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
