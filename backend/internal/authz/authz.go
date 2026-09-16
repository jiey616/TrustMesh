package authz

import (
	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/middleware"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

// Has 报告权限点集合是否包含指定权限点。集合规模小（≤ 20），线性扫描即可。
func Has(perms []string, perm string) bool {
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}

// PermResolver 按请求 Scope 实时解析权限点集合。
//
// 🔴 实现方必须每次请求实时读取权威数据源（membership / org_roles），
// 禁止跨请求进程内缓存 —— T3.1 多实例下角色变更必须即时生效（设计文档 §8.6）。
type PermResolver func(sc store.Scope) []string

// Authorizer 是路由鉴权入口：持有权限解析器，产出 RequirePerm 中间件。
// 一个进程一个实例，在 app/router.go 构造后供全路由表复用。
type Authorizer struct {
	resolve PermResolver
}

func NewAuthorizer(resolve PermResolver) *Authorizer {
	return &Authorizer{resolve: resolve}
}

// Resolve 暴露实时解析结果（供 /users/me 等需要回传 permissions[] 的接口使用）。
func (a *Authorizer) Resolve(sc store.Scope) []string {
	return a.resolve(sc)
}

// NewBuiltinResolver 返回只识别内置三角色的解析器（步骤 1；自定义角色
// role_id 的 store 版解析器在步骤 4 接入，签名不变）：
//
//   - System Scope → 企业层全集：HTTP 路径永远拿不到 System（org_scope_test.go
//     已钉死该不变量），这里仅为 agent webhook / 内部定时器等系统调用兜底；
//   - Scope.Role 为空（无租户头的存量客户端 / 个人空间）→ owner 全集：
//     个人空间内用户对自己的数据历来拥有全部操作权，数据隔离由 scope.go 保证；
//   - 内置角色 → 对应矩阵权限集；
//   - 未知角色 → nil（默认拒绝）。
func NewBuiltinResolver(legacyMember bool) PermResolver {
	return func(sc store.Scope) []string {
		if sc.System {
			return AllOrgPermissions()
		}
		if sc.Role == "" {
			return ownerPerms
		}
		return BuiltinPermissions(sc.Role, legacyMember)
	}
}

// RoleSource 是企业角色数据源（由 store.Store 实现）：步骤 4 的
// 「membership.role_id → org_roles」解析链所需的最小能力。
//
// 只暴露按 ID 取角色，不暴露「按角色串查内置角色」——内置角色的权限集
// 永远由代码矩阵决定（设计文档 §5：内置权限集锁定不可改），集合里存的那份
// Permissions 只是自描述镜像，不参与裁决。
type RoleSource interface {
	// RoleByID 按 ID 取角色定义（只读副本）；不存在返回 (nil, false)。
	RoleByID(roleID string) (*model.OrgRole, bool)
}

// NewStoreResolver 步骤 4 的解析链（设计文档 §5）：
//
//	membership.role_id → org_roles.permissions[]
//
// 与 NewBuiltinResolver 的差异只在「带租户上下文」这一分支：
//   - role_id 命中内置角色 → 仍走代码矩阵（权限集锁定，见 RoleSource 注释）；
//   - role_id 命中自定义角色 → 用该角色自己的 permissions[]；
//   - role_id 为空（兼容期未迁移）或角色已被删除 → 回落内置矩阵（Scope.Role 字符串），
//     保证迁移期间与角色误删之后不会「权限静默归零」把人锁死。
//
// System 与个人空间（无租户上下文）的语义与 NewBuiltinResolver 完全一致。
func NewStoreResolver(src RoleSource, legacyMember bool) PermResolver {
	return func(sc store.Scope) []string {
		if sc.System {
			return AllOrgPermissions()
		}
		if !sc.HasOrg() {
			return ownerPerms
		}
		if sc.RoleID != "" && src != nil {
			if role, ok := src.RoleByID(sc.RoleID); ok && role.OrgID == sc.OrgID {
				if role.Builtin {
					return BuiltinPermissions(role.BuiltinKey, legacyMember)
				}
				return role.Permissions
			}
		}
		return BuiltinPermissions(sc.Role, legacyMember)
	}
}

// RequirePerm 声明路由所需权限点：实时解析请求者权限集，缺失即 403。
//
// 只判功能级权限；数据行级裁决（能看到哪条数据）仍在 store 层 scope.go，
// 二者正交。未标注权限点的路由视为基础权限（登录即可用）——「默认拒绝」
// 的防漏标机制随步骤 2 的路由收敛一并落地（设计文档 §9）。
func (a *Authorizer) RequirePerm(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if Has(a.resolve(middleware.Scope(c)), perm) {
			c.Next()
			return
		}
		transport.WriteError(c, transport.Forbidden("missing permission: "+perm))
		c.Abort()
	}
}
