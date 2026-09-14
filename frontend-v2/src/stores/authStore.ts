import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { User } from '@/types'

interface AuthState {
  accessToken: string | null
  refreshToken: string | null
  user: User | null
  activeOrgId: string | null // null = 个人空间；发请求时回落到 personalOrgId 发真头
  personalOrgId: string | null // 个人租户 ID，由 MainLayout 从 orgs 列表水合（kind === 'personal'）
  setActiveOrg: (orgId: string | null) => void
  setPersonalOrgId: (orgId: string | null) => void
  setAuth: (accessToken: string, refreshToken: string, user: User) => void
  setUser: (user: User) => void
  setTokens: (accessToken: string, refreshToken: string) => void
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
      setActiveOrg: (orgId) => set({ activeOrgId: orgId }),
      setPersonalOrgId: (orgId) => set({ personalOrgId: orgId }),
      setAuth: (accessToken, refreshToken, user) => set({ accessToken, refreshToken, user }),
      setUser: (user) => set({ user }),
      setTokens: (accessToken, refreshToken) => set({ accessToken, refreshToken }),
      logout: () =>
        set({ accessToken: null, refreshToken: null, user: null, activeOrgId: null, personalOrgId: null }),
      isAuthenticated: () => !!get().refreshToken,
    }),
    {
      name: 'trustmesh-v2:auth',
      partialize: (state) => ({
        refreshToken: state.refreshToken,
        user: state.user,
        activeOrgId: state.activeOrgId,
        personalOrgId: state.personalOrgId,
      }),
    },
  ),
)
