package store

import (
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// 企业角色（org_roles，设计文档 §5）：内置角色种子 + 自定义角色 CRUD +
// role_id 双读迁移。
//
// 分层约定：本文件只做「角色是什么」的存储与校验；「谁能操作角色」由路由层
// authz.RequirePerm 裁决，权限集本身始终以 authz 矩阵/角色定义为唯一来源。

// ---------- 模板注入 ----------

// SetBuiltinRoleTemplates 注入内置角色权限模板（语义键 → 权限集）。
//
// 调用方是 app 层（在 store 构造之后、任何请求之前调用一次）：权限矩阵的权威
// 定义在 internal/authz，store 不反向依赖它，故由上层把矩阵值传进来。
// 模板有两处用途：
//   - 内置角色种子的 permissions 镜像（不参与裁决，见 model.OrgRole 注释）；
//   - 自定义角色「≤ admin 全集」的校验上限。
func (s *Store) SetBuiltinRoleTemplates(templates map[string][]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.roleTemplates = make(map[string][]string, len(templates))
	for key, perms := range templates {
		s.roleTemplates[key] = append([]string{}, perms...)
	}
}

// ---------- 种子与迁移 ----------

// MigrateOrgRoles 启动跑批（幂等，设计文档 §5）：
//  1. 为每个租户补齐缺失的内置角色种子（新建租户已内联种子，这里兜住存量租户）；
//  2. 把 role_id 为空的 membership 按旧 role 字符串映射到本企业的内置角色。
//
// 返回 (补种的内置角色数, 回填的 membership 数)。第 2 步只依赖确定性 ID
// （model.BuiltinOrgRoleID），不依赖权限模板，故模板未注入也不会漏跑迁移。
func (s *Store) MigrateOrgRoles() (seeded, backfilled int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for orgID := range s.organizations {
		n, err := s.seedBuiltinRolesUnsafe(orgID)
		seeded += n
		if err != nil && s.log != nil {
			s.log.Warn("seed builtin org roles failed", zap.String("org_id", orgID), zap.Error(err))
		}
	}

	for _, m := range s.orgMemberships {
		if m == nil || m.RoleID != "" {
			continue
		}
		key := strings.TrimSpace(m.Role)
		if !model.IsBuiltinRoleKey(key) {
			// 未知遗留值：不猜测、不写入（宁可让解析链回落默认拒绝），只告警。
			if s.log != nil {
				s.log.Warn("skip membership with unknown legacy role",
					zap.String("membership_id", m.ID), zap.String("org_id", m.OrgID), zap.String("role", m.Role))
			}
			continue
		}
		id := model.BuiltinOrgRoleID(m.OrgID, key)
		m.RoleID = id
		if err := s.persistMembershipUnsafe(m); err != nil {
			m.RoleID = "" // 持久化失败即回滚内存，与其余写路径的「零副作用」约定一致
			if s.log != nil {
				s.log.Warn("backfill membership role_id failed",
					zap.String("membership_id", m.ID), zap.Error(err))
			}
			continue
		}
		backfilled++
	}

	if s.log != nil {
		s.log.Info("org roles migration done",
			zap.Int("builtin_roles_seeded", seeded), zap.Int("memberships_backfilled", backfilled))
	}
	return seeded, backfilled
}

// seedBuiltinRolesUnsafe 为租户补齐内置角色（幂等：已存在则跳过）。调用方须持写锁。
func (s *Store) seedBuiltinRolesUnsafe(orgID string) (int, error) {
	if orgID == "" {
		return 0, nil
	}
	now := time.Now().UTC()
	created := 0
	for _, key := range model.BuiltinRoleKeys {
		role := &model.OrgRole{
			ID:          model.BuiltinOrgRoleID(orgID, key),
			OrgID:       orgID,
			Name:        model.BuiltinRoleDisplayName(key),
			Permissions: append([]string{}, s.roleTemplates[key]...),
			Builtin:     true,
			BuiltinKey:  key,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if _, ok := s.orgRoles[role.ID]; ok {
			continue
		}
		if err := s.persistOrgRoleUnsafe(role); err != nil {
			return created, err
		}
		s.orgRoles[role.ID] = role
		s.orgRoleIndex[orgID] = append(s.orgRoleIndex[orgID], role.ID)
		created++
	}
	return created, nil
}

// ---------- 读 ----------

// RoleByID 按 ID 取角色定义（只读副本）。供 authz 解析链每请求调用 ——
// 返回副本而非内部指针，避免调用方就地改写内存状态机。
func (s *Store) RoleByID(roleID string) (*model.OrgRole, bool) {
	if roleID == "" {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	role, ok := s.orgRoles[roleID]
	if !ok {
		return nil, false
	}
	return cloneOrgRole(role), true
}

// ListOrgRoles 列出租户角色：内置角色在前（owner → admin → member），
// 自定义角色随后按创建时间升序。
func (s *Store) ListOrgRoles(orgID string) []*model.OrgRole {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := s.orgRoleIndex[orgID]
	out := make([]*model.OrgRole, 0, len(ids))
	for _, id := range ids {
		if role, ok := s.orgRoles[id]; ok {
			out = append(out, cloneOrgRole(role))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Builtin != b.Builtin {
			return a.Builtin
		}
		if a.Builtin {
			return builtinRoleOrder(a.BuiltinKey) < builtinRoleOrder(b.BuiltinKey)
		}
		return a.CreatedAt.Before(b.CreatedAt)
	})
	return out
}

// OrgRoleMemberCount 返回引用该角色的成员数（角色列表的占用计数）。
func (s *Store) OrgRoleMemberCount(roleID string) int {
	if roleID == "" {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.countMembersByRoleUnsafe(roleID)
}

// ---------- 自定义角色 CRUD ----------

// CreateOrgRole 新建企业自定义角色（设计文档 §5）。
func (s *Store) CreateOrgRole(orgID, name string, permissions []string) (*model.OrgRole, *transport.AppError) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, transport.Validation("name: required", nil)
	}
	if len([]rune(name)) > 32 {
		return nil, transport.Validation("name: at most 32 chars", nil)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	org, ok := s.organizations[orgID]
	if !ok {
		return nil, transport.NotFound("organization not found")
	}
	if org.Kind != model.OrgKindEnterprise {
		return nil, transport.BadRequest("PERSONAL_ORG", "personal organization does not support custom roles")
	}
	if s.roleNameTakenUnsafe(orgID, name, "") {
		return nil, transport.Conflict("ROLE_NAME_TAKEN", "a role with this name already exists in the organization")
	}
	clean, appErr := s.normalizeRolePermissionsUnsafe(permissions)
	if appErr != nil {
		return nil, appErr
	}

	now := time.Now().UTC()
	role := &model.OrgRole{
		ID: "role_" + newID(), OrgID: orgID, Name: name,
		Permissions: clean, Builtin: false, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.persistOrgRoleUnsafe(role); err != nil {
		return nil, mongoWriteError(err)
	}
	s.orgRoles[role.ID] = role
	s.orgRoleIndex[orgID] = append(s.orgRoleIndex[orgID], role.ID)
	return cloneOrgRole(role), nil
}

// UpdateOrgRole 修改自定义角色的名称/权限集。内置角色锁定不可改（设计文档 §5）。
//
// name / permissions 为 nil 语义的「不改」由 handler 侧归一（空 name、nil 切片），
// 这里收到的都是最终值。
func (s *Store) UpdateOrgRole(orgID, roleID, name string, permissions []string) (*model.OrgRole, *transport.AppError) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, transport.Validation("name: required", nil)
	}
	if len([]rune(name)) > 32 {
		return nil, transport.Validation("name: at most 32 chars", nil)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	role, appErr := s.customRoleUnsafe(orgID, roleID)
	if appErr != nil {
		return nil, appErr
	}
	if s.roleNameTakenUnsafe(orgID, name, roleID) {
		return nil, transport.Conflict("ROLE_NAME_TAKEN", "a role with this name already exists in the organization")
	}
	clean, appErr := s.normalizeRolePermissionsUnsafe(permissions)
	if appErr != nil {
		return nil, appErr
	}

	prevName, prevPerms := role.Name, role.Permissions
	role.Name = name
	role.Permissions = clean
	role.UpdatedAt = time.Now().UTC()
	if err := s.persistOrgRoleUnsafe(role); err != nil {
		role.Name, role.Permissions = prevName, prevPerms // 持久化失败零副作用
		return nil, mongoWriteError(err)
	}
	return cloneOrgRole(role), nil
}

// DeleteOrgRole 删除自定义角色。内置角色不可删；仍被成员引用的角色不可删
// （避免 role_id 悬空后该成员的权限静默回落）。
func (s *Store) DeleteOrgRole(orgID, roleID string) *transport.AppError {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, appErr := s.customRoleUnsafe(orgID, roleID); appErr != nil {
		return appErr
	}
	if inUse := s.countMembersByRoleUnsafe(roleID); inUse > 0 {
		appErr := transport.Conflict("ROLE_IN_USE", "role is still assigned to members; reassign them first")
		appErr.Details = map[string]any{"member_count": inUse}
		return appErr
	}
	if err := s.deleteOrgRoleUnsafe(roleID); err != nil {
		return mongoWriteError(err)
	}
	delete(s.orgRoles, roleID)
	s.orgRoleIndex[orgID] = removeString(s.orgRoleIndex[orgID], roleID)
	return nil
}

// ---------- 成员 role_id 解析与写入（供 store_org.go 调用，调用方须持锁） ----------

// resolveRoleRefUnsafe 把一个「角色引用串」解析成本企业的角色定义。
//
// ref 既可以是内置角色语义键（owner/admin/member，向后兼容既有 API），
// 也可以是本企业某角色的 role_id。owner 不可通过成员管理接口授予
// （设计文档 §5：owner 不可被替代，转让走专用流程）—— 这里统一拒绝，
// 让「新增成员 / 改角色」两条路径共享同一套校验。
func (s *Store) resolveRoleRefUnsafe(orgID, ref string) (*model.OrgRole, *transport.AppError) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = model.OrgRoleMember
	}
	roleID := ref
	if model.IsBuiltinRoleKey(ref) {
		roleID = model.BuiltinOrgRoleID(orgID, ref)
	}
	role, ok := s.orgRoles[roleID]
	if !ok || role.OrgID != orgID {
		return nil, transport.BadRequest("VALIDATION_ERROR",
			"role: must be owner, admin, member or a role_id of this organization")
	}
	if role.Builtin && role.BuiltinKey == model.OrgRoleOwner {
		return nil, transport.Forbidden("owner role can only be changed via ownership transfer")
	}
	return role, nil
}

// legacyRoleString 返回成员兼容字段 Role 的取值：内置角色写语义键，
// 自定义角色写 role_id（保持非空且不被内置分支误命中）。
func legacyRoleString(role *model.OrgRole) string {
	if role.Builtin {
		return role.BuiltinKey
	}
	return role.ID
}

// countMembersByRoleUnsafe 统计引用某角色的成员数。调用方须持锁。
func (s *Store) countMembersByRoleUnsafe(roleID string) int {
	n := 0
	for _, m := range s.orgMemberships {
		if m != nil && m.RoleID == roleID {
			n++
		}
	}
	return n
}

// ---------- 内部校验工具 ----------

// customRoleUnsafe 取本企业的自定义角色（内置角色 → 409，非本企业/不存在 → 404）。
func (s *Store) customRoleUnsafe(orgID, roleID string) (*model.OrgRole, *transport.AppError) {
	role, ok := s.orgRoles[roleID]
	if !ok || role.OrgID != orgID {
		return nil, transport.NotFound("role not found")
	}
	if role.Builtin {
		return nil, transport.Conflict("BUILTIN_ROLE_LOCKED",
			"builtin role is locked; create a custom role instead")
	}
	return role, nil
}

// roleNameTakenUnsafe 报告租户内是否已有同名角色（excludeID 为被排除的自身）。
func (s *Store) roleNameTakenUnsafe(orgID, name, excludeID string) bool {
	for _, id := range s.orgRoleIndex[orgID] {
		role, ok := s.orgRoles[id]
		if !ok || role.ID == excludeID {
			continue
		}
		if strings.EqualFold(role.Name, name) {
			return true
		}
	}
	return false
}

// normalizeRolePermissionsUnsafe 校验并规范化自定义角色权限集（调用方须持锁）：
// 去空白、去空项、去重、必须 ⊆ admin 全集，并按 admin 矩阵顺序输出
// （同一勾选集合的存储形态稳定，便于 diff 与幂等）。
//
// 「≤ admin 全集」这一条同时排除了 org.settings 与 org.role.mgr
// （admin 矩阵本来就没有这两项）—— 即自定义角色无法复制 owner 语义，
// 也无法自我提权（设计文档 §5）。
func (s *Store) normalizeRolePermissionsUnsafe(permissions []string) ([]string, *transport.AppError) {
	ceiling := s.roleTemplates[model.OrgRoleAdmin]
	allowed := make(map[string]bool, len(ceiling))
	for _, p := range ceiling {
		allowed[p] = true
	}
	seen := make(map[string]bool, len(permissions))
	for _, p := range permissions {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !allowed[p] {
			return nil, transport.Validation(
				"permissions: "+p+" is not assignable (must be a subset of the admin permission set)",
				map[string]any{"permission": p})
		}
		seen[p] = true
	}
	out := make([]string, 0, len(seen))
	for _, p := range ceiling { // 按 admin 矩阵顺序输出
		if seen[p] {
			out = append(out, p)
		}
	}
	return out, nil
}

func cloneOrgRole(role *model.OrgRole) *model.OrgRole {
	if role == nil {
		return nil
	}
	out := *role
	out.Permissions = append([]string{}, role.Permissions...)
	return &out
}

func builtinRoleOrder(key string) int {
	for i, k := range model.BuiltinRoleKeys {
		if k == key {
			return i
		}
	}
	return len(model.BuiltinRoleKeys)
}

// ---------- Mongo 持久化 ----------

func (s *Store) persistOrgRoleUnsafe(role *model.OrgRole) error {
	if !s.mongoEnabled || s.mongoOrgRoles == nil || role == nil {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoOrgRoles.ReplaceOne(ctx, bson.M{"_id": role.ID}, role, options.Replace().SetUpsert(true))
	return err
}

func (s *Store) deleteOrgRoleUnsafe(roleID string) error {
	if !s.mongoEnabled || s.mongoOrgRoles == nil || roleID == "" {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoOrgRoles.DeleteOne(ctx, bson.M{"_id": roleID})
	return err
}

func (s *Store) loadOrgRoles() (map[string]*model.OrgRole, map[string][]string, error) {
	items := make(map[string]*model.OrgRole)
	byOrg := make(map[string][]string)
	if s.mongoOrgRoles == nil {
		return items, byOrg, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoOrgRoles.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, nil, err
	}
	defer cursor.Close(ctx)

	var roles []model.OrgRole
	if err := cursor.All(ctx, &roles); err != nil {
		return nil, nil, err
	}
	for i := range roles {
		role := roles[i]
		items[role.ID] = cloneOrgRole(&role)
		byOrg[role.OrgID] = append(byOrg[role.OrgID], role.ID)
	}
	return items, byOrg, nil
}
