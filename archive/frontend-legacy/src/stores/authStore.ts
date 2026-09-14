import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { User } from '@/types'

interface AuthState {
  accessToken: string | null
  refreshToken: string | null
  user: User | null
  activeOrgId: string | null // null = 个人空间（不携带 X-Org-Id，保持无头兼容语义）
  setActiveOrg: (orgId: string | null) => void
  setAuth: (accessToken: string, refreshToken: string, user: User) => void
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
      setActiveOrg: (orgId) => set({ activeOrgId: orgId }),
      setAuth: (accessToken, refreshToken, user) => {
        set({ accessToken, refreshToken, user })
      },
      setTokens: (accessToken, refreshToken) => {
        set({ accessToken, refreshToken })
      },
      logout: () => {
        set({ accessToken: null, refreshToken: null, user: null, activeOrgId: null })
      },
      isAuthenticated: () => !!get().refreshToken,
    }),
    {
      name: 'trustmesh:auth',
      partialize: (state) => ({
        refreshToken: state.refreshToken,
        user: state.user,
        activeOrgId: state.activeOrgId,
      }),
    }
  )
)
