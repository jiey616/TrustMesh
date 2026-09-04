package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

// 多租户阶段 0：只做 Scope 的解析与注入，不参与任何业务裁决。
// 设计文档：docs/multi-tenant-enterprise-plan.md §4.4

const scopeKey = "org_scope"

// OrgScope 在 RequireAuth 之后解析活跃租户：
//   - 未带 X-Org-Id（存量客户端一律如此）→ 只注入 UserID，行为与改造前完全一致
//   - 带了 X-Org-Id 且用户是该租户成员 → 注入完整 Scope（含 OrgID / Role）
//   - 带了 X-Org-Id 但用户不属于该租户 → 401（防止伪租户头越权）
func OrgScope(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := UserID(c)
		sc := store.Scope{UserID: userID}

		orgID := strings.TrimSpace(c.GetHeader("X-Org-Id"))
		if orgID != "" && st != nil {
			m, ok := st.GetMembership(orgID, userID)
			if !ok {
				transport.WriteError(c, transport.Unauthorized("not a member of the requested organization"))
				c.Abort()
				return
			}
			sc.OrgID = orgID
			sc.Role = m.Role
		}

		c.Set(scopeKey, sc)
		c.Next()
	}
}

// Scope 取本次请求的归属上下文。
//
// 只跑了 RequireAuth（设置 user_id）而没跑 OrgScope 时，退回 user-only Scope ——
// 这与「未带 X-Org-Id 的存量客户端」语义完全一致，保证任何拿到 userID 的请求
// 都能得到一个可用的 Scope，而不是零值导致 handler 直接 401。
func Scope(c *gin.Context) store.Scope {
	if v, ok := c.Get(scopeKey); ok {
		if sc, ok := v.(store.Scope); ok {
			return sc
		}
	}
	return store.Scope{UserID: UserID(c)}
}
