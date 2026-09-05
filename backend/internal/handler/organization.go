package handler

import (
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/middleware"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

// OrgHandler exposes multi-tenant organization management (阶段 4-A).
// Visibility rule: a caller must be a member of the org (or its owner) to
// see it; invisible = nonexistent (404) per the existence-leak rule.
type OrgHandler struct {
	store *store.Store
}

func NewOrgHandler(s *store.Store) *OrgHandler {
	return &OrgHandler{store: s}
}

var orgSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)

type orgView struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Slug      string             `json:"slug"`
	Kind      string             `json:"kind"`
	OwnerID   string             `json:"owner_id"`
	MyRole    string             `json:"my_role"`
	Quota     model.OrgQuota     `json:"quota"`
	CreatedAt string             `json:"created_at"`
	Members   []memberView       `json:"members,omitempty"`
}

type memberView struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	Role     string `json:"role"`
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

// adminGuard additionally requires owner|admin and an enterprise org.
func (h *OrgHandler) adminGuard(c *gin.Context, orgID string) (*model.Organization, bool) {
	org, role, ok := h.memberGuard(c, orgID)
	if !ok {
		return nil, false
	}
	if org.Kind != model.OrgKindEnterprise {
		transport.WriteError(c, transport.BadRequest("PERSONAL_ORG", "personal organization does not support member management"))
		return nil, false
	}
	if role != model.OrgRoleOwner && role != model.OrgRoleAdmin {
		transport.WriteError(c, transport.Forbidden("owner or admin role required"))
		return nil, false
	}
	return org, true
}

func toOrgView(org *model.Organization, role string) orgView {
	return orgView{
		ID: org.ID, Name: org.Name, Slug: org.Slug, Kind: org.Kind,
		OwnerID: org.OwnerID, MyRole: role, Quota: org.Quota,
		CreatedAt: org.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// List returns every org the caller belongs to (personal + enterprise).
func (h *OrgHandler) List(c *gin.Context) {
	userID := middleware.Scope(c).UserID
	orgs := h.store.ListUserOrganizations(userID)
	items := make([]orgView, 0, len(orgs))
	for _, org := range orgs {
		items = append(items, toOrgView(org, h.roleOf(org.ID, userID)))
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
		slug = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(name), "-")
		slug = strings.Trim(slug, "-")
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
	transport.WriteData(c, 201, toOrgView(org, model.OrgRoleOwner))
}

// Get returns one org (members only; others get 404).
func (h *OrgHandler) Get(c *gin.Context) {
	org, role, ok := h.memberGuard(c, c.Param("id"))
	if !ok {
		return
	}
	transport.WriteData(c, 200, toOrgView(org, role))
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
		v := memberView{ID: m.ID, UserID: m.UserID, Role: m.Role, JoinedAt: m.JoinedAt.Format("2006-01-02T15:04:05Z07:00")}
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
}

// AddMember adds an existing user (looked up by email) to the org.
// Owner/admin only. Allowed roles: admin, member.
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
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = model.OrgRoleMember
	}
	if role != model.OrgRoleAdmin && role != model.OrgRoleMember {
		transport.WriteError(c, transport.BadRequest("VALIDATION_ERROR", "role: must be admin or member"))
		return
	}
	u, ok := h.store.FindUserByEmail(strings.TrimSpace(req.Email))
	if !ok {
		transport.WriteError(c, transport.NotFound("user not found"))
		return
	}
	m, appErr := h.store.AddOrgMember(orgID, u.ID, role)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, 201, memberView{ID: m.ID, UserID: m.UserID, Role: m.Role, JoinedAt: m.JoinedAt.Format("2006-01-02T15:04:05Z07:00"), Email: u.Email, Name: u.Name})
}

type updateMemberRoleRequest struct {
	Role string `json:"role"`
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
	role := strings.TrimSpace(req.Role)
	if role != model.OrgRoleAdmin && role != model.OrgRoleMember {
		transport.WriteError(c, transport.BadRequest("VALIDATION_ERROR", "role: must be admin or member"))
		return
	}
	target, ok := h.store.GetMembership(orgID, targetID)
	if !ok {
		transport.WriteError(c, transport.NotFound("member not found"))
		return
	}
	if target.Role == model.OrgRoleOwner {
		transport.WriteError(c, transport.Forbidden("owner role can only be changed via ownership transfer"))
		return
	}
	m, appErr := h.store.UpdateOrgMemberRole(orgID, targetID, role)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, 200, memberView{ID: m.ID, UserID: m.UserID, Role: m.Role, JoinedAt: m.JoinedAt.Format("2006-01-02T15:04:05Z07:00")})
}

// RemoveMember removes a non-owner member. Admins cannot remove the owner
// (or other admins? no — admins may remove members only; owner may remove
// admins and members).
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
	// permission matrix: owner removes anyone but owner(s); admin removes members only
	if role == model.OrgRoleAdmin {
		if target.Role != model.OrgRoleMember {
			transport.WriteError(c, transport.Forbidden("admin can only remove members"))
			return
		}
	} else if role != model.OrgRoleOwner {
		transport.WriteError(c, transport.Forbidden("owner or admin role required"))
		return
	}
	if target.Role == model.OrgRoleOwner {
		transport.WriteError(c, transport.Forbidden("owner cannot be removed"))
		return
	}
	if appErr := h.store.RemoveOrgMember(orgID, targetID); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, 200, gin.H{"removed": targetID})
}
