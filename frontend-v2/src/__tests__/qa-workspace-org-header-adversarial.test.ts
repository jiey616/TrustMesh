import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { apiClient } from '@/api/client'
import { resolveWorkspaceTarget } from '@/lib/workspaceMemory'
import { useAuthStore } from '@/stores/authStore'
import type { User, WorkspaceMemory } from '@/types'

/**
 * 【QA 对抗验收 · 独立文件】T04 请求头层对抗测试（真实触发 ky 钩子）。
 *
 * 覆盖 AC1 / AC3 / AC4 / AC5 与 INV-3 的**行为级**证据：
 *   - 跨账号：B 登录后首轮所有请求的 `X-Org-Id` 绝不等于 A 的任何 org id
 *   - 无 401 风暴：首轮请求的 `x-org-id ∈ {undefined, 本账号 org}`
 *   - 校验请求（GET /organizations）先于任何带 org 头的请求
 *   - 冷启动 refresh 端点自身不带 org 头
 *
 * 注：`api/client.ts` 冻结不改，头来源保持 `activeOrgId ?? personalOrgId`。
 */

interface Captured {
  url: string
  method: string
  headers: Record<string, string>
}

let calls: Captured[]
let responder: (req: Request, index: number) => Response | Promise<Response>

const ORGS_URL = 'http://localhost:8080/api/v1/organizations'
const PROJECTS_URL = 'http://localhost:8080/api/v1/projects'

const A_ORGS = { enterprise: 'org-A-ent', personal: 'org-A-personal' }

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  })
}

function mkUser(id: string): User {
  return { id, name: id, email: `${id}@example.com` } as unknown as User
}

function orgHeaders(): Array<string | undefined> {
  return calls.map((c) => c.headers['x-org-id'])
}

beforeEach(() => {
  calls = []
  responder = () => jsonResponse({ data: { items: [] } })
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
    accessToken: 'tok-1',
    refreshToken: 'r-1',
    user: mkUser('u-1'),
    activeOrgId: null,
    personalOrgId: null,
    workspaceMemory: null,
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('[QA] AC1/AC5：换账号后首轮请求绝不携带上一账号的 org id', () => {
  it('A(带企业+个人 org 与记忆) → setAuth(B) → 首轮 4 个请求的 x-org-id 均 ≠ A 的任何 org', async () => {
    // A 的会话残留（含运行时两 id 与 A 的记忆）
    useAuthStore.setState({
      activeOrgId: A_ORGS.enterprise,
      personalOrgId: A_ORGS.personal,
      workspaceMemory: { userId: 'u-A', kind: 'enterprise', orgId: A_ORGS.enterprise } as WorkspaceMemory,
    })

    // B 登录：setAuth 必须复位两个运行时 id
    useAuthStore.getState().setAuth('tok-B', 'r-B', mkUser('u-B'))
    expect(useAuthStore.getState().activeOrgId).toBeNull()
    expect(useAuthStore.getState().personalOrgId).toBeNull()

    // B 的首批请求（MainLayout 挂载即触发的四个查询）
    await Promise.all([
      apiClient.get('organizations'),
      apiClient.get('projects'),
      apiClient.get('notifications/unread-count'),
      apiClient.get('external-apps'),
    ])

    for (const [i, h] of orgHeaders().entries()) {
      expect(h, `call#${i} ${calls[i].url}`).not.toBe(A_ORGS.enterprise)
      expect(h, `call#${i} ${calls[i].url}`).not.toBe(A_ORGS.personal)
      // 更严：首轮 B 尚未校准 → 头必须为 undefined（退化 user 维度）
      expect(h, `call#${i} ${calls[i].url}`).toBeUndefined()
    }
  })

  it('AC5：冷启动（记忆=企业 X，两 id 均 null）→ 首轮请求 x-org-id ∈ {undefined}，无 401 风暴前提', async () => {
    useAuthStore.setState({
      workspaceMemory: { userId: 'u-1', kind: 'enterprise', orgId: 'org-e-1' } as WorkspaceMemory,
    })

    await Promise.all([
      apiClient.get('organizations'),
      apiClient.get('projects'),
      apiClient.get('notifications/unread-count'),
      apiClient.get('external-apps'),
    ])

    // 两 id 皆 null ⇒ 首轮全部无 org 头（后端 user 维度兜底，不因陈旧头 401）
    expect(orgHeaders().every((h) => h === undefined)).toBe(true)
  })
})

describe('[QA] INV-3：租户校验请求天然无头，且先于任何带 org 头的请求', () => {
  it('GET /organizations（无头）先于带 org 头的重取请求', async () => {
    const memory = { userId: 'u-1', kind: 'enterprise', orgId: 'org-e-1' } as WorkspaceMemory
    const orgs = [
      { id: 'org-p-1', kind: 'personal' as const },
      { id: 'org-e-1', kind: 'enterprise' as const },
    ]
    useAuthStore.setState({ workspaceMemory: memory })

    // ① 首轮：无头校验请求
    await apiClient.get('organizations')
    // ② 校准 effect：同一 tick 内先写个人、再写已校验企业 id
    const nextActive = resolveWorkspaceTarget(memory, 'u-1', orgs)
    expect(nextActive).toBe('org-e-1')
    useAuthStore.getState().setPersonalOrgId('org-p-1')
    useAuthStore.getState().setActiveOrg(nextActive)
    // ③ 校准后重取
    await apiClient.get('projects')

    const orgCallIdx = calls.findIndex((c) => c.url === ORGS_URL)
    const projCallIdx = calls.findIndex((c) => c.url === PROJECTS_URL)
    expect(orgCallIdx).toBeGreaterThan(-1)
    expect(projCallIdx).toBeGreaterThan(-1)
    expect(calls[orgCallIdx].headers['x-org-id']).toBeUndefined()
    expect(calls[projCallIdx].headers['x-org-id']).toBe('org-e-1')
    // 关键时序：带 org 头的请求必在校验请求之后
    expect(orgCallIdx).toBeLessThan(projCallIdx)
  })

  it('AC4：记忆失效（org 不在本账号 orgs）→ 回落个人空间，重取带 personalOrgId（非企业）', async () => {
    const stale = { userId: 'u-1', kind: 'enterprise', orgId: 'org-gone' } as WorkspaceMemory
    useAuthStore.setState({ workspaceMemory: stale })

    const nextActive = resolveWorkspaceTarget(stale, 'u-1', [{ id: 'org-p-1', kind: 'personal' }])
    expect(nextActive).toBeNull()
    useAuthStore.getState().setPersonalOrgId('org-p-1')
    useAuthStore.getState().setActiveOrg(nextActive)

    await apiClient.get('projects')

    expect(useAuthStore.getState().activeOrgId).toBeNull()
    expect(calls[0].headers['x-org-id']).toBe('org-p-1')
  })

  it('AC3：个人空间路径（activeOrgId 为 null）→ 头只能是 personalOrgId', async () => {
    useAuthStore.setState({ activeOrgId: null, personalOrgId: 'org-p-1' })
    await apiClient.get('projects')
    expect(calls[0].headers['x-org-id']).toBe('org-p-1')
  })
})

describe('[QA] 冷启动门闩：refresh 端点自身不带 org 头', () => {
  it('accessToken 缺失时先 refresh，refresh 请求无 X-Org-Id，业务请求按当时运行时态取头', async () => {
    useAuthStore.setState({
      accessToken: null,
      refreshToken: 'r-old',
      activeOrgId: null,
      personalOrgId: null,
      workspaceMemory: { userId: 'u-1', kind: 'enterprise', orgId: 'org-e-1' } as WorkspaceMemory,
    })
    responder = (req) =>
      req.url.includes('auth/refresh')
        ? jsonResponse({ data: { access_token: 'fresh', refresh_token: 'r-new', expires_in: 3600 } })
        : jsonResponse({ data: { items: [] } })

    await apiClient.get('organizations')

    const refreshCall = calls.find((c) => c.url.includes('auth/refresh'))
    const bizCall = calls.find((c) => !c.url.includes('auth/refresh'))
    expect(refreshCall).toBeDefined()
    expect(bizCall).toBeDefined()
    // refresh 端点不得携带被校验的租户头
    expect(refreshCall!.headers['x-org-id']).toBeUndefined()
    // 业务请求仍为无头（两 id 均 null）→ 后端 user 维度校验
    expect(bizCall!.headers['x-org-id']).toBeUndefined()
  })
})

describe('[QA] 攻击面2（请求层）：personalOrgId 污染在头层的可见性', () => {
  it('personalOrgId 为 null 时绝不发头；为已校验个人值时发个人头——绝无第三条路径', async () => {
    useAuthStore.setState({ activeOrgId: null, personalOrgId: null })
    await apiClient.get('projects')
    expect(calls[0].headers['x-org-id']).toBeUndefined()

    calls = []
    useAuthStore.setState({ activeOrgId: null, personalOrgId: 'org-p-own' })
    await apiClient.get('projects')
    expect(calls[0].headers['x-org-id']).toBe('org-p-own')
  })

  it('activeOrgId 优先于 personalOrgId（企业态下发企业头）', async () => {
    useAuthStore.setState({ activeOrgId: 'org-e-1', personalOrgId: 'org-p-1' })
    await apiClient.get('projects')
    expect(calls[0].headers['x-org-id']).toBe('org-e-1')
  })
})
