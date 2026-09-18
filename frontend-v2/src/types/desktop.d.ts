/** 主进程连接探测结果（含 Chromium net 错误码） */
export interface ProbeOutcome {
  ok: boolean
  status: number
  error: string | null
}

import type { DesktopUpdateState } from '@/lib/desktopUpdate'

export {}

declare global {
  interface Window {
    /** Electron 渲染进程中由 preload 注入；浏览器环境为 undefined */
    process?: {
      versions?: {
        electron?: string
      }
    }
    /** 桌面端专属桥接；浏览器 / Web 部署环境为 undefined */
    desktop?: {
      getConfig: () => Promise<{ trustInsecureTls?: boolean }>
      setTrustInsecureTls: (enabled: boolean) => Promise<{ trustInsecureTls?: boolean }>
      probeServer: (baseUrl: string, timeoutMs?: number) => Promise<ProbeOutcome>
      showNotification?: (options: {
        title: string
        body?: string
        tag?: string
      }) => Promise<boolean>
      onNotificationClicked?: (callback: (tag?: string) => void) => () => void

      // ── 自动更新（方案 docs/desktop-app-update-plan-2026-09-18.md §6）──
      //
      // 🔴 全部声明为**可选**：线上仍有 0.1.0 的壳，其 preload 不含这些通道。
      //    声明成必存在只会把运行时崩溃提前变成类型层的假安全。
      /** 桌面壳版本（Electron app.getVersion） */
      getAppVersion?: () => Promise<string>
      /** 拉取一次更新状态快照 */
      getUpdateState?: () => Promise<DesktopUpdateState>
      /** 主动检查更新（设置页「检查更新」兜底入口） */
      updateCheck?: () => Promise<DesktopUpdateState>
      /** 退出并安装已下载的更新 */
      updateInstall?: () => Promise<boolean>
      /** 订阅更新状态推送，返回取消订阅函数 */
      onUpdateState?: (callback: (state: DesktopUpdateState) => void) => () => void
    }
  }
}
