package handler

import (
	"errors"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/auth"
	"trustmesh/backend/internal/authz"
	"trustmesh/backend/internal/middleware"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

// ExternalAppHandler exposes the SSO "connect external platform" registry and
// the launch endpoint that mints a short-lived SSO token for an external app.
//
// 三级作用域（2026-09-17）后，「能否管理」取决于**应用的作用域**（数据相关），
// 路由级权限点表达不了，因此业务侧路由保持基础权限，由 handler + store 裁决：
//   - 组织级：要求 org.app.mgr（owner/admin / 勾选了该点的自定义角色）；
//   - 个人级：仅创建者本人；
//   - 全局级：只能走平台命名空间（见 platform_external_app.go）。
type ExternalAppHandler struct {
	store    *store.Store
	az       *authz.Authorizer
	issuer   string
	tokenTTL time.Duration
}

func NewExternalAppHandler(s *store.Store, az *authz.Authorizer, issuer string, tokenTTL time.Duration) *ExternalAppHandler {
	return &ExternalAppHandler{store: s, az: az, issuer: issuer, tokenTTL: tokenTTL}
}

// hasPerm 实时解析权限（禁止跨请求缓存，T3.1 多实例下角色变更须即时生效）。
// az 未注入（单测直接构造 handler）时默认拒绝。
func (h *ExternalAppHandler) hasPerm(sc store.Scope, perm string) bool {
	if h.az == nil {
		return false
	}
	return authz.Has(h.az.Resolve(sc), perm)
}

// managerOf 解析调用者的外部应用管理权来源（供 store 做数据行级裁决）。
func (h *ExternalAppHandler) managerOf(c *gin.Context) store.ExternalAppManager {
	return store.ExternalAppManager{
		OrgAppMgr: h.hasPerm(middleware.Scope(c), authz.PermOrgAppMgr),
	}
}

// wantsOrgScope 判断本次创建是否会落到**组织级**：
// 显式 scope=org，或 scope 为空时当前工作区是企业租户（store 会派生为组织级）。
// 后者必须一并把关，否则 member 只要省略 scope 就能借企业工作区创建组织级应用。
func (h *ExternalAppHandler) wantsOrgScope(sc store.Scope, requested string) bool {
	switch strings.TrimSpace(requested) {
	case model.ExternalAppScopeOrg:
		return true
	case model.ExternalAppScopePersonal, model.ExternalAppScopeGlobal:
		return false
	}
	if !sc.HasOrg() {
		return false
	}
	org, appErr := h.store.GetOrganization(sc.OrgID)
	return appErr == nil && org != nil && org.Kind == model.OrgKindEnterprise
}

type createExternalAppRequest struct {
	Name      string `json:"name"`
	BaseURL   string `json:"base_url"`
	ClientID  string `json:"client_id"`
	SSOType   string `json:"sso_type"`
	FrameMode string `json:"frame_mode"`
	Scopes    string `json:"scopes"`
	Placement string `json:"placement"`
	IconURL   string `json:"icon_url"`
	SortOrder int    `json:"sort_order"`
	// Scope 是请求的作用域（org|personal）；global 由本接口一律拒绝（只有平台侧能建全局级）。
	// 空值由 store 按当前工作区派生。
	Scope string `json:"scope"`
}

// Create registers a new external platform. Returns the safe view plus the
// generated client_secret (shown once — copy it into the external platform's
// SSO config to verify TrustMesh-issued tokens).
func (h *ExternalAppHandler) Create(c *gin.Context) {
	sc := middleware.Scope(c)
	var req createExternalAppRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	// 组织级应用的创建仅 owner/admin（org.app.mgr）；个人级人人可为（个人空间即本人所有）。
	// 必须在 store 裁决作用域之前拒绝，避免 member 借企业工作区创建组织级应用。
	if h.wantsOrgScope(sc, req.Scope) && !h.hasPerm(sc, authz.PermOrgAppMgr) {
		transport.WriteError(c, transport.Forbidden("missing permission: "+authz.PermOrgAppMgr))
		return
	}
	view, secret, appErr := h.store.CreateExternalApp(sc, store.CreateExternalAppInput{
		Name:      req.Name,
		BaseURL:   req.BaseURL,
		ClientID:  req.ClientID,
		SSOType:   req.SSOType,
		FrameMode: req.FrameMode,
		Scopes:    req.Scopes,
		Placement: req.Placement,
		IconURL:   req.IconURL,
		SortOrder: req.SortOrder,
		Scope:     req.Scope,
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, 201, gin.H{
		"external_app": view,
		// client_secret is returned exactly once; TrustMesh does not store it
		// for display again.
		"client_secret": secret,
	})
}

// List returns the external platforms visible to the caller (safe view, no
// secret): 全局级全员可见 + 本组织级（同组织成员）+ 本人创建的个人级。
func (h *ExternalAppHandler) List(c *gin.Context) {
	sc := middleware.Scope(c)
	items := h.store.ListExternalApps(sc)
	transport.WriteData(c, 200, gin.H{"external_apps": items})
}

// Get returns a single external platform (safe view), only if visible to the
// caller.
func (h *ExternalAppHandler) Get(c *gin.Context) {
	sc := middleware.Scope(c)
	id := c.Param("id")
	view, appErr := h.store.GetExternalApp(sc, id)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, 200, gin.H{"external_app": view})
}

type updateExternalAppRequest struct {
	Name      *string `json:"name,omitempty"`
	BaseURL   *string `json:"base_url,omitempty"`
	SSOType   *string `json:"sso_type,omitempty"`
	FrameMode *string `json:"frame_mode,omitempty"`
	Scopes    *string `json:"scopes,omitempty"`
	Status    *string `json:"status,omitempty"`
	Placement *string `json:"placement,omitempty"`
	IconURL   *string `json:"icon_url,omitempty"`
	SortOrder *int    `json:"sort_order,omitempty"`
	// 无 scope/visibility 字段：作用域创建后不可变更（改层级只能重建）。
}

// Update modifies an external platform. 管理权由 store 按应用作用域裁决
// （创建者本人 / org.app.mgr+本组织 / 平台管理员+全局级）。
func (h *ExternalAppHandler) Update(c *gin.Context) {
	sc := middleware.Scope(c)
	id := c.Param("id")
	var req updateExternalAppRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	view, appErr := h.store.UpdateExternalApp(sc, id, store.UpdateExternalAppInput{
		Name:      req.Name,
		BaseURL:   req.BaseURL,
		SSOType:   req.SSOType,
		FrameMode: req.FrameMode,
		Scopes:    req.Scopes,
		Status:    req.Status,
		Placement: req.Placement,
		IconURL:   req.IconURL,
		SortOrder: req.SortOrder,
	}, h.managerOf(c))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, 200, gin.H{"external_app": view})
}

// Delete disconnects an external platform (revokes token issuance).
func (h *ExternalAppHandler) Delete(c *gin.Context) {
	sc := middleware.Scope(c)
	id := c.Param("id")
	if appErr := h.store.DeleteExternalApp(sc, id, h.managerOf(c)); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, 200, gin.H{"deleted": true})
}

type launchExternalAppRequest struct {
	ProjectID string `json:"project_id,omitempty"`
	TaskID    string `json:"task_id,omitempty"`
}

// Launch mints a short-lived SSO token for the external app and returns the
// URL to open. The token is appended as a query parameter (new-tab mode), so
// the external platform must set Referrer-Policy: no-referrer and keep the TTL
// short to limit leakage.
func (h *ExternalAppHandler) Launch(c *gin.Context) {
	sc := middleware.Scope(c)
	id := c.Param("id")
	var req launchExternalAppRequest
	// Launch 的 body 字段可选，空 body（io.EOF）合法；
	// 但畸形 JSON 必须 400 —— Launch 会签发 SSO token，
	// 静默吞错会让参数缺失被当成默认值继续放行。
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		transport.WriteError(c, transport.Validation("invalid launch payload", map[string]any{"body": "malformed json"}))
		return
	}

	// Visibility is enforced here: launching mints a token, so a user must not
	// be able to launch an app they cannot see.
	app, appErr := h.store.GetExternalAppForLaunch(sc, id)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if app.Status != "enabled" {
		transport.WriteError(c, transport.BadRequest("APP_DISABLED", "external app is disabled"))
		return
	}
	user, ok := h.store.FindUserByID(sc.UserID)
	if !ok {
		transport.WriteError(c, transport.Unauthorized("user not found"))
		return
	}

	token, err := auth.IssueExternalToken(
		app.ClientSecret,
		h.issuer,
		user.ID,
		user.Email,
		user.Name,
		app.ClientID,
		app.Scopes,
		req.ProjectID,
		req.TaskID,
		h.tokenTTL,
	)
	if err != nil {
		transport.WriteError(c, &transport.AppError{Status: 500, Code: "INTERNAL_ERROR", Message: "failed to issue sso token", Details: map[string]any{}})
		return
	}

	launchURL := buildLaunchURL(app.BaseURL, token, req.ProjectID, req.TaskID)
	h.store.RecordExternalAppLaunch(sc, app.ID, app.Name, req.ProjectID, req.TaskID)

	transport.WriteData(c, 200, gin.H{
		"app_id":     app.ID,
		"app_name":   app.Name,
		"launch_url": launchURL,
		"expires_in": int(h.tokenTTL.Seconds()),
	})
}

// buildLaunchURL safely appends the token (and optional context) as query
// parameters, preserving any existing query string in the base URL.
func buildLaunchURL(baseURL, token, projectID, taskID string) string {
	q := url.Values{}
	q.Set("token", token)
	if strings.TrimSpace(projectID) != "" {
		q.Set("project_id", projectID)
	}
	if strings.TrimSpace(taskID) != "" {
		q.Set("task_id", taskID)
	}
	if u, err := url.Parse(baseURL); err == nil {
		merged := u.Query()
		for k, vs := range q {
			merged.Set(k, vs[0])
		}
		u.RawQuery = merged.Encode()
		return u.String()
	}
	sep := "?"
	if strings.Contains(baseURL, "?") {
		sep = "&"
	}
	return baseURL + sep + q.Encode()
}
