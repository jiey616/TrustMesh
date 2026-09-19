import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export const DEFAULT_SERVER_URL = 'https://175.27.135.91'

/**
 * 规范化用户输入的服务端地址：
 * - 补全协议（缺省按 http）
 * - 去掉末尾斜杠与多余的 /api/v1 后缀（由 apiBase 统一拼接）
 * 非法输入返回 null。
 */
export function normalizeServerUrl(input: string): string | null {
  let raw = input.trim()
  if (!raw) return null
  if (!/^https?:\/\//i.test(raw)) {
    raw = `http://${raw}`
  }
  try {
    const url = new URL(raw)
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return null
    const pathname = url.pathname.replace(/\/+$/, '').replace(/\/api\/v1$/i, '')
    const base = `${url.origin}${pathname}`
    return base.replace(/\/+$/, '')
  } catch {
    return null
  }
}

interface ServerConfigState {
  /** 服务端根地址，如 http://localhost:8080；null 表示未配置 */
  serverUrl: string | null
  /** 显式设置并校验地址；非法输入返回 false */
  setServerUrl: (input: string) => boolean
  /** 清除自定义地址（回退到默认地址） */
  clearServerUrl: () => void
  /** 信任自签名 / 私有 CA 证书（仅桌面端可开关，Web 端浏览器自身策略不可控） */
  trustInsecureTls: boolean
  /** 开关自签名证书信任：同步给 Electron 主进程（Web 环境仅本地生效） */
  setTrustInsecureTls: (enabled: boolean) => void
}

export const useServerConfigStore = create<ServerConfigState>()(
  persist(
    (set) => ({
      serverUrl: null,
      trustInsecureTls: false,
      setServerUrl: (input) => {
        const normalized = normalizeServerUrl(input)
        if (!normalized) return false
        set({ serverUrl: normalized })
        return true
      },
      clearServerUrl: () => set({ serverUrl: null }),
      setTrustInsecureTls: (enabled) => {
        set({ trustInsecureTls: enabled })
        // 主进程持有证书校验开关；失败不影响 UI 状态（下次打开可再同步）
        void window.desktop?.setTrustInsecureTls(enabled)
      },
    }),
    {
      name: 'trustmesh-server-config',
      version: 2,
      migrate: (persisted) => {
        // 兼容早期可能写入空字符串 / 相对路径的脏数据
        const state = persisted as { serverUrl?: unknown; trustInsecureTls?: unknown }
        const raw = typeof state?.serverUrl === 'string' ? state.serverUrl : null
        return {
          serverUrl: raw ? normalizeServerUrl(raw) : null,
          trustInsecureTls: state?.trustInsecureTls === true,
        }
      },
    },
  ),
)

/** 当前生效的服务端根地址（未配置时用默认地址） */
export function getEffectiveServerUrl(): string {
  return useServerConfigStore.getState().serverUrl ?? DEFAULT_SERVER_URL
}

/**
 * API 根地址（以 / 结尾，含 /api/v1）。
 * - Web（浏览器 / 容器 nginx 托管）：固定同源 `/api/v1/`，不读本地存储的服务端地址，
 *   避免历史脏数据或非本机部署时打向 localhost:8080。容器已配 VITE_API_BASE_URL 可覆盖。
 * - 桌面版（Electron）：走 serverConfigStore 的可配置服务端地址（设置页修改后整页刷新生效）。
 */
export function getApiBase(): string {
  const envBase = (import.meta.env.VITE_API_BASE_URL as string | undefined)?.replace(/\/+$/, '')
  if (envBase) return `${envBase}/`
  if (isElectronRuntime()) return `${getEffectiveServerUrl()}/api/v1/`
  return '/api/v1/'
}

/** 是否运行在 Electron 桌面壳内（preload 的 process 桥一贯存在，不依赖新增的 desktop 桥） */
export function isElectronRuntime(): boolean {
  return typeof window !== 'undefined' && !!window.process?.versions?.electron
}

/** 桌面桥（window.desktop）是否可用——仅桌面端为 true */
export function isDesktopShell(): boolean {
  return typeof window !== 'undefined' && !!window.desktop
}

/**
 * 桌面端：渲染进程与主进程的 TLS 开关双向同步。
 * 主进程配置文件（userData/trustmesh-desktop.json）丢失/重装时，用本地持久化值回灌。
 */
export function syncTlsTrustToMain(): void {
  if (!isDesktopShell()) return
  const local = useServerConfigStore.getState().trustInsecureTls
  void window.desktop
    ?.getConfig()
    .then((cfg) => {
      const main = cfg?.trustInsecureTls === true
      if (local && !main) {
        void window.desktop?.setTrustInsecureTls(true)
      } else if (!local && main) {
        useServerConfigStore.setState({ trustInsecureTls: true })
      }
    })
    .catch(() => {
      // 主进程不可用时忽略，UI 仍能记录本地状态
    })
}
