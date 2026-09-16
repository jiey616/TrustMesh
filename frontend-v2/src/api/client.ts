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

// ============================================================================
// 401 决策：区分「access token 过期/无效」与「其它 401」，并给刷新重放加上限
// ============================================================================
//
// 后端事实（读源码确认，非猜测）：
//   - access token 缺失/无效/过期：middleware/auth.go:26,33,38 → transport.Unauthorized(...)
//     → transport/response.go:65-67 固定写死 code="UNAUTHORIZED"。
//   - 租户上下文非法（带 X-Org-Id 但非该 org 成员）：middleware/org_scope.go:29
//     → 同样 transport.Unauthorized(...) → code 也是 "UNAUTHORIZED"。
//   即：**两者的 error.code 完全相同**，无法只靠 code 区分，只能靠 message 兜底识别租户越权。
//   这是后端契约缺口，建议后续给 org_scope 一个独立 code（如 NOT_A_MEMBER）。

/** 只有这些 code 才代表「access token 本身有问题」，值得 refresh + 重放。 */
const REFRESHABLE_401_CODES: ReadonlySet<string> = new Set(['UNAUTHORIZED'])

/** 明确属于「非 token 过期」的 401 message 特征：org_scope 的租户越权拒绝。 */
const ORG_SCOPE_401_HINTS: readonly string[] = ['not a member of the requested organization']

/**
 * 已被「刷新 + 重放」处理过的请求。重放会复用同一 Request 语义，若该请求再次 401，
 * 绝不允许二次 refresh / 二次重放（防御性上限，配合下方结构化「只重放一次」）。
 */
const replayedRequests = new WeakSet<Request>()

interface ApiErrorBody {
  code?: string
  message?: string
  details?: Record<string, unknown>
}

/** 读取后端统一错误体 { error: { code, message, details } }，不可解析时返回空对象。 */
async function readErrorBody(response: Response): Promise<ApiErrorBody> {
  try {
    const data = (await response.json()) as { error?: ApiErrorBody }
    return data.error ?? {}
  } catch {
    // 响应体不可解析（非 JSON / 空体）→ 无 code，交由 isRefreshableToken401 保守处理。
    return {}
  }
}

function apiErrorFrom(status: number, body: ApiErrorBody): ApiRequestError {
  return new ApiRequestError(
    body.code || 'UNKNOWN',
    body.message || 'Request failed',
    status,
    body.details || {},
  )
}

/**
 * 判定一次 401 是否属于「access token 过期/无效」——只有这种情况才值得 refresh+重放。
 *
 * 策略（写死于此，不做含糊处理）：
 *   1. **拿不到 code（响应体不可解析）→ 判为「不可刷新」，直接抛错、绝不重放。**
 *      理由：本后端所有错误恒经 transport.WriteError 输出 {error:{code,...}}，
 *      正常的 token 过期 401 必然带 code；拿不到 code 说明链路中存在非标准代理/网关。
 *      对这类 401 盲目 refresh+重放，正是「refresh/401 风暴」的来源，而保守不放行
 *      的代价为零（真正的过期 401 一定有 code）。
 *   2. code 不在 REFRESHABLE_401_CODES 白名单 → 不可刷新。
 *   3. code 命中白名单，但 message 命中租户越权特征（org_scope 非成员）→ 不可刷新。
 */
function isRefreshableToken401(body: ApiErrorBody): boolean {
  if (!body.code || !REFRESHABLE_401_CODES.has(body.code)) return false
  const message = (body.message ?? '').toLowerCase()
  if (ORG_SCOPE_401_HINTS.some((hint) => message.includes(hint))) return false
  return true
}

/** 单飞刷新：并发 401 共享同一次 refresh；失败时把错误抛给调用方。 */
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

export const apiClient = ky.create({
  prefixUrl: API_BASE,
  retry: {
    // afterResponse 会把失败响应归一为 ApiRequestError（不带 ky 的 HTTPError 语义）。
    // ky 的默认重试无法把非 HTTPError 当作「不可重试」，会对 GET 无差别重试（最多 3 次），
    // 从而把 401「刷新 + 重放」链路重复触发（实测 refresh 被调用 3 次）。
    // 这里让 401 的 ApiRequestError 表现得与 ky 原生 HTTPError(401) 完全一致——不重试，
    // 保证「一次 refresh + 一次重放」这个上限成立；其余错误返回 undefined，沿用 ky 默认策略。
    shouldRetry: ({ error }) =>
      error instanceof ApiRequestError && error.status === 401 ? false : undefined,
  },
  hooks: {
    beforeRequest: [
      async (request) => {
        await ensureAccessToken()
        const { accessToken, activeOrgId, personalOrgId } = useAuthStore.getState()
        if (accessToken) {
          request.headers.set('Authorization', `Bearer ${accessToken}`)
        }
        // 多租户上下文：activeOrgId 为 null = 个人空间 → 发个人租户 ID 真头；
        // personalOrgId 尚未水合（冷启动首轮）时才退回无头，走后端 user 维度兜底。
        const orgId = activeOrgId ?? personalOrgId
        if (orgId) {
          request.headers.set('X-Org-Id', orgId)
        }
      },
    ],
    afterResponse: [
      async (request, _options, response) => {
        // 仅在 401 且非刷新端点本身时，才评估「刷新 token 后重放」。
        // 读到的错误体缓存下来，保证只消费一次 response 流。
        let preReadBody: ApiErrorBody | null = null

        if (response.status === 401 && !request.url.includes('auth/refresh')) {
          preReadBody = await readErrorBody(response)

          const retriable =
            isRefreshableToken401(preReadBody) &&
            !replayedRequests.has(request) &&
            Boolean(useAuthStore.getState().refreshToken)

          if (retriable) {
            // 打标记 + 结构化上限：同一请求至多「刷新一次 + 重放一次」，其后一律抛错。
            replayedRequests.add(request)
            try {
              await refreshOnce()
            } catch {
              // 刷新本身失败：不再重放，按原始 401 抛错，把真实 code 交给调用方。
              throw apiErrorFrom(response.status, preReadBody)
            }

            const { accessToken } = useAuthStore.getState()
            if (accessToken) {
              request.headers.set('Authorization', `Bearer ${accessToken}`)
            }
            // 用「无 hooks 的默认 ky」重放：重放不会再次进入本 afterResponse，
            // 从结构上杜绝 401 → refresh → 重放 → 401 → refresh … 的无界循环。
            const replay = await ky(request, { throwHttpErrors: false })
            if (replay.ok) {
              return replay
            }
            // 换新 token 后依旧 401：根因不是 token 过期（例如租户上下文非法），
            // 不再刷新、不再重放，抛后端真实错误。
            throw apiErrorFrom(replay.status, await readErrorBody(replay))
          }
          // 非「token 过期」类 401（含租户越权）、已重放过、或无 refreshToken：
          // 落到下方统一的失败分支，绝不触发 refresh。
        }

        if (!response.ok) {
          // preReadBody 仅在 401 分支被设置过，其它错误码现场读取，避免二次消费。
          throw apiErrorFrom(response.status, preReadBody ?? (await readErrorBody(response)))
        }
      },
    ],
  },
})
