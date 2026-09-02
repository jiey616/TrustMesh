import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { User } from '@/types'

interface AuthState {
  accessToken: string | null
  refreshToken: string | null
  user: User | null
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
      setAuth: (accessToken, refreshToken, user) => set({ accessToken, refreshToken, user }),
      setTokens: (accessToken, refreshToken) => set({ accessToken, refreshToken }),
      logout: () => set({ accessToken: null, refreshToken: null, user: null }),
      isAuthenticated: () => !!get().refreshToken,
    }),
    {
      name: 'trustmesh-v2:auth',
      partialize: (state) => ({
        refreshToken: state.refreshToken,
        user: state.user,
      }),
    },
  ),
)
