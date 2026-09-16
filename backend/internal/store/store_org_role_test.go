package store

import (
	"testing"

	"trustmesh/backend/internal/model"
)

// testRoleTemplates 复刻 internal/authz 的生产矩阵（字面量 + 顺序一致）。
//
// 为什么在 store 测试里复刻而不是 import authz：authz 反向依赖 store，
// 同包测试再 import authz 会构成导入环。生产环境由 app 层注入真实矩阵
// （router.go 的 SetBuiltinRoleTemplates），这里只验证 store 侧的两条路径：
// 内置角色种子的 permissions 镜像、自定义角色「≤ admin 全集」的校验上限。
func testRoleTemplates() map[string][]string {
	admin := []string{
		"org.member.mgr",
		"project.create", "project.manage",
		"task.create", "task.dispatch",
		"workflow.template.mgr",
		"agent.view", "agent.manage",
		"market.browse", "market.install",
		"meeting.manage", "knowledge.manage",
		"join_request.approve",
		"ops.view", "ops.manage",
	}
	owner := append([]string{"org.settings", "org.role.mgr"}, admin...)
	member := []string{
		"project.create",
		"task.create", "task.dispatch",
		"agent.view",
		"market.browse",
	}
	return map[string][]string{
		model.OrgRoleOwner:  owner,
		model.OrgRoleAdmin:  admin,
		model.OrgRoleMember: member,
	}
}

func newRoleFixture(t *testing.T) (*Store, string) {
	t.Helper()
	s := New()
	s.SetBuiltinRoleTemplates(testRoleTemplates())
	org, appErr := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	if appErr != nil {
		t.Fatalf("create org: %v", appErr)
	}
	return s, org.ID
}

// TestOrgRolesSeededOnCreate 新租户创建即补齐三个内置角色，且 ID 是确定性的
// （种子 / 迁移 / 解析三处共用 model.BuiltinOrgRoleID，跑批才能幂等）。
func TestOrgRolesSeededOnCreate(t *testing.T) {
	s, orgID := newRoleFixture(t)

	roles := s.ListOrgRoles(orgID)
	if len(roles) != 3 {
		t.Fatalf("内置角色数 = %d, want 3", len(roles))
	}
	for i, key := range model.BuiltinRoleKeys { // 展示顺序 owner → admin → member
		role := roles[i]
		if role.ID != model.BuiltinOrgRoleID(orgID, key) {
			t.Errorf("roles[%d].ID = %q, want 确定性 ID %q", i, role.ID, model.BuiltinOrgRoleID(orgID, key))
		}
		if !role.Builtin || role.BuiltinKey != key {
			t.Errorf("roles[%d] = %+v, want builtin key=%q", i, role, key)
		}
		if role.OrgID != orgID {
			t.Errorf("roles[%d].OrgID = %q", i, role.OrgID)
		}
	}
	// 种子里的 permissions 是自描述镜像（不参与裁决），取自注入模板。
	if got := roles[0].Permissions; len(got) != len(testRoleTemplates()[model.OrgRoleOwner]) {
		t.Errorf("owner 镜像权限数 = %d, want %d", len(got), len(testRoleTemplates()[model.OrgRoleOwner]))
	}
	// owner membership 创建即带 role_id。
	m, ok := s.GetMembership(orgID, "u1")
	if !ok || m.RoleID != model.BuiltinOrgRoleID(orgID, model.OrgRoleOwner) {
		t.Fatalf("owner membership = %+v", m)
	}
}

// TestMigrateOrgRolesIdempotent 跑批幂等：已种子的租户不重复种、已回填的不重复填。
func TestMigrateOrgRolesIdempotent(t *testing.T) {
	s, orgID := newRoleFixture(t)
	if _, appErr := s.AddOrgMember(orgID, "u2", model.OrgRoleAdmin); appErr != nil {
		t.Fatalf("add member: %v", appErr)
	}

	if seeded, backfilled := s.MigrateOrgRoles(); seeded != 0 || backfilled != 0 {
		t.Fatalf("第二次跑批应零改动, got seeded=%d backfilled=%d", seeded, backfilled)
	}
	if seeded, _ := s.MigrateOrgRoles(); seeded != 0 {
		t.Fatalf("重复跑批仍应在种子上零改动, got seeded=%d", seeded)
	}
	if roles := s.ListOrgRoles(orgID); len(roles) != 3 {
		t.Fatalf("跑批不得产生重复内置角色, got %d", len(roles))
	}
}

// TestMigrateOrgRolesSeedsMissingBuiltin 兜住存量租户：缺内置角色时补齐。
func TestMigrateOrgRolesSeedsMissingBuiltin(t *testing.T) {
	s := New()
	s.SetBuiltinRoleTemplates(testRoleTemplates())

	// 直接构造一个「先于本次改造存在的租户」（无任何内置角色）。
	s.organizations["org_legacy"] = &model.Organization{
		ID: "org_legacy", Name: "Legacy", Kind: model.OrgKindEnterprise, Status: model.OrgStatusActive,
	}
	seeded, backfilled := s.MigrateOrgRoles()
	if seeded != 3 || backfilled != 0 {
		t.Fatalf("seeded=%d backfilled=%d, want 3/0", seeded, backfilled)
	}
	if roles := s.ListOrgRoles("org_legacy"); len(roles) != 3 {
		t.Fatalf("补齐后角色数 = %d, want 3", len(roles))
	}
}

// TestMigrateOrgRolesBackfillsLegacyMembership 把 role_id 为空的 membership
// 按旧 role 字符串回填为确定性内置角色 ID；未知遗留值不猜不写。
func TestMigrateOrgRolesBackfillsLegacyMembership(t *testing.T) {
	s, orgID := newRoleFixture(t)
	if _, appErr := s.AddOrgMember(orgID, "u2", model.OrgRoleAdmin); appErr != nil {
		t.Fatalf("add admin: %v", appErr)
	}
	if _, appErr := s.AddOrgMember(orgID, "u3", model.OrgRoleMember); appErr != nil {
		t.Fatalf("add member: %v", appErr)
	}

	// 模拟本次改造前的状态：role_id 一律为空。
	m2, _ := s.GetMembership(orgID, "u2")
	m2.RoleID = ""
	m3, _ := s.GetMembership(orgID, "u3")
	m3.RoleID = ""

	if _, backfilled := s.MigrateOrgRoles(); backfilled != 2 {
		t.Fatalf("backfilled = %d, want 2", backfilled)
	}
	if m2.RoleID != model.BuiltinOrgRoleID(orgID, model.OrgRoleAdmin) {
		t.Errorf("admin role_id = %q", m2.RoleID)
	}
	if m3.RoleID != model.BuiltinOrgRoleID(orgID, model.OrgRoleMember) {
		t.Errorf("member role_id = %q", m3.RoleID)
	}

	// 未知遗留值（历史脏数据）：跳过、不写入，让解析链按默认拒绝处理。
	m3.RoleID = ""
	m3.Role = "superuser"
	if _, backfilled := s.MigrateOrgRoles(); backfilled != 0 {
		t.Fatalf("未知遗留 role 不应被回填, got backfilled=%d", backfilled)
	}
	if m3.RoleID != "" || m3.Role != "superuser" {
		t.Fatalf("未知遗留 role 不得被改写, got %+v", m3)
	}
}

// TestCustomOrgRoleLifecycle 自定义角色全生命周期：创建 → 规范化 → 授权成员 →
// 占用保护 → 改名改权限 → 删除。
func TestCustomOrgRoleLifecycle(t *testing.T) {
	s, orgID := newRoleFixture(t)

	role, appErr := s.CreateOrgRole(orgID, "  审计员  ", []string{"ops.view", " ops.view ", "", "agent.view"})
	if appErr != nil {
		t.Fatalf("create role: %v", appErr)
	}
	if role.Name != "审计员" {
		t.Errorf("name 应去空白, got %q", role.Name)
	}
	// 去重 + 按 admin 矩阵顺序输出（agent.view 在 ops.view 之前）。
	want := []string{"agent.view", "ops.view"}
	if len(role.Permissions) != len(want) {
		t.Fatalf("permissions = %v, want %v", role.Permissions, want)
	}
	for i := range want {
		if role.Permissions[i] != want[i] {
			t.Fatalf("permissions = %v, want %v", role.Permissions, want)
		}
	}
	if role.Builtin || role.BuiltinKey != "" {
		t.Errorf("自定义角色不应带内置标记, got %+v", role)
	}

	// 同名（忽略大小写）拒绝。
	if _, appErr := s.CreateOrgRole(orgID, "审计员", nil); appErr == nil || appErr.Code != "ROLE_NAME_TAKEN" {
		t.Fatalf("expected ROLE_NAME_TAKEN, got %v", appErr)
	}

	// 成员引用自定义角色：兼容字段 Role 写 role_id 本身（保持非空、不被内置分支误命中）。
	m, appErr := s.AddOrgMember(orgID, "u2", role.ID)
	if appErr != nil {
		t.Fatalf("assign custom role: %v", appErr)
	}
	if m.RoleID != role.ID || m.Role != role.ID {
		t.Fatalf("membership = %+v, want role_id 双写 %q", m, role.ID)
	}
	if n := s.OrgRoleMemberCount(role.ID); n != 1 {
		t.Fatalf("member_count = %d, want 1", n)
	}

	// 列表顺序：内置在前（owner → admin → member），自定义按创建时间在后。
	roles := s.ListOrgRoles(orgID)
	if len(roles) != 4 || roles[3].ID != role.ID {
		t.Fatalf("list = %+v, want 3 内置 + 1 自定义", roles)
	}

	// 仍被引用 → 不可删（避免 role_id 悬空导致权限静默回落）。
	if appErr := s.DeleteOrgRole(orgID, role.ID); appErr == nil || appErr.Code != "ROLE_IN_USE" {
		t.Fatalf("expected ROLE_IN_USE, got %v", appErr)
	} else if appErr.Details["member_count"] != 1 {
		t.Errorf("ROLE_IN_USE 应带 member_count, got %v", appErr.Details)
	}

	// 改名 + 改权限集。
	updated, appErr := s.UpdateOrgRole(orgID, role.ID, "安全审计员", []string{"ops.view", "ops.manage"})
	if appErr != nil {
		t.Fatalf("update role: %v", appErr)
	}
	if updated.Name != "安全审计员" || len(updated.Permissions) != 2 {
		t.Fatalf("updated = %+v", updated)
	}
	if updated.UpdatedAt.Before(updated.CreatedAt) {
		t.Errorf("UpdatedAt 不应早于 CreatedAt, got %v < %v", updated.UpdatedAt, updated.CreatedAt)
	}
	// RoleByID 返回只读副本：调用方改写不得污染内存状态机。
	if got, ok := s.RoleByID(role.ID); ok {
		got.Name = "被偷改"
		if again, _ := s.RoleByID(role.ID); again.Name != "安全审计员" {
			t.Fatalf("RoleByID 必须返回副本, got %q", again.Name)
		}
	}

	// 改派回内置角色后即可删除。
	if _, appErr := s.UpdateOrgMemberRole(orgID, "u2", model.OrgRoleMember); appErr != nil {
		t.Fatalf("reassign: %v", appErr)
	}
	if m, _ := s.GetMembership(orgID, "u2"); m.RoleID != model.BuiltinOrgRoleID(orgID, model.OrgRoleMember) {
		t.Fatalf("reassign role_id = %q", m.RoleID)
	}
	if appErr := s.DeleteOrgRole(orgID, role.ID); appErr != nil {
		t.Fatalf("delete role: %v", appErr)
	}
	if _, ok := s.RoleByID(role.ID); ok {
		t.Fatal("删除后 RoleByID 不应命中")
	}
	if appErr := s.DeleteOrgRole(orgID, role.ID); appErr == nil || appErr.Code != "NOT_FOUND" {
		t.Fatalf("重复删除应 404, got %v", appErr)
	}
}

// TestCustomOrgRolePermissionCeiling 自定义角色的权限集上限 = admin 全集：
// 这同时排除了 org.settings 与 org.role.mgr —— 无法复制 owner 语义，也无法自我提权。
func TestCustomOrgRolePermissionCeiling(t *testing.T) {
	s, orgID := newRoleFixture(t)

	for _, perm := range []string{"org.settings", "org.role.mgr", "nonsense.perm"} {
		_, appErr := s.CreateOrgRole(orgID, "越权角色", []string{perm})
		if appErr == nil || appErr.Code != "VALIDATION_ERROR" {
			t.Fatalf("perm %q 应被拒绝, got %v", perm, appErr)
		}
		if appErr.Details["permission"] != perm {
			t.Errorf("VALIDATION_ERROR 应带 permission 明细, got %v", appErr.Details)
		}
	}

	// 空权限集合法（相当于「只读基础权限」的占位角色）。
	role, appErr := s.CreateOrgRole(orgID, "观察者", nil)
	if appErr != nil {
		t.Fatalf("空权限集应合法: %v", appErr)
	}
	if len(role.Permissions) != 0 {
		t.Fatalf("permissions = %v, want 空", role.Permissions)
	}
}

// TestBuiltinOrgRoleLocked 内置角色锁定：不可改名、不可改权限、不可删。
func TestBuiltinOrgRoleLocked(t *testing.T) {
	s, orgID := newRoleFixture(t)

	for _, key := range model.BuiltinRoleKeys {
		id := model.BuiltinOrgRoleID(orgID, key)
		if _, appErr := s.UpdateOrgRole(orgID, id, "改名试试", nil); appErr == nil || appErr.Code != "BUILTIN_ROLE_LOCKED" {
			t.Errorf("%s: 内置角色应锁定, got %v", key, appErr)
		}
		if appErr := s.DeleteOrgRole(orgID, id); appErr == nil || appErr.Code != "BUILTIN_ROLE_LOCKED" {
			t.Errorf("%s: 内置角色不可删, got %v", key, appErr)
		}
	}
}

// TestOwnerRoleCannotBeGranted owner 不可经成员管理接口授予（转让走专用流程）：
// 语义键与 role_id 两条入口都必须被拒。
func TestOwnerRoleCannotBeGranted(t *testing.T) {
	s, orgID := newRoleFixture(t)

	if _, appErr := s.AddOrgMember(orgID, "u2", model.OrgRoleOwner); appErr == nil || appErr.Status != 403 {
		t.Fatalf("语义键 owner 应 403, got %v", appErr)
	}
	ownerID := model.BuiltinOrgRoleID(orgID, model.OrgRoleOwner)
	if _, appErr := s.AddOrgMember(orgID, "u2", ownerID); appErr == nil || appErr.Status != 403 {
		t.Fatalf("role_id owner 应 403, got %v", appErr)
	}

	// 既有成员改角色同样不得升级为 owner。
	if _, appErr := s.AddOrgMember(orgID, "u2", model.OrgRoleMember); appErr != nil {
		t.Fatalf("add member: %v", appErr)
	}
	if _, appErr := s.UpdateOrgMemberRole(orgID, "u2", model.OrgRoleOwner); appErr == nil || appErr.Status != 403 {
		t.Fatalf("升级 owner 应 403, got %v", appErr)
	}
}

// TestOrgRoleRefValidation 角色引用的校验：未知引用、跨租户引用、空引用。
func TestOrgRoleRefValidation(t *testing.T) {
	s, orgID := newRoleFixture(t)
	other, appErr := s.CreateOrganization("u9", "Globex", "globex", model.OrgKindEnterprise)
	if appErr != nil {
		t.Fatalf("create other org: %v", appErr)
	}
	otherRole, appErr := s.CreateOrgRole(other.ID, "外部角色", []string{"agent.view"})
	if appErr != nil {
		t.Fatalf("create other role: %v", appErr)
	}

	// 未知引用（历史测试用例同款）。
	if _, appErr := s.AddOrgMember(orgID, "u2", "superuser"); appErr == nil || appErr.Code != "VALIDATION_ERROR" {
		t.Fatalf("未知角色应 400, got %v", appErr)
	}
	// 跨租户 role_id：不可引用他企业的角色。
	if _, appErr := s.AddOrgMember(orgID, "u2", otherRole.ID); appErr == nil || appErr.Code != "VALIDATION_ERROR" {
		t.Fatalf("跨租户 role_id 应 400, got %v", appErr)
	}
	// 空引用回落 member（与改造前一致）。
	m, appErr := s.AddOrgMember(orgID, "u2", "")
	if appErr != nil {
		t.Fatalf("空角色应回落 member: %v", appErr)
	}
	if m.Role != model.OrgRoleMember || m.RoleID != model.BuiltinOrgRoleID(orgID, model.OrgRoleMember) {
		t.Fatalf("membership = %+v, want member", m)
	}
}

// TestPersonalOrgRejectsCustomRoles 个人租户不参与角色体系（设计文档 §5）。
func TestPersonalOrgRejectsCustomRoles(t *testing.T) {
	s := New()
	s.SetBuiltinRoleTemplates(testRoleTemplates())
	org, appErr := s.CreateOrganization("u1", "Me", "me", model.OrgKindPersonal)
	if appErr != nil {
		t.Fatalf("create personal org: %v", appErr)
	}
	if _, appErr := s.CreateOrgRole(org.ID, "自定义", []string{"agent.view"}); appErr == nil || appErr.Code != "PERSONAL_ORG" {
		t.Fatalf("个人租户应拒绝自定义角色, got %v", appErr)
	}
}
