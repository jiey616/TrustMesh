/**
 * 桌面端自动更新的类型与订阅辅助。
 *
 * 方案：docs/desktop-app-update-plan-2026-09-18.md
 *
 * ⚠️ 这里的所有桥接方法都按**可选**处理：线上还跑着 0.1.0 的壳，它的 preload 里
 * 没有 `tm:update-*` 通道。当成必存在会让你在旧机器上拿到 `undefined is not a function`，
 * 而「改了 preload 就必须重新发一次桌面包」正是这套机制自带的发布节奏约束。
 */

export type DesktopUpdateStatus =
  | 'idle'
  | 'checking'
  | 'available'
  | 'downloading'
  | 'downloaded'
  | 'up-to-date'
  | 'error'

export interface DesktopUpdateState {
  status: DesktopUpdateStatus
  version: string | null
  percent: number
  message: string | null
  /** 桌面壳是否具备自动更新能力（开发模式 / 未打包 = false） */
  enabled?: boolean
  /** 烘焙在包内的更新源地址（仅供排查，界面上不提供修改入口，见方案 §1 决策 11） */
  feedUrl?: string
}

/** 本壳是否具备更新桥（旧版壳为 false，调用方据此隐藏整块 UI）。 */
export function hasDesktopUpdateBridge(): boolean {
  return typeof window !== 'undefined' && typeof window.desktop?.onUpdateState === 'function'
}

/** 订阅主进程推送的更新状态快照；无桥时返回空卸载函数（可直接作为 useEffect 返回值）。 */
export function subscribeDesktopUpdate(
  cb: (state: DesktopUpdateState) => void,
): () => void {
  const off = window.desktop?.onUpdateState
  if (typeof off !== 'function') return () => {}
  return off(cb)
}

/** 拉取一次当前状态快照（用于组件挂载时补齐「挂载前已发生」的状态）。 */
export async function fetchDesktopUpdateState(): Promise<DesktopUpdateState | null> {
  const get = window.desktop?.getUpdateState
  if (typeof get !== 'function') return null
  try {
    return await get()
  } catch {
    return null
  }
}

/** 状态 → 中文描述（卡片与提示共用，避免两处文案漂移）。 */
export function describeUpdateStatus(state: DesktopUpdateState | null): string {
  if (!state) return '尚未检查'
  switch (state.status) {
    case 'checking':
      return '正在检查更新…'
    case 'available':
      return state.version ? `发现新版本 ${state.version}，正在后台下载` : '发现新版本，正在后台下载'
    case 'downloading':
      return `正在下载更新 ${state.percent}%`
    case 'downloaded':
      return state.version ? `新版本 ${state.version} 已就绪，重启后生效` : '更新已就绪，重启后生效'
    case 'up-to-date':
      return '已是最新版本'
    case 'error':
      return state.message ? `检查更新失败：${state.message}` : '检查更新失败'
    default:
      return '尚未检查'
  }
}
