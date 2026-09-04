package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/middleware"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

// currentScope 取本次请求的归属上下文（含活跃租户）。
// 未带 X-Org-Id 时 Scope.OrgID 为空，store 侧一律退回 user 维度裁决，
// 行为与改造前完全一致。
func currentScope(c *gin.Context) (store.Scope, bool) {
	sc := middleware.Scope(c)
	if sc.UserID == "" {
		transport.WriteError(c, &transport.AppError{Status: http.StatusUnauthorized, Code: "UNAUTHORIZED", Message: "missing auth context", Details: map[string]any{}})
		return store.Scope{}, false
	}
	return sc, true
}

func currentUserID(c *gin.Context) (string, bool) {
	uid := middleware.UserID(c)
	if uid == "" {
		transport.WriteError(c, &transport.AppError{Status: http.StatusUnauthorized, Code: "UNAUTHORIZED", Message: "missing auth context", Details: map[string]any{}})
		return "", false
	}
	return uid, true
}
