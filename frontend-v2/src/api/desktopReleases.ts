import { apiClient, ensureAccessTokenReady, refreshAccessTokenOnce } from './client'
import { getApiBase } from '@/stores/serverConfigStore'
import { useAuthStore } from '@/stores/authStore'
import { ApiRequestError } from '@/types'
import type {
  ApiResponse,
  DesktopReleaseUploadInput,
  PlatformDesktopReleaseView,
} from '@/types'

// ─── 桌面端发行版（平台命名空间，权限点 platform.desktop.release）───
//
// 方案：docs/desktop-app-update-plan-2026-09-18.md
//
// 列表/发布/回滚/删除走 apiClient（ky）；**上传走 XMLHttpRequest** ——
// 87MB 的安装包必须有进度条，而 fetch 没有上传进度事件（ky 基于 fetch，同样没有）。
// 鉴权复用 client.ts 导出的两个原语（冷启动门闩 + 401 单飞刷新），不重复实现。

export async function listDesktopReleases() {
  return apiClient
    .get('platform/desktop-releases')
    .json<ApiResponse<{ desktop_releases: PlatformDesktopReleaseView[] }>>()
}

export async function publishDesktopRelease(id: string) {
  return apiClient
    .post(`platform/desktop-releases/${id}/publish`)
    .json<
      ApiResponse<{
        desktop_release: PlatformDesktopReleaseView
        /** 保留上限清理掉的旧版本号（发布顺带触发，方案 §4「清理」） */
        pruned: string[] | null
      }>
    >()
}

export async function rollbackDesktopRelease(id: string) {
  return apiClient
    .post(`platform/desktop-releases/${id}/rollback`)
    .json<ApiResponse<{ desktop_release: PlatformDesktopReleaseView }>>()
}

export async function deleteDesktopRelease(id: string) {
  return apiClient
    .delete(`platform/desktop-releases/${id}`)
    .json<ApiResponse<{ deleted: boolean }>>()
}

interface UploadEnvelope {
  data?: { desktop_release: PlatformDesktopReleaseView }
  error?: { code?: string; message?: string }
}

function sendUpload(form: FormData, onProgress?: (percent: number) => void) {
  return new Promise<{ status: number; body: string }>((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', `${getApiBase()}platform/desktop-releases`)
    const { accessToken, activeOrgId, personalOrgId } = useAuthStore.getState()
    if (accessToken) xhr.setRequestHeader('Authorization', `Bearer ${accessToken}`)
    // 与 apiClient.beforeRequest 保持同一套租户头规则，否则平台请求会走成个人维度。
    const orgId = activeOrgId ?? personalOrgId
    if (orgId) xhr.setRequestHeader('X-Org-Id', orgId)
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress?.(Math.round((e.loaded / e.total) * 100))
    }
    xhr.onload = () => resolve({ status: xhr.status, body: xhr.responseText })
    xhr.onerror = () => reject(new Error('网络错误，上传失败'))
    xhr.onabort = () => reject(new Error('上传已取消'))
    xhr.send(form)
  })
}

/**
 * 上传一个发行版（安装包 + release.json [+ .blockmap]），返回落库后的记录。
 *
 * 401 会**刷新一次并整包重传一次**：数十秒的上传期间 token 到期是真实场景，
 * 而重传的代价（87MB）仍然小于「传完才发现 401、进度全白跑」。与 apiClient 的策略
 * 一致：只对 401 刷新，且至多重传一次。
 *
 * 服务端会再复核一次 size 与 sha512（见 store.CreateDesktopRelease），
 * 因此这里的前端校验只承担「早失败、省一次 87MB 往返」的职责，不是安全边界。
 */
export async function uploadDesktopRelease(
  input: DesktopReleaseUploadInput,
  onProgress?: (percent: number) => void,
): Promise<PlatformDesktopReleaseView> {
  await ensureAccessTokenReady()
  const form = new FormData()
  form.append('installer', input.installer)
  form.append('metadata', input.metadata)
  if (input.blockmap) form.append('blockmap', input.blockmap)

  let res = await sendUpload(form, onProgress)
  if (res.status === 401 && useAuthStore.getState().refreshToken) {
    await refreshAccessTokenOnce()
    res = await sendUpload(form, onProgress)
  }

  let parsed: UploadEnvelope = {}
  try {
    parsed = JSON.parse(res.body) as UploadEnvelope
  } catch {
    // 非 JSON 响应（网关 HTML 错误页 / 502 等）→ 下面统一按失败处理，不吞成「成功」。
  }
  if (res.status < 200 || res.status >= 300 || !parsed.data) {
    throw new ApiRequestError(
      parsed.error?.code ?? 'UNKNOWN',
      parsed.error?.message ?? `上传失败（HTTP ${res.status}）`,
      res.status,
    )
  }
  return parsed.data.desktop_release
}
