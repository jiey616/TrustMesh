package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// 读路径放宽阈值（2026-09-28）的回归护栏。
//
// 背景（生产 48h 日志实测）：135 次 429 **全部是 ip rate limit exceeded**，
// 排除探针后仍有 6 个真实用户 IP 命中 105 次，路径清一色 `/projects/*`(74) 与
// `/tasks/*`(53) —— 全是「打开一个页面就并发拉一批」的读接口，也正是「待人工
// 确认」链路依赖的那几个（被限 ⇒ 查询失败 ⇒ UI 静默停在旧数据，用户看到的是
// 「确认完了框还在」）。
//
// 这里钉死两条：① 读路径额度明显高于写路径；② 写路径额度**不变**（写有副作用，
// 收紧才有意义）。两条都硬编码常量比对，避免有人日后「顺手」把两边一起调大。

func newRateLimitRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimit(zap.NewNop()))
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	r.GET("/read", ok)
	// gin 不像 net/http 的 ServeMux 那样把 HEAD 自动映射到 GET，必须显式注册，
	// 否则 HEAD 请求 404、根本测不到「HEAD 与 GET 同档」。
	r.HEAD("/read", ok)
	r.POST("/write", ok)
	return r
}

// hit 连续发 n 次请求，返回 (放行数, 被限数)。httptest 的 RemoteAddr 固定，
// 所以这 n 次都算同一个 ClientIP。
func hit(t *testing.T, r *gin.Engine, method, path string, n int) (allowed, limited int) {
	t.Helper()
	for i := 0; i < n; i++ {
		req := httptest.NewRequest(method, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		switch w.Code {
		case http.StatusOK:
			allowed++
		case http.StatusTooManyRequests:
			limited++
		default:
			t.Fatalf("%s %s: unexpected status %d", method, path, w.Code)
		}
	}
	return allowed, limited
}

func TestRateLimitReadPathIsMorePermissiveThanWrite(t *testing.T) {
	if readIPRateLimit <= ipRateLimit {
		t.Fatalf("readIPRateLimit=%d 必须大于 ipRateLimit=%d（否则放宽毫无意义）",
			readIPRateLimit, ipRateLimit)
	}
	if readGlobalRateLimit <= globalRateLimit {
		t.Fatalf("readGlobalRateLimit=%d 必须大于 globalRateLimit=%d",
			readGlobalRateLimit, globalRateLimit)
	}

	// 写路径：用完 ipRateLimit 额度后开始 429，额度不得变化。
	wAllowed, wLimited := hit(t, newRateLimitRouter(), http.MethodPost, "/write", ipRateLimit+5)
	if wAllowed != ipRateLimit {
		t.Fatalf("写路径放行 %d 次，want %d（写侧阈值不应被改动）", wAllowed, ipRateLimit)
	}
	if wLimited != 5 {
		t.Fatalf("写路径被限 %d 次，want 5", wLimited)
	}

	// 读路径：同样次数应全部放行（额度更高）。
	rAllowed, rLimited := hit(t, newRateLimitRouter(), http.MethodGet, "/read", ipRateLimit+5)
	if rAllowed != ipRateLimit+5 || rLimited != 0 {
		t.Fatalf("读路径放行 %d / 被限 %d，want 全放行（读额度=%d）", rAllowed, rLimited, readIPRateLimit)
	}
}

func TestRateLimitReadPathStillEnforcedEventually(t *testing.T) {
	allowed, limited := hit(t, newRateLimitRouter(), http.MethodGet, "/read", readIPRateLimit+10)
	if allowed != readIPRateLimit {
		t.Fatalf("读路径放行 %d 次，want %d（放宽 ≠ 无限）", allowed, readIPRateLimit)
	}
	if limited != 10 {
		t.Fatalf("读路径被限 %d 次，want 10", limited)
	}
}

func TestRateLimitHeadCountsAsRead(t *testing.T) {
	allowed, limited := hit(t, newRateLimitRouter(), http.MethodHead, "/read", ipRateLimit+5)
	if allowed != ipRateLimit+5 || limited != 0 {
		t.Fatalf("HEAD 放行 %d / 被限 %d，want 全放行（HEAD 应与 GET 同档）", allowed, limited)
	}
}

func TestRateLimit429CarriesRetryAfter(t *testing.T) {
	r := newRateLimitRouter()
	hit(t, r, http.MethodPost, "/write", ipRateLimit) // 打满额度

	req := httptest.NewRequest(http.MethodPost, "/write", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}
	// 客户端（ky）靠这个头算退避；缺了它会变成盲目重试。
	if got := w.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
}
