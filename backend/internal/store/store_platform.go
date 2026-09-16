package store

import (
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// ─────────────────────────────────────────────────────────────────────────────
// 平台层存储（设计文档 §6.3 / §6.4）：
//   - 平台管理员种子账号（PLATFORM_ADMIN_EMAILS）
//   - 平台全局配置（platform_settings 单文档 _id=global）
//   - 企业生命周期（列表/状态）+ 平台用量聚合
//
// 平台层只碰"元数据"，不读企业业务内容（用量为 count 级聚合）。
// ─────────────────────────────────────────────────────────────────────────────

// ---------- 平台管理员种子 ----------

// UserIsPlatformAdmin 判断用户是否平台管理员（标记来源见 SeedPlatformAdmins）。
func (s *Store) UserIsPlatformAdmin(userID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[userID]
	return ok && u.IsAdmin
}

// SeedPlatformAdmins 按 PLATFORM_ADMIN_EMAILS 同步平台管理员标记（启动时调用一次）。
//
// emails 非空 = 种子模式：账号集合以 env 为唯一准绳 —— 命中的账号置 IsAdmin=true，
// 未命中的撤销（含历史自动提升的账号），界面不可授予/撤销（设计文档 §0/§6.3）。
// 因业务 API 在种子模式下对平台管理员反向拒绝，撤销旧管理员时显式告警，
// 避免运维在没给旧账号配普通身份的情况下被锁在业务之外。
//
// emails 为空 = 未配置 env：保持历史行为（首个注册用户自动提升为平台管理员，
// 业务 API 不做反向拒绝），保证存量部署零行为变化。
func (s *Store) SeedPlatformAdmins(emails []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(emails) == 0 {
		s.platformAdminEmails = make(map[string]bool)
		if s.log != nil {
			s.log.Info("PLATFORM_ADMIN_EMAILS not set: platform admin falls back to legacy first-user promotion")
		}
		s.promoteFirstAdminUnsafe()
		return
	}

	seed := make(map[string]bool, len(emails))
	for _, e := range emails {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			seed[e] = true
		}
	}
	s.platformAdminEmails = seed

	now := time.Now().UTC()
	granted, revoked := 0, 0
	for _, u := range s.users {
		want := seed[u.Email]
		if u.IsAdmin == want {
			continue
		}
		u.IsAdmin = want
		u.UpdatedAt = now
		if err := s.persistUserUnsafe(u); err != nil && s.log != nil {
			s.log.Warn("sync platform admin flag failed", zap.String("user_id", u.ID), zap.Error(err))
		}
		if want {
			granted++
		} else {
			revoked++
			if s.log != nil {
				s.log.Warn("platform admin revoked by seed list (business API will now reject this account)",
					zap.String("user_id", u.ID), zap.String("email", u.Email))
			}
		}
	}
	if s.log != nil {
		s.log.Info("platform admins seeded from env",
			zap.Int("seeded", len(seed)), zap.Int("granted", granted), zap.Int("revoked", revoked))
	}
}

// PlatformAdminSeedActive 报告是否处于 env 种子模式。
// 种子模式下业务 API 对平台管理员反向拒绝（authz.RequireBusinessAccount）。
func (s *Store) PlatformAdminSeedActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.platformAdminEmails) > 0
}

// IsPlatformAdminEmail 报告某邮箱是否在种子白名单内（注册时直接打标，免重启）。
func (s *Store) IsPlatformAdminEmail(email string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.platformAdminEmails[strings.ToLower(strings.TrimSpace(email))]
}

// promoteFirstAdminUnsafe 历史兜底：没有任何平台管理员时，把最早注册的用户提升为管理员。
// 仅在未配置 PLATFORM_ADMIN_EMAILS 时启用（幂等）。调用方须持写锁。
func (s *Store) promoteFirstAdminUnsafe() {
	if len(s.users) == 0 {
		return
	}
	for _, u := range s.users {
		if u.IsAdmin {
			return
		}
	}
	oldest := (*model.User)(nil)
	for _, u := range s.users {
		if oldest == nil || u.CreatedAt.Before(oldest.CreatedAt) {
			oldest = u
		}
	}
	oldest.IsAdmin = true
	oldest.UpdatedAt = time.Now().UTC()
	if err := s.persistUserUnsafe(oldest); err != nil && s.log != nil {
		s.log.Warn("promote first admin failed", zap.Error(err))
	}
	if s.log != nil {
		s.log.Info("promoted first platform admin", zap.String("user_id", oldest.ID), zap.String("email", oldest.Email))
	}
}

// ---------- 平台全局配置（单文档） ----------

// GetPlatformGlobalConfig 返回全局配置；未落库时返回默认值（全部不限）。
func (s *Store) GetPlatformGlobalConfig() *model.PlatformGlobalConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.platformGlobalCfg == nil {
		return model.DefaultPlatformGlobalConfig()
	}
	return clonePlatformGlobalConfig(s.platformGlobalCfg)
}

// SetPlatformGlobalConfig 覆盖全局配置（幂等 upsert）。
func (s *Store) SetPlatformGlobalConfig(setterID string, in model.PlatformGlobalConfig) (*model.PlatformGlobalConfig, *transport.AppError) {
	in.DefaultModel = strings.TrimSpace(in.DefaultModel)
	if len(in.DefaultModel) > 128 {
		return nil, transport.Validation("default_model: at most 128 chars", nil)
	}
	if appErr := validateQuota(in.Quota); appErr != nil {
		return nil, appErr
	}
	params, appErr := normalizeNodeParameters(in.NodeParameters)
	if appErr != nil {
		return nil, appErr
	}
	// 全局菜单基线与 org.menu_overrides 同规则（去空白/去重/上限），复用同一份规范化。
	hiddenMenus, appErr := normalizeMenuOverrides(in.HiddenMenus)
	if appErr != nil {
		return nil, appErr
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := &model.PlatformGlobalConfig{
		ID:             model.PlatformGlobalConfigID,
		DefaultModel:   in.DefaultModel,
		Quota:          in.Quota,
		NodeParameters: params,
		HiddenMenus:    hiddenMenus,
		UpdatedAt:      time.Now().UTC(),
		UpdatedBy:      setterID,
	}
	if err := s.persistPlatformGlobalConfigUnsafe(cfg); err != nil {
		return nil, mongoWriteError(err)
	}
	s.platformGlobalCfg = cfg
	return clonePlatformGlobalConfig(cfg), nil
}

// loadPlatformGlobalConfig 启动时装载单文档全局配置（不存在返回默认值）。
func (s *Store) loadPlatformGlobalConfig() (*model.PlatformGlobalConfig, error) {
	if !s.mongoEnabled || s.mongoLLMSettings == nil {
		return nil, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	var doc model.PlatformGlobalConfig
	err := s.mongoLLMSettings.FindOne(ctx, bson.M{"_id": model.PlatformGlobalConfigID}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &doc, nil
}

func (s *Store) persistPlatformGlobalConfigUnsafe(cfg *model.PlatformGlobalConfig) error {
	if !s.mongoEnabled || s.mongoLLMSettings == nil || cfg == nil {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoLLMSettings.ReplaceOne(ctx, bson.M{"_id": model.PlatformGlobalConfigID}, cfg, options.Replace().SetUpsert(true))
	return err
}

// quotaForNewOrgUnsafe 新建租户使用的默认配额（调用方须持锁）。
// 企业租户取平台全局配置的默认配额；个人租户恒不限（全局配置只管企业侧，
// 见设计文档 §6.4 全局配置用途）。未配置全局配置时与历史默认一致（全部不限）。
func (s *Store) quotaForNewOrgUnsafe(kind string) model.OrgQuota {
	if kind != model.OrgKindEnterprise || s.platformGlobalCfg == nil {
		return model.DefaultOrgQuota()
	}
	return s.platformGlobalCfg.Quota
}

func clonePlatformGlobalConfig(cfg *model.PlatformGlobalConfig) *model.PlatformGlobalConfig {
	out := *cfg
	if len(cfg.NodeParameters) > 0 {
		out.NodeParameters = make(map[string]string, len(cfg.NodeParameters))
		for k, v := range cfg.NodeParameters {
			out.NodeParameters[k] = v
		}
	}
	// HiddenMenus 是切片：不深拷贝会让调用方经返回值改写 store 内部状态。
	if len(cfg.HiddenMenus) > 0 {
		out.HiddenMenus = append([]string(nil), cfg.HiddenMenus...)
	}
	return &out
}

// PlatformHiddenMenus 返回平台全局菜单基线（未落库配置时为空）。
// 供 /users/me 与企业级 menu_overrides 合并下发；返回副本，调用方改动不回流。
func (s *Store) PlatformHiddenMenus() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.platformGlobalCfg == nil || len(s.platformGlobalCfg.HiddenMenus) == 0 {
		return nil
	}
	return append([]string(nil), s.platformGlobalCfg.HiddenMenus...)
}

func validateQuota(q model.OrgQuota) *transport.AppError {
	fields := map[string]int{
		"quota.max_members": q.MaxMembers, "quota.max_nodes": q.MaxNodes,
		"quota.max_projects": q.MaxProjects,
	}
	for name, v := range fields {
		if v < -1 {
			return transport.Validation(name+": must be -1 (unlimited) or a non-negative integer", nil)
		}
	}
	if q.MaxStorageBytes < -1 {
		return transport.Validation("quota.max_storage_bytes: must be -1 (unlimited) or a non-negative integer", nil)
	}
	return nil
}

// normalizeNodeParameters 规范化节点参数（去空白、丢弃空键、上限 20 项、键值 ≤64 字符）。
func normalizeNodeParameters(in map[string]string) (map[string]string, *transport.AppError) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" {
			continue
		}
		if len(k) > 64 || len(v) > 64 {
			return nil, transport.Validation("node_parameters: key and value must be at most 64 chars", map[string]any{"key": k})
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil, nil
	}
	if len(out) > 20 {
		return nil, transport.Validation("node_parameters: at most 20 entries", nil)
	}
	return out, nil
}

// ---------- 企业生命周期（平台侧） ----------

// ListEnterpriseOrganizations 列出全部企业租户（按创建时间升序），个人租户不入列。
func (s *Store) ListEnterpriseOrganizations() []*model.Organization {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*model.Organization, 0, len(s.organizations))
	for _, org := range s.organizations {
		if org.Kind == model.OrgKindEnterprise {
			out = append(out, org)
		}
	}
	sortOrgsByCreatedAt(out)
	return out
}

// SetOrganizationStatus 切换企业租户状态（active | disabled）。仅企业租户可切换。
func (s *Store) SetOrganizationStatus(orgID, status string) (*model.Organization, *transport.AppError) {
	if status != model.OrgStatusActive && status != model.OrgStatusDisabled {
		return nil, transport.Validation("status: must be active or disabled", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	org, ok := s.organizations[orgID]
	if !ok {
		return nil, transport.NotFound("organization not found")
	}
	if org.Kind != model.OrgKindEnterprise {
		return nil, transport.BadRequest("PERSONAL_ORG", "personal organization cannot be disabled")
	}
	if org.StatusOrActive() == status {
		return org, nil // 幂等：无变化不写库、不落审计（handler 侧据返回判断）
	}
	org.Status = status
	org.UpdatedAt = time.Now().UTC()
	if err := s.persistOrganizationUnsafe(org); err != nil {
		return nil, mongoWriteError(err)
	}
	return org, nil
}

// SetOrgMenuOverrides 覆盖设置企业菜单隐藏项（只能缩小，设计文档 §4）。
// 仅企业租户可设置；传空列表 = 清除覆盖。
func (s *Store) SetOrgMenuOverrides(orgID string, overrides []string) (*model.Organization, *transport.AppError) {
	clean, appErr := normalizeMenuOverrides(overrides)
	if appErr != nil {
		return nil, appErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	org, ok := s.organizations[orgID]
	if !ok {
		return nil, transport.NotFound("organization not found")
	}
	if org.Kind != model.OrgKindEnterprise {
		return nil, transport.BadRequest("PERSONAL_ORG", "personal organization does not support menu overrides")
	}
	org.MenuOverrides = clean
	org.UpdatedAt = time.Now().UTC()
	if err := s.persistOrganizationUnsafe(org); err != nil {
		return nil, mongoWriteError(err)
	}
	return org, nil
}

// OrgMenuOverrides 读某租户的菜单隐藏项（个人租户/不存在返回 nil）。
func (s *Store) OrgMenuOverrides(orgID string) []string {
	if orgID == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	org, ok := s.organizations[orgID]
	if !ok || len(org.MenuOverrides) == 0 {
		return nil
	}
	return append([]string(nil), org.MenuOverrides...)
}

// OrgResourceCounts 返回租户的元数据计数（成员数/项目数/任务数），不读业务内容。
func (s *Store) OrgResourceCounts(orgID string) (members, projects, tasks int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	members = len(s.orgMemberIndex[orgID])
	for _, p := range s.projects {
		if p.OrgID == orgID {
			projects++
		}
	}
	for _, t := range s.tasks {
		if t.OrgID == orgID {
			tasks++
		}
	}
	return members, projects, tasks
}

// PlatformUsageStats 全平台用量聚合（count 级，不返回任何业务内容）。
func (s *Store) PlatformUsageStats() model.PlatformUsage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var u model.PlatformUsage
	u.OrgsTotal = len(s.organizations)
	for _, org := range s.organizations {
		if org.Kind == model.OrgKindEnterprise {
			u.OrgsEnterprise++
		}
		if org.IsDisabled() {
			u.OrgsDisabled++
		}
	}
	u.Users = len(s.users)
	u.Agents = len(s.agents)
	u.Projects = len(s.projects)
	u.Tasks = len(s.tasks)
	for _, f := range s.projectFiles {
		if f.FileSize > 0 {
			u.StorageBytes += f.FileSize
		}
	}
	return u
}

func sortOrgsByCreatedAt(orgs []*model.Organization) {
	for i := 1; i < len(orgs); i++ {
		for j := i; j > 0 && orgs[j].CreatedAt.Before(orgs[j-1].CreatedAt); j-- {
			orgs[j], orgs[j-1] = orgs[j-1], orgs[j]
		}
	}
}

// ---------- 平台侧用户管理 ----------

// ListAllUsers 列出全部账号（按注册时间升序），返回只读副本。
// 平台视角的账号清单：不区分租户归属（归属关系由 handler 用 userOrgIndex 组合）。
func (s *Store) ListAllUsers() []*model.User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*model.User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, copyUser(u))
	}
	sortUsersByCreatedAt(out)
	return out
}

// SetUserDisabled 切换账号的禁用状态（幂等：无变化不写库，handler 据此决定是否落审计）。
//
// 平台管理员账号（IsAdmin）拒绝操作：其账号集合以 env PLATFORM_ADMIN_EMAILS 为唯一权威
// （见 SeedPlatformAdmins），控制台改不了 —— 禁用会造成 env 与库长期不一致，且重启后
// 种子逻辑只同步 IsAdmin 标记、不会把账号重新启用。
func (s *Store) SetUserDisabled(userID string, disabled bool) (*model.User, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	u, ok := s.users[userID]
	if !ok {
		return nil, transport.NotFound("user not found")
	}
	if u.IsAdmin {
		return nil, transport.Forbidden("platform admin accounts are managed by the deployment env seed")
	}
	if u.Disabled == disabled {
		return copyUser(u), nil
	}

	now := time.Now().UTC()
	u.Disabled = disabled
	u.UpdatedAt = now
	// 字段级落库（不用整文档 ReplaceOne）：本实例内存可能落后于其它实例，整文档写会把
	// 它们刚写的字段覆盖回旧值 —— 禁用这种安全控制必须免疫跨实例覆盖。
	set := bson.M{"disabled": disabled, "updated_at": now}
	unset := bson.M(nil)
	if disabled {
		u.DisabledAt = &now
		set["disabled_at"] = now
	} else {
		u.DisabledAt = nil
		unset = bson.M{"disabled_at": ""}
	}
	if err := s.persistUserFieldsUnsafe(u.ID, set, unset); err != nil {
		return nil, mongoWriteError(err)
	}
	return copyUser(u), nil
}

func sortUsersByCreatedAt(users []*model.User) {
	for i := 1; i < len(users); i++ {
		for j := i; j > 0 && users[j].CreatedAt.Before(users[j-1].CreatedAt); j-- {
			users[j], users[j-1] = users[j-1], users[j]
		}
	}
}

// normalizeMenuOverrides 规范化菜单隐藏项：去空白、去重、上限 50 项、单键 ≤64 字符。
func normalizeMenuOverrides(in []string) ([]string, *transport.AppError) {
	if len(in) == 0 {
		return nil, nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, key := range in {
		key = strings.TrimSpace(key)
		if key == "" || seen[key] {
			continue
		}
		if len(key) > 64 {
			return nil, transport.Validation("menu_overrides: each key must be at most 64 chars", map[string]any{"key": key})
		}
		seen[key] = true
		out = append(out, key)
	}
	if len(out) > 50 {
		return nil, transport.Validation("menu_overrides: at most 50 keys", nil)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
