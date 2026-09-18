package authz

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// 本文件锁定平台层的两道门禁（设计文档 §6.3 的两条正交通道）：
//   - RequirePlatformPerm：/api/v1/platform/* 只认平台管理员标记，不看企业角色；
//   - RequireBusinessAccount：种子模式下业务 API 反向拒绝平台管理员（白名单除外）。
//
// 平台管理员的判定在 app 层由 store.UserIsPlatformAdmin 注入，这里用桩函数模拟。

// runGateProbe 以最小 gin 链路跑一次门禁裁决：注入 user_id → 中间件 → 终末 handler。
func runGateProbe(t *testing.T, mw gin.HandlerFunc, userID, path string) permProbe {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var reached bool
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Next()
	})
	r.GET(path, mw, func(c *gin.Context) {
		reached = true
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return permProbe{status: w.Code, body: w.Body.String(), reached: reached}
}

// platformAdminStub 只把 "u-platform" 视为平台管理员。
func platformAdminStub(userID string) bool { return userID == "u-platform" }

func TestRequirePlatformPerm(t *testing.T) {
	cases := []struct {
		name       string
		perm       string
		userID     string
		checker    PlatformAdminChecker
		wantStatus int
		wantReach  bool
	}{
		{"平台管理员+平台权限点放行", PermPlatformOrgLifecycle, "u-platform", platformAdminStub, http.StatusOK, true},
		{"平台管理员+审计权限点放行", PermPlatformAuditView, "u-platform", platformAdminStub, http.StatusOK, true},
		// 桌面端发行版权限点（docs/desktop-app-update-plan-2026-09-18.md）。
		// 「平台管理员放行」这条同时守住一个易漏点：权限点必须真的登记进
		// PlatformPermissions()，否则第 49 行的兜底校验会把平台管理员自己也 403 掉。
		{"平台管理员+桌面发行版权限点放行", PermPlatformDesktopRelease, "u-platform", platformAdminStub, http.StatusOK, true},
		{"企业 owner 调桌面发行版拒绝", PermPlatformDesktopRelease, "u-owner", platformAdminStub, http.StatusForbidden, false},
		{"企业 owner 拒绝（平台命名空间不认企业角色）", PermPlatformOrgLifecycle, "u-owner", platformAdminStub, http.StatusForbidden, false},
		{"未登录用户拒绝", PermPlatformOrgLifecycle, "", platformAdminStub, http.StatusForbidden, false},
		{"checker 为 nil 时默认拒绝", PermPlatformOrgLifecycle, "u-platform", nil, http.StatusForbidden, false},
		// 企业层权限点绝不能在平台命名空间被放行（正交铁律）。
		{"平台管理员+企业权限点拒绝", PermAgentManage, "u-platform", platformAdminStub, http.StatusForbidden, false},
		{"未知权限点默认拒绝", "platform.does.not.exist", "u-platform", platformAdminStub, http.StatusForbidden, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := runGateProbe(t, RequirePlatformPerm(c.perm, c.checker), c.userID, "/api/v1/platform/orgs")
			if p.status != c.wantStatus {
				t.Fatalf("status = %d, want %d (body=%s)", p.status, c.wantStatus, p.body)
			}
			if p.reached != c.wantReach {
				t.Fatalf("handler reached = %v, want %v", p.reached, c.wantReach)
			}
			if c.wantStatus == http.StatusForbidden && !strings.Contains(p.body, "FORBIDDEN") {
				t.Fatalf("403 必须暴露 code=FORBIDDEN, body=%s", p.body)
			}
		})
	}
}

// TestRequireBusinessAccount 锁定种子模式下的严格分离：
// 平台管理员调业务 API 被 403，账号/会话类白名单前缀放行，普通用户不受影响。
func TestRequireBusinessAccount(t *testing.T) {
	cases := []struct {
		name       string
		strict     bool
		userID     string
		checker    PlatformAdminChecker
		path       string
		wantStatus int
	}{
		// strict=false（未配置 PLATFORM_ADMIN_EMAILS）：不做反向拒绝，存量环境零行为变化。
		{"非种子模式平台管理员调业务API放行", false, "u-platform", platformAdminStub, "/api/v1/projects", http.StatusOK},
		// strict=true：业务 API 一律拒绝。
		{"种子模式平台管理员调业务API拒绝", true, "u-platform", platformAdminStub, "/api/v1/projects", http.StatusForbidden},
		{"种子模式平台管理员调agent拒绝", true, "u-platform", platformAdminStub, "/api/v1/agents", http.StatusForbidden},
		{"种子模式平台管理员调运维拒绝", true, "u-platform", platformAdminStub, "/api/v1/ops/incidents", http.StatusForbidden},
		// 白名单：外壳能力与配置页共用入口必须放行，否则前端外壳直接白屏。
		{"白名单_用户资料放行", true, "u-platform", platformAdminStub, "/api/v1/users/me", http.StatusOK},
		{"白名单_工作区列表放行", true, "u-platform", platformAdminStub, "/api/v1/organizations", http.StatusOK},
		{"白名单_通知放行", true, "u-platform", platformAdminStub, "/api/v1/notifications", http.StatusOK},
		{"白名单_实时事件流放行", true, "u-platform", platformAdminStub, "/api/v1/events/stream", http.StatusOK},
		{"白名单_LLM连通性测试放行", true, "u-platform", platformAdminStub, "/api/v1/llm-config/test", http.StatusOK},
		// 普通用户在任何模式下都不受影响。
		{"种子模式普通用户调业务API放行", true, "u-owner", platformAdminStub, "/api/v1/projects", http.StatusOK},
		{"checker 为 nil 时恒放行", true, "u-platform", nil, "/api/v1/projects", http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := runGateProbe(t, RequireBusinessAccount(c.strict, c.checker), c.userID, c.path)
			if p.status != c.wantStatus {
				t.Fatalf("status = %d, want %d (body=%s)", p.status, c.wantStatus, p.body)
			}
			if p.status == http.StatusOK && !p.reached {
				t.Fatalf("放行请求必须到达 handler (body=%s)", p.body)
			}
			if c.wantStatus == http.StatusForbidden {
				if p.reached {
					t.Fatal("被拒绝的请求不得落到下游 handler")
				}
				if !strings.Contains(p.body, "FORBIDDEN") {
					t.Fatalf("403 必须暴露 code=FORBIDDEN, body=%s", p.body)
				}
			}
		})
	}
}
