package store

import (
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// 多租户阶段 0：org 与 membership 的 CRUD + Mongo 持久化。
// 本文件不改动任何现有业务逻辑，纯新增能力，可整体删除回滚。

// CreateOrganization 创建租户，并把创建者写入 Owner 成员关系。
func (s *Store) CreateOrganization(userID, name, slug, kind string) (*model.Organization, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if userID == "" {
		return nil, transport.BadRequest("VALIDATION_ERROR", "user_id: required")
	}
	if name == "" {
		return nil, transport.BadRequest("VALIDATION_ERROR", "name: required")
	}
	if slug == "" {
		return nil, transport.BadRequest("VALIDATION_ERROR", "slug: required")
	}
	if kind == "" {
		kind = model.OrgKindEnterprise
	}
	// slug 全局唯一（预留域名/URL 隔离）
	for _, org := range s.organizations {
		if org.Slug == slug {
			return nil, transport.Conflict("ORG_SLUG_TAKEN", "slug already in use")
		}
	}

	now := time.Now().UTC()
	org := &model.Organization{
		ID:      "org_" + newID(),
		Name:    name,
		Slug:    slug,
		Kind:    kind,
		OwnerID: userID,
		// 配额取平台全局配置的新建企业默认（未配置全局配置时 = 全部不限，与历史一致）；
		// 个人租户不受全局配置影响，仍为不限。
		Quota:     s.quotaForNewOrgUnsafe(kind),
		Status:    model.OrgStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.persistOrganizationUnsafe(org); err != nil {
		return nil, mongoWriteError(err)
	}
	s.organizations[org.ID] = org

	// 角色种子（设计文档 §5）：新租户创建即补齐三个内置角色，
	// 使「建成员 → 写 role_id」不会引用到不存在的角色。
	if _, err := s.seedBuiltinRolesUnsafe(org.ID); err != nil {
		return nil, mongoWriteError(err)
	}

	member := &model.OrgMembership{
		ID:       "om_" + newID(),
		OrgID:    org.ID,
		UserID:   userID,
		Role:     model.OrgRoleOwner,
		RoleID:   model.BuiltinOrgRoleID(org.ID, model.OrgRoleOwner),
		JoinedAt: now,
	}
	if err := s.persistMembershipUnsafe(member); err != nil {
		return nil, mongoWriteError(err)
	}
	s.orgMemberships[member.ID] = member
	s.orgMemberIndex[org.ID] = append(s.orgMemberIndex[org.ID], member.ID)
	s.userOrgIndex[userID] = append(s.userOrgIndex[userID], member.ID)

	return org, nil
}

// GetOrganization 按 ID 取租户（不做权限判断，权限由上层 scope 裁决）。
func (s *Store) GetOrganization(orgID string) (*model.Organization, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	org, ok := s.organizations[orgID]
	if !ok {
		return nil, transport.NotFound("organization not found")
	}
	return org, nil
}

// ListUserOrganizations 列出用户所属的全部租户（个人 + 企业）。
func (s *Store) ListUserOrganizations(userID string) []*model.Organization {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*model.Organization, 0, 1)
	seen := map[string]bool{}
	for _, mid := range s.userOrgIndex[userID] {
		m, ok := s.orgMemberships[mid]
		if !ok || seen[m.OrgID] {
			continue
		}
		seen[m.OrgID] = true
		if org, ok := s.organizations[m.OrgID]; ok {
			out = append(out, org)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// GetMembership 取用户在某租户下的成员关系。
func (s *Store) GetMembership(orgID, userID string) (*model.OrgMembership, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, mid := range s.userOrgIndex[userID] {
		m, ok := s.orgMemberships[mid]
		if ok && m.OrgID == orgID {
			return m, true
		}
	}
	return nil, false
}

// ListOrgMembers 列出租户成员（按加入时间升序）。
func (s *Store) ListOrgMembers(orgID string) []*model.OrgMembership {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := s.orgMemberIndex[orgID]
	out := make([]*model.OrgMembership, 0, len(ids))
	for _, id := range ids {
		if m, ok := s.orgMemberships[id]; ok {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].JoinedAt.Before(out[j].JoinedAt) })
	return out
}

// AddOrgMember 向租户添加成员。存在则返回既有关系，不重复创建。
//
// roleRef 既接受内置角色语义键（owner/admin/member），也接受本企业某角色的
// role_id（自定义角色，设计文档 §5）；owner 不可通过本接口授予。
func (s *Store) AddOrgMember(orgID, userID, roleRef string) (*model.OrgMembership, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.organizations[orgID]; !ok {
		return nil, transport.NotFound("organization not found")
	}
	if userID == "" {
		return nil, transport.BadRequest("VALIDATION_ERROR", "user_id: required")
	}
	role, appErr := s.resolveRoleRefUnsafe(orgID, roleRef)
	if appErr != nil {
		return nil, appErr
	}
	if _, exists := s.findMembershipUnsafe(orgID, userID); exists {
		return nil, transport.Conflict("ALREADY_MEMBER", "user is already a member of this organization")
	}

	m := &model.OrgMembership{
		ID:       "om_" + newID(),
		OrgID:    orgID,
		UserID:   userID,
		Role:     legacyRoleString(role),
		RoleID:   role.ID,
		JoinedAt: time.Now().UTC(),
	}
	if err := s.persistMembershipUnsafe(m); err != nil {
		return nil, mongoWriteError(err)
	}
	s.orgMemberships[m.ID] = m
	s.orgMemberIndex[orgID] = append(s.orgMemberIndex[orgID], m.ID)
	s.userOrgIndex[userID] = append(s.userOrgIndex[userID], m.ID)
	return m, nil
}

// UpdateOrgMemberRole 改成员角色；Owner 不允许降级（转让需走专用流程，二期）。
// roleRef 语义同 AddOrgMember。
func (s *Store) UpdateOrgMemberRole(orgID, userID, roleRef string) (*model.OrgMembership, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, exists := s.findMembershipUnsafe(orgID, userID)
	if !exists {
		return nil, transport.NotFound("membership not found")
	}
	role, appErr := s.resolveRoleRefUnsafe(orgID, roleRef)
	if appErr != nil {
		return nil, appErr
	}
	prevRole, prevRoleID := m.Role, m.RoleID
	m.Role, m.RoleID = legacyRoleString(role), role.ID
	if err := s.persistMembershipUnsafe(m); err != nil {
		m.Role, m.RoleID = prevRole, prevRoleID // 持久化失败零副作用
		return nil, mongoWriteError(err)
	}
	return m, nil
}

// RemoveOrgMember 移除成员。租户最后一个 Owner 不允许被移除。
func (s *Store) RemoveOrgMember(orgID, userID string) *transport.AppError {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, exists := s.findMembershipUnsafe(orgID, userID)
	if !exists {
		return transport.NotFound("membership not found")
	}
	if m.Role == model.OrgRoleOwner && s.countOwnersUnsafe(orgID) <= 1 {
		return transport.BadRequest("LAST_OWNER", "cannot remove the last owner of an organization")
	}
	if err := s.deleteMembershipUnsafe(m.ID); err != nil {
		return mongoWriteError(err)
	}
	delete(s.orgMemberships, m.ID)
	s.orgMemberIndex[orgID] = removeString(s.orgMemberIndex[orgID], m.ID)
	s.userOrgIndex[userID] = removeString(s.userOrgIndex[userID], m.ID)
	return nil
}

// UpdateOrgProfile 更新组织展示资料：名称与简称。
// 仅企业租户可设置；short_name 上限 5 字（按 rune 计，兼容中文）。
// 调用方（handler）负责 owner/admin 门禁；此处只做字段校验与持久化。
func (s *Store) UpdateOrgProfile(orgID, name, shortName string) (*model.Organization, *transport.AppError) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, transport.BadRequest("VALIDATION_ERROR", "name: required")
	}
	if runes := []rune(strings.TrimSpace(shortName)); len(runes) > 5 {
		return nil, transport.BadRequest("VALIDATION_ERROR", "short_name: 最多 5 个字")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	org, ok := s.organizations[orgID]
	if !ok {
		return nil, transport.NotFound("organization not found")
	}
	if org.Kind != model.OrgKindEnterprise {
		return nil, transport.BadRequest("PERSONAL_ORG", "personal organization does not support profile")
	}
	org.Name = name
	org.ShortName = strings.TrimSpace(shortName)
	org.UpdatedAt = time.Now().UTC()
	if err := s.persistOrganizationUnsafe(org); err != nil {
		return nil, mongoWriteError(err)
	}
	return org, nil
}

// SetOrgLogo 设置组织 logo 的文件标识（bucket org-logos）。fileID 为空 = 清除 logo。
// 调用方（handler）负责 owner/admin 门禁。
func (s *Store) SetOrgLogo(orgID, fileID, fileName string) (*model.Organization, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	org, ok := s.organizations[orgID]
	if !ok {
		return nil, transport.NotFound("organization not found")
	}
	if org.Kind != model.OrgKindEnterprise {
		return nil, transport.BadRequest("PERSONAL_ORG", "personal organization does not support logo")
	}
	org.LogoFileID = fileID
	org.LogoFileName = fileName
	org.UpdatedAt = time.Now().UTC()
	if err := s.persistOrganizationUnsafe(org); err != nil {
		return nil, mongoWriteError(err)
	}
	return org, nil
}

// SetProjectMembers 覆盖设置项目成员白名单（空列表 = 清除，回到全员可见）。
func (s *Store) SetProjectMembers(projectID string, members []model.ProjectMember) *transport.AppError {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	next := make([]model.ProjectMember, 0, len(members))
	for _, m := range members {
		if m.UserID == "" {
			continue
		}
		m.ID = "pm_" + newID()
		m.ProjectID = projectID
		if m.Role != model.ProjectMemberRoleEditor {
			m.Role = model.ProjectMemberRoleViewer
		}
		if m.CreatedAt.IsZero() {
			m.CreatedAt = now
		}
		next = append(next, m)
	}
	if err := s.replaceProjectMembersUnsafe(projectID, next); err != nil {
		return mongoWriteError(err)
	}
	if len(next) == 0 {
		delete(s.projectMembers, projectID)
		return nil
	}
	s.projectMembers[projectID] = next
	return nil
}

// ListProjectMembers 列项目成员（空 = 未设白名单，全员可见）。
func (s *Store) ListProjectMembers(projectID string) []model.ProjectMember {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := s.projectMembers[projectID]
	if len(out) == 0 {
		return nil
	}
	cloned := append([]model.ProjectMember(nil), out...)
	return cloned
}

// EnsurePersonalOrg 保证用户拥有个人租户（阶段 1 起在注册时自动开通，存量用户由回填脚本补齐）。
func (s *Store) EnsurePersonalOrg(userID, displayName string) (*model.Organization, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ensurePersonalOrgUnsafe(userID, displayName)
}

// ensurePersonalOrgUnsafe 与 EnsurePersonalOrg 等价，但要求调用方已持有写锁，
// 供 CreateUser 等持锁路径内联调用，避免重复加锁死锁。
// personalOrgOfUnsafe 解析用户的个人租户 ID（调用方必须持锁）。
// 阶段 1：新写入的资源统一挂到作者的个人租户下，保证增量数据不再是孤儿。
// 返回空串表示用户尚无个人租户（回填前存量账号或降级场景），此时保持 org_id 为空，
// 由后续回填/补偿任务兜底，不阻断写入。
func (s *Store) personalOrgOfUnsafe(userID string) string {
	if userID == "" {
		return ""
	}
	for _, mid := range s.userOrgIndex[userID] {
		m, ok := s.orgMemberships[mid]
		if !ok {
			continue
		}
		if org, ok := s.organizations[m.OrgID]; ok && org.Kind == model.OrgKindPersonal {
			return org.ID
		}
	}
	return ""
}

func (s *Store) ensurePersonalOrgUnsafe(userID, displayName string) (*model.Organization, *transport.AppError) {
	for _, mid := range s.userOrgIndex[userID] {
		m, ok := s.orgMemberships[mid]
		if !ok {
			continue
		}
		org, ok := s.organizations[m.OrgID]
		if ok && org.Kind == model.OrgKindPersonal {
			return org, nil
		}
	}

	now := time.Now().UTC()
	org := &model.Organization{
		ID:        "org_" + newID(),
		Name:      displayName,
		Slug:      "u-" + userID,
		Kind:      model.OrgKindPersonal,
		OwnerID:   userID,
		Quota:     model.DefaultOrgQuota(),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.persistOrganizationUnsafe(org); err != nil {
		return nil, mongoWriteError(err)
	}
	s.organizations[org.ID] = org
	if _, err := s.seedBuiltinRolesUnsafe(org.ID); err != nil {
		return nil, mongoWriteError(err)
	}

	m := &model.OrgMembership{
		ID:       "om_" + newID(),
		OrgID:    org.ID,
		UserID:   userID,
		Role:     model.OrgRoleOwner,
		RoleID:   model.BuiltinOrgRoleID(org.ID, model.OrgRoleOwner),
		JoinedAt: now,
	}
	if err := s.persistMembershipUnsafe(m); err != nil {
		return nil, mongoWriteError(err)
	}
	s.orgMemberships[m.ID] = m
	s.orgMemberIndex[org.ID] = append(s.orgMemberIndex[org.ID], m.ID)
	s.userOrgIndex[userID] = append(s.userOrgIndex[userID], m.ID)
	return org, nil
}

// ---------- 内部工具（调用方必须持锁） ----------

func (s *Store) findMembershipUnsafe(orgID, userID string) (*model.OrgMembership, bool) {
	for _, mid := range s.userOrgIndex[userID] {
		m, ok := s.orgMemberships[mid]
		if ok && m.OrgID == orgID {
			return m, true
		}
	}
	return nil, false
}

func (s *Store) countOwnersUnsafe(orgID string) int {
	n := 0
	for _, mid := range s.orgMemberIndex[orgID] {
		if m, ok := s.orgMemberships[mid]; ok && m.Role == model.OrgRoleOwner {
			n++
		}
	}
	return n
}

func removeString(list []string, target string) []string {
	out := make([]string, 0, len(list))
	for _, v := range list {
		if v != target {
			out = append(out, v)
		}
	}
	return out
}

// ---------- Mongo 持久化 ----------

func (s *Store) persistOrganizationUnsafe(org *model.Organization) error {
	if !s.mongoEnabled || s.mongoOrganizations == nil || org == nil {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoOrganizations.ReplaceOne(ctx, bson.M{"_id": org.ID}, org, options.Replace().SetUpsert(true))
	return err
}

func (s *Store) persistMembershipUnsafe(m *model.OrgMembership) error {
	if !s.mongoEnabled || s.mongoOrgMemberships == nil || m == nil {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoOrgMemberships.ReplaceOne(ctx, bson.M{"_id": m.ID}, m, options.Replace().SetUpsert(true))
	return err
}

func (s *Store) deleteMembershipUnsafe(id string) error {
	if !s.mongoEnabled || s.mongoOrgMemberships == nil || id == "" {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoOrgMemberships.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (s *Store) replaceProjectMembersUnsafe(projectID string, members []model.ProjectMember) error {
	if !s.mongoEnabled || s.mongoProjectMembers == nil || projectID == "" {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	if _, err := s.mongoProjectMembers.DeleteMany(ctx, bson.M{"project_id": projectID}); err != nil {
		return err
	}
	if len(members) == 0 {
		return nil
	}
	docs := make([]any, 0, len(members))
	for i := range members {
		docs = append(docs, members[i])
	}
	_, err := s.mongoProjectMembers.InsertMany(ctx, docs)
	return err
}

func (s *Store) loadOrganizations() (map[string]*model.Organization, error) {
	items := make(map[string]*model.Organization)
	if s.mongoOrganizations == nil {
		return items, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoOrganizations.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var orgs []model.Organization
	if err := cursor.All(ctx, &orgs); err != nil {
		return nil, err
	}
	for i := range orgs {
		items[orgs[i].ID] = &orgs[i]
	}
	return items, nil
}

func (s *Store) loadOrgMemberships() (
	map[string]*model.OrgMembership,
	map[string][]string,
	map[string][]string,
	error,
) {
	items := make(map[string]*model.OrgMembership)
	byOrg := make(map[string][]string)
	byUser := make(map[string][]string)
	if s.mongoOrgMemberships == nil {
		return items, byOrg, byUser, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoOrgMemberships.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "joined_at", Value: 1}}))
	if err != nil {
		return nil, nil, nil, err
	}
	defer cursor.Close(ctx)

	var members []model.OrgMembership
	if err := cursor.All(ctx, &members); err != nil {
		return nil, nil, nil, err
	}
	for i := range members {
		m := members[i]
		items[m.ID] = &m
		byOrg[m.OrgID] = append(byOrg[m.OrgID], m.ID)
		byUser[m.UserID] = append(byUser[m.UserID], m.ID)
	}
	return items, byOrg, byUser, nil
}

func (s *Store) loadProjectMembers() (map[string][]model.ProjectMember, error) {
	items := make(map[string][]model.ProjectMember)
	if s.mongoProjectMembers == nil {
		return items, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoProjectMembers.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var members []model.ProjectMember
	if err := cursor.All(ctx, &members); err != nil {
		return nil, err
	}
	for _, m := range members {
		items[m.ProjectID] = append(items[m.ProjectID], m)
	}
	return items, nil
}
