package store

import (
	"net/url"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// CreateExternalAppInput is the payload for registering a new external platform.
type CreateExternalAppInput struct {
	Name      string `json:"name"`
	BaseURL   string `json:"base_url"`
	ClientID  string `json:"client_id"`
	SSOType   string `json:"sso_type"`
	FrameMode string `json:"frame_mode"`
	Scopes    string `json:"scopes"`
	Placement string `json:"placement"`
	IconURL   string `json:"icon_url"`
	SortOrder int    `json:"sort_order"`
	// Scope 是**请求的**作用域（org|personal），不是最终值 ——
	// 最终值由 resolveCreateScopeUnsafe 依据当前工作区裁决，global 一律拒绝
	// （全局级只能由平台侧 CreateGlobalExternalApp 创建）。
	// 空值兜底：按当前工作区派生（企业租户 → org，否则 personal），绝不放大到全局。
	Scope string `json:"scope"`
}

// UpdateExternalAppInput is the payload for updating an external platform.
type UpdateExternalAppInput struct {
	Name      *string `json:"name,omitempty"`
	BaseURL   *string `json:"base_url,omitempty"`
	SSOType   *string `json:"sso_type,omitempty"`
	FrameMode *string `json:"frame_mode,omitempty"`
	Scopes    *string `json:"scopes,omitempty"`
	Status    *string `json:"status,omitempty"`
	Placement *string `json:"placement,omitempty"`
	IconURL   *string `json:"icon_url,omitempty"`
	SortOrder *int    `json:"sort_order,omitempty"`
	// 作用域创建后不可变更：没有 Scope 字段（改层级只能重建，避免「个人级升级为全局级」越权）。
}

// ExternalAppManager 描述调用者管理权的来源：由 handler 按权限点实时解析后注入，
// store 只做数据行级裁决，不解析权限（与 authz 正交，见 scope.go 顶部说明）。
type ExternalAppManager struct {
	// OrgAppMgr 命中 org.app.mgr：可管理本组织的任意组织级应用（不限创建者）。
	OrgAppMgr bool
	// Platform 平台管理员：可管理全局级应用（/api/v1/platform/external-apps）。
	Platform bool
}

// validateBaseURL rejects anything that is not an absolute http(s) URL. Without
// this a malicious registration could make TrustMesh mint SSO tokens for
// javascript:, data: or file: targets.
func validateBaseURL(raw string) *transport.AppError {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return transport.Validation("invalid base_url", map[string]any{"base_url": "must be an absolute http(s) URL"})
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return nil
	default:
		return transport.Validation("invalid base_url", map[string]any{"base_url": "scheme must be http or https"})
	}
}

// validateFrameMode restricts the launch strategy to the supported set.
func validateFrameMode(v string) *transport.AppError {
	switch v {
	case model.ExternalFrameNewTab, model.ExternalFrameIframe:
		return nil
	default:
		return transport.Validation("invalid frame_mode", map[string]any{"frame_mode": "must be newtab|iframe"})
	}
}

// validatePlacement accepts a comma-separated set of known mount points
// (empty means the app is not mounted anywhere).
func validatePlacement(v string) *transport.AppError {
	for _, p := range strings.Split(v, ",") {
		switch strings.TrimSpace(p) {
		case "", model.ExternalPlacementSidebar, model.ExternalPlacementProjectTab:
			continue
		default:
			return transport.Validation("invalid placement", map[string]any{
				"placement": "must be a comma-separated set of sidebar|project_tab",
			})
		}
	}
	return nil
}

// resolveCreateScopeUnsafe 裁决新建应用的作用域与归属租户（调用方必须持锁）：
//   - platform=true（平台侧路径）→ 强制 global，OrgID 为空（不属于任何租户）；
//   - 请求 org → 必须是**企业租户**工作区，OrgID 取当前租户；
//   - 请求 personal → OrgID 取创建者个人租户；
//   - 请求为空 → 按当前工作区派生（企业租户 → org，否则 personal）；
//   - 请求 global（业务侧）→ 拒绝：全局级只允许平台管理员创建。
func (s *Store) resolveCreateScopeUnsafe(sc Scope, requested string, platform bool) (string, string, *transport.AppError) {
	if platform {
		return model.ExternalAppScopeGlobal, "", nil
	}
	requested = strings.TrimSpace(requested)
	if requested == "" {
		if org, ok := s.organizations[sc.OrgID]; ok && org.Kind == model.OrgKindEnterprise {
			requested = model.ExternalAppScopeOrg
		} else {
			requested = model.ExternalAppScopePersonal
		}
	}
	switch requested {
	case model.ExternalAppScopeOrg:
		org, ok := s.organizations[sc.OrgID]
		if !sc.HasOrg() || !ok || org.Kind != model.OrgKindEnterprise {
			return "", "", transport.Validation("invalid scope", map[string]any{
				"scope": "org scope requires an enterprise workspace",
			})
		}
		return model.ExternalAppScopeOrg, sc.OrgID, nil
	case model.ExternalAppScopePersonal:
		return model.ExternalAppScopePersonal, s.personalOrgOfUnsafe(sc.UserID), nil
	default:
		return "", "", transport.Validation("invalid scope", map[string]any{"scope": "must be org|personal"})
	}
}

// CreateExternalApp 业务侧注册外部平台（组织级 / 个人级）。
// client_secret 由 TrustMesh 生成，仅在创建响应里返回一次（调用方拿返回值展示）。
// 作用域由 resolveCreateScopeUnsafe 依据 in.Scope + 当前工作区裁决，业务侧永不可创建全局级。
func (s *Store) CreateExternalApp(sc Scope, in CreateExternalAppInput) (*model.ExternalAppView, string, *transport.AppError) {
	return s.createExternalApp(sc, in, false)
}

// CreateGlobalExternalApp 平台侧注册**全局级**外部平台（全员可见可打开）。
// 🔴 只能由 /api/v1/platform/external-apps 调用（平台权限点 platform.extapp.mgr 把关）。
func (s *Store) CreateGlobalExternalApp(sc Scope, in CreateExternalAppInput) (*model.ExternalAppView, string, *transport.AppError) {
	return s.createExternalApp(sc, in, true)
}

func (s *Store) createExternalApp(sc Scope, in CreateExternalAppInput, platform bool) (*model.ExternalAppView, string, *transport.AppError) {
	name := strings.TrimSpace(in.Name)
	baseURL := strings.TrimSpace(in.BaseURL)
	clientID := strings.TrimSpace(in.ClientID)
	if name == "" || baseURL == "" || clientID == "" {
		return nil, "", transport.Validation("invalid external app payload", map[string]any{
			"name":      "required",
			"base_url":  "required",
			"client_id": "required",
		})
	}
	if appErr := validateBaseURL(baseURL); appErr != nil {
		return nil, "", appErr
	}
	ssoType := strings.TrimSpace(in.SSOType)
	if ssoType == "" {
		ssoType = model.ExternalSSOTrustmeshJWT
	}
	frameMode := strings.TrimSpace(in.FrameMode)
	if frameMode == "" {
		frameMode = model.ExternalFrameNewTab
	}
	if appErr := validateFrameMode(frameMode); appErr != nil {
		return nil, "", appErr
	}
	placement := strings.TrimSpace(in.Placement)
	if appErr := validatePlacement(placement); appErr != nil {
		return nil, "", appErr
	}
	now := time.Now().UTC()
	app := &model.ExternalApp{
		ID:           newID(),
		Name:         name,
		BaseURL:      baseURL,
		ClientID:     clientID,
		ClientSecret: newID(), // 24-char random hex used as the shared HS256 secret
		SSOType:      ssoType,
		FrameMode:    frameMode,
		Scopes:       strings.TrimSpace(in.Scopes),
		Placement:    placement,
		IconURL:      strings.TrimSpace(in.IconURL),
		SortOrder:    in.SortOrder,
		Status:       model.ExternalAppStatusEnabled,
		CreatedBy:    sc.UserID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	s.mu.Lock()
	// 作用域与归属租户都在锁内裁决（需读 s.organizations / 个人租户索引）。
	scope, orgID, appErr := s.resolveCreateScopeUnsafe(sc, in.Scope, platform)
	if appErr != nil {
		s.mu.Unlock()
		return nil, "", appErr
	}
	app.Scope = scope
	app.OrgID = orgID
	s.externalApps[app.ID] = app
	if err := s.persistExternalAppUnsafe(app); err != nil {
		delete(s.externalApps, app.ID)
		s.mu.Unlock()
		return nil, "", mongoWriteError(err)
	}
	s.mu.Unlock()

	view := s.externalAppViewUnsafe(app)
	return &view, app.ClientSecret, nil
}

// externalAppScopeUnsafe 解析应用的作用域（调用方必须持锁）。
//
// 历史文档（2026-09-17 三级改造前写入）没有 scope 字段：按**归属租户**派生 ——
// 企业租户 → 组织级，个人租户或空 org_id → 个人级。存量一律不看旧的 visibility
// 字段（产品决策：原「公共」应用不再全员可见），且只读不写库，回滚无需改数据。
func (s *Store) externalAppScopeUnsafe(a *model.ExternalApp) string {
	if a == nil {
		return model.ExternalAppScopePersonal
	}
	if a.Scope != "" {
		return a.Scope
	}
	if org, ok := s.organizations[a.OrgID]; ok && org.Kind == model.OrgKindEnterprise {
		return model.ExternalAppScopeOrg
	}
	return model.ExternalAppScopePersonal
}

// externalAppVisibleUnsafe 三级可见性裁决（调用方必须持锁）：
//   - global   全员可见（平台级挂载的产品语义，不是泄漏）；
//   - org      归属租户成员可见；无租户头时退回 user 维度（创建者本人可见），
//     与 scope.go 的「无租户上下文 → user 兜底」约定一致，保证存量客户端零回归；
//   - personal 仅创建者本人可见（任意工作区，本人对自己的应用始终可见）。
//
// 不可见的应用一律按 NotFound 处理（不暴露存在性），见 Get* / Launch。
func (s *Store) externalAppVisibleUnsafe(sc Scope, a *model.ExternalApp) bool {
	if a == nil {
		return false
	}
	if sc.System {
		return true
	}
	switch s.externalAppScopeUnsafe(a) {
	case model.ExternalAppScopeGlobal:
		return true
	case model.ExternalAppScopeOrg:
		if sc.HasOrg() && a.OrgID != "" {
			return a.OrgID == sc.OrgID
		}
		// 无租户上下文（存量客户端 / 校准前的冷启动首轮）：退回 user 维度。
		return sc.UserID != "" && a.CreatedBy == sc.UserID
	default:
		return sc.UserID != "" && a.CreatedBy == sc.UserID
	}
}

// canManageExternalAppUnsafe 管理权裁决（调用方必须持锁）：
// 创建者本人 ‖（org.app.mgr 且本组织的组织级应用）‖（平台管理员且全局级应用）。
func (s *Store) canManageExternalAppUnsafe(sc Scope, a *model.ExternalApp, mgr ExternalAppManager) bool {
	if a == nil {
		return false
	}
	if sc.System {
		return true
	}
	if sc.UserID != "" && a.CreatedBy == sc.UserID {
		return true
	}
	switch s.externalAppScopeUnsafe(a) {
	case model.ExternalAppScopeGlobal:
		return mgr.Platform
	case model.ExternalAppScopeOrg:
		return mgr.OrgAppMgr && ownedByOrg(sc, a.OrgID)
	default:
		// 个人级：只有创建者本人（上面已判），组织管理员也无权改别人的个人应用。
		return false
	}
}

// externalAppViewUnsafe 输出安全视图，并补齐派生作用域 / 归属租户。
// 调用方必须持锁（Unsafe 约定，见 scope.go）。
func (s *Store) externalAppViewUnsafe(a *model.ExternalApp) model.ExternalAppView {
	v := *a.ToView()
	v.Scope = s.externalAppScopeUnsafe(a)
	return v
}

// ListExternalApps returns the external platforms visible to userID (safe
// view): 三级可见性合并结果 —— 全局级（全员）+ 本组织的组织级 + 本人创建的个人级。
// 不可见的应用一律不出现（挂载到侧边栏的应用曾因「人人可见」变成 token 签发风险）。
func (s *Store) ListExternalApps(sc Scope) []model.ExternalAppView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]model.ExternalAppView, 0, len(s.externalApps))
	for _, a := range s.externalApps {
		if !s.externalAppVisibleUnsafe(sc, a) {
			continue
		}
		items = append(items, s.externalAppViewUnsafe(a))
	}
	sortExternalAppViews(items)
	return items
}

// ListGlobalExternalApps 平台侧列表：只返回**全局级**应用（含历史文档派生的全局级）。
// 只由 /api/v1/platform/external-apps 调用（平台权限点把关）。
func (s *Store) ListGlobalExternalApps() []model.ExternalAppView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]model.ExternalAppView, 0, len(s.externalApps))
	for _, a := range s.externalApps {
		if s.externalAppScopeUnsafe(a) != model.ExternalAppScopeGlobal {
			continue
		}
		items = append(items, s.externalAppViewUnsafe(a))
	}
	sortExternalAppViews(items)
	return items
}

// sortExternalAppViews orders mounted apps by sort_order then name so the
// sidebar and project tabs render in a stable, predictable sequence.
func sortExternalAppViews(items []model.ExternalAppView) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].SortOrder != items[j].SortOrder {
			return items[i].SortOrder < items[j].SortOrder
		}
		return items[i].Name < items[j].Name
	})
}

// GetExternalApp returns a single external platform (safe view), enforcing
// the same visibility rule as List.
func (s *Store) GetExternalApp(sc Scope, id string) (*model.ExternalAppView, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.externalApps[id]
	if !ok || !s.externalAppVisibleUnsafe(sc, a) {
		return nil, transport.NotFound("external app not found")
	}
	view := s.externalAppViewUnsafe(a)
	return &view, nil
}

// GetExternalAppForLaunch resolves an app for the launch flow, enforcing
// visibility. Returns NotFound (not Forbidden) for invisible apps so the
// existence of a private registration is never disclosed.
func (s *Store) GetExternalAppForLaunch(sc Scope, id string) (*model.ExternalApp, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.externalApps[id]
	if !ok || !s.externalAppVisibleUnsafe(sc, a) {
		return nil, transport.NotFound("external app not found")
	}
	return a, nil
}

// GetExternalAppRaw returns the full record including the client_secret.
// Used internally by the launch flow (never serialized to API clients).
func (s *Store) GetExternalAppRaw(id string) (*model.ExternalApp, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.externalApps[id]
	if !ok {
		return nil, transport.NotFound("external app not found")
	}
	return a, nil
}

// UpdateExternalApp updates an existing external platform. 管理权见
// canManageExternalAppUnsafe：创建者本人、本组织的 org.app.mgr、或平台管理员（限全局级）。
// 作用域不可变更（UpdateExternalAppInput 没有 Scope 字段）。
func (s *Store) UpdateExternalApp(sc Scope, id string, in UpdateExternalAppInput, mgr ExternalAppManager) (*model.ExternalAppView, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.externalApps[id]
	if !ok {
		return nil, transport.NotFound("external app not found")
	}
	if !s.canManageExternalAppUnsafe(sc, a, mgr) {
		return nil, transport.Forbidden("not allowed to manage this external app")
	}
	if in.Name != nil {
		if n := strings.TrimSpace(*in.Name); n == "" {
			return nil, transport.Validation("invalid name", map[string]any{"name": "cannot be empty"})
		} else {
			a.Name = n
		}
	}
	if in.BaseURL != nil {
		if u := strings.TrimSpace(*in.BaseURL); u == "" {
			return nil, transport.Validation("invalid base_url", map[string]any{"base_url": "cannot be empty"})
		} else if appErr := validateBaseURL(u); appErr != nil {
			return nil, appErr
		} else {
			a.BaseURL = u
		}
	}
	if in.SSOType != nil && strings.TrimSpace(*in.SSOType) != "" {
		a.SSOType = strings.TrimSpace(*in.SSOType)
	}
	if in.FrameMode != nil && strings.TrimSpace(*in.FrameMode) != "" {
		fm := strings.TrimSpace(*in.FrameMode)
		if appErr := validateFrameMode(fm); appErr != nil {
			return nil, appErr
		}
		a.FrameMode = fm
	}
	if in.Scopes != nil {
		a.Scopes = strings.TrimSpace(*in.Scopes)
	}
	if in.Placement != nil {
		p := strings.TrimSpace(*in.Placement)
		if appErr := validatePlacement(p); appErr != nil {
			return nil, appErr
		}
		a.Placement = p
	}
	if in.IconURL != nil {
		a.IconURL = strings.TrimSpace(*in.IconURL)
	}
	if in.SortOrder != nil {
		a.SortOrder = *in.SortOrder
	}
	if in.Status != nil {
		st := strings.TrimSpace(*in.Status)
		if st != model.ExternalAppStatusEnabled && st != model.ExternalAppStatusDisabled {
			return nil, transport.Validation("invalid status", map[string]any{"status": "must be enabled|disabled"})
		}
		a.Status = st
	}
	a.UpdatedAt = time.Now().UTC()
	if err := s.persistExternalAppUnsafe(a); err != nil {
		return nil, mongoWriteError(err)
	}
	view := s.externalAppViewUnsafe(a)
	return &view, nil
}

// DeleteExternalApp disconnects (removes) an external platform. 管理权同 Update。
// Revocation is immediate: no new tokens can be issued.
func (s *Store) DeleteExternalApp(sc Scope, id string, mgr ExternalAppManager) *transport.AppError {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.externalApps[id]
	if !ok {
		return transport.NotFound("external app not found")
	}
	if !s.canManageExternalAppUnsafe(sc, a, mgr) {
		return transport.Forbidden("not allowed to manage this external app")
	}
	delete(s.externalApps, id)
	if s.mongoEnabled && s.mongoExternalApps != nil {
		ctx, cancel := s.mongoContext()
		defer cancel()
		if _, err := s.mongoExternalApps.DeleteOne(ctx, bson.M{"_id": id}); err != nil {
			return mongoWriteError(err)
		}
	}
	return nil
}

// RecordExternalAppLaunch writes an audit event for a launch action.
func (s *Store) RecordExternalAppLaunch(sc Scope, appID, appName, projectID, taskID string) {
	if !s.mongoEnabled || s.mongoEvents == nil {
		return
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	content := "launched external app: " + appName
	event := model.Event{
		ID:        newID(),
		UserID:    sc.UserID,
		ProjectID: projectID,
		TaskID:    taskID,
		ActorType: "user",
		ActorID:   sc.UserID,
		EventType: "external_app_launch",
		Content:   &content,
		Metadata: map[string]any{
			"app_id":     appID,
			"app_name":   appName,
			"project_id": projectID,
			"task_id":    taskID,
		},
		CreatedAt: time.Now().UTC(),
	}
	if _, err := s.mongoEvents.InsertOne(ctx, event); err != nil && s.log != nil {
		s.log.Warn("failed to record external app launch audit event", zap.Error(err))
	}
}

// loadExternalApps loads all external apps from mongo into the in-memory map.
func (s *Store) loadExternalApps() (map[string]*model.ExternalApp, error) {
	out := make(map[string]*model.ExternalApp)
	if !s.mongoEnabled || s.mongoExternalApps == nil {
		return out, nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	cur, err := s.mongoExternalApps.Find(ctx, bson.M{})
	if err != nil {
		return out, err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var app model.ExternalApp
		if err := cur.Decode(&app); err != nil {
			return out, err
		}
		// 历史文档没有 scope 字段：这里**不写默认值**，由 externalAppScopeUnsafe
		// 在读取时按归属租户派生（企业租户 → 组织级，其余 → 个人级），不改库。
		out[app.ID] = &app
	}
	return out, cur.Err()
}

// persistExternalAppUnsafe upserts an external app. Caller must hold s.mu.
func (s *Store) persistExternalAppUnsafe(app *model.ExternalApp) error {
	if !s.mongoEnabled || s.mongoExternalApps == nil || app == nil {
		return nil
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoExternalApps.ReplaceOne(ctx, bson.M{"_id": app.ID}, app, options.Replace().SetUpsert(true))
	return err
}
