package handler

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"trustmesh/backend/internal/middleware"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/project"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

// OrgHandler exposes multi-tenant organization management (阶段 4-A).
// Visibility rule: a caller must be a member of the org (or its owner) to
// see it; invisible = nonexistent (404) per the existence-leak rule.
type OrgHandler struct {
	store       *store.Store
	storage     project.FileStorage
	basePath    string
	externalURL string
}

func NewOrgHandler(s *store.Store, storage project.FileStorage, basePath, externalURL string) *OrgHandler {
	return &OrgHandler{store: s, storage: storage, basePath: basePath, externalURL: externalURL}
}

var orgSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)

// slugSanitizeRe 把任意字符串压成 slug 允许的字符集（非 [a-z0-9] 折叠为 '-'）。
var slugSanitizeRe = regexp.MustCompile(`[^a-z0-9]+`)

type orgView struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Slug    string `json:"slug"`
	Kind    string `json:"kind"`
	OwnerID string `json:"owner_id"`
	MyRole  string `json:"my_role"`
	// MyRoleID 是调用者在 org_roles 里的角色引用（自定义角色场景下 my_role 会是
	// 该 role_id 本身，前端展示角色名请优先用本字段查角色表）。
	MyRoleID string         `json:"my_role_id,omitempty"`
	Quota    model.OrgQuota `json:"quota"`
	// MenuOverrides 企业级菜单隐藏项（owner 在设置页勾选，只能缩小；设计文档 §4）。
	MenuOverrides []string `json:"menu_overrides,omitempty"`
	// LogoURL 组织 logo 的访问地址（成员鉴权直出）；未设置时为空。
	LogoURL   string       `json:"logo_url,omitempty"`
	CreatedAt string       `json:"created_at"`
	Members   []memberView `json:"members,omitempty"`
}

type memberView struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	// Role 兼容字段：内置角色为语义键，自定义角色为 role_id（见 model.OrgMembership）。
	Role string `json:"role"`
	// RoleID 是 org_roles 的角色引用（权威），RoleName 是其展示名，便于前端直接渲染。
	RoleID   string `json:"role_id,omitempty"`
	RoleName string `json:"role_name,omitempty"`
	JoinedAt string `json:"joined_at"`
	Email    string `json:"email,omitempty"`
	Name     string `json:"name,omitempty"`
}

// roleOf resolves the caller's role in an org ("" if not a member).
func (h *OrgHandler) roleOf(orgID, userID string) string {
	if m, ok := h.store.GetMembership(orgID, userID); ok {
		return m.Role
	}
	return ""
}

// roleIDOf 取调用者在租户内的角色引用（无成员关系返回空）。
func (h *OrgHandler) roleIDOf(orgID, userID string) string {
	if m, ok := h.store.GetMembership(orgID, userID); ok {
		return m.RoleID
	}
	return ""
}

// toMemberView 组装成员视图，并把 role_id 解析成展示名（找不到角色时留空，
// 前端回落显示 role 字段）。
func (h *OrgHandler) toMemberView(m *model.OrgMembership) memberView {
	v := memberView{
		ID: m.ID, UserID: m.UserID, Role: m.Role, RoleID: m.RoleID,
		JoinedAt: m.JoinedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if role, ok := h.store.RoleByID(m.RoleID); ok {
		v.RoleName = role.Name
	}
	return v
}

// isOwnerMembership 报告成员是否为租户所有者（role_id 与兼容字段任一命中即可）。
func isOwnerMembership(orgID string, m *model.OrgMembership) bool {
	if m == nil {
		return false
	}
	return m.Role == model.OrgRoleOwner ||
		m.RoleID == model.BuiltinOrgRoleID(orgID, model.OrgRoleOwner)
}

// isAdminMembership 报告成员是否为内置 admin 角色（自定义角色不算）。
func isAdminMembership(orgID string, m *model.OrgMembership) bool {
	if m == nil {
		return false
	}
	return m.Role == model.OrgRoleAdmin ||
		m.RoleID == model.BuiltinOrgRoleID(orgID, model.OrgRoleAdmin)
}

// memberGuard checks the caller is a member; returns the org and role.
func (h *OrgHandler) memberGuard(c *gin.Context, orgID string) (*model.Organization, string, bool) {
	org, appErr := h.store.GetOrganization(orgID)
	if appErr != nil {
		transport.WriteError(c, transport.NotFound("organization not found"))
		return nil, "", false
	}
	role := h.roleOf(orgID, middleware.Scope(c).UserID)
	if role == "" {
		// invisible = nonexistent
		transport.WriteError(c, transport.NotFound("organization not found"))
		return nil, "", false
	}
	return org, role, true
}

// adminGuard 在 memberGuard 之上附加「仅企业租户」约束。
//
// 历史：这里曾兼任角色门禁（owner/admin 才放行）。权限体系落地后，调用方角色
// 由路由层 authz.RequirePerm(org.member.mgr) 统一裁决（设计文档 §6.2），
// 本函数只保留「个人租户不支持成员管理」的业务约束。
func (h *OrgHandler) adminGuard(c *gin.Context, orgID string) (*model.Organization, bool) {
	org, _, ok := h.memberGuard(c, orgID)
	if !ok {
		return nil, false
	}
	if org.Kind != model.OrgKindEnterprise {
		transport.WriteError(c, transport.BadRequest("PERSONAL_ORG", "personal organization does not support member management"))
		return nil, false
	}
	return org, true
}

func (h *OrgHandler) toOrgView(org *model.Organization, role, roleID string) orgView {
	return orgView{
		ID: org.ID, Name: org.Name, Slug: org.Slug, Kind: org.Kind,
		OwnerID: org.OwnerID, MyRole: role, MyRoleID: roleID, Quota: org.Quota,
		MenuOverrides: org.MenuOverrides,
		LogoURL:       h.orgLogoURL(org),
		CreatedAt:     org.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// orgLogoURL 返回组织 logo 的访问地址。未设置 logo 时返回空串。
// 优先用 externalURL 拼绝对路径（桌面端跨进程直连也能加载）；缺省回落相对 API 路径。
func (h *OrgHandler) orgLogoURL(org *model.Organization) string {
	if org.LogoFileID == "" {
		return ""
	}
	base := strings.TrimRight(h.externalURL, "/")
	if base == "" {
		return "/api/v1/organizations/" + org.ID + "/logo"
	}
	return base + "/api/v1/organizations/" + org.ID + "/logo"
}

// ownerOrAdminGuard 在 memberGuard 之上要求调用者为 owner 或内置 admin
// （组织资料 / logo 改动的门禁；设计文档 §6.2 权限体系落地后由本函数兜底裁决）。
func (h *OrgHandler) ownerOrAdminGuard(c *gin.Context, orgID string) (*model.Organization, bool) {
	org, _, ok := h.memberGuard(c, orgID)
	if !ok {
		return nil, false
	}
	if org.Kind != model.OrgKindEnterprise {
		transport.WriteError(c, transport.BadRequest("PERSONAL_ORG", "personal organization does not support this action"))
		return nil, false
	}
	m, _ := h.store.GetMembership(orgID, middleware.Scope(c).UserID)
	if !isOwnerMembership(orgID, m) && !isAdminMembership(orgID, m) {
		transport.WriteError(c, transport.Forbidden("only the owner or an admin can modify the organization profile"))
		return nil, false
	}
	return org, true
}

// validateImageExtension 校验上传文件名是否为受支持的图片格式。
func validateImageExtension(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".svg":
		return true
	}
	return false
}

// UpdateProfile PATCH /organizations/:id —— 更新组织名称与简称（owner/admin）。
func (h *OrgHandler) UpdateProfile(c *gin.Context) {
	orgID := c.Param("id")
	if _, ok := h.ownerOrAdminGuard(c, orgID); !ok {
		return
	}
	var req struct {
		Name      string `json:"name"`
		ShortName string `json:"short_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	updated, appErr := h.store.UpdateOrgProfile(orgID, req.Name, req.ShortName)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	recordAudit(c, h.store, orgID, model.AuditActionOrgUpdate, model.AuditTargetOrg, orgID,
		map[string]any{"name": updated.Name, "short_name": updated.ShortName})
	transport.WriteData(c, 200, h.toOrgView(updated, h.roleOf(orgID, middleware.Scope(c).UserID),
		h.roleIDOf(orgID, middleware.Scope(c).UserID)))
}

// orgLogoBucket 文件存储桶名（与 chat 附件隔离）。
const orgLogoBucket = "org-logos"

// UploadLogo POST /organizations/:id/logo —— 上传组织 logo（multipart 字段 "file"，owner/admin）。
// 落盘到 org-logos 桶后把文件标识写入组织；返回含 logo_url 的最新 OrgView。
func (h *OrgHandler) UploadLogo(c *gin.Context) {
	orgID := c.Param("id")
	if _, ok := h.ownerOrAdminGuard(c, orgID); !ok {
		return
	}
	applyUploadLimit(c)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "file is required"))
		return
	}
	defer file.Close()
	if !validateImageExtension(header.Filename) {
		transport.WriteError(c, transport.BadRequest("UNSUPPORTED_FILE_TYPE",
			"仅支持图片格式：png / jpg / jpeg / webp / gif / svg"))
		return
	}
	safeName := filepath.Base(header.Filename)
	id := uuid.NewString()
	// 仅用基础名持久化，避免浏览器/桌面提供的路径逃逸出 bucket。
	logoPath := filepath.Join(h.basePath, orgLogoBucket, "uploads", id+"_"+safeName)
	if _, err := h.storage.Save(c.Request.Context(), orgLogoBucket, id, safeName, file); err != nil {
		transport.WriteError(c, transport.NewError(http.StatusInternalServerError, "UPLOAD_FAILED", "failed to store file"))
		return
	}
	updated, appErr := h.store.SetOrgLogo(orgID, id, safeName)
	if appErr != nil {
		// 元数据写入失败：尽力清理已落盘的孤儿文件，避免磁盘泄漏。
		_ = h.storage.Delete(c.Request.Context(), logoPath)
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, 200, h.toOrgView(updated, h.roleOf(orgID, middleware.Scope(c).UserID),
		h.roleIDOf(orgID, middleware.Scope(c).UserID)))
}

// GetLogo GET /organizations/:id/logo —— 成员鉴权直出组织 logo 字节。
func (h *OrgHandler) GetLogo(c *gin.Context) {
	orgID := c.Param("id")
	org, _, ok := h.memberGuard(c, orgID)
	if !ok {
		return
	}
	if org.LogoFileID == "" {
		transport.WriteError(c, transport.NotFound("logo not found"))
		return
	}
	path := filepath.Join(h.basePath, orgLogoBucket, "uploads", org.LogoFileID+"_"+org.LogoFileName)
	f, err := os.Open(path)
	if err != nil {
		transport.WriteError(c, transport.NotFound("logo not found on disk"))
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		transport.WriteError(c, transport.NotFound("logo not found on disk"))
		return
	}
	c.Header("Cache-Control", "private, max-age=3600")
	c.Header("Content-Type", mime.TypeByExtension(filepath.Ext(org.LogoFileName)))
	http.ServeContent(c.Writer, c.Request, org.LogoFileName, info.ModTime(), f)
}

// List returns every org the caller belongs to (personal + enterprise).
func (h *OrgHandler) List(c *gin.Context) {
	userID := middleware.Scope(c).UserID
	orgs := h.store.ListUserOrganizations(userID)
	items := make([]orgView, 0, len(orgs))
	for _, org := range orgs {
		items = append(items, h.toOrgView(org, h.roleOf(org.ID, userID), h.roleIDOf(org.ID, userID)))
	}
	transport.WriteList(c, items, len(items))
}

type createOrgRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Create registers a new enterprise org; the caller becomes its owner.
func (h *OrgHandler) Create(c *gin.Context) {
	userID := middleware.Scope(c).UserID
	var req createOrgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	name := strings.TrimSpace(req.Name)
	slug := strings.ToLower(strings.TrimSpace(req.Slug))
	if slug == "" {
		// derive from name if omitted: keep [a-z0-9], collapse runs to '-'
		slug = orgSlugFrom(name)
	}
	if !orgSlugRe.MatchString(slug) {
		transport.WriteError(c, transport.BadRequest("VALIDATION_ERROR", "slug: 3-64 chars, lowercase letters/digits/hyphens"))
		return
	}
	org, appErr := h.store.CreateOrganization(userID, name, slug, model.OrgKindEnterprise)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	recordAudit(c, h.store, org.ID, model.AuditActionOrgCreate, model.AuditTargetOrg, org.ID,
		map[string]any{"name": org.Name, "slug": org.Slug})
	transport.WriteData(c, 201, h.toOrgView(org, model.OrgRoleOwner,
		model.BuiltinOrgRoleID(org.ID, model.OrgRoleOwner)))
}

// Get returns one org (members only; others get 404).
func (h *OrgHandler) Get(c *gin.Context) {
	org, role, ok := h.memberGuard(c, c.Param("id"))
	if !ok {
		return
	}
	transport.WriteData(c, 200, h.toOrgView(org, role, h.roleIDOf(org.ID, middleware.Scope(c).UserID)))
}

// ListMembers returns the org roster with user email/name attached.
func (h *OrgHandler) ListMembers(c *gin.Context) {
	orgID := c.Param("id")
	if _, _, ok := h.memberGuard(c, orgID); !ok {
		return
	}
	members := h.store.ListOrgMembers(orgID)
	items := make([]memberView, 0, len(members))
	for _, m := range members {
		v := h.toMemberView(m)
		if u, ok := h.store.FindUserByID(m.UserID); ok {
			v.Email = u.Email
			v.Name = u.Name
		}
		items = append(items, v)
	}
	transport.WriteList(c, items, len(items))
}

type addMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
	// RoleID 指定自定义角色（设计文档 §5）；与 Role 同传时以 RoleID 为准。
	RoleID string `json:"role_id"`
}

// AddMember adds an existing user (looked up by email) to the org.
// Owner/admin only. Allowed roles: admin, member 或本企业任一自定义角色。
func (h *OrgHandler) AddMember(c *gin.Context) {
	orgID := c.Param("id")
	if _, ok := h.adminGuard(c, orgID); !ok {
		return
	}
	var req addMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	roleRef := memberRoleRef(req.RoleID, req.Role)
	u, ok := h.store.FindUserByEmail(strings.TrimSpace(req.Email))
	if !ok {
		transport.WriteError(c, transport.NotFound("user not found"))
		return
	}
	m, appErr := h.store.AddOrgMember(orgID, u.ID, roleRef)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	recordAudit(c, h.store, orgID, model.AuditActionOrgMemberAdd, model.AuditTargetMembership, m.ID,
		map[string]any{"user_id": u.ID, "email": u.Email, "role": m.Role, "role_id": m.RoleID})
	v := h.toMemberView(m)
	v.Email, v.Name = u.Email, u.Name
	transport.WriteData(c, 201, v)
}

// memberRoleRef 归一「成员角色引用」：role_id 优先，缺省回落内置角色串
// （最终校验与 owner 保护都在 store 层，handler 不做角色白名单）。
func memberRoleRef(roleID, role string) string {
	if ref := strings.TrimSpace(roleID); ref != "" {
		return ref
	}
	if ref := strings.TrimSpace(role); ref != "" {
		return ref
	}
	return model.OrgRoleMember
}

type updateMemberRoleRequest struct {
	Role   string `json:"role"`
	RoleID string `json:"role_id"`
}

// UpdateMemberRole changes a member's role. Admins cannot touch the owner;
// nobody can grant owner (transfer is out of scope this phase).
func (h *OrgHandler) UpdateMemberRole(c *gin.Context) {
	orgID := c.Param("id")
	targetID := c.Param("userId")
	if _, ok := h.adminGuard(c, orgID); !ok {
		return
	}
	var req updateMemberRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	roleRef := memberRoleRef(req.RoleID, req.Role)
	target, ok := h.store.GetMembership(orgID, targetID)
	if !ok {
		transport.WriteError(c, transport.NotFound("member not found"))
		return
	}
	if isOwnerMembership(orgID, target) {
		transport.WriteError(c, transport.Forbidden("owner role can only be changed via ownership transfer"))
		return
	}
	m, appErr := h.store.UpdateOrgMemberRole(orgID, targetID, roleRef)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	recordAudit(c, h.store, orgID, model.AuditActionOrgMemberRoleChange, model.AuditTargetMembership, m.ID,
		map[string]any{
			"user_id": targetID,
			"from":    map[string]any{"role": target.Role, "role_id": target.RoleID},
			"to":      map[string]any{"role": m.Role, "role_id": m.RoleID},
		})
	transport.WriteData(c, 200, h.toMemberView(m))
}

// RemoveMember removes a non-owner member. Admins cannot remove the owner
// (or other admins? no — admins may remove members only; owner may remove
// admins and members).
//
// 调用方是否具备成员管理权限由路由层 RequirePerm(org.member.mgr) 裁决；
// 这里只保留「目标侧」层级规则：admin 只能移除 member，owner 不可被移除。
func (h *OrgHandler) RemoveMember(c *gin.Context) {
	orgID := c.Param("id")
	targetID := c.Param("userId")
	org, role, ok := h.memberGuard(c, orgID)
	if !ok {
		return
	}
	if org.Kind != model.OrgKindEnterprise {
		transport.WriteError(c, transport.BadRequest("PERSONAL_ORG", "personal organization does not support member management"))
		return
	}
	target, ok := h.store.GetMembership(orgID, targetID)
	if !ok {
		transport.WriteError(c, transport.NotFound("member not found"))
		return
	}
	// 层级规则：owner 不可被移除；非 owner 的调用者不得移除 admin 及以上
	// （保持收紧前「admin 只能移除 member」的语义，同时让自定义角色也不会
	// 因为 Role 串不匹配而绕过该限制）。
	if isOwnerMembership(orgID, target) {
		transport.WriteError(c, transport.Forbidden("owner cannot be removed"))
		return
	}
	if role != model.OrgRoleOwner && isAdminMembership(orgID, target) {
		transport.WriteError(c, transport.Forbidden("only the owner can remove an admin"))
		return
	}
	if appErr := h.store.RemoveOrgMember(orgID, targetID); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	recordAudit(c, h.store, orgID, model.AuditActionOrgMemberRemove, model.AuditTargetMembership, target.ID,
		map[string]any{"user_id": targetID, "role": target.Role, "role_id": target.RoleID})
	transport.WriteData(c, 200, gin.H{"removed": targetID})
}

type menuOverridesRequest struct {
	MenuOverrides []string `json:"menu_overrides"`
}

// SetMenuOverrides PATCH /organizations/:id/menu-overrides —— 企业级菜单覆盖（只能缩小）。
//
// 需要 org.settings（owner）：这是企业设置项，不是成员管理。覆盖只影响前端菜单可见性，
// 不改变任何 API 鉴权结果（设计文档 §4：API 鉴权以权限点为唯一准绳）。
func (h *OrgHandler) SetMenuOverrides(c *gin.Context) {
	orgID := c.Param("id")
	org, ok := h.adminGuard(c, orgID)
	if !ok {
		return
	}
	var req menuOverridesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	updated, appErr := h.store.SetOrgMenuOverrides(orgID, req.MenuOverrides)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	recordAudit(c, h.store, orgID, model.AuditActionOrgMenuOverrideUpdate, model.AuditTargetOrg, orgID,
		map[string]any{"from": org.MenuOverrides, "to": updated.MenuOverrides})
	transport.WriteData(c, 200, h.toOrgView(updated, h.roleOf(orgID, middleware.Scope(c).UserID),
		h.roleIDOf(orgID, middleware.Scope(c).UserID)))
}
