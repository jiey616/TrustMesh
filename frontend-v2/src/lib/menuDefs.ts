import { PERM } from './perms'

/**
 * 菜单声明（设计文档 §4）：每个菜单项声明 `perm`（权限点）或 `anyPerms`（任一命中），
 * 两者皆无 = 基础权限（登录即见，不受角色影响）。
 *
 * 这里只放「键 / 标题 / 权限绑定」，图标等展示细节留在 MainLayout。
 */
export interface MenuDef {
  key: string
  label: string
  perm?: string
  anyPerms?: readonly string[]
}

/**
 * 业务菜单（平台管理员一律不可见）。
 *
 * 注意：`/organizations`（组织管理）与 `/external-apps`（外部应用）**不在**侧边栏，
 * 它们的入口统一收在「个人信息」页（ProfilePage），路由本身保持不变（深链仍可用）。
 */
export const BUSINESS_MENUS: MenuDef[] = [
  { key: '/dashboard', label: '仪表盘' },
  { key: '/office', label: 'AI 办公室' },
  { key: '/projects', label: '项目' },
  { key: '/workflows', label: '工作流' },
  { key: '/agents', label: '数字员工', perm: PERM.AGENT_VIEW },
  { key: '/meetings', label: '会议' },
  { key: '/knowledge', label: '知识库' },
  { key: '/market', label: '市场', perm: PERM.MARKET_BROWSE },
  { key: '/ops', label: '运维工单', perm: PERM.OPS_VIEW },
]

/** 平台管理菜单组（仅平台管理员可见，设计文档 §3.2）。 */
export const PLATFORM_MENUS: MenuDef[] = [
  { key: '/platform/orgs', label: '组织管理' },
  { key: '/platform/users', label: '用户管理' },
  { key: '/platform/config', label: '全局配置' },
  { key: '/platform/external-apps', label: '外部应用' },
  { key: '/platform/desktop-releases', label: '桌面端安装包' },
  { key: '/platform/audit', label: '审计日志' },
  { key: '/platform/usage', label: '用量总览' },
]

/** 企业可隐藏的菜单（菜单覆盖只能缩小可见性；API 鉴权不受影响）。 */
export const HIDEABLE_MENUS: MenuDef[] = BUSINESS_MENUS

export interface MenuFilterInput {
  permissions: readonly string[]
  menuOverrides: readonly string[]
  isPlatformAdmin: boolean
  /** 权限视图是否已就绪；未就绪时 fail-open（不隐藏任何菜单，避免界面被锁死） */
  ready: boolean
}

/**
 * 菜单可见性：`visible = hasPerm(menu.perm) && !menuOverrides.includes(menu.key)`。
 *
 * 平台管理员只看到平台菜单组（他调业务 API 会被 403，界面不应诱导点击）；
 * 权限视图未就绪（/users/me 尚未返回）时 fail-open，由后端 API 兜底鉴权。
 */
export function visibleMenus(menus: MenuDef[], input: MenuFilterInput): MenuDef[] {
  const { permissions, menuOverrides, isPlatformAdmin, ready } = input
  if (!ready) return menus.filter((m) => !menuOverrides.includes(m.key))
  return menus.filter((m) => {
    if (m.perm && !permissions.includes(m.perm)) return false
    if (m.anyPerms && !m.anyPerms.some((p) => permissions.includes(p))) return false
    if (menuOverrides.includes(m.key)) return false
    return !isPlatformAdmin || PLATFORM_MENUS.some((p) => p.key === m.key)
  })
}