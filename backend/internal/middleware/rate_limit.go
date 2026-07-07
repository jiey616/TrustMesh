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
	globalRateLimit    = 100            // max requests per second globally
	ipRateLimit        = 20             // max requests per second per IP
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

	ips     map[string]*ipEntry
	lastCleanup time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		ips: make(map[string]*ipEntry),
	}
}

// allow checks if a request from the given IP should be allowed.
func (rl *rateLimiter) allow(ip string) (allowed bool, reason string) {
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

	// Check global rate limit
	if rl.globalCount >= globalRateLimit {
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

	if entry.count >= ipRateLimit {
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

		allowed, reason := limiter.allow(ip)
		if !allowed {
			logger.Warn("rate limit exceeded",
				zap.String("ip", ip),
				zap.String("path", c.Request.URL.Path),
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
