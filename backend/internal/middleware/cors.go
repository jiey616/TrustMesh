package middleware

import "github.com/gin-gonic/gin"

func CORS(allowAll bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if allowAll {
			c.Header("Access-Control-Allow-Origin", "*")
		}
		c.Header("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")
		// 🔴 X-Org-Id 必须放行：多租户头，前端（含 Capacitor 原生壳 https://localhost
		// 这类跨源环境）每个鉴权请求都带 ⇒ 缺了它预检必败，全部业务查询被浏览器拦截。
		c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Org-Id")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
