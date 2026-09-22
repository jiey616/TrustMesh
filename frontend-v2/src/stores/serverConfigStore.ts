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
  /** 用户是否**主动**动过这个开关。未动过 ⇒ 跟随默认值 / 主进程，动过 ⇒ 以用户选择为准。 */
  trustInsecureTlsExplicit: boolean
  /** 开关自签名证书信任：同步给 Electron 主进程（Web 环境仅本地生效） */
  setTrustInsecureTls: (enabled: boolean) => void
}

/**
 * 「信任自签名 / 私有 CA 证书」的默认值。
 *
 * 🔴 桌面端默认 **true**（2026-09-22 改）。原因：生产 / 内网后端的证书是自签的
 *   （`subject == issuer == CN=175.27.135.91`），默认关掉时首次安装的客户端
 *   **连登录接口都打不通**（渲染进程只会得到笼统的 `Failed to fetch`），
 *   而用户不可能知道要去「服务器设置」里勾一个复选框 —— 现象就是「登录不上去」。
 *   已用真 Electron 矩阵实测：
 *     开关关 → 渲染进程 `Failed to fetch`、主进程 `ERR_CERT_AUTHORITY_INVALID`；
 *     开关开 → 两条路径都 200。
 * Web 端固定 false：浏览器证书策略不受页面控制，勾了也不解决问题（且 Web 端不渲染该开关）。
 *
 * 纯函数 + 可注入 isDesktop 是为了单测：本仓 jsdom 里**伪造 `window.process` 会连带
 * 打断 vitest 自身**（见 09-19 移动端同类坑），所以运行环境判断一律从参数进。
 */
export function defaultTrustInsecureTls(isDesktop: boolean = isElectronRuntime()): boolean {
  return isDesktop
}

/** persist 里保存的字段形状（migrate 的产出必须与之一致）。 */
export interface PersistedServerConfig {
  serverUrl: string | null
  trustInsecureTls: boolean
  trustInsecureTlsExplicit: boolean
}

/**
 * 持久化数据迁移（纯函数，便于穷举单测）。
 *
 * 🔴 v2 → v3 的关键语义：**旧记录里的 `trustInsecureTls: false` 不能当成「用户主动关闭」**。
 *   旧版本的默认值就是 false，绝大多数用户从没碰过这个开关（他们只是登录不上）。
 *   所以 v2 及以前一律按「未显式选择」处理 ⇒ 采用新默认值（桌面端 true）。
 *   只有 v3 起写入过 `trustInsecureTlsExplicit: true` 的记录才保留用户的选择。
 */
export function migrateServerConfig(
  persisted: unknown,
  isDesktop: boolean = isElectronRuntime(),
): PersistedServerConfig {
  const state = persisted as
    | { serverUrl?: unknown; trustInsecureTls?: unknown; trustInsecureTlsExplicit?: unknown }
    | null
    | undefined
  const raw = typeof state?.serverUrl === 'string' ? state.serverUrl : null
  const explicit = state?.trustInsecureTlsExplicit === true
  return {
    serverUrl: raw ? normalizeServerUrl(raw) : null,
    trustInsecureTls: explicit ? state?.trustInsecureTls === true : defaultTrustInsecureTls(isDesktop),
    trustInsecureTlsExplicit: explicit,
  }
}

export const useServerConfigStore = create<ServerConfigState>()(
  persist(
    (set) => ({
      serverUrl: null,
      trustInsecureTls: defaultTrustInsecureTls(),
      trustInsecureTlsExplicit: false,
      setServerUrl: (input) => {
        const normalized = normalizeServerUrl(input)
        if (!normalized) return false
        set({ serverUrl: normalized })
        return true
      },
      clearServerUrl: () => set({ serverUrl: null }),
      setTrustInsecureTls: (enabled) => {
        // explicit=true：从此以用户的选择为准，不再被默认值 / 主进程覆盖
        set({ trustInsecureTls: enabled, trustInsecureTlsExplicit: true })
        // 主进程持有证书校验开关；失败不影响 UI 状态（下次打开可再同步）
        void window.desktop?.setTrustInsecureTls(enabled)
      },
    }),
    {
      name: 'trustmesh-server-config',
      version: 3,
      migrate: (persisted) => migrateServerConfig(persisted),
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
 *
 * 主进程配置文件（userData/trustmesh-desktop.json）丢失/重装时，用本地持久化值回灌；
 * 反之，用户**从没主动动过**开关时，以主进程为准（它才是证书校验真正的执行方）。
 *
 * 🔴 为什么要加 `explicit` 判据：老版本把「默认 false」也持久化了下来，若不分
 *    「用户主动关闭」与「从没碰过」，升级后要么把用户显式关闭的选择重置掉，
 *    要么让从没碰过的用户继续连不上后端。
 */
export function syncTlsTrustToMain(): void {
  if (!isDesktopShell()) return
  const { trustInsecureTls: local, trustInsecureTlsExplicit: explicit } =
    useServerConfigStore.getState()
  void window.desktop
    ?.getConfig()
    .then((cfg) => {
      const main = cfg?.trustInsecureTls === true
      if (explicit) {
        // 用户主动选过 ⇒ 以用户为准，把主进程拉齐
        if (local !== main) void window.desktop?.setTrustInsecureTls(local)
        return
      }
      // 从没动过 ⇒ 跟主进程（含配置文件缺失时的新默认 true）
      if (local !== main) useServerConfigStore.setState({ trustInsecureTls: main })
    })
    .catch(() => {
      // 主进程不可用时忽略，UI 仍能记录本地状态
    })
}
