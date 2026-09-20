import { create } from 'zustand'
import { persist, createJSONStorage } from 'zustand/middleware'

export type PollInterval = 0 | 30_000 | 60_000 | 300_000

export const POLL_OPTIONS: Array<{ value: PollInterval; label: string }> = [
  { value: 0, label: '关闭' },
  { value: 30_000, label: '30 秒' },
  { value: 60_000, label: '1 分钟' },
  { value: 300_000, label: '5 分钟' },
]

interface SettingsState {
  /** 未读数轮询间隔；0 = 关闭（后端没有推送通道前的默认值档位之一） */
  unreadPollInterval: PollInterval
  /** 底部导航是否显示未读红点 */
  showUnreadBadge: boolean

  setUnreadPollInterval: (v: PollInterval) => void
  setShowUnreadBadge: (v: boolean) => void
}

/**
 * 纯本地偏好：**后端目前没有通知设置接口**（`api/user.ts` 只有 getMe / 改资料 / 改密码），
 * 所以"通知设置"只控制前端的轮询与红点。真正的推送要等 P3（Capacitor 壳 + 厂商通道）。
 */
export const useSettingsStore = create<SettingsState>()(
  persist(
    (set) => ({
      unreadPollInterval: 60_000,
      showUnreadBadge: true,
      setUnreadPollInterval: (unreadPollInterval) => set({ unreadPollInterval }),
      setShowUnreadBadge: (showUnreadBadge) => set({ showUnreadBadge }),
    }),
    {
      name: 'tm-mobile-settings',
      storage: createJSONStorage(() => localStorage),
    },
  ),
)
