package authz

import (
	"strings"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/middleware"
	"trustmesh/backend/internal/transport"
)

// 平台层的两道中间件（设计文档 §6.3）。两条链**正交**：
//   - RequirePlatformPerm：/api/v1/platform/* 只认平台管理员标记，不看企业角色；
//   - RequireBusinessAccount：业务 API 反向拒绝平台管理员（仅种子模式下生效）。
//
// 平台管理员的判定由 app 层注入（store.UserIsPlatformAdmin），authz 不反向依赖 store。

// PlatformAdminChecker 报告某用户是否平台管理员。
type PlatformAdminChecker func(userID string) bool

// accountRoutePrefixes 是种子模式下平台管理员仍可访问的**账号与会话类**业务路由前缀。
//
// 它们不是企业业务数据，而是任何登录身份都要用的外壳能力：个人信息页、工作区列表
// （前端侧边栏切换器）、通知、实时事件流。设计文档 §4 的「平台管理员只看到平台管理
// 菜单组 + 用户区」依赖这些接口可用，否则外壳直接白屏。
//
// /api/v1/llm-config 同样放行：它是 LLM 连接测试/模型列表/个人空间配置的共用入口，
// 平台管理页（/platform/llm-config）保存前要调它测连通性；企业层配置挂在
// /api/v1/organizations/:id/llm-config 下，已被上面的 organizations 前缀覆盖。
var accountRoutePrefixes = []string{
	"/api/v1/users",
	"/api/v1/organizations",
	"/api/v1/notifications",
	"/api/v1/events",
	"/api/v1/llm-config",
}

// RequirePlatformPerm 平台命名空间鉴权：先确认平台管理员标记，再校验平台权限点。
//
// perm 必须是 authz 定义的平台权限点（清单测试 TestRoutePermManifestValuesValid
// 会校验），显式声明让「谁放行了哪个平台能力」可从路由表直接读出。
func RequirePlatformPerm(perm string, isPlatformAdmin PlatformAdminChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := middleware.UserID(c)
		if isPlatformAdmin == nil || !isPlatformAdmin(userID) {
			transport.WriteError(c, transport.Forbidden("platform admin only"))
			c.Abort()
			return
		}
		if !Has(PlatformPermissions(), perm) {
			transport.WriteError(c, transport.Forbidden("missing platform permission: "+perm))
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireBusinessAccount 业务 API 的反向门禁：种子模式下平台管理员不得调用业务 API。
//
// strict=false（未配置 PLATFORM_ADMIN_EMAILS）时恒放行 —— 此时平台管理员是历史
// 自动提升的账号，很可能同时是日常业务使用者，硬拒绝会把他锁在业务之外。
// 账号/会话类路由（accountRoutePrefixes）始终放行，见其注释。
func RequireBusinessAccount(strict bool, isPlatformAdmin PlatformAdminChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strict || isPlatformAdmin == nil {
			c.Next()
			return
		}
		path := c.Request.URL.Path
		for _, prefix := range accountRoutePrefixes {
			if strings.HasPrefix(path, prefix) {
				c.Next()
				return
			}
		}
		if isPlatformAdmin(middleware.UserID(c)) {
			transport.WriteError(c, transport.Forbidden(
				"platform admin account cannot access business APIs; use /api/v1/platform/* or a regular account"))
			c.Abort()
			return
		}
		c.Next()
	}
}
