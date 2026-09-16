package handler

import (
	"strings"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/authz"
	"trustmesh/backend/internal/middleware"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

// 企业角色管理（设计文档 §5）：内置角色展示 + 自定义角色 CRUD。
//
// 权限：读写分开 —— 列表走 org.member.mgr（成员改角色的下拉框需要它），
// 增删改走 org.role.mgr（仅 owner）。「谁能操作」由路由层裁决，本文件只做
// 「怎么改」的校验（内置锁定、≤ admin 全集、owner 不可替代由 store 保证）。
type OrgRoleHandler struct {
	store *store.Store
	// legacyMember 与 authz 解析链同源（PERM_LEGACY_MEMBER 回滚开关）：
	// 内置角色的权限集以代码矩阵为准，回传时也走同一矩阵，避免界面与鉴权不一致。
	legacyMember bool
}

func NewOrgRoleHandler(s *store.Store, legacyMember bool) *OrgRoleHandler {
	return &OrgRoleHandler{store: s, legacyMember: legacyMember}
}

type orgRoleView struct {
	ID          string   `json:"id"`
	OrgID       string   `json:"org_id"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
	Builtin     bool     `json:"builtin"`
	MemberCount int      `json:"member_count"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

// view 组装角色视图。内置角色的权限集以 authz 矩阵为准（集合里的镜像是给运维看的，
// 可能与当前开关下的矩阵不同），保证界面展示与鉴权结果永远是同一份数据。
func (h *OrgRoleHandler) view(role *model.OrgRole) orgRoleView {
	perms := role.Permissions
	if role.Builtin {
		perms = authz.BuiltinPermissions(role.BuiltinKey, h.legacyMember)
	}
	if perms == nil {
		perms = []string{}
	}
	return orgRoleView{
		ID: role.ID, OrgID: role.OrgID, Name: role.Name,
		Permissions: perms, Builtin: role.Builtin,
		MemberCount: h.store.OrgRoleMemberCount(role.ID),
		CreatedAt:   role.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   role.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// guard 校验调用者是该租户成员（不可见即不存在，与 OrgHandler 的裁决一致）。
func (h *OrgRoleHandler) guard(c *gin.Context, orgID string) bool {
	if _, ok := h.store.GetMembership(orgID, middleware.Scope(c).UserID); !ok {
		transport.WriteError(c, transport.NotFound("organization not found"))
		return false
	}
	return true
}

// List GET /organizations/:id/roles
func (h *OrgRoleHandler) List(c *gin.Context) {
	orgID := c.Param("id")
	if !h.guard(c, orgID) {
		return
	}
	roles := h.store.ListOrgRoles(orgID)
	items := make([]orgRoleView, 0, len(roles))
	for _, role := range roles {
		items = append(items, h.view(role))
	}
	transport.WriteList(c, items, len(items))
}

type orgRoleRequest struct {
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

// Create POST /organizations/:id/roles
func (h *OrgRoleHandler) Create(c *gin.Context) {
	orgID := c.Param("id")
	if !h.guard(c, orgID) {
		return
	}
	var req orgRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	role, appErr := h.store.CreateOrgRole(orgID, req.Name, req.Permissions)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	recordAudit(c, h.store, orgID, model.AuditActionOrgRoleCreate, model.AuditTargetRole, role.ID,
		map[string]any{"name": role.Name, "permissions": role.Permissions})
	transport.WriteData(c, 201, h.view(role))
}

// Update PATCH /organizations/:id/roles/:roleId
//
// name 为空 = 不改名；permissions 为 null = 不改权限集（PATCH 语义）。
// 内置角色的任何修改都被 store 拒绝（BUILTIN_ROLE_LOCKED）。
func (h *OrgRoleHandler) Update(c *gin.Context) {
	orgID := c.Param("id")
	if !h.guard(c, orgID) {
		return
	}
	roleID := c.Param("roleId")
	current, ok := h.store.RoleByID(roleID)
	if !ok || current.OrgID != orgID {
		transport.WriteError(c, transport.NotFound("role not found"))
		return
	}
	var req orgRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = current.Name
	}
	perms := req.Permissions
	if perms == nil {
		perms = current.Permissions
	}
	updated, appErr := h.store.UpdateOrgRole(orgID, roleID, name, perms)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	recordAudit(c, h.store, orgID, model.AuditActionOrgRoleUpdate, model.AuditTargetRole, roleID,
		map[string]any{
			"from": map[string]any{"name": current.Name, "permissions": current.Permissions},
			"to":   map[string]any{"name": updated.Name, "permissions": updated.Permissions},
		})
	transport.WriteData(c, 200, h.view(updated))
}

// Delete DELETE /organizations/:id/roles/:roleId
func (h *OrgRoleHandler) Delete(c *gin.Context) {
	orgID := c.Param("id")
	if !h.guard(c, orgID) {
		return
	}
	roleID := c.Param("roleId")
	role, ok := h.store.RoleByID(roleID)
	if !ok || role.OrgID != orgID {
		transport.WriteError(c, transport.NotFound("role not found"))
		return
	}
	if appErr := h.store.DeleteOrgRole(orgID, roleID); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	recordAudit(c, h.store, orgID, model.AuditActionOrgRoleDelete, model.AuditTargetRole, roleID,
		map[string]any{"name": role.Name})
	transport.WriteData(c, 200, gin.H{"deleted": roleID})
}
