// Package authz 是权限体系的唯一权威定义点：权限点常量、内置角色权限矩阵、
// 请求级鉴权中间件。设计文档：docs/permission-system-design-2026-09-16.md
//
// 分层职责（正交）：
//   - authz 判「这个角色能不能做这类操作」（权限点，功能级）
//   - store/scope.go 判「这条数据归不归你可见」（数据行级裁决，零改动）
package authz

// ---------------------------------------------------------------------------
// 企业层权限点（设计文档 §3.1）
//
// 基础权限（仪表盘/AI办公室/项目与任务的浏览使用、知识库浏览、会议参与、
// 外部应用浏览等）为所有登录用户隐式拥有，不枚举、不参与角色配置；
// 下表只收「管理类」或「需按角色区分」的操作。
// ---------------------------------------------------------------------------

const (
	PermOrgSettings  = "org.settings"   // 企业信息/解散/菜单覆盖配置
	PermOrgMemberMgr = "org.member.mgr" // 邀请/移除/改角色（不可动 owner）
	PermOrgRoleMgr   = "org.role.mgr"   // 自定义角色管理

	PermProjectCreate = "project.create" // 新建项目
	PermProjectManage = "project.manage" // 归档/配置/成员白名单

	PermTaskCreate   = "task.create"   // 建任务
	PermTaskDispatch = "task.dispatch" // 指派 agent / 催办

	PermWorkflowTemplateMgr = "workflow.template.mgr" // 模板创建/复制/curate/删除/同步

	PermAgentView   = "agent.view"   // 数字员工列表/详情
	PermAgentManage = "agent.manage" // 增删改/技能/能力配置

	PermMarketBrowse  = "market.browse"  // 浏览岗位市场
	PermMarketInstall = "market.install" // 安装角色包

	PermMeetingManage   = "meeting.manage"   // 开始/结束/删除会议
	PermKnowledgeManage = "knowledge.manage" // 知识条目增删改（浏览为基础权限）

	PermJoinRequestApprove = "join_request.approve" // 数字员工加入审批

	PermOpsView   = "ops.view"   // 运维工单查看
	PermOpsManage = "ops.manage" // 运维工单处理
)

// ---------------------------------------------------------------------------
// 平台层权限点（设计文档 §3.2）
//
// 只授予 platform_admin（env 种子账号），绝不进入企业角色矩阵；
// 企业角色（含 owner）调 /api/v1/platform/* 一律 403，反之亦然。
// ---------------------------------------------------------------------------

const (
	PermPlatformOrgLifecycle = "platform.org.lifecycle" // 企业列表/详情/开通/禁用/恢复
	PermPlatformConfigRead   = "platform.config.read"   // 全局配置查看（含全局菜单基线）
	PermPlatformConfigWrite  = "platform.config.write"  // 全局配置修改（含全局菜单基线）
	PermPlatformAuditView    = "platform.audit.view"    // 全局审计日志查询
	PermPlatformUsageView    = "platform.usage.view"    // 全平台用量总览
	PermPlatformUserRead     = "platform.user.read"     // 用户列表/详情、企业成员查看
	PermPlatformUserManage   = "platform.user.manage"   // 用户重置密码/禁用/启用
)

// AllOrgPermissions 返回企业层全部权限点（= owner 权限集）。
// 步骤 4 自定义角色的勾选上限（≤ admin 全集）校验也以此为基础。
func AllOrgPermissions() []string {
	return append([]string(nil), ownerPerms...)
}

// PlatformPermissions 返回平台层全部权限点（= platform_admin 权限集）。
func PlatformPermissions() []string {
	return []string{
		PermPlatformOrgLifecycle,
		PermPlatformConfigRead,
		PermPlatformConfigWrite,
		PermPlatformAuditView,
		PermPlatformUsageView,
		PermPlatformUserRead,
		PermPlatformUserManage,
	}
}
