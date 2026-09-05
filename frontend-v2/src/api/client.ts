import ky from 'ky'
import { useAuthStore } from '@/stores/authStore'
import { ApiRequestError } from '@/types'

const API_BASE = import.meta.env.VITE_API_BASE_URL || ''

let isRefreshing = false
let refreshPromise: Promise<void> | null = null

async function refreshAccessToken(): Promise<void> {
  const { refreshToken } = useAuthStore.getState()
  if (!refreshToken) throw new Error('No refresh token available')

  const res = await ky
    .create({ prefixUrl: API_BASE })
    .post('/api/v1/auth/refresh', { json: { refresh_token: refreshToken } })
    .json<{ data: { access_token: string; refresh_token: string; expires_in: number } }>()

  useAuthStore.getState().setTokens(res.data.access_token, res.data.refresh_token)
}

export const apiClient = ky.create({
  prefixUrl: API_BASE,
  hooks: {
    beforeRequest: [
      (request) => {
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
