package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
)

// T0.0 测试前置：middleware 包此前零测试文件，而 OrgScope（org_scope.go）
// 正是多租户隔离的第一道裁决闸门 —— 非成员带 X-Org-Id 必须 401。
// 本文件用 gin.New() + httptest 注入 header 验证裁决，不依赖真实 HTTP 服务。

// scopeProbe 记录一次 OrgScope 裁决的可断言结果。
type scopeProbe struct {
	status  int         // HTTP 状态码
	body    string      // 响应体（用于断言错误信息）
	aborted bool        // c.IsAborted()
	scope   store.Scope // 注入到上下文的归属上下文
	reached bool        // 终末 handler 是否被执行（false = 已被拦截）
}

// runOrgScopeProbe 用最小 gin 链路跑一次 OrgScope 裁决：
//
//	injectUser（模拟 RequireAuth 已解析出 user_id）→ OrgScope → 终末 handler
func runOrgScopeProbe(t *testing.T, st *store.Store, userID, orgHeader string) scopeProbe {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var (
		got     store.Scope
		reached bool
		ctx     *gin.Context
	)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", userID)
		// defer 在 c.Next() 返回后执行：此时 OrgScope 已裁决完毕，
		// 可以准确断言 IsAborted()（在 OrgScope 之前断言恒为 false）。
		defer func() { ctx = c }()
		c.Next()
	})
	r.GET("/probe", OrgScope(st), func(c *gin.Context) {
		reached = true
		got = Scope(c)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	if orgHeader != "" {
		req.Header.Set("X-Org-Id", orgHeader)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	p := scopeProbe{status: w.Code, body: w.Body.String(), scope: got, reached: reached}
	if ctx != nil {
		p.aborted = ctx.IsAborted()
	}
	return p
}

// newOrgScopeFixture 建一个企业租户 orgA：u1 为创建者，u2 为显式成员。
func newOrgScopeFixture(t *testing.T) (*store.Store, string) {
	t.Helper()
	s := store.New()
	orgA, appErr := s.CreateOrganization("u1", "Acme", "acme", model.OrgKindEnterprise)
	if appErr != nil {
		t.Fatalf("create org: %v", appErr)
	}
	if _, appErr := s.AddOrgMember(orgA.ID, "u2", model.OrgRoleMember); appErr != nil {
		t.Fatalf("add member: %v", appErr)
	}
	return s, orgA.ID
}

// TestOrgScopeTenantIsolation 覆盖 OrgScope 的四条关键路径。
func TestOrgScopeTenantIsolation(t *testing.T) {
	s, orgA := newOrgScopeFixture(t)

	cases := []struct {
		name      string
		userID    string
		orgHeader string
		wantCode  int
		wantOrgID string
		wantRole  string
		wantAbort bool
		wantErrIn string
	}{
		{
			name:      "成员带租户头_注入OrgID与Role",
			userID:    "u2",
			orgHeader: orgA,
			wantCode:  http.StatusOK,
			wantOrgID: orgA,
			wantRole:  model.OrgRoleMember,
			wantAbort: false,
		},
		{
			name:      "非成员带租户头_必须401并拦截",
			userID:    "u3",
			orgHeader: orgA,
			wantCode:  http.StatusUnauthorized,
			wantAbort: true,
			wantErrIn: "not a member",
		},
		{
			name:      "伪租户头_必须401并拦截",
			userID:    "u2",
			orgHeader: "org-bogus-does-not-exist",
			wantCode:  http.StatusUnauthorized,
			wantAbort: true,
			wantErrIn: "not a member",
		},
		{
			name:      "无租户头存量客户端_仅注入UserID不得401",
			userID:    "u1",
			orgHeader: "",
			wantCode:  http.StatusOK,
			wantOrgID: "",
			wantAbort: false,
		},
		{
			name:      "空白租户头_按无租户头处理",
			userID:    "u1",
			orgHeader: "   ",
			wantCode:  http.StatusOK,
			wantOrgID: "",
			wantAbort: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := runOrgScopeProbe(t, s, c.userID, c.orgHeader)

			if p.status != c.wantCode {
				t.Fatalf("status = %d, want %d (body=%s)", p.status, c.wantCode, p.body)
			}
			if p.aborted != c.wantAbort {
				t.Fatalf("IsAborted = %v, want %v", p.aborted, c.wantAbort)
			}
			if c.wantErrIn != "" && !strings.Contains(p.body, c.wantErrIn) {
				t.Fatalf("body %q must contain %q", p.body, c.wantErrIn)
			}
			// 被拦截的请求绝不允许落到业务 handler。
			if c.wantAbort && p.reached {
				t.Fatal("aborted request must NOT reach the downstream handler")
			}
			if !c.wantAbort {
				if !p.reached {
					t.Fatalf("non-aborted request must reach the handler (body=%s)", p.body)
				}
				if p.scope.UserID != c.userID {
					t.Fatalf("scope.UserID = %q, want %q", p.scope.UserID, c.userID)
				}
				if p.scope.OrgID != c.wantOrgID {
					t.Fatalf("scope.OrgID = %q, want %q", p.scope.OrgID, c.wantOrgID)
				}
				if p.scope.Role != c.wantRole {
					t.Fatalf("scope.Role = %q, want %q", p.scope.Role, c.wantRole)
				}
			}
		})
	}
}

// TestOrgScopeNeverGrantsSystemFromHTTP 守住一条安全不变量：
// HTTP 路径永远拿不到 System Scope（scope.go 注释：System 只能由 store 内部
// 与 clawsynapse webhook 构造）。一旦 HTTP 能拿到 System=true，等于给越权留后门。
func TestOrgScopeNeverGrantsSystemFromHTTP(t *testing.T) {
	s, orgA := newOrgScopeFixture(t)

	for _, hdr := range []string{"", orgA} {
		p := runOrgScopeProbe(t, s, "u2", hdr)
		if p.status != http.StatusOK {
			t.Fatalf("header %q: status = %d, want 200 (body=%s)", hdr, p.status, p.body)
		}
		if p.scope.System {
			t.Fatal("HTTP request must never obtain a System scope")
		}
	}
}
