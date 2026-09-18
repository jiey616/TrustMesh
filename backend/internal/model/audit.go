package model

import "time"

// 审计日志（设计文档 §6.4）：企业敏感操作与平台操作各落一条，
// 本期查询入口只在平台侧开放（企业侧审计页后置）。
//
// 存储：audit_logs 集合，created_at 上的 TTL 索引保留 180 天（见 store/mongo_state.go）。
// 追加写、只读展示，不参与全内存状态机（无 FlushPersistAll 镜像）。

// AuditScopePlatform 平台级审计的 scope 值；企业级审计的 scope 为该企业 org_id。
const AuditScopePlatform = "platform"

// 审计 action 枚举（首版，设计文档 §6.4）。
const (
	AuditActionOrgCreate  = "org.create"
	AuditActionOrgDisable = "org.disable"
	AuditActionOrgRestore = "org.restore"

	AuditActionOrgMemberAdd        = "org.member.add"
	AuditActionOrgMemberRemove     = "org.member.remove"
	AuditActionOrgMemberRoleChange = "org.member.role_change"

	AuditActionOrgRoleCreate = "org.role.create" // 步骤 4（自定义角色）落此三态
	AuditActionOrgRoleUpdate = "org.role.update"
	AuditActionOrgRoleDelete = "org.role.delete"

	AuditActionOrgMenuOverrideUpdate = "org.menu_override.update"

	AuditActionProjectArchive = "project.archive"
	AuditActionAgentDelete    = "agent.delete"

	AuditActionPlatformConfigUpdate = "platform.config.update"
	AuditActionPlatformAdminLogin   = "platform.admin.login"

	// 平台用户管理（平台管理员视角的账号运维动作）。
	AuditActionUserPasswordReset = "user.password.reset"
	AuditActionUserDisable       = "user.disable"
	AuditActionUserEnable        = "user.enable"

	// 桌面端发行版管理（docs/desktop-app-update-plan-2026-09-18.md）：
	// 上传 / 发布（指针前移）/ 回滚（指针后移）/ 删除。
	// 发布与回滚都要留证 —— 「为什么某台机器拿到了旧版本」的答案只在这条审计里。
	AuditActionDesktopReleaseUpload   = "desktop_release.upload"
	AuditActionDesktopReleasePublish  = "desktop_release.publish"
	AuditActionDesktopReleaseRollback = "desktop_release.rollback"
	AuditActionDesktopReleaseDelete   = "desktop_release.delete"
)

// 审计目标类型（target_type 枚举）。
const (
	AuditTargetOrg           = "org"
	AuditTargetMembership    = "membership"
	AuditTargetUser          = "user"
	AuditTargetRole          = "role"
	AuditTargetProject       = "project"
	AuditTargetAgent         = "agent"
	AuditTargetPlatformConf  = "platform_config"
	AuditTargetPlatformLogin = "platform_login"
	// AuditTargetDesktopRelease 的 target_id 用**版本号**而不是文档 ID：
	// 排查时的检索键是「用户在界面上看到的 0.3.0」，不是内部 24 位随机串。
	AuditTargetDesktopRelease = "desktop_release"
)

// AuditLog 一条审计记录。
type AuditLog struct {
	ID          string         `json:"id" bson:"_id"`
	ActorUserID string         `json:"actor_user_id" bson:"actor_user_id"`
	ActorEmail  string         `json:"actor_email" bson:"actor_email"`
	Scope       string         `json:"scope" bson:"scope"` // platform | org_id
	Action      string         `json:"action" bson:"action"`
	TargetType  string         `json:"target_type" bson:"target_type"`
	TargetID    string         `json:"target_id" bson:"target_id"`
	Detail      map[string]any `json:"detail,omitempty" bson:"detail,omitempty"`
	IP          string         `json:"ip" bson:"ip"`
	CreatedAt   time.Time      `json:"created_at" bson:"created_at"`
}

// AuditQuery 审计查询过滤条件（平台侧）。
type AuditQuery struct {
	Scope       string // 空 = 不过滤
	Action      string // 空 = 不过滤
	ActorUserID string // 空 = 不过滤
	Limit       int    // <=0 取默认值（100），上限 500
}
