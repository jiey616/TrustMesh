import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { apiClient } from '@/api/client'
import { useAuthStore } from '@/stores/authStore'

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
})
