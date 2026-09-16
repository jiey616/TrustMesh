package model

import "time"

// 企业角色（设计文档 §5 自定义角色）。集合 org_roles：
//   - 每个租户创建时种子三个内置角色（owner/admin/member，Builtin=true）；
//   - 内置角色权限集**锁定不可改**，且始终以 internal/authz 的代码矩阵为准 ——
//     本文档里的 Permissions 只是自描述镜像（便于运维直接看集合内容），不参与裁决；
//   - 自定义角色由持有 org.role.mgr 的角色维护，权限集必须 ⊆ admin 全集。
//
// 权限解析链：org_memberships.role_id → org_roles → authz.Has（见 internal/authz）。

// OrgRole 一个企业角色的定义。
type OrgRole struct {
	ID          string   `json:"id" bson:"_id"`
	OrgID       string   `json:"org_id" bson:"org_id"`
	Name        string   `json:"name" bson:"name"`
	Permissions []string `json:"permissions" bson:"permissions"`
	Builtin     bool     `json:"builtin" bson:"builtin"`
	// BuiltinKey 内置角色的语义键（owner|admin|member）；自定义角色为空。
	// 解析内置角色权限集时以它索引代码矩阵 —— role_id / name 都可能被改名或重建，
	// 只有这个键是稳定的语义锚点。
	BuiltinKey string    `json:"builtin_key,omitempty" bson:"builtin_key,omitempty"`
	CreatedAt  time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" bson:"updated_at"`
}

// BuiltinRoleKeys 内置角色语义键，顺序即种子顺序与展示顺序（owner → admin → member）。
var BuiltinRoleKeys = []string{OrgRoleOwner, OrgRoleAdmin, OrgRoleMember}

// BuiltinRoleDisplayName 返回内置角色的展示名。
func BuiltinRoleDisplayName(key string) string {
	switch key {
	case OrgRoleOwner:
		return "所有者"
	case OrgRoleAdmin:
		return "管理员"
	case OrgRoleMember:
		return "成员"
	}
	return key
}

// IsBuiltinRoleKey 报告字符串是否是内置角色语义键。
func IsBuiltinRoleKey(key string) bool {
	for _, k := range BuiltinRoleKeys {
		if k == key {
			return true
		}
	}
	return false
}

// BuiltinOrgRoleID 返回内置角色在 org_roles 中的**确定性** ID。
//
// 确定性（而非随机）是刻意的：种子、启动跑批与 role_id 迁移三处都能 O(1) 判断
// 「该企业的内置角色是否已存在」「迁移目标 ID 是什么」，跑批天然幂等、无需扫描。
func BuiltinOrgRoleID(orgID, key string) string {
	return "role_" + orgID + "_" + key
}
