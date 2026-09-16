import { create } from 'zustand'
import { BUSINESS_MENUS, visibleMenus, type MenuDef, type MenuFilterInput } from '@/lib/menuDefs'

/**
 * 权限投影 store（设计文档 §7）。
 *
 * 🔴 这里只做**界面投影**：菜单可见性、按钮显隐、路由兜底。
 * 唯一权威是后端 per-request 鉴权（权限集按 `X-Org-Id` 实时解析、多实例即时生效），
 * 前端绝不据此放行任何写操作。
 *
 * 不持久化：权限视图是运行时数据，只由 `/users/me` 灌入（登录 / 切工作区后刷新），
 * 登出时复位为空。
 */
interface PermState extends MenuFilterInput {
  setPermView: (v: {
    permissions: string[]
    menu_overrides: string[]
    is_platform_admin: boolean
  }) => void
  reset: () => void
  /** 是否拥有权限点；权限视图未就绪时 fail-open（由后端 403 兜底） */
  hasPerm: (perm: string) => boolean
  /** 是否拥有任一权限点 */
  hasAnyPerm: (perms: readonly string[]) => boolean
  /** 业务菜单按权限点 + 企业覆盖过滤后的结果 */
  visibleBusinessMenus: () => MenuDef[]
}

const emptyView = {
  ready: false,
  permissions: [] as string[],
  menuOverrides: [] as string[],
  isPlatformAdmin: false,
}

export const usePermStore = create<PermState>()((set, get) => ({
  ...emptyView,
  setPermView: (v) =>
    set({
      ready: true,
      permissions: v.permissions ?? [],
      menuOverrides: v.menu_overrides ?? [],
      isPlatformAdmin: !!v.is_platform_admin,
    }),
  reset: () => set({ ...emptyView }),
  hasPerm: (perm) => {
    const s = get()
    if (!s.ready) return true
    return s.permissions.includes(perm)
  },
  hasAnyPerm: (perms) => {
    const s = get()
    if (!s.ready) return true
    return perms.some((p) => s.permissions.includes(p))
  },
  visibleBusinessMenus: () => {
    const { permissions, menuOverrides, isPlatformAdmin, ready } = get()
    return visibleMenus(BUSINESS_MENUS, { permissions, menuOverrides, isPlatformAdmin, ready })
  },
}))