import { getApiBase, getEffectiveServerUrl, isElectronRuntime } from '@/stores/serverConfigStore'

/**
 * 客户端下载 / 扫码入口（登录页用）。
 *
 * 桌面端安装包走**公开** feed（免登录，与 electron-updater 同源同协议）：
 *   GET /api/v1/desktop/releases/feed/latest.yml   → 动态生成（version/path/sha512）
 *   GET /api/v1/desktop/releases/feed/:filename    → 安装包本体（支持 Range）
 * 移动端安装包（Android APK）走**公开** meta/下载（免登录 —— 扫码发生在
 * 未登录的手机浏览器里）：
 *   GET /api/v1/mobile/app/latest    → 当前包元信息（无包 → mobile_app:null）
 *   GET /api/v1/mobile/app/download  → APK 本体（attachment 直下）
 * 后端保证两者都按「当前发布指针 / current 记录」动态返回，上传者无法伪造版本。
 */
export interface DesktopReleaseInfo {
  version: string
  /** 安装包文件名，如 TrustMesh-Setup-0.2.3.exe */
  fileName: string
  /** 完整下载地址（已含 apiBase 前缀） */
  downloadUrl: string
}

export interface MobileAppReleaseInfo {
  version: string
  fileName: string
  /** 二维码内容：APK 直接下载地址（**绝对 URL**，扫码器才能打开） */
  downloadUrl: string
}

/**
 * 绝对 **origin**（不含 `/api/v1` 前缀）——仅供「后端已返回含前缀的路径」时拼装。
 *
 * 🔴 为什么不能复用 getApiBase()：`/mobile/app/latest` 返回的 `download_path`
 *    本身**已含** `/api/v1`（后端 mobile_app_release.go 写死
 *    `"/api/v1/mobile/app/download"`）。若再拼一次 getApiBase()，就会得到
 *    `.../api/v1/api/v1/mobile/app/download` ⇒ 扫码 404（2026-09-21 事故）。
 * ⇒ 这里只取 origin：
 *   - 桌面端：getEffectiveServerUrl() = `https://175.27.135.91`（已剥掉 /api/v1）。
 *   - Web：同源，用 window.location.origin。
 */
export function getAbsoluteOrigin(): string {
  if (isElectronRuntime()) return getEffectiveServerUrl().replace(/\/+$/, '')
  return window.location.origin.replace(/\/+$/, '')
}

/**
 * 拉取当前移动端安装包元信息。任何失败（无网络 / 未上传 / 字段缺失）
 * 一律返回 null，由调用方隐藏扫码入口 —— 登录页不得因该可选项报错。
 */
export async function fetchLatestMobileRelease(signal?: AbortSignal): Promise<MobileAppReleaseInfo | null> {
  try {
    const res = await fetch(`${getApiBase()}mobile/app/latest`, { cache: 'no-store', signal })
    if (!res.ok) return null
    const body = (await res.json()) as {
      data?: { mobile_app?: { version?: string; file_name?: string; download_path?: string } | null }
    }
    const app = body.data?.mobile_app
    const version = app?.version?.trim()
    const rawPath = app?.download_path?.trim()
    if (!version || !rawPath) return null
    return {
      version,
      fileName: app?.file_name ?? `TrustMesh-${version}.apk`,
      // 用 origin + 规范化后的 path 拼装（见 normalizeApiPath 注释）。
      downloadUrl: `${getAbsoluteOrigin()}${normalizeApiPath(rawPath)}`,
    }
  } catch {
    return null
  }
}

/**
 * 把后端给的下载路径规范化成「恰好一层 /api/v1 前缀」的绝对路径。
 *
 * 后端当前返回 `"/api/v1/mobile/app/download"`（含前缀）。这里做防御式处理：
 * 若将来改成返回 `"/mobile/app/download"`（去掉前缀），本函数自动补上，
 * 使 payload 形态无论哪种都拼出唯一正确地址，不再出现双前缀 / 缺前缀。
 */
export function normalizeApiPath(path: string): string {
  let s = path.replace(/^\/+/, '')
  // 反复剥离可能存在的多层 api/v1 前缀（幂等：无论 0/1/N 层都收敛到 1 层）
  while (/^api\/v1\/+/i.test(s)) {
    s = s.replace(/^api\/v1\/+/i, '')
  }
  return `/api/v1/${s}`
}

/** latest.yml 是受控 YAML 子集（所有值带双引号），按行正则提取即可，无需引 YAML 解析器。 */
function matchQuotedValue(text: string, key: string): string | null {
  const m = new RegExp(`^${key}:\\s*"([^"]+)"`, 'm').exec(text)
  return m?.[1] ?? null
}

/**
 * 解析 latest.yml 文本 → 下载信息。抽成纯函数（apiBase 注入）以便单测：
 * 这份 YAML 是更新链路的唯一协议面，解析错一个字段下载就打到 404。
 */
export function parseLatestYML(text: string, apiBase: string): DesktopReleaseInfo | null {
  const version = matchQuotedValue(text, 'version')
  // files[0].url 与 path 同值（RenderLatestYML 两处都写文件名），取 path 即可
  const fileName = matchQuotedValue(text, 'path')
  if (!version || !fileName) return null
  return {
    version,
    fileName,
    downloadUrl: `${apiBase}desktop/releases/feed/${encodeURIComponent(fileName)}`,
  }
}

/**
 * 拉取最新已发布的桌面安装包信息。任何失败（无网络 / 404 无发布 / 字段缺失）
 * 一律返回 null，由调用方决定隐藏下载入口 —— 登录页不得因该可选项报错。
 */
export async function fetchLatestDesktopRelease(signal?: AbortSignal): Promise<DesktopReleaseInfo | null> {
  try {
    const res = await fetch(`${getApiBase()}desktop/releases/feed/latest.yml`, {
      cache: 'no-store',
      signal,
    })
    if (!res.ok) return null
    const text = await res.text()
    return parseLatestYML(text, getApiBase())
  } catch {
    return null
  }
}
