package middleware

import (
	"net/http"
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
//   - 带了 X-Org-Id 但用户不属于该租户 → 401 + code=NOT_A_MEMBER（防止伪租户头越权）。
//     使用独立 code（而非 token 过期所用的 UNAUTHORIZED），使客户端能靠 code 天然区分
//     「租户越权」与「token 过期」，不会误触发 refresh 重放。
func OrgScope(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := UserID(c)
		sc := store.Scope{UserID: userID}

		orgID := strings.TrimSpace(c.GetHeader("X-Org-Id"))
		if orgID != "" && st != nil {
			m, ok := st.GetMembership(orgID, userID)
			if !ok {
				transport.WriteError(c, transport.OrgScopeDenied("not a member of the requested organization"))
				c.Abort()
				return
			}
			// 企业生命周期（权限体系 §6.3）：被平台侧禁用的企业，其成员带该企业
			// 租户头的请求一律 403。个人空间（不带该头）不受影响，数据仍可见。
			if org, appErr := st.GetOrganization(orgID); appErr == nil && org.IsDisabled() {
				transport.WriteError(c, &transport.AppError{
					Status:  http.StatusForbidden,
					Code:    "ORG_DISABLED",
					Message: "organization is disabled by platform admin",
					Details: map[string]any{"org_id": orgID},
				})
				c.Abort()
				return
			}
			sc.OrgID = orgID
			sc.Role = m.Role
			sc.RoleID = m.RoleID
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
