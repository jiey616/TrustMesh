package authz

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/middleware"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
)

// TestBuiltinRoleMatrix 把设计文档 §3.1 的权限矩阵**逐格钉死**：
// 18 个企业层权限点 × 3 个内置角色，每格的允许/拒绝都与设计表一致。
// 任何对 role.go 矩阵的改动若不同步设计文档，这里立刻变红。
func TestBuiltinRoleMatrix(t *testing.T) {
	// want[role][perm] = 设计文档 §3.1 表格逐格转录。
	want := map[string]map[string]bool{
		model.OrgRoleOwner: {
			PermOrgSettings: true, PermOrgMemberMgr: true, PermOrgRoleMgr: true,
			PermProjectCreate: true, PermProjectManage: true,
			PermTaskCreate: true, PermTaskDispatch: true,
			PermWorkflowTemplateMgr: true,
			PermAgentView:           true, PermAgentManage: true,
			PermMarketBrowse: true, PermMarketInstall: true,
			PermMeetingManage: true, PermKnowledgeManage: true,
			PermJoinRequestApprove: true,
			PermOrgAppMgr:          true,
			PermOpsView:            true, PermOpsManage: true,
		},
		model.OrgRoleAdmin: {
			PermOrgSettings: false, PermOrgMemberMgr: true, PermOrgRoleMgr: false,
			PermProjectCreate: true, PermProjectManage: true,
			PermTaskCreate: true, PermTaskDispatch: true,
			PermWorkflowTemplateMgr: true,
			PermAgentView:           true, PermAgentManage: true,
			PermMarketBrowse: true, PermMarketInstall: true,
			PermMeetingManage: true, PermKnowledgeManage: true,
			PermJoinRequestApprove: true,
			PermOrgAppMgr:          true,
			PermOpsView:            true, PermOpsManage: true,
		},
		model.OrgRoleMember: {
			PermOrgSettings: false, PermOrgMemberMgr: false, PermOrgRoleMgr: false,
			PermProjectCreate: true, PermProjectManage: false,
			PermTaskCreate: true, PermTaskDispatch: true,
			PermWorkflowTemplateMgr: false,
			PermAgentView:           true, PermAgentManage: false,
			PermMarketBrowse: true, PermMarketInstall: false,
			PermMeetingManage: false, PermKnowledgeManage: false,
			PermJoinRequestApprove: false,
			PermOrgAppMgr:          false,
			PermOpsView:            false, PermOpsManage: false,
		},
	}

	allPerms := AllOrgPermissions()
	if len(allPerms) != 18 {
		t.Fatalf("企业层权限点数量 = %d, want 18（设计文档 §3.1 + 2026-09-17 org.app.mgr）", len(allPerms))
	}

	for role, permWant := range want {
		got := BuiltinPermissions(role, false)
		if len(got) == 0 {
			t.Fatalf("role %q: 权限集为空", role)
		}
		for _, perm := range allPerms {
			w, ok := permWant[perm]
			if !ok {
				t.Fatalf("设计表缺少 role=%q perm=%q 的格子（矩阵未闭合）", role, perm)
			}
			if g := Has(got, perm); g != w {
				t.Errorf("role=%q perm=%q: Has = %v, want %v", role, perm, g, w)
			}
		}
	}
}

// TestLegacyMemberFallback 锁定 PERM_LEGACY_MEMBER=1 的回退语义：
// member 恢复收紧前能力（≈ admin 减成员管理），但组织管理三项仍不放开。
func TestLegacyMemberFallback(t *testing.T) {
	legacy := BuiltinPermissions(model.OrgRoleMember, true)

	// 收紧前 member 可用的能力必须全部恢复。
	for _, perm := range []string{
		PermProjectManage, PermWorkflowTemplateMgr, PermAgentManage,
		PermMarketInstall, PermMeetingManage, PermKnowledgeManage,
		PermJoinRequestApprove, PermOpsView, PermOpsManage,
	} {
		if !Has(legacy, perm) {
			t.Errorf("legacy member 应恢复 %q", perm)
		}
	}
	// 组织管理三项即便回退也不属于 member（收紧前后一致）。
	for _, perm := range []string{PermOrgSettings, PermOrgMemberMgr, PermOrgRoleMgr} {
		if Has(legacy, perm) {
			t.Errorf("legacy member 绝不应获得 %q", perm)
		}
	}

	// 收紧后（默认）这些能力不存在。
	strict := BuiltinPermissions(model.OrgRoleMember, false)
	for _, perm := range []string{PermAgentManage, PermOpsView, PermMarketInstall} {
		if Has(strict, perm) {
			t.Errorf("strict member 不应拥有 %q", perm)
		}
	}
}

// TestPlatformPermsIsolatedFromOrgRoles 守住平台/企业正交铁律（§2）：
// 任何内置企业角色（含 owner、含 legacy member）都不含平台层权限点。
func TestPlatformPermsIsolatedFromOrgRoles(t *testing.T) {
	roleSets := map[string][]string{
		model.OrgRoleOwner:              BuiltinPermissions(model.OrgRoleOwner, false),
		model.OrgRoleAdmin:              BuiltinPermissions(model.OrgRoleAdmin, false),
		model.OrgRoleMember:             BuiltinPermissions(model.OrgRoleMember, false),
		"legacy_" + model.OrgRoleMember: BuiltinPermissions(model.OrgRoleMember, true),
	}
	for role, perms := range roleSets {
		for _, pp := range PlatformPermissions() {
			if Has(perms, pp) {
				t.Errorf("角色 %q 绝不应包含平台权限点 %q", role, pp)
			}
		}
	}
}

func TestHas(t *testing.T) {
	perms := []string{"a", "b", "c"}
	if !Has(perms, "b") {
		t.Error("Has 应命中存在的权限点")
	}
	if Has(perms, "x") {
		t.Error("Has 不应命中不存在的权限点")
	}
	if Has(nil, "a") {
		t.Error("nil 集合必须默认拒绝")
	}
	if Has(perms, "") {
		t.Error("空权限点必须默认拒绝")
	}
}

// ---------------------------------------------------------------------------
// RequirePerm 中间件测试（复用 middleware 测试的探针模式：
// 注入 user_id → OrgScope → RequirePerm → 终末 handler）
// ---------------------------------------------------------------------------

type permProbe struct {
	status  int
	body    string
	reached bool // 终末 handler 是否被执行（false = 已被 403 拦截）
}

func newPermFixture(t *testing.T) (*store.Store, string) {
	t.Helper()
	s := store.New()
	org, appErr := s.CreateOrganization("u-owner", "Acme", "acme", model.OrgKindEnterprise)
	if appErr != nil {
		t.Fatalf("create org: %v", appErr)
	}
	if _, appErr := s.AddOrgMember(org.ID, "u-admin", model.OrgRoleAdmin); appErr != nil {
		t.Fatalf("add admin: %v", appErr)
	}
	if _, appErr := s.AddOrgMember(org.ID, "u-member", model.OrgRoleMember); appErr != nil {
		t.Fatalf("add member: %v", appErr)
	}
	return s, org.ID
}

// runPermProbe 以指定用户跑一条受 RequirePerm(perm) 保护的路由。
// orgHeader 为空模拟无租户头的存量客户端（个人空间语义）。
func runPermProbe(t *testing.T, st *store.Store, az *Authorizer, perm, userID, orgHeader string) permProbe {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var reached bool
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Next()
	})
	r.GET("/probe", middleware.OrgScope(st), az.RequirePerm(perm), func(c *gin.Context) {
		reached = true
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	if orgHeader != "" {
		req.Header.Set("X-Org-Id", orgHeader)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return permProbe{status: w.Code, body: w.Body.String(), reached: reached}
}

func TestRequirePerm(t *testing.T) {
	s, orgID := newPermFixture(t)
	az := NewAuthorizer(NewBuiltinResolver(false))

	cases := []struct {
		name       string
		perm       string
		userID     string
		orgHeader  string
		wantStatus int
		wantReach  bool
	}{
		{"owner 管理 agent 放行", PermAgentManage, "u-owner", orgID, http.StatusOK, true},
		{"admin 管理 agent 放行", PermAgentManage, "u-admin", orgID, http.StatusOK, true},
		{"member 管理 agent 拒绝", PermAgentManage, "u-member", orgID, http.StatusForbidden, false},
		{"member 查看 agent 放行", PermAgentView, "u-member", orgID, http.StatusOK, true},
		{"member 处理运维工单拒绝", PermOpsManage, "u-member", orgID, http.StatusForbidden, false},
		{"admin 改企业设置拒绝", PermOrgSettings, "u-admin", orgID, http.StatusForbidden, false},
		{"owner 改企业设置放行", PermOrgSettings, "u-owner", orgID, http.StatusOK, true},
		// 无租户头（个人空间/存量客户端）：等价 owner，管理操作放行。
		{"无租户头个人空间管理 agent 放行", PermAgentManage, "u-member", "", http.StatusOK, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := runPermProbe(t, s, az, c.perm, c.userID, c.orgHeader)
			if p.status != c.wantStatus {
				t.Fatalf("status = %d, want %d (body=%s)", p.status, c.wantStatus, p.body)
			}
			if p.reached != c.wantReach {
				t.Fatalf("handler reached = %v, want %v", p.reached, c.wantReach)
			}
			if c.wantStatus == http.StatusForbidden {
				// 403 契约：携带 FORBIDDEN code + 缺失的权限点名（便于前端排障）。
				if !strings.Contains(p.body, "FORBIDDEN") {
					t.Fatalf("403 响应必须暴露 code=FORBIDDEN, body=%s", p.body)
				}
				if !strings.Contains(p.body, c.perm) {
					t.Fatalf("403 响应应指明缺失的权限点 %q, body=%s", c.perm, p.body)
				}
			}
		})
	}
}

// TestRequirePermLegacyMemberSwitch 验证回滚开关在请求链路上真实生效。
func TestRequirePermLegacyMemberSwitch(t *testing.T) {
	s, orgID := newPermFixture(t)
	az := NewAuthorizer(NewBuiltinResolver(true))

	p := runPermProbe(t, s, az, PermAgentManage, "u-member", orgID)
	if p.status != http.StatusOK || !p.reached {
		t.Fatalf("legacy 开关下 member 管理 agent 应放行, status=%d body=%s", p.status, p.body)
	}
	// 但组织管理仍不放开。
	p = runPermProbe(t, s, az, PermOrgMemberMgr, "u-member", orgID)
	if p.status != http.StatusForbidden || p.reached {
		t.Fatalf("legacy 开关下 member 仍不得管理成员, status=%d", p.status)
	}
}

// TestBuiltinResolverUnknownRoleDenied 未知角色（含伪造角色串）必须默认拒绝。
func TestBuiltinResolverUnknownRoleDenied(t *testing.T) {
	resolve := NewBuiltinResolver(false)
	for _, role := range []string{"superuser", "root", "OWNER", " admin "} {
		if perms := resolve(store.Scope{UserID: "u", OrgID: "o", Role: role}); len(perms) != 0 {
			t.Errorf("未知角色 %q 必须解析为空权限集, got %v", role, perms)
		}
	}
	// System Scope 兜底放行（HTTP 拿不到 System，仅内部调用路径）。
	if perms := resolve(store.Scope{System: true}); !Has(perms, PermAgentManage) {
		t.Error("System Scope 应解析为企业层全集")
	}
}

// ---------------------------------------------------------------------------
// 步骤 4 解析链：membership.role_id → org_roles（NewStoreResolver）
// ---------------------------------------------------------------------------

// newStoreResolverFixture 造一个「真实 store 作 RoleSource」的租户：
// 内置角色模板从本包矩阵派生（生产由 app 层做同一件事，见 router.go）。
func newStoreResolverFixture(t *testing.T) (*store.Store, string) {
	t.Helper()
	s := store.New()
	s.SetBuiltinRoleTemplates(map[string][]string{
		model.OrgRoleOwner:  BuiltinPermissions(model.OrgRoleOwner, false),
		model.OrgRoleAdmin:  BuiltinPermissions(model.OrgRoleAdmin, false),
		model.OrgRoleMember: BuiltinPermissions(model.OrgRoleMember, false),
	})
	org, appErr := s.CreateOrganization("u-owner", "Acme", "acme", model.OrgKindEnterprise)
	if appErr != nil {
		t.Fatalf("create org: %v", appErr)
	}
	return s, org.ID
}

// TestStoreResolverCustomRole 自定义角色按自己的 permissions[] 裁决，
// 且不受 role_id 之外的任何字符串影响。
func TestStoreResolverCustomRole(t *testing.T) {
	s, orgID := newStoreResolverFixture(t)
	role, appErr := s.CreateOrgRole(orgID, "审计员", []string{PermAgentView, PermOpsView})
	if appErr != nil {
		t.Fatalf("create custom role: %v", appErr)
	}
	resolve := NewStoreResolver(s, false)

	// 兼容字段 Role 写 role_id 本身（自定义角色），解析以 role_id 为准。
	perms := resolve(store.Scope{UserID: "u", OrgID: orgID, Role: role.ID, RoleID: role.ID})
	if !Has(perms, PermOpsView) || !Has(perms, PermAgentView) {
		t.Fatalf("自定义角色权限集 = %v, 应含 %s/%s", perms, PermAgentView, PermOpsView)
	}
	if Has(perms, PermAgentManage) || Has(perms, PermOrgMemberMgr) {
		t.Fatalf("自定义角色不得越界, got %v", perms)
	}
}

// TestStoreResolverBuiltinRoleByID 内置角色即便有 role_id，仍走代码矩阵
// （集合里的 permissions 只是自描述镜像，不参与裁决）。
func TestStoreResolverBuiltinRoleByID(t *testing.T) {
	s, orgID := newStoreResolverFixture(t)
	adminID := model.BuiltinOrgRoleID(orgID, model.OrgRoleAdmin)

	resolve := NewStoreResolver(s, false)
	perms := resolve(store.Scope{UserID: "u", OrgID: orgID, Role: model.OrgRoleAdmin, RoleID: adminID})
	if !Has(perms, PermOrgMemberMgr) || !Has(perms, PermAgentManage) {
		t.Fatalf("admin 权限集 = %v, 应含成员管理与 agent 管理", perms)
	}
	if Has(perms, PermOrgSettings) || Has(perms, PermOrgRoleMgr) {
		t.Fatalf("admin 不得拥有 owner 专属权限, got %v", perms)
	}

	// 回滚开关同样经 role_id 分支生效（矩阵由代码决定，与集合内容无关）。
	legacy := NewStoreResolver(s, true)
	memberID := model.BuiltinOrgRoleID(orgID, model.OrgRoleMember)
	if p := legacy(store.Scope{UserID: "u", OrgID: orgID, Role: model.OrgRoleMember, RoleID: memberID}); !Has(p, PermAgentManage) {
		t.Errorf("legacy 开关下 member 应恢复 agent.manage, got %v", p)
	}
}

// TestStoreResolverFallbackChain role_id 缺失/悬空/跨租户时的回落语义：
// 回落内置矩阵（Scope.Role），绝不静默归零把人锁死，也绝不越权放行。
func TestStoreResolverFallbackChain(t *testing.T) {
	s, orgID := newStoreResolverFixture(t)
	other, appErr := s.CreateOrganization("u9", "Globex", "globex", model.OrgKindEnterprise)
	if appErr != nil {
		t.Fatalf("create other org: %v", appErr)
	}
	otherID := model.BuiltinOrgRoleID(other.ID, model.OrgRoleAdmin)
	resolve := NewStoreResolver(s, false)

	// 兼容期未迁移（role_id 为空）→ 按旧 role 字符串映射内置矩阵。
	if p := resolve(store.Scope{UserID: "u", OrgID: orgID, Role: model.OrgRoleAdmin}); !Has(p, PermOrgMemberMgr) {
		t.Errorf("role_id 为空应回落内置矩阵, got %v", p)
	}
	// 角色被删（悬空 role_id）且兼容字段是 role_id → 默认拒绝（而非全放行）。
	if p := resolve(store.Scope{UserID: "u", OrgID: orgID, Role: "role_deleted", RoleID: "role_deleted"}); len(p) != 0 {
		t.Errorf("悬空 role_id 应默认拒绝, got %v", p)
	}
	// 跨租户 role_id：不认他企业的角色，回落 Scope.Role（此处是 role_id → 拒绝）。
	if p := resolve(store.Scope{UserID: "u", OrgID: orgID, Role: otherID, RoleID: otherID}); len(p) != 0 {
		t.Errorf("跨租户 role_id 必须被拒, got %v", p)
	}
	// 无租户上下文（个人空间/存量客户端）→ owner 全集，与 NewBuiltinResolver 一致。
	if p := resolve(store.Scope{UserID: "u"}); !Has(p, PermOrgSettings) {
		t.Errorf("无租户上下文应等于 owner 全集, got %v", p)
	}
	// System → 企业层全集。
	if p := resolve(store.Scope{System: true}); !Has(p, PermOrgRoleMgr) {
		t.Errorf("System 应为企业层全集, got %v", p)
	}
}
