import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { apiClient } from '@/api/client'
import { useAuthStore } from '@/stores/authStore'
import { ApiRequestError } from '@/types'

/**
 * 多租户请求头契约（T1.5）。
 *
 * 这些断言证明「同一份代码在不同租户上下文下会发出不同的请求头」——
 * 这正是后端 X-Org-Id 隔离生效的前提。仅断言源码字符串是不够的：
 * 这里真实触发 ky 的 beforeRequest 钩子并把最终 Request 抓下来检查。
 */

interface Captured {
  url: string
  method: string
  headers: Record<string, string>
}

let calls: Captured[]
let responder: (req: Request, index: number) => Response | Promise<Response>

const OK = () => jsonResponse({ data: { ok: true } })

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  })
}

beforeEach(() => {
  calls = []
  responder = OK
  vi.stubGlobal('fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
    const req = input instanceof Request ? input : new Request(input as string, init)
    const headers: Record<string, string> = {}
    req.headers.forEach((value, key) => {
      headers[key.toLowerCase()] = value
    })
    const index = calls.length
    calls.push({ url: req.url, method: req.method, headers })
    return responder(req, index)
  })
  useAuthStore.setState({
    accessToken: null,
    refreshToken: null,
    user: null,
    activeOrgId: null,
    personalOrgId: null,
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('apiClient 租户请求头', () => {
  it('切到企业空间时带上该企业的 X-Org-Id 与 Bearer token', async () => {
    useAuthStore.setState({ accessToken: 'tok-1', activeOrgId: 'org-enterprise', personalOrgId: 'org-personal' })

    await apiClient.get('agents')

    expect(calls).toHaveLength(1)
    expect(calls[0].headers['x-org-id']).toBe('org-enterprise')
    expect(calls[0].headers['authorization']).toBe('Bearer tok-1')
  })

  it('个人空间（activeOrgId 为空）回落到 personalOrgId 发真头', async () => {
    useAuthStore.setState({ accessToken: 'tok-2', activeOrgId: null, personalOrgId: 'org-personal' })

    await apiClient.get('agents')

    expect(calls[0].headers['x-org-id']).toBe('org-personal')
  })

  it('两个租户 id 都未水合时不发 X-Org-Id，交给后端按 user 维度兜底', async () => {
    useAuthStore.setState({ accessToken: 'tok-3', activeOrgId: null, personalOrgId: null })

    await apiClient.get('agents')

    expect(calls[0].headers['x-org-id']).toBeUndefined()
    expect(calls[0].headers['authorization']).toBe('Bearer tok-3')
  })

  it('没有 accessToken 时不发 Authorization 头', async () => {
    useAuthStore.setState({ accessToken: null, refreshToken: 'r-1', activeOrgId: 'org-x', personalOrgId: null })
    // 有 refreshToken 会先走冷启动门闩，让 refresh 立即失败以放行原请求
    responder = (req) =>
      req.url.includes('auth/refresh')
        ? jsonResponse({ error: { code: 'INVALID', message: 'no' } }, 401)
        : OK()

    await apiClient.get('agents')

    const dataCall = calls.find((c) => !c.url.includes('auth/refresh'))
    expect(dataCall).toBeDefined()
    expect(dataCall!.headers['authorization']).toBeUndefined()
    expect(dataCall!.headers['x-org-id']).toBe('org-x')
  })

  it('401 时自动 refresh 并用新 token 重放原请求', async () => {
    useAuthStore.setState({ accessToken: 'stale', refreshToken: 'r-old', activeOrgId: null, personalOrgId: 'org-personal' })

    responder = (req, index) => {
      if (req.url.includes('auth/refresh')) {
        return jsonResponse({ data: { access_token: 'fresh', refresh_token: 'r-new', expires_in: 3600 } })
      }
      // 第一次业务请求返回 401，触发 afterResponse 的刷新分支
      return index === 0
        ? jsonResponse({ error: { code: 'UNAUTHORIZED', message: 'expired' } }, 401)
        : OK()
    }

    await apiClient.get('agents')

    expect(calls.map((c) => c.url)).toEqual([
      'http://localhost:8080/api/v1/agents',
      'http://localhost:8080/api/v1/auth/refresh',
      'http://localhost:8080/api/v1/agents',
    ])
    const replay = calls[calls.length - 1]
    expect(replay.headers['authorization']).toBe('Bearer fresh')
    expect(useAuthStore.getState().accessToken).toBe('fresh')
  })

  // ── 401 刷新重放的上限与甄别（本缺陷的关键闸门）─────────────────────────────
  // 后端事实：token 过期（middleware/auth.go）与租户非成员（middleware/org_scope.go:29）
  // 都经 transport.Unauthorized(...)，code 同为 "UNAUTHORIZED"，只有 message 不同。
  // 因此 client 必须以 message 兜底区分，且刷新重放必须有界、终态必须抛真实 code。

  it('业务端点持续 401 时只刷新一次、至多重放一次，并抛出带真实 code 的 ApiRequestError', async () => {
    useAuthStore.setState({
      accessToken: 'stale',
      refreshToken: 'r-old',
      activeOrgId: null,
      personalOrgId: 'org-personal',
    })

    // 业务端点永远 401（token 过期语义）；刷新端点每次都成功换发新 token。
    responder = (req) =>
      req.url.includes('auth/refresh')
        ? jsonResponse({ data: { access_token: 'fresh', refresh_token: 'r-new', expires_in: 3600 } })
        : jsonResponse({ error: { code: 'UNAUTHORIZED', message: 'invalid or expired token' } }, 401)

    const err = await apiClient.get('agents').then(
      () => {
        throw new Error('expected the request to reject')
      },
      (e: unknown) => e,
    )

    const refreshCalls = calls.filter((c) => c.url.includes('auth/refresh'))
    const bizCalls = calls.filter((c) => !c.url.includes('auth/refresh'))

    // ① refresh 次数有界：恰好 1 次（不会持续 401 → 不断刷新）
    expect(refreshCalls).toHaveLength(1)
    // ② 业务端点请求次数有界：≤2（原始 1 次 + 重放 1 次）
    expect(bizCalls.length).toBeLessThanOrEqual(2)
    // ③ 终态失败语义正确：抛 ApiRequestError 且携带后端真实 code（而非 HTTPError）
    expect(err).toBeInstanceOf(ApiRequestError)
    expect((err as ApiRequestError).code).toBe('UNAUTHORIZED')
  })

  it('租户越权 401（非成员 org）不触发 refresh，直接抛出带真实 code 的 ApiRequestError', async () => {
    // 模拟 authStore——包含已持久化、但用户已不属于的陈旧租户 id（真实触发场景）。
    useAuthStore.setState({
      accessToken: 'valid-not-expired',
      refreshToken: 'r-old',
      activeOrgId: 'org-stale',
      personalOrgId: 'org-personal',
    })

    responder = (req) =>
      req.url.includes('auth/refresh')
        ? jsonResponse({ data: { access_token: 'fresh', refresh_token: 'r-new', expires_in: 3600 } })
        : jsonResponse(
            { error: { code: 'UNAUTHORIZED', message: 'not a member of the requested organization' } },
            401,
          )

    const err = await apiClient.get('agents').then(
      () => {
        throw new Error('expected the request to reject')
      },
      (e: unknown) => e,
    )

    // 关键：租户越权 401 绝不能触发 refresh（否则就是无谓的 refresh 风暴源头）。
    expect(calls.filter((c) => c.url.includes('auth/refresh'))).toHaveLength(0)
    // 落到统一失败分支，把后端真实 code 交给调用方。
    expect(err).toBeInstanceOf(ApiRequestError)
    expect((err as ApiRequestError).code).toBe('UNAUTHORIZED')
  })
})
