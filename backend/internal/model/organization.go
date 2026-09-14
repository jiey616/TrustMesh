package model

import "time"

// 多租户阶段 0：仅建模与骨架，现有业务判断逻辑全部保持原样。
// 设计文档：docs/multi-tenant-enterprise-plan.md

// OrgKind 区分个人租户与企业租户。
const (
	OrgKindPersonal   = "personal"
	OrgKindEnterprise = "enterprise"
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
	ID        string    `json:"id" bson:"_id"`
	Name      string    `json:"name" bson:"name"`
	Slug      string    `json:"slug" bson:"slug"`         // 唯一，预留域名/URL 隔离
	Kind      string    `json:"kind" bson:"kind"`         // personal | enterprise
	OwnerID   string    `json:"owner_id" bson:"owner_id"` // 唯一 Owner，可转让
	Quota     OrgQuota  `json:"quota" bson:"quota"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

// OrgMembership user ↔ org 多对多 + 角色。
type OrgMembership struct {
	ID       string    `json:"id" bson:"_id"`
	OrgID    string    `json:"org_id" bson:"org_id"`
	UserID   string    `json:"user_id" bson:"user_id"`
	Role     string    `json:"role" bson:"role"` // owner | admin | member
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
