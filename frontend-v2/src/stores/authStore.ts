import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { migratePersistedAuth, parseWorkspaceMemory, type PersistedAuthV1 } from '@/lib/workspaceMemory'
import type { User, WorkspaceKind, WorkspaceMemory } from '@/types'

/**
 * 认证 / 租户上下文 store。
 *
 * 设计核心：**「记忆」与「运行时权威租户态」彻底分离**（见
 * docs/remember-last-workspace-design-2026-09-16.md §3.3(b)）。
 *
 *   - 运行时权威态 `activeOrgId` / `personalOrgId`：**绝不持久化**，恢复前恒为 null。
 *     唯一写点见下方各 action 注释（INV-2）。
 *   - 记忆 `workspaceMemory`：持久化（按 userId 维度），仅在通过 `userId` 守门 +
 *     本账号 orgs 校验（`resolveWorkspaceTarget`）后才被采用——它只是提示数据。
 */
interface AuthState {
  accessToken: string | null
  refreshToken: string | null
  user: User | null
  /** 运行时权威态：当前会话【已校验】的活跃企业租户；null = 个人空间。绝不持久化。 */
  activeOrgId: string | null
  /** 运行时权威态：本账号个人租户 id，由本账号 orgs 水合。绝不持久化。 */
  personalOrgId: string | null
  /** 记忆（提示数据）：按 userId 维度记录上次选中的空间；校验通过后才被采用。 */
  workspaceMemory: WorkspaceMemory | null
  setActiveOrg: (orgId: string | null) => void
  setPersonalOrgId: (orgId: string | null) => void
  setAuth: (accessToken: string, refreshToken: string, user: User) => void
  setUser: (user: User) => void
  setTokens: (accessToken: string, refreshToken: string) => void
  /** 记录当前登录用户「上次选中的空间」；未登录（无 userId）时忽略。 */
  rememberWorkspace: (kind: WorkspaceKind, orgId?: string | null) => void
  logout: () => void
  isAuthenticated: () => boolean
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      accessToken: null,
      refreshToken: null,
      user: null,
      activeOrgId: null,
      personalOrgId: null,
      workspaceMemory: null,
      setActiveOrg: (orgId) => set({ activeOrgId: orgId }),
      setPersonalOrgId: (orgId) => set({ personalOrgId: orgId }),
      // R1：换账号不继承租户上下文——两个运行时 id 一律复位为 null。
      setAuth: (accessToken, refreshToken, user) =>
        set({ accessToken, refreshToken, user, activeOrgId: null, personalOrgId: null }),
      setUser: (user) => set({ user }),
      setTokens: (accessToken, refreshToken) => set({ accessToken, refreshToken }),
      // 记忆写入带 userId 守门：只写当前登录用户的记忆（R3 落到存储层）。
      rememberWorkspace: (kind, orgId) => {
        const uid = get().user?.id
        if (!uid) return
        if (kind === 'enterprise') {
          if (!orgId) return
          set({ workspaceMemory: { userId: uid, kind, orgId } })
          return
        }
        set({ workspaceMemory: { userId: uid, kind: 'personal' } })
      },
      // R2：回归既有契约——登出清空两个运行时 id（否则下次登录会继承上一账号的租户）。
      // workspaceMemory 刻意保留：它是「经校验的提示数据」、按 userId 隔离，不构成运行时授权。
      logout: () =>
        set({
          accessToken: null,
          refreshToken: null,
          user: null,
          activeOrgId: null,
          personalOrgId: null,
        }),
      isAuthenticated: () => !!get().refreshToken,
    }),
    {
      name: 'trustmesh-v2:auth',
      version: 1,
      // INV-1：只输出 { refreshToken, user, workspaceMemory }，运行时两 id 绝不落盘。
      partialize: (state): PersistedAuthV1 => ({
        refreshToken: state.refreshToken,
        user: state.user,
        workspaceMemory: state.workspaceMemory,
      }),
      // 存量迁移：结构性剥离旧 activeOrgId/personalOrgId，并（可）合成按 userId 的记忆。
      migrate: (persisted, version) => migratePersistedAuth(persisted, version),
      // INV-1：无条件把运行时两 id 置 null——任何来源（旧脏值 / 篡改 / rehydrate 时序）
      // 都无法让运行时态在校验完成前非空。
      merge: (persisted, current) => {
        const p = (persisted ?? {}) as Partial<AuthState>
        return {
          ...current,
          refreshToken: p.refreshToken ?? null,
          user: p.user ?? null,
          workspaceMemory: parseWorkspaceMemory(p.workspaceMemory),
          activeOrgId: null,
          personalOrgId: null,
        }
      },
    },
  ),
)
