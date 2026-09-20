package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newCORSTestServer(allowAll bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS(allowAll))
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	return r
}

// 预检必须放行前端实际使用的全部自定义头——尤其 X-Org-Id（多租户必需头）。
// 缺了它，Capacitor 原生壳（origin https://localhost）等跨源环境下
// 所有带 X-Org-Id 的请求都会被浏览器在预检阶段拦截（真机实测 204+ERR_FAILED，
// 页面表现为"暂无可用空间"）。
func TestCORSPreflightAllowsXOrgId(t *testing.T) {
	r := newCORSTestServer(true)
	req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "https://localhost")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "Authorization,Content-Type,X-Org-Id")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want *", got)
	}
	allowHeaders := w.Header().Get("Access-Control-Allow-Headers")
	for _, h := range []string{"Authorization", "Content-Type", "X-Org-Id"} {
		if !strings.Contains(allowHeaders, h) {
			t.Fatalf("Access-Control-Allow-Headers = %q, missing %q", allowHeaders, h)
		}
	}
}

// 实际请求也要带上 ACAO（跨源响应否则不可读），且非 OPTIONS 不被 abort。
func TestCORSActualRequestCarriesAllowOrigin(t *testing.T) {
	r := newCORSTestServer(true)
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://localhost")
	req.Header.Set("Authorization", "Bearer x")
	req.Header.Set("X-Org-Id", "org_test")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want *", got)
	}
	if w.Body.String() != "pong" {
		t.Fatalf("body = %q, want pong", w.Body.String())
	}
}

// allowAll=false 时不发 ACAO（与既有行为保持一致，防回归）。
func TestCORSDisabledOmitsAllowOrigin(t *testing.T) {
	r := newCORSTestServer(false)
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://localhost")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}
