package handler

import (
	"crypto/rand"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"trustmesh/backend/internal/middleware"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// 平台侧用户管理（/api/v1/platform/users*，设计文档 §3.2 的账号运维扩展）：
// 账号列表/详情、重置密码、禁用/启用，以及企业维度的成员查看。
//
// 职责边界：只碰**账号元数据**与状态，读不到任何企业业务内容。
// 方法挂在 PlatformAdminHandler 上（同包不同文件），复用其 store 字段。
//
// 禁用语义（本期）：登录 / refresh 一律拒绝；已签发的 access token 在 TTL
// （默认 15min）内自然过期 —— 无状态 JWT 不做逐请求状态校验（见 middleware.RequireAuth）。

// platformTS 统一时间字段的 JSON 形态（与 platform_admin.go 的 org 视图保持同格式）。
func platformTS(t time.Time) string {
	return t.Format("2006-01-02T15:04:05Z07:00")
}

// platformUserOrgRef 用户在某租户下的归属与角色（平台视图用）。
type platformUserOrgRef struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Role     string `json:"role,omitempty"`
	RoleID   string `json:"role_id,omitempty"`
	RoleName string `json:"role_name,omitempty"`
}

// platformUserView 平台侧账号视图（不含密码哈希；password_hash 本身 json:"-"）。
type platformUserView struct {
	ID              string               `json:"id"`
	Email           string               `json:"email"`
	Name            string               `json:"name"`
	Disabled        bool                 `json:"disabled"`
	DisabledAt      string               `json:"disabled_at,omitempty"`
	IsPlatformAdmin bool                 `json:"is_platform_admin"`
	CreatedAt       string               `json:"created_at"`
	Orgs            []platformUserOrgRef `json:"orgs,omitempty"`
}

// platformOrgMemberView 企业成员视图（平台视角，比企业侧多一个账号禁用态）。
type platformOrgMemberView struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email,omitempty"`
	Name     string `json:"name,omitempty"`
	Role     string `json:"role"`
	RoleID   string `json:"role_id,omitempty"`
	RoleName string `json:"role_name,omitempty"`
	JoinedAt string `json:"joined_at"`
	Disabled bool   `json:"disabled"`
}

// userOrgRefs 组合用户的租户归属（既有 ListUserOrganizations + GetMembership）。
func (h *PlatformAdminHandler) userOrgRefs(userID string) []platformUserOrgRef {
	orgs := h.store.ListUserOrganizations(userID)
	out := make([]platformUserOrgRef, 0, len(orgs))
	for _, org := range orgs {
		ref := platformUserOrgRef{ID: org.ID, Name: org.Name, Kind: org.Kind}
		if m, ok := h.store.GetMembership(org.ID, userID); ok {
			ref.Role, ref.RoleID = m.Role, m.RoleID
			if role, ok := h.store.RoleByID(m.RoleID); ok {
				ref.RoleName = role.Name
			}
		}
		out = append(out, ref)
	}
	return out
}

func (h *PlatformAdminHandler) toUserView(u *model.User) platformUserView {
	v := platformUserView{
		ID: u.ID, Email: u.Email, Name: u.Name,
		Disabled: u.Disabled, IsPlatformAdmin: u.IsAdmin,
		CreatedAt: platformTS(u.CreatedAt),
		Orgs:      h.userOrgRefs(u.ID),
	}
	if u.DisabledAt != nil {
		v.DisabledAt = platformTS(*u.DisabledAt)
	}
	return v
}

// ListUsers GET /api/v1/platform/users —— 账号列表，支持 keyword / org_id / status 过滤。
func (h *PlatformAdminHandler) ListUsers(c *gin.Context) {
	keyword := strings.ToLower(strings.TrimSpace(c.Query("keyword")))
	orgID := strings.TrimSpace(c.Query("org_id"))
	status := strings.TrimSpace(c.Query("status"))

	users := h.store.ListAllUsers()
	items := make([]platformUserView, 0, len(users))
	for _, u := range users {
		if keyword != "" &&
			!strings.Contains(strings.ToLower(u.Email), keyword) &&
			!strings.Contains(strings.ToLower(u.Name), keyword) {
			continue
		}
		switch status {
		case "disabled":
			if !u.Disabled {
				continue
			}
		case "active":
			if u.Disabled {
				continue
			}
		}
		if orgID != "" {
			if _, ok := h.store.GetMembership(orgID, u.ID); !ok {
				continue
			}
		}
		items = append(items, h.toUserView(u))
	}
	transport.WriteList(c, items, len(items))
}

// GetUser GET /api/v1/platform/users/:id —— 单个账号详情（含所属租户与角色）。
func (h *PlatformAdminHandler) GetUser(c *gin.Context) {
	u, ok := h.store.FindUserByID(c.Param("id"))
	if !ok {
		transport.WriteError(c, transport.NotFound("user not found"))
		return
	}
	transport.WriteData(c, http.StatusOK, h.toUserView(u))
}

// tempPasswordAlphabet 剔除易混字符（0/O/1/l/I），降低人工转述时的抄错概率。
const tempPasswordAlphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

const tempPasswordLength = 12

// generateTempPassword 用 crypto/rand 生成临时密码（拒绝采样，无取模偏置）。
// 明文只进 HTTP 响应，绝不落库、绝不落日志。
func generateTempPassword() (string, error) {
	bound := big.NewInt(int64(len(tempPasswordAlphabet)))
	var b strings.Builder
	b.Grow(tempPasswordLength)
	for i := 0; i < tempPasswordLength; i++ {
		n, err := rand.Int(rand.Reader, bound)
		if err != nil {
			return "", err
		}
		b.WriteByte(tempPasswordAlphabet[n.Int64()])
	}
	return b.String(), nil
}

// ResetUserPassword POST /api/v1/platform/users/:id/reset-password
//
// 平台管理员重置任意账号密码：生成随机临时密码 → bcrypt 入库 → 明文只在本次响应里返回一次。
// 审计只记邮箱，绝不记密码。
func (h *PlatformAdminHandler) ResetUserPassword(c *gin.Context) {
	userID := c.Param("id")
	u, ok := h.store.FindUserByID(userID)
	if !ok {
		transport.WriteError(c, transport.NotFound("user not found"))
		return
	}

	temp, err := generateTempPassword()
	if err != nil {
		transport.WriteError(c, &transport.AppError{
			Status: http.StatusInternalServerError, Code: "INTERNAL_ERROR",
			Message: "failed to generate temp password", Details: map[string]any{},
		})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(temp), bcrypt.DefaultCost)
	if err != nil {
		transport.WriteError(c, &transport.AppError{
			Status: http.StatusInternalServerError, Code: "INTERNAL_ERROR",
			Message: "failed to hash password", Details: map[string]any{},
		})
		return
	}
	if appErr := h.store.UpdateUserPassword(userID, string(hash)); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// 审计只留「谁给谁重置了密码」，不含明文与哈希。
	recordAudit(c, h.store, model.AuditScopePlatform, model.AuditActionUserPasswordReset,
		model.AuditTargetUser, userID, map[string]any{"email": u.Email})
	transport.WriteData(c, http.StatusOK, gin.H{
		"user_id":       userID,
		"email":         u.Email,
		"temp_password": temp,
	})
}

// DisableUser / EnableUser POST /api/v1/platform/users/:id/{disable,enable}
//
// 禁用即拦截：登录与 refresh 一律 403 USER_DISABLED（见 handler.AuthHandler）；
// 已签发的 access token 在 TTL 内自然过期（本期不做主动吊销）。
func (h *PlatformAdminHandler) DisableUser(c *gin.Context) {
	h.setUserDisabled(c, true, model.AuditActionUserDisable)
}

func (h *PlatformAdminHandler) EnableUser(c *gin.Context) {
	h.setUserDisabled(c, false, model.AuditActionUserEnable)
}

func (h *PlatformAdminHandler) setUserDisabled(c *gin.Context, disabled bool, action string) {
	userID := c.Param("id")
	// 不能禁用自己：否则本次会话之外无法再登录平台，只能靠运维改库恢复。
	if disabled && userID == middleware.UserID(c) {
		transport.WriteError(c, transport.Forbidden("cannot disable your own account"))
		return
	}
	before, ok := h.store.FindUserByID(userID)
	if !ok {
		transport.WriteError(c, transport.NotFound("user not found"))
		return
	}
	after, appErr := h.store.SetUserDisabled(userID, disabled)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	// 幂等：状态未变化时不落审计（避免重复点击产生噪音记录）。
	if before.Disabled != after.Disabled {
		recordAudit(c, h.store, model.AuditScopePlatform, action, model.AuditTargetUser, userID,
			map[string]any{"email": after.Email, "from": before.Disabled, "to": after.Disabled})
	}
	transport.WriteData(c, http.StatusOK, h.toUserView(after))
}

// ListOrgMembers GET /api/v1/platform/orgs/:id/members —— 平台视角的企业成员列表。
// 仅企业租户（个人租户非 404 语义下的「企业管理」目标）。
func (h *PlatformAdminHandler) ListOrgMembers(c *gin.Context) {
	orgID := c.Param("id")
	org, appErr := h.store.GetOrganization(orgID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if org.Kind != model.OrgKindEnterprise {
		transport.WriteError(c, transport.NotFound("enterprise organization not found"))
		return
	}

	members := h.store.ListOrgMembers(orgID)
	items := make([]platformOrgMemberView, 0, len(members))
	for _, m := range members {
		v := platformOrgMemberView{
			UserID: m.UserID, Role: m.Role, RoleID: m.RoleID,
			JoinedAt: platformTS(m.JoinedAt),
		}
		if role, ok := h.store.RoleByID(m.RoleID); ok {
			v.RoleName = role.Name
		}
		if u, ok := h.store.FindUserByID(m.UserID); ok {
			v.Email, v.Name, v.Disabled = u.Email, u.Name, u.Disabled
		}
		items = append(items, v)
	}
	transport.WriteList(c, items, len(items))
}
