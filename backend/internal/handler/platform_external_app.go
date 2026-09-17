package handler

import (
	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/middleware"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

// PlatformExternalAppHandler 是平台命名空间的**全局级**外部应用管理
// （/api/v1/platform/external-apps，权限点 platform.extapp.mgr）。
//
// 与业务侧 handler/external_app.go 的分工：全局级应用不属于任何租户、全员可见可打开，
// 语义上是平台级产品挂载，因此只能由平台管理员创建与管理（用户决策 2026-09-17）。
// 业务侧 Create 一律拒绝 scope=global，杜绝越权放大可见范围。
type PlatformExternalAppHandler struct {
	store *store.Store
}

func NewPlatformExternalAppHandler(s *store.Store) *PlatformExternalAppHandler {
	return &PlatformExternalAppHandler{store: s}
}

// List 返回全部全局级应用（安全视图，不含 client_secret）。
func (h *PlatformExternalAppHandler) List(c *gin.Context) {
	items := h.store.ListGlobalExternalApps()
	transport.WriteData(c, 200, gin.H{"external_apps": items})
}

// Create 注册全局级外部应用。请求里的 scope 被忽略：本命名空间下强制 global、
// OrgID 为空（见 store.CreateGlobalExternalApp）。client_secret 仅在本响应返回一次。
func (h *PlatformExternalAppHandler) Create(c *gin.Context) {
	sc := middleware.Scope(c)
	var req createExternalAppRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	view, secret, appErr := h.store.CreateGlobalExternalApp(sc, store.CreateExternalAppInput{
		Name:      req.Name,
		BaseURL:   req.BaseURL,
		ClientID:  req.ClientID,
		SSOType:   req.SSOType,
		FrameMode: req.FrameMode,
		Scopes:    req.Scopes,
		Placement: req.Placement,
		IconURL:   req.IconURL,
		SortOrder: req.SortOrder,
	})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, 201, gin.H{
		"external_app":  view,
		"client_secret": secret,
	})
}

// Update 修改全局级应用。管理权由 store 按「全局级 + Platform」裁决。
func (h *PlatformExternalAppHandler) Update(c *gin.Context) {
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
	}, store.ExternalAppManager{Platform: true})
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, 200, gin.H{"external_app": view})
}

// Delete 删除全局级应用（立即停止 token 签发）。
func (h *PlatformExternalAppHandler) Delete(c *gin.Context) {
	sc := middleware.Scope(c)
	id := c.Param("id")
	if appErr := h.store.DeleteExternalApp(sc, id, store.ExternalAppManager{Platform: true}); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, 200, gin.H{"deleted": true})
}
