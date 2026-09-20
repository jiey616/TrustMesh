import ky from 'ky'
import { useAuthStore } from '@/stores/authStore'
import { useWorkspaceStore } from '@/stores/workspaceStore'
import { resolveApiBase } from '@/lib/resolveApiBase'
import { ApiRequestError } from '@/types'

// 后端错误体统一为 { error: { code, message } }。
interface ApiErrorBody {
  code?: string
  message?: string
}

// 地址在模块加载时裁决一次；用户改地址后整页刷新生效（与桌面端同款行为）。
// SSE（events/stream）不走 ky，需要直接引用同一 base。
export const API_BASE = resolveApiBase({
  override: useAuthStore.getState().apiBaseOverride,
  envBase: import.meta.env.VITE_API_BASE_URL,
})

/** 裸 ky（无 hooks）：仅用于 401 重放，避免重入 afterResponse 形成刷新风暴。 */
const baseKy = ky.create({ prefixUrl: API_BASE })

let isRefreshing = false
let refreshPromise: Promise<void> | null = null

async function refreshAccessToken(): Promise<void> {
  const { refreshToken, setTokens } = useAuthStore.getState()
  if (!refreshToken) throw new Error('No refresh token available')
  const res = await baseKy
    .post('auth/refresh', { json: { refresh_token: refreshToken } })
    .json<{ data: { access_token: string; refresh_token: string } }>()
  setTokens(res.data.access_token, res.data.refresh_token)
}

/**
 * 冷启动门闩：access token 刻意不持久化，刷新页面后并发请求都没有 token，
 * 会各自 401 再重试。这里在请求前统一等一次 refresh，让首轮请求直接带上新 token。
 */
function ensureAccessToken(): Promise<void> {
  const { accessToken, refreshToken } = useAuthStore.getState()
  if (accessToken || !refreshToken) return Promise.resolve()
  if (!isRefreshing) {
    isRefreshing = true
    refreshPromise = refreshAccessToken()
      .catch(() => undefined)
      .finally(() => {
        isRefreshing = false
        refreshPromise = null
      })
  }
  return refreshPromise ?? Promise.resolve()
}

function refreshOnce(): Promise<void> {
  if (!isRefreshing) {
    isRefreshing = true
    refreshPromise = refreshAccessToken().finally(() => {
      isRefreshing = false
      refreshPromise = null
    })
  }
  return refreshPromise ?? Promise.resolve()
}

async function readErrorBody(response: Response): Promise<ApiErrorBody> {
  try {
    const data = (await response.json()) as { error?: ApiErrorBody }
    return data.error ?? {}
  } catch {
    return {}
  }
}

function apiErrorFrom(status: number, body: ApiErrorBody): ApiRequestError {
  return new ApiRequestError(body.code || 'UNKNOWN', body.message || '请求失败', status)
}

const replayed = new WeakSet<Request>()

export const apiClient = baseKy.extend({
  retry: {
    // 401 的 ApiRequestError 不是 ky 的 HTTPError，默认重试判定不出"不可重试"，
    // 会让刷新链路被放大成 refresh×3。这里显式禁止，保证"一次 refresh + 一次重放"上限。
    shouldRetry: ({ error }) =>
      error instanceof ApiRequestError && error.status === 401 ? false : undefined,
  },
  hooks: {
    beforeRequest: [
      async (request) => {
        await ensureAccessToken()
        const { accessToken } = useAuthStore.getState()
        const { activeOrgId, personalOrgId } = useWorkspaceStore.getState()
        if (accessToken) request.headers.set('Authorization', `Bearer ${accessToken}`)
        // 🔴 多租户：activeOrgId 为 null = 个人空间 ⇒ 也要发个人空间 ID。
        // 不带 X-Org-Id 的列表接口只会返回"当前个人空间"的数据（实测 9 vs 11 条）。
        const orgId = activeOrgId ?? personalOrgId
        if (orgId) request.headers.set('X-Org-Id', orgId)
      },
    ],
    afterResponse: [
      async (request, _options, response) => {
        // 只在 401 分支预读错误体，且缓存下来（响应流只能消费一次）。
        let preReadBody: ApiErrorBody | null = null

        if (response.status === 401 && !request.url.includes('auth/refresh')) {
          preReadBody = await readErrorBody(response)
          const retriable =
            preReadBody.code === 'UNAUTHORIZED' &&
            !replayed.has(request) &&
            Boolean(useAuthStore.getState().refreshToken)

          if (retriable) {
            replayed.add(request)
            try {
              await refreshOnce()
            } catch {
              throw apiErrorFrom(response.status, preReadBody)
            }
            const { accessToken } = useAuthStore.getState()
            if (accessToken) request.headers.set('Authorization', `Bearer ${accessToken}`)
            const replay = await baseKy(request, { throwHttpErrors: false })
            if (replay.ok) return replay
            throw apiErrorFrom(replay.status, await readErrorBody(replay))
          }
        }

        if (!response.ok) {
          throw apiErrorFrom(response.status, preReadBody ?? (await readErrorBody(response)))
        }
        return response
      },
    ],
  },
})
