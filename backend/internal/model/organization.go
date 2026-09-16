package model

import "time"

// 多租户阶段 0：仅建模与骨架，现有业务判断逻辑全部保持原样。
// 设计文档：docs/multi-tenant-enterprise-plan.md

// OrgKind 区分个人租户与企业租户。
const (
	OrgKindPersonal   = "personal"
	OrgKindEnterprise = "enterprise"
)

// 企业生命周期状态（平台侧「禁用/恢复」）。
// 空值按 active 处理（存量文档无 status 字段，语义不变）。
const (
	OrgStatusActive   = "active"
	OrgStatusDisabled = "disabled"
)

// OrgRole 成员角色（Owner 唯一，可转让）。
const (
	OrgRoleOwner  = "owner"
	OrgRoleAdmin  = "admin"
	OrgRoleMember = "member"
)

// ProjectMemberRole 项目级成员角色（分支 C：不设成员 = 全员可见）。
const (
	ProjectMemberRoleViewer = "viewer"
	ProjectMemberRoleEditor = "editor"
)

// OrgQuota 只留字段与上限校验，不做计费（分支 F）。
type OrgQuota struct {
	MaxMembers      int   `json:"max_members" bson:"max_members"`             // -1 = 不限
	MaxNodes        int   `json:"max_nodes" bson:"max_nodes"`                 // -1 = 不限
	MaxProjects     int   `json:"max_projects" bson:"max_projects"`           // -1 = 不限
	MaxStorageBytes int64 `json:"max_storage_bytes" bson:"max_storage_bytes"` // -1 = 不限
}

// Organization 租户实体。
type Organization struct {
	ID      string   `json:"id" bson:"_id"`
	Name    string   `json:"name" bson:"name"`
	Slug    string   `json:"slug" bson:"slug"`         // 唯一，预留域名/URL 隔离
	Kind    string   `json:"kind" bson:"kind"`         // personal | enterprise
	OwnerID string   `json:"owner_id" bson:"owner_id"` // 唯一 Owner，可转让
	Quota   OrgQuota `json:"quota" bson:"quota"`
	Status  string   `json:"status,omitempty" bson:"status,omitempty"` // active | disabled（空 = active）
	// MenuOverrides 企业级菜单覆盖（只能缩小）：被点名的菜单键对全员隐藏。
	// 只影响前端菜单可见性，不影响 API 鉴权（设计文档 §4）。
	MenuOverrides []string  `json:"menu_overrides,omitempty" bson:"menu_overrides,omitempty"`
	CreatedAt     time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" bson:"updated_at"`
}

// IsDisabled 报告租户是否被平台侧禁用（空 status 视为启用，兼容存量文档）。
func (o *Organization) IsDisabled() bool {
	return o != nil && o.Status == OrgStatusDisabled
}

// StatusOrActive 返回落库状态，空值归一为 active。
func (o *Organization) StatusOrActive() string {
	if o == nil || o.Status == "" {
		return OrgStatusActive
	}
	return o.Status
}

// OrgMembership user ↔ org 多对多 + 角色。
type OrgMembership struct {
	ID     string `json:"id" bson:"_id"`
	OrgID  string `json:"org_id" bson:"org_id"`
	UserID string `json:"user_id" bson:"user_id"`
	// Role 是角色语义串（兼容字段，见 RoleID）：
	//   - 内置角色 → owner | admin | member；
	//   - 自定义角色 → 该角色的 role_id（保持非空，避免被任何「按内置角色串判断」
	//     的存量分支误命中；权限裁决一律先看 RoleID）。
	Role string `json:"role" bson:"role"`
	// RoleID 是 org_roles 的角色引用（权威字段，设计文档 §5）。
	// 空 = 兼容期尚未迁移，权限解析回落按 Role 字符串映射内置角色。
	RoleID   string    `json:"role_id,omitempty" bson:"role_id,omitempty"`
	JoinedAt time.Time `json:"joined_at" bson:"joined_at"`
}

// ProjectMember 项目级成员白名单。空名单 = 全员可见（分支 C）。
type ProjectMember struct {
	ID        string    `json:"id" bson:"_id"`
	ProjectID string    `json:"project_id" bson:"project_id"`
	UserID    string    `json:"user_id" bson:"user_id"`
	Role      string    `json:"role" bson:"role"` // viewer | editor
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}

// DefaultOrgQuota 返回默认配额（全部不限，二期接计费后再收紧）。
func DefaultOrgQuota() OrgQuota {
	return OrgQuota{MaxMembers: -1, MaxNodes: -1, MaxProjects: -1, MaxStorageBytes: -1}
}
