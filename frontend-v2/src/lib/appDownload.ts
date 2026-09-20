import { getApiBase } from '@/stores/serverConfigStore'

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
 * 绝对 API 根地址（二维码必须绝对 URL）。
 * - 桌面端：getApiBase() 本就是绝对地址（serverUrl 拼装）。
 * - Web：getApiBase() 是同源相对路径 `/api/v1/`，补上当前 origin。
 */
export function getAbsoluteApiBase(): string {
  const base = getApiBase()
  if (/^https?:\/\//i.test(base)) return base
  return `${window.location.origin}${base}`
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
    const downloadPath = app?.download_path?.trim()
    if (!version || !downloadPath) return null
    return {
      version,
      fileName: app?.file_name ?? `TrustMesh-${version}.apk`,
      downloadUrl: `${getAbsoluteApiBase().replace(/\/+$/, '')}${downloadPath}`,
    }
  } catch {
    return null
  }
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
