import { create } from 'zustand'
import { persist, createJSONStorage } from 'zustand/middleware'
import type { User } from '@/types'

interface AuthState {
  /** 仅内存：access token 短时效，刷新页面后靠 refreshToken 重新换取 */
  accessToken: string | null
  /** 持久化：移动端 WebView / 浏览器 localStorage 足够持久 */
  refreshToken: string | null
  user: User | null
  /** 用户在「我的 → 服务器地址」手填的基址，优先级最高 */
  apiBaseOverride: string | null

  setTokens: (accessToken: string, refreshToken: string) => void
  setUser: (user: User | null) => void
  setApiBaseOverride: (base: string | null) => void
  logout: () => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      accessToken: null,
      refreshToken: null,
      user: null,
      apiBaseOverride: null,

      setTokens: (accessToken, refreshToken) => set({ accessToken, refreshToken }),
      setUser: (user) => set({ user }),
      setApiBaseOverride: (apiBaseOverride) => set({ apiBaseOverride }),
      logout: () => set({ accessToken: null, refreshToken: null, user: null }),
    }),
    {
      name: 'tm-mobile-auth',
      storage: createJSONStorage(() => localStorage),
      // accessToken 刻意不持久化
      partialize: (s) => ({
        refreshToken: s.refreshToken,
        user: s.user,
        apiBaseOverride: s.apiBaseOverride,
      }),
    },
  ),
)
