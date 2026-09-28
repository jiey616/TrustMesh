package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Rate limiter configuration.
const (
	globalRateLimit = 100 // max requests per second globally
	ipRateLimit     = 20  // max requests per second per IP

	// 读路径（GET/HEAD）单独放宽（2026-09-28）。
	// 依据（生产 48h 日志实测）：135 次 429 **全部是 ip rate limit exceeded**（0 次全局），
	// 排除探针后仍有 6 个真实用户 IP 命中 105 次，路径清一色是 `/projects/*`(74) 与
	// `/tasks/*`(53) —— 也就是「打开一个页面就并发拉一批」的读接口，正是「待人工确认」
	// 链路依赖的那几个（被限 ⇒ 查询失败 ⇒ UI 静默停留在旧数据）。
	// 写请求（POST/PATCH/DELETE）才是有副作用、值得收紧的那部分，阈值保持不变。
	// 两者共用同一计数器，只按请求类型选阈值，不引入第二套计数。
	readGlobalRateLimit = 300
	readIPRateLimit     = 60

	rateLimitWindow    = 1 * time.Second // sliding window size
	rateLimitCleanupMs = 5 * time.Minute // cleanup interval for stale IP entries
)

// ipEntry tracks request count for a single IP within the current window.
type ipEntry struct {
	count     int
	windowEnd time.Time
}

// rateLimiter provides a simple sliding-window rate limiter.
type rateLimiter struct {
	mu sync.Mutex

	globalCount int
	globalEnd   time.Time

	ips         map[string]*ipEntry
	lastCleanup time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		ips: make(map[string]*ipEntry),
	}
}

// allow checks if a request from the given IP should be allowed.
// read 为 true 时使用放宽后的读路径阈值（GET/HEAD）。
func (rl *rateLimiter) allow(ip string, read bool) (allowed bool, reason string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Periodic cleanup of stale IP entries
	if now.Sub(rl.lastCleanup) > rateLimitCleanupMs {
		for k, v := range rl.ips {
			if now.After(v.windowEnd) {
				delete(rl.ips, k)
			}
		}
		rl.lastCleanup = now
	}

	// Reset global window if expired
	if now.After(rl.globalEnd) {
		rl.globalCount = 0
		rl.globalEnd = now.Add(rateLimitWindow)
	}

	gLimit, iLimit := globalRateLimit, ipRateLimit
	if read {
		gLimit, iLimit = readGlobalRateLimit, readIPRateLimit
	}

	// Check global rate limit
	if rl.globalCount >= gLimit {
		return false, "global rate limit exceeded"
	}

	// Check per-IP rate limit
	entry, exists := rl.ips[ip]
	if !exists || now.After(entry.windowEnd) {
		entry = &ipEntry{
			count:     0,
			windowEnd: now.Add(rateLimitWindow),
		}
		rl.ips[ip] = entry
	}

	if entry.count >= iLimit {
		return false, "ip rate limit exceeded"
	}

	// Allow the request
	rl.globalCount++
	entry.count++
	return true, ""
}

// RateLimit returns a gin middleware that enforces rate limiting.
func RateLimit(logger *zap.Logger) gin.HandlerFunc {
	limiter := newRateLimiter()

	return func(c *gin.Context) {
		ip := c.ClientIP()
		// 读路径（GET/HEAD）走放宽阈值：打开一个页面会并发拉若干读接口，
		// 而写请求才有副作用、值得收紧（见上方常量注释）。
		read := c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead

		allowed, reason := limiter.allow(ip, read)
		if !allowed {
			logger.Warn("rate limit exceeded",
				zap.String("ip", ip),
				zap.String("path", c.Request.URL.Path),
				zap.Bool("read", read),
				zap.String("reason", reason),
			)
			c.Header("Retry-After", "1")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":  "RATE_LIMIT_EXCEEDED",
				"detail": "请求过于频繁，请稍后重试",
			})
			return
		}

		c.Next()
	}
}
