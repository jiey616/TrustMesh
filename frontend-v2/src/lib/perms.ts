/**
 * 权限点常量（与后端 `internal/authz/permission.go` 一一对应，改动必须两边同步）。
 *
 * 前端权限只用于**界面投影**（菜单可见性、按钮显隐、路由兜底），
 * 唯一权威是后端 API 鉴权 —— 任何越权最终都会被 403 挡住，
 * 前端过滤不构成安全边界（设计文档 §4）。
 */

export const PERM = {
  ORG_SETTINGS: 'org.settings',
  ORG_MEMBER_MGR: 'org.member.mgr',
  ORG_ROLE_MGR: 'org.role.mgr',

  PROJECT_CREATE: 'project.create',
  PROJECT_MANAGE: 'project.manage',

  TASK_CREATE: 'task.create',
  TASK_DISPATCH: 'task.dispatch',

  WORKFLOW_TEMPLATE_MGR: 'workflow.template.mgr',

  AGENT_VIEW: 'agent.view',
  AGENT_MANAGE: 'agent.manage',

  MARKET_BROWSE: 'market.browse',
  MARKET_INSTALL: 'market.install',

  MEETING_MANAGE: 'meeting.manage',
  KNOWLEDGE_MANAGE: 'knowledge.manage',

  JOIN_REQUEST_APPROVE: 'join_request.approve',

  /** 组织级外部应用管理（2026-09-17 三级作用域）：仅 owner/admin，个人级不受约束 */
  ORG_APP_MGR: 'org.app.mgr',

  OPS_VIEW: 'ops.view',
  OPS_MANAGE: 'ops.manage',
} as const

/** 平台层权限点（只授予 platform_admin，绝不进入企业角色矩阵）。 */
export const PLATFORM_PERM = {
  ORG_LIFECYCLE: 'platform.org.lifecycle',
  CONFIG_READ: 'platform.config.read',
  CONFIG_WRITE: 'platform.config.write',
  AUDIT_VIEW: 'platform.audit.view',
  USAGE_VIEW: 'platform.usage.view',
  /** 全局级外部应用管理（/api/v1/platform/external-apps） */
  EXTAPP_MGR: 'platform.extapp.mgr',
} as const

/** 权限点集合是否包含指定权限点（集合规模 ≤ 20，线性扫描足够）。 */
export function hasPerm(perms: readonly string[], perm: string): boolean {
  return perms.includes(perm)
}

/** 权限点集合是否命中候选中的任意一个（菜单/路由的「任一命中即可见」语义）。 */
export function hasAnyPerm(perms: readonly string[], candidates: readonly string[]): boolean {
  return candidates.some((p) => perms.includes(p))
}

/**
 * 「组织管理」入口/路由的可见条件：组织域任一权限点（设计文档 §4）。
 * admin 有 org.member.mgr 所以能进，页内「组织设置」标签再按 org.settings 单独隐藏。
 *
 * 入口位置：不在左侧主菜单，收在「个人信息」页（ProfilePage）与用户下拉菜单。
 */
export const ORG_DOMAIN_PERMS = [
  PERM.ORG_SETTINGS,
  PERM.ORG_MEMBER_MGR,
  PERM.ORG_ROLE_MGR,
] as const

/** 角色管理勾选矩阵的分组元数据（自定义角色只能勾选 admin 全集的子集）。 */
export const PERM_GROUPS: { group: string; items: { perm: string; label: string }[] }[] = [
  {
    group: '组织',
    items: [
      { perm: PERM.ORG_SETTINGS, label: '组织设置（信息/菜单可见性）' },
      { perm: PERM.ORG_MEMBER_MGR, label: '成员管理（邀请/移除/改角色）' },
      { perm: PERM.ORG_ROLE_MGR, label: '角色管理' },
    ],
  },
  {
    group: '项目与任务',
    items: [
      { perm: PERM.PROJECT_CREATE, label: '新建项目' },
      { perm: PERM.PROJECT_MANAGE, label: '项目归档/配置/成员白名单' },
      { perm: PERM.TASK_CREATE, label: '新建任务' },
      { perm: PERM.TASK_DISPATCH, label: '指派数字员工/催办' },
      { perm: PERM.WORKFLOW_TEMPLATE_MGR, label: '工作流模板管理' },
    ],
  },
  {
    group: '数字员工与市场',
    items: [
      { perm: PERM.AGENT_VIEW, label: '数字员工查看' },
      { perm: PERM.AGENT_MANAGE, label: '数字员工管理' },
      { perm: PERM.MARKET_BROWSE, label: '岗位市场浏览' },
      { perm: PERM.MARKET_INSTALL, label: '岗位市场安装' },
      { perm: PERM.JOIN_REQUEST_APPROVE, label: '入职审批' },
    ],
  },
  {
    group: '会议与知识库',
    items: [
      { perm: PERM.MEETING_MANAGE, label: '会议管理（开始/结束/删除）' },
      { perm: PERM.KNOWLEDGE_MANAGE, label: '知识条目管理' },
    ],
  },
  {
    group: '外部应用',
    items: [
      { perm: PERM.ORG_APP_MGR, label: '组织级外部应用管理（新建/编辑/删除）' },
    ],
  },
  {
    group: '运维',
    items: [
      { perm: PERM.OPS_VIEW, label: '运维工单查看' },
      { perm: PERM.OPS_MANAGE, label: '运维工单处理' },
    ],
  },
]

/** 权限点 → 中文名（无分组元数据时的兜底展示）。 */
export const PERM_LABELS: Record<string, string> = Object.fromEntries(
  PERM_GROUPS.flatMap((g) => g.items.map((i) => [i.perm, i.label])),
)