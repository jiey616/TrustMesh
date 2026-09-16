package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/middleware"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

// PlatformAdminHandler 是平台命名空间（/api/v1/platform/*）的业务实现（设计文档 §3.2）：
// 企业生命周期（元数据）/ 全局配置 / 全局审计 / 用量总览。
//
// 职责边界：**纯平台运维型** —— 只碰元数据与聚合计数，读不到企业业务内容；
// 路由层的平台管理员门禁与权限点声明见 app/router.go 的 plat 分组。
type PlatformAdminHandler struct {
	store *store.Store
}

func NewPlatformAdminHandler(s *store.Store) *PlatformAdminHandler {
	return &PlatformAdminHandler{store: s}
}

// platformOrgView 平台侧的企业视图（元数据 + count 级用量）。
type platformOrgView struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Slug          string         `json:"slug"`
	Status        string         `json:"status"`
	OwnerID       string         `json:"owner_id"`
	OwnerEmail    string         `json:"owner_email,omitempty"`
	OwnerName     string         `json:"owner_name,omitempty"`
	Quota         model.OrgQuota `json:"quota"`
	MenuOverrides []string       `json:"menu_overrides,omitempty"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
	MemberCount   int            `json:"member_count"`
	ProjectCount  int            `json:"project_count"`
	TaskCount     int            `json:"task_count"`
}

func (h *PlatformAdminHandler) toOrgView(org *model.Organization) platformOrgView {
	members, projects, tasks := h.store.OrgResourceCounts(org.ID)
	v := platformOrgView{
		ID: org.ID, Name: org.Name, Slug: org.Slug, Status: org.StatusOrActive(),
		OwnerID: org.OwnerID, Quota: org.Quota, MenuOverrides: org.MenuOverrides,
		CreatedAt:   org.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   org.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		MemberCount: members, ProjectCount: projects, TaskCount: tasks,
	}
	if u, ok := h.store.FindUserByID(org.OwnerID); ok {
		v.OwnerEmail = u.Email
		v.OwnerName = u.Name
	}
	return v
}

// ListOrgs GET /api/v1/platform/orgs —— 企业列表（元数据），支持 keyword / status 过滤。
func (h *PlatformAdminHandler) ListOrgs(c *gin.Context) {
	keyword := strings.ToLower(strings.TrimSpace(c.Query("keyword")))
	status := strings.TrimSpace(c.Query("status"))

	orgs := h.store.ListEnterpriseOrganizations()
	items := make([]platformOrgView, 0, len(orgs))
	for _, org := range orgs {
		if status != "" && org.StatusOrActive() != status {
			continue
		}
		v := h.toOrgView(org)
		if keyword != "" &&
			!strings.Contains(strings.ToLower(v.Name), keyword) &&
			!strings.Contains(strings.ToLower(v.Slug), keyword) &&
			!strings.Contains(strings.ToLower(v.OwnerEmail), keyword) {
			continue
		}
		items = append(items, v)
	}
	transport.WriteList(c, items, len(items))
}

// GetOrg GET /api/v1/platform/orgs/:id —— 单个企业元数据。
func (h *PlatformAdminHandler) GetOrg(c *gin.Context) {
	org, appErr := h.store.GetOrganization(c.Param("id"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if org.Kind != model.OrgKindEnterprise {
		transport.WriteError(c, transport.NotFound("enterprise organization not found"))
		return
	}
	transport.WriteData(c, http.StatusOK, h.toOrgView(org))
}

type platformCreateOrgRequest struct {
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	OwnerEmail string `json:"owner_email"`
}

// CreateOrg POST /api/v1/platform/orgs —— 平台侧代开企业。
// 必须指定现有账号为 Owner（企业租户恒有唯一 Owner，避免产生无主租户）。
func (h *PlatformAdminHandler) CreateOrg(c *gin.Context) {
	var req platformCreateOrgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Slug = strings.ToLower(strings.TrimSpace(req.Slug))
	req.OwnerEmail = strings.TrimSpace(req.OwnerEmail)
	if req.Name == "" || req.OwnerEmail == "" {
		transport.WriteError(c, transport.Validation("name and owner_email are required", map[string]any{
			"name": "required", "owner_email": "required",
		}))
		return
	}
	if req.Slug == "" {
		req.Slug = orgSlugFrom(req.Name)
	}
	if !orgSlugRe.MatchString(req.Slug) {
		transport.WriteError(c, transport.Validation("slug: 3-64 chars, lowercase letters/digits/hyphens", map[string]any{"slug": req.Slug}))
		return
	}
	owner, ok := h.store.FindUserByEmail(req.OwnerEmail)
	if !ok {
		transport.WriteError(c, transport.NotFound("owner user not found"))
		return
	}
	org, appErr := h.store.CreateOrganization(owner.ID, req.Name, req.Slug, model.OrgKindEnterprise)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	recordAudit(c, h.store, model.AuditScopePlatform, model.AuditActionOrgCreate, model.AuditTargetOrg, org.ID,
		map[string]any{"name": org.Name, "slug": org.Slug, "owner_email": owner.Email})
	transport.WriteData(c, http.StatusCreated, h.toOrgView(org))
}

// DisableOrg / RestoreOrg POST /api/v1/platform/orgs/:id/{disable,restore}
//
// 禁用即拦截：被禁用企业的成员带该企业租户头的请求返回 403 ORG_DISABLED
// （见 middleware.OrgScope），恢复后即时生效；个人空间不受影响。
func (h *PlatformAdminHandler) DisableOrg(c *gin.Context) {
	h.setStatus(c, model.OrgStatusDisabled, model.AuditActionOrgDisable)
}

func (h *PlatformAdminHandler) RestoreOrg(c *gin.Context) {
	h.setStatus(c, model.OrgStatusActive, model.AuditActionOrgRestore)
}

func (h *PlatformAdminHandler) setStatus(c *gin.Context, status, action string) {
	orgID := c.Param("id")
	before, appErr := h.store.GetOrganization(orgID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if before.Kind != model.OrgKindEnterprise {
		transport.WriteError(c, transport.NotFound("enterprise organization not found"))
		return
	}
	org, appErr := h.store.SetOrganizationStatus(orgID, status)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	// 幂等：状态未变化时不落审计（避免重复点击产生噪音记录）。
	if before.StatusOrActive() != org.StatusOrActive() {
		recordAudit(c, h.store, model.AuditScopePlatform, action, model.AuditTargetOrg, org.ID,
			map[string]any{"from": before.StatusOrActive(), "to": org.StatusOrActive()})
	}
	transport.WriteData(c, http.StatusOK, h.toOrgView(org))
}

// GetConfig / PutConfig /api/v1/platform/config —— 全局配置（默认模型/配额/节点参数）。
func (h *PlatformAdminHandler) GetConfig(c *gin.Context) {
	transport.WriteData(c, http.StatusOK, gin.H{"config": h.store.GetPlatformGlobalConfig()})
}

// PutConfig 保存全局配置；配额变化只影响**新建**企业租户，不改动存量企业的配额。
func (h *PlatformAdminHandler) PutConfig(c *gin.Context) {
	var req model.PlatformGlobalConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	cfg, appErr := h.store.SetPlatformGlobalConfig(middleware.UserID(c), req)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	recordAudit(c, h.store, model.AuditScopePlatform, model.AuditActionPlatformConfigUpdate,
		model.AuditTargetPlatformConf, model.PlatformGlobalConfigID,
		map[string]any{"default_model": cfg.DefaultModel, "quota": cfg.Quota})
	transport.WriteData(c, http.StatusOK, gin.H{"config": cfg})
}

// ListAuditLogs GET /api/v1/platform/audit-logs —— 全局审计查询（本期仅平台侧可见）。
func (h *PlatformAdminHandler) ListAuditLogs(c *gin.Context) {
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			transport.WriteError(c, transport.Validation("limit: must be a non-negative integer", nil))
			return
		}
		limit = n
	}
	items := h.store.ListAuditLogs(model.AuditQuery{
		Scope:       strings.TrimSpace(c.Query("scope")),
		Action:      strings.TrimSpace(c.Query("action")),
		ActorUserID: strings.TrimSpace(c.Query("actor_user_id")),
		Limit:       limit,
	})
	transport.WriteList(c, items, len(items))
}

// Usage GET /api/v1/platform/usage —— 全平台用量总览（count 级聚合，不含业务内容）。
func (h *PlatformAdminHandler) Usage(c *gin.Context) {
	transport.WriteData(c, http.StatusOK, gin.H{"usage": h.store.PlatformUsageStats()})
}

// orgSlugFrom 从企业名派生 slug（与 handler.organization.go 的 Create 同规则）。
func orgSlugFrom(name string) string {
	slug := slugSanitizeRe.ReplaceAllString(strings.ToLower(name), "-")
	return strings.Trim(slug, "-")
}
