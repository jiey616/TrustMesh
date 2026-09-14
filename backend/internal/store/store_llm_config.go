package store

import (
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// ─────────────────────────────────────────────────────────────────────────────
// LLM 配置存储（A1+B2+C1+D1）：平台默认 + 每租户覆盖，内存镜像 + Mongo 持久化。
//
// 🔴 解析优先级：租户配置 > 平台默认 > env 兜底（SetLLMEnvDefaults 注入）。
// 🔴 热生效：ResolveLLMParams 每次调用实时解析，保存即生效，无需重启。
// 🔴 权限：平台默认仅 IsAdmin 可改；租户覆盖仅该 org 的 owner/admin 可改
//   （权限判定在 handler 层，store 提供判定 helper）。
// ─────────────────────────────────────────────────────────────────────────────

// SetLLMEnvDefaults 注入 env 兜底配置（bootstrap 时调用一次）。
// 后续即使在 UI 建了平台默认配置，删除后仍回落到这套 env 值。
func (s *Store) SetLLMEnvDefaults(apiURL, apiKey, model string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.llmEnvURL = apiURL
	s.llmEnvKey = apiKey
	s.llmEnvModel = model
}

// UserIsPlatformAdmin 判断用户是否平台管理员。
func (s *Store) UserIsPlatformAdmin(userID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[userID]
	return ok && u.IsAdmin
}

// PersonalOrgIDOf 返回用户的个人租户 ID（无则空串）。
func (s *Store) PersonalOrgIDOf(userID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.personalOrgOfUnsafe(userID)
}

// UserIsOrgAdmin 判断用户是否某租户的 owner/admin（org 成员角色）。
func (s *Store) UserIsOrgAdmin(userID, orgID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.userIsOrgAdminLocked(userID, orgID)
}

func (s *Store) userIsOrgAdminLocked(userID, orgID string) bool {
	for _, mid := range s.userOrgIndex[userID] {
		m, ok := s.orgMemberships[mid]
		if ok && m.OrgID == orgID && (m.Role == "owner" || m.Role == "admin") {
			return true
		}
	}
	return false
}

// EnsurePlatformAdminExists 兜底：存量部署没有任何 admin 时，
// 把最早注册的用户提升为平台管理员（幂等，启动时调用一次）。
func (s *Store) EnsurePlatformAdminExists() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.IsAdmin {
			return
		}
	}
	if len(s.users) == 0 {
		return
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

// resolveLLMLayeredUnsafe 按优先级解析生效配置。orgID 为空 = 个人空间请求，
// 直接走平台默认 → env。
func (s *Store) resolveLLMLayeredUnsafe(orgID string) (url, key, opsModel, source string) {
	if orgID != "" {
		if cfg := s.llmConfigs[orgID]; cfg != nil && cfg.APIURL != "" && cfg.APIKey != "" {
			om := cfg.OpsModel
			if om == "" {
				om = cfg.Model
			}
			return cfg.APIURL, cfg.APIKey, om, model.LLMSourceOrg
		}
	}
	if cfg := s.llmConfigs[""]; cfg != nil && cfg.APIURL != "" && cfg.APIKey != "" {
		om := cfg.OpsModel
		if om == "" {
			om = cfg.Model
		}
		return cfg.APIURL, cfg.APIKey, om, model.LLMSourcePlatform
	}
	return s.llmEnvURL, s.llmEnvKey, s.llmEnvModel, model.LLMSourceEnv
}

// ResolveLLMParams 解析某租户（或个人空间）的生效 LLM 参数。
// 供 assistant.LLMProvider 每次调用时取配置（C1 热生效核心）。
// 个人空间（orgID 空）按 userID 定位其个人租户键解析：个人配置 > 平台默认 > env。
func (s *Store) ResolveLLMParams(orgID, userID string) (url, key, chatModel, opsModel, source string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	personal := false
	if orgID == "" && userID != "" {
		orgID = s.personalOrgOfUnsafe(userID)
		personal = orgID != ""
	}
	url, key, opsModel, source = s.resolveLLMLayeredUnsafe(orgID)
	// 个人空间命中的是个人租户键（存储上与 org 层同构），来源标记纠正为 personal。
	if personal && source == model.LLMSourceOrg {
		source = model.LLMSourcePersonal
	}
	chatModel = opsModel // chat 与归因同模型；ops_model 是归因的显式覆盖
	return url, key, chatModel, opsModel, source
}

// llmMaskedKey 掩码：sk-ab****XY → 前 5 位 + **** + 末 4 位；过短只留前缀。
func llmMaskedKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 9 {
		return key[:3] + "***"
	}
	return key[:5] + "***" + key[len(key)-4:]
}

// GetPlatformLLMConfigView 平台默认层视图（未配置时展示 env 兜底值并标注 source）。
func (s *Store) GetPlatformLLMConfigView() *model.LLMConfigView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if cfg := s.llmConfigs[""]; cfg != nil {
		return &model.LLMConfigView{
			OrgID:        "",
			APIURL:       cfg.APIURL,
			APIKeyMasked: llmMaskedKey(cfg.APIKey),
			Model:        cfg.Model,
			OpsModel:     cfg.OpsModel,
			Source:       model.LLMSourcePlatform,
			HasOverride:  true,
			UpdatedAt:    cfg.UpdatedAt,
			UpdatedBy:    cfg.UpdatedBy,
		}
	}
	return &model.LLMConfigView{
		OrgID:        "",
		APIURL:       s.llmEnvURL,
		APIKeyMasked: llmMaskedKey(s.llmEnvKey),
		Model:        s.llmEnvModel,
		Source:       model.LLMSourceEnv,
		HasOverride:  false,
	}
}

// GetOrgLLMConfigView 租户层视图（未配置时回落展示生效值并标注来源）。
func (s *Store) GetOrgLLMConfigView(orgID string) *model.LLMConfigView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getOrgLLMConfigViewLocked(orgID)
}

// getOrgLLMConfigViewLocked 要求持锁。
func (s *Store) getOrgLLMConfigViewLocked(orgID string) *model.LLMConfigView {
	if cfg := s.llmConfigs[orgID]; cfg != nil {
		return &model.LLMConfigView{
			OrgID:        orgID,
			APIURL:       cfg.APIURL,
			APIKeyMasked: llmMaskedKey(cfg.APIKey),
			Model:        cfg.Model,
			OpsModel:     cfg.OpsModel,
			Source:       model.LLMSourceOrg,
			HasOverride:  true,
			UpdatedAt:    cfg.UpdatedAt,
			UpdatedBy:    cfg.UpdatedBy,
		}
	}
	url, _, _, source := s.resolveLLMLayeredUnsafe(orgID)
	// 回落展示：把生效链上的值带出来，让用户知道"当前实际在用什么"
	if p := s.llmConfigs[""]; p != nil {
		return &model.LLMConfigView{
			OrgID:        orgID,
			APIURL:       p.APIURL,
			APIKeyMasked: llmMaskedKey(p.APIKey),
			Model:        p.Model,
			Source:       model.LLMSourcePlatform,
			HasOverride:  false,
		}
	}
	return &model.LLMConfigView{
		OrgID:        orgID,
		APIURL:       url,
		APIKeyMasked: llmMaskedKey(s.llmEnvKey),
		Model:        s.llmEnvModel,
		Source:       source,
		HasOverride:  false,
	}
}

// GetPersonalLLMConfigView 个人空间视图：配置挂在用户个人租户键上。
// 未配置时回落展示平台默认 / env 生效值。
func (s *Store) GetPersonalLLMConfigView(userID string) *model.LLMConfigView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pid := s.personalOrgOfUnsafe(userID)
	if cfg := s.llmConfigs[pid]; cfg != nil && pid != "" {
		return &model.LLMConfigView{
			OrgID:        pid,
			APIURL:       cfg.APIURL,
			APIKeyMasked: llmMaskedKey(cfg.APIKey),
			Model:        cfg.Model,
			OpsModel:     cfg.OpsModel,
			Source:       model.LLMSourcePersonal,
			HasOverride:  true,
			UpdatedAt:    cfg.UpdatedAt,
			UpdatedBy:    cfg.UpdatedBy,
		}
	}
	v := s.getOrgLLMConfigViewLocked(pid)
	if v != nil {
		v.OrgID = "" // 个人层视图不外泄内部键
	}
	return v
}

// SetPersonalLLMSetting 保存个人空间配置（幂等 upsert，任何登录用户可配自己的）。
func (s *Store) SetPersonalLLMSetting(userID, setterID string, in model.LLMConfigInput) (*model.LLMConfigView, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pid := s.personalOrgOfUnsafe(userID)
	if pid == "" {
		return nil, transport.Validation("personal org not found for user", nil)
	}
	view, appErr := s.setLLMSettingLocked(pid, setterID, in)
	if view != nil {
		view.Source = model.LLMSourcePersonal
	}
	return view, appErr
}

// SetPlatformLLMSetting 保存平台默认配置（幂等 upsert）。
func (s *Store) SetPlatformLLMSetting(setterID string, in model.LLMConfigInput) (*model.LLMConfigView, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.setLLMSettingLocked("", setterID, in)
}

// SetOrgLLMSetting 保存租户覆盖配置。
func (s *Store) SetOrgLLMSetting(orgID, setterID string, in model.LLMConfigInput) (*model.LLMConfigView, *transport.AppError) {
	if orgID == "" {
		return nil, transport.Validation("org_id required", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.setLLMSettingLocked(orgID, setterID, in)
}

func (s *Store) setLLMSettingLocked(orgID, setterID string, in model.LLMConfigInput) (*model.LLMConfigView, *transport.AppError) {
	in.APIURL = strings.TrimSpace(in.APIURL)
	in.Model = strings.TrimSpace(in.Model)
	in.OpsModel = strings.TrimSpace(in.OpsModel)
	// 完整性：三层字段要么都有、要么该层视为无效——这里要求 url+model 必填，
	// key 允许留空（保持已有）。
	if in.APIURL == "" || in.Model == "" {
		return nil, transport.Validation("api_url and model are required", map[string]any{
			"api_url": "required", "model": "required",
		})
	}
	cfg := s.llmConfigs[orgID]
	if cfg == nil {
		cfg = &model.PlatformLLMSetting{OrgID: orgID}
	}
	if in.APIKey != "" {
		cfg.APIKey = in.APIKey
	} else if in.ResetAPIKey {
		cfg.APIKey = ""
	}
	cfg.APIURL = in.APIURL
	cfg.Model = in.Model
	cfg.OpsModel = in.OpsModel
	cfg.UpdatedAt = time.Now().UTC()
	cfg.UpdatedBy = setterID

	// write-only 红线：url+model 齐全但 key 为空 = 配置不完整，不允许保存成
	// 「看似生效实则回退」的状态（避免用户误以为已生效）。
	if cfg.APIKey == "" {
		return nil, transport.Validation("api_key required (empty = keep existing, but this layer has no key yet)", nil)
	}

	s.llmConfigs[orgID] = cfg
	if err := s.persistLLMSettingUnsafe(cfg); err != nil && s.log != nil {
		s.log.Warn("persist llm setting failed", zap.Error(err))
	}
	return s.llmConfigViewLocked(orgID), nil
}

// ClearLLMSetting 删除某层配置（orgID 空 = 删平台默认），回到回退链。
func (s *Store) ClearLLMSetting(orgID string) *transport.AppError {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.llmConfigs[orgID]; !ok {
		return nil // 幂等
	}
	delete(s.llmConfigs, orgID)
	if s.mongoLLMSettings != nil {
		ctx, cancel := s.mongoContext()
		defer cancel()
		if _, err := s.mongoLLMSettings.DeleteOne(ctx, map[string]any{"org_id": orgID}); err != nil && s.log != nil {
			s.log.Warn("delete llm setting failed", zap.Error(err))
		}
	}
	return nil
}

func (s *Store) llmConfigViewLocked(orgID string) *model.LLMConfigView {
	cfg := s.llmConfigs[orgID]
	if cfg == nil {
		return nil
	}
	return &model.LLMConfigView{
		OrgID:        cfg.OrgID,
		APIURL:       cfg.APIURL,
		APIKeyMasked: llmMaskedKey(cfg.APIKey),
		Model:        cfg.Model,
		OpsModel:     cfg.OpsModel,
		Source:       llmLayerSource(orgID),
		HasOverride:  true,
		UpdatedAt:    cfg.UpdatedAt,
		UpdatedBy:    cfg.UpdatedBy,
	}
}

// llmLayerSource 按层返回来源标记（"" = 平台默认层）。
func llmLayerSource(orgID string) string {
	if orgID == "" {
		return model.LLMSourcePlatform
	}
	return model.LLMSourceOrg
}

// loadLLMConfigs 启动时从 Mongo 装载全部 LLM 配置（含平台默认单例）。
func (s *Store) loadLLMConfigs() (map[string]*model.PlatformLLMSetting, error) {
	out := make(map[string]*model.PlatformLLMSetting)
	if s.mongoLLMSettings == nil {
		return out, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cursor, err := s.mongoLLMSettings.Find(ctx, map[string]any{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var doc model.PlatformLLMSetting
		if err := cursor.Decode(&doc); err != nil {
			continue
		}
		out[doc.OrgID] = &doc
	}
	return out, cursor.Err()
}

func (s *Store) persistLLMSettingUnsafe(cfg *model.PlatformLLMSetting) error {
	if s.mongoLLMSettings == nil {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoLLMSettings.ReplaceOne(ctx, map[string]any{"org_id": cfg.OrgID}, cfg, options.Replace().SetUpsert(true))
	return err
}

// llmSortedOrgIDs 仅测试/调试用：返回配置覆盖的 orgID 列表（稳定排序）。
func (s *Store) llmSortedOrgIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.llmConfigs))
	for id := range s.llmConfigs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
