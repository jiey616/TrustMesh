import ky from 'ky'
import type { ApiError } from '@/types'
import { apiBaseUrl } from '@/lib/apiBase'
import { useAuthStore } from '@/stores/authStore'
import { refresh as refreshApi } from '@/api/auth'

export class ApiRequestError extends Error {
  code: string
  details: Record<string, unknown>
  status: number

  constructor(error: ApiError, status: number) {
    super(error.message)
    this.name = 'ApiRequestError'
    this.code = error.code
    this.details = error.details
    this.status = status
  }
}

// Concurrency lock: only one refresh at a time
let refreshPromise: Promise<{ access_token: string; refresh_token: string }> | null = null

async function doRefresh(): Promise<{ access_token: string; refresh_token: string }> {
  const { refreshToken } = useAuthStore.getState()
  if (!refreshToken) {
    throw new Error('no refresh token')
  }

  if (refreshPromise) {
    return refreshPromise
  }

  refreshPromise = refreshApi(refreshToken)
    .then((res) => {
      const { access_token, refresh_token } = res.data
      useAuthStore.getState().setTokens(access_token, refresh_token)
      return { access_token, refresh_token }
    })
    .finally(() => {
      refreshPromise = null
    })

  return refreshPromise
}

function redirectToLogin() {
  useAuthStore.getState().logout()
  if (window.location.pathname !== '/login' && window.location.pathname !== '/register') {
    window.location.href = '/login'
  }
}

export const api = ky.create({
  prefixUrl: apiBaseUrl,
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
      async (request, options, response) => {
        if (response.status === 401) {
          // Try silent refresh
          try {
            const tokens = await doRefresh()
            // Retry original request with new access token
            request.headers.set('Authorization', `Bearer ${tokens.access_token}`)
            return ky(request, { ...options, hooks: {} })
          } catch {
            redirectToLogin()
          }
        }

        if (!response.ok) {
          let apiError: ApiError | undefined
          try {
            const body = await response.json<{ error: ApiError }>()
            apiError = body.error
          } catch {
            // response body is not valid JSON or not in expected format
          }
          throw new ApiRequestError(
            apiError ?? { code: 'UNKNOWN', message: `请求失败 (${response.status})`, details: {} },
            response.status,
          )
        }
      },
    ],
  },
})
