package authz

import "trustmesh/backend/internal/model"

// 内置三角色权限矩阵（设计文档 §3.1）。
//
// 内置角色权限集**锁定不可改**（保证 owner/admin/member 三档语义稳定，
// 避免 owner 把 member 改成隐形 admin）；需要差异化就走企业自定义角色（步骤 4）。
//
// 矩阵的正确性由 authz_test.go 的 TestBuiltinRoleMatrix 全量钉死 ——
// 改动这里的任何一行都必须同步改设计文档，否则测试立刻变红。
var (
	// owner：企业层全部权限点（18 个）。
	ownerPerms = []string{
		PermOrgSettings,
		PermOrgMemberMgr,
		PermOrgRoleMgr,
		PermProjectCreate,
		PermProjectManage,
		PermTaskCreate,
		PermTaskDispatch,
		PermWorkflowTemplateMgr,
		PermAgentView,
		PermAgentManage,
		PermMarketBrowse,
		PermMarketInstall,
		PermMeetingManage,
		PermKnowledgeManage,
		PermJoinRequestApprove,
		PermOrgAppMgr,
		PermOpsView,
		PermOpsManage,
	}

	// admin：owner 全集减去 org.settings / org.role.mgr（16 个）。
	adminPerms = []string{
		PermOrgMemberMgr,
		PermProjectCreate,
		PermProjectManage,
		PermTaskCreate,
		PermTaskDispatch,
		PermWorkflowTemplateMgr,
		PermAgentView,
		PermAgentManage,
		PermMarketBrowse,
		PermMarketInstall,
		PermMeetingManage,
		PermKnowledgeManage,
		PermJoinRequestApprove,
		PermOrgAppMgr,
		PermOpsView,
		PermOpsManage,
	}

	// member（收紧后）：只保留日常生产所需的显式权限点（5 个）。
	// 收紧是本期唯一改变现有用户行为的点，已确认生产无 member 使用
	// agent管理/运维/审批/安装等能力（设计文档 §0）。
	memberPerms = []string{
		PermProjectCreate,
		PermTaskCreate,
		PermTaskDispatch,
		PermAgentView,
		PermMarketBrowse,
	}

	// legacyMemberPerms：收紧前的 member 旧语义 —— 与 admin 仅差成员管理
	// （即 owner 全集减去 org.settings / org.role.mgr / org.member.mgr）。
	// 仅在 PERM_LEGACY_MEMBER=1 回滚开关下生效（设计文档 §9）。
	legacyMemberPerms = []string{
		PermProjectCreate,
		PermProjectManage,
		PermTaskCreate,
		PermTaskDispatch,
		PermWorkflowTemplateMgr,
		PermAgentView,
		PermAgentManage,
		PermMarketBrowse,
		PermMarketInstall,
		PermMeetingManage,
		PermKnowledgeManage,
		PermJoinRequestApprove,
		PermOpsView,
		PermOpsManage,
	}
)

// BuiltinPermissions 返回内置角色的权限点集合；未知角色返回 nil（默认拒绝）。
// legacyMember=true 时 member 回退到收紧前语义。
//
// 返回值是包内共享切片，调用方**只读**，禁止修改（要可写副本请用
// AllOrgPermissions 或自行拷贝）。
func BuiltinPermissions(role string, legacyMember bool) []string {
	switch role {
	case model.OrgRoleOwner:
		return ownerPerms
	case model.OrgRoleAdmin:
		return adminPerms
	case model.OrgRoleMember:
		if legacyMember {
			return legacyMemberPerms
		}
		return memberPerms
	}
	return nil
}
