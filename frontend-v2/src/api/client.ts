import ky from 'ky'
import { useAuthStore } from '@/stores/authStore'
import { getApiBase } from '@/stores/serverConfigStore'
import { ApiRequestError } from '@/types'

// API 根地址：Web 容器构建走 VITE_API_BASE_URL（同源 nginx 代理）；
// 桌面版/本地开发走 serverConfigStore 可配置地址（修改后整页刷新生效）。
// 注意：使用 prefixUrl 时 ky 要求调用路径不能以 / 开头，且此处 base 必须以 / 结尾。
const API_BASE = getApiBase()

let isRefreshing = false
let refreshPromise: Promise<void> | null = null

async function refreshAccessToken(): Promise<void> {
  const { refreshToken } = useAuthStore.getState()
  if (!refreshToken) throw new Error('No refresh token available')

  const res = await ky
    .create({ prefixUrl: API_BASE })
    .post('auth/refresh', { json: { refresh_token: refreshToken } })
    .json<{ data: { access_token: string; refresh_token: string; expires_in: number } }>()

  useAuthStore.getState().setTokens(res.data.access_token, res.data.refresh_token)
}

/**
 * 冷启动门闩：accessToken 刻意不持久化（短时效设计），整页刷新后首轮并发请求
 * 都没有 token，会各自 401 后再走 refresh 重试，浪费一整轮往返。
 * 这里在请求发出前统一等待一次 refresh，全部请求直接带上新 token。
 * refresh 失败时放行（由 afterResponse 的 401 分支走既有失败路径，不在此登出）。
 */
function ensureAccessToken(): Promise<void> {
  const { accessToken, refreshToken } = useAuthStore.getState()
  if (accessToken || !refreshToken) return Promise.resolve()
  if (!isRefreshing) {
    isRefreshing = true
    refreshPromise = refreshAccessToken()
      .catch(() => {
        // 放行，交给 afterResponse 处理
      })
      .finally(() => {
        isRefreshing = false
        refreshPromise = null
      })
  }
  return refreshPromise ?? Promise.resolve()
}

export const apiClient = ky.create({
  prefixUrl: API_BASE,
  hooks: {
    beforeRequest: [
      async (request) => {
        await ensureAccessToken()
        const { accessToken, activeOrgId } = useAuthStore.getState()
        if (accessToken) {
          request.headers.set('Authorization', `Bearer ${accessToken}`)
        }
        // 多租户上下文：activeOrgId 为 null = 个人空间，不带头发请求（与旧客户端行为一致）
        if (activeOrgId) {
          request.headers.set('X-Org-Id', activeOrgId)
        }
      },
    ],
    afterResponse: [
      async (request, _options, response) => {
        if (response.status === 401 && !request.url.includes('auth/refresh')) {
          if (!isRefreshing) {
            isRefreshing = true
            refreshPromise = refreshAccessToken().finally(() => {
              isRefreshing = false
              refreshPromise = null
            })
          }

          await refreshPromise

          const { accessToken } = useAuthStore.getState()
          request.headers.set('Authorization', `Bearer ${accessToken}`)
          return ky(request)
        }

        if (!response.ok) {
          const body = (await response.json().catch(() => ({}))) as {
            error?: { code?: string; message?: string; details?: Record<string, unknown> }
          }
          throw new ApiRequestError(
            body.error?.code || 'UNKNOWN',
            body.error?.message || 'Request failed',
            response.status,
            body.error?.details || {},
          )
        }
      },
    ],
  },
})
