package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	sseHeartbeatInterval = 15 * time.Second
	sseMaxDuration       = 30 * time.Minute // auto-disconnect after 30 min to prevent stale connections
)

func beginSSE(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	// 清除 WriteTimeout，允许 SSE 长连接
	// 必须在 WriteHeaderNow 之前调用（一旦 HTTP 头发送，SetWriteDeadline 可能不生效）
	rc := http.NewResponseController(c.Writer)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		// 某些 ResponseWriter 可能不支持 SetWriteDeadline，记录但不中断
		// 在生产环境中，这可能导致 SSE 连接在 WriteTimeout 后断开
	}
}

func streamEvents[T any](c *gin.Context, updates <-chan T) {
	beginSSE(c)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	// Force-write HTTP headers immediately so the client's fetch() resolves.
	// Without this, Gin defers header writing until the first body write,
	// causing streams with no initial data (e.g. user event stream) to hang.
	c.Writer.WriteHeaderNow()
	flusher.Flush()

	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()

	// Max duration timer — client should reconnect after this.
	// This prevents stale SSE connections from accumulating.
	maxDuration := time.NewTimer(sseMaxDuration)
	defer maxDuration.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-maxDuration.C:
			// Send a close event so the client knows to reconnect
			c.SSEvent("close", gin.H{"reason": "max_duration_reached"})
			flusher.Flush()
			return
		case update, ok := <-updates:
			if !ok {
				return
			}
			c.SSEvent("snapshot", update)
			flusher.Flush()
		case <-heartbeat.C:
			c.SSEvent("ping", gin.H{"ts": time.Now().UTC()})
			flusher.Flush()
		}
	}
}
