import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { apiClient } from '@/api/client'
import { resolveWorkspaceTarget } from '@/lib/workspaceMemory'
import { useAuthStore } from '@/stores/authStore'
import type { User, WorkspaceMemory } from '@/types'

/**
 * 工作区上下文的请求头契约（T03 / INV-3）。
 *
 * 设计不变量 INV-3：`GET /organizations`（本账号租户校验请求）**必然**发生在两个运行时
 * org id 均为 `null` 的时刻 ⇒ 该请求天然「无头」，后端退化为 user 维度返回 200，从而
 * 破除「校验请求先带陈旧 org 头 → 401 → orgs 永不就绪 → 死锁」的顺序死锁。
 *
 * 这里真实触发 ky 的 beforeRequest（stub fetch 抓最终 Request），断言：
 *   - 冷启动（两 id 均 null）→ /organizations 不带 X-Org-Id
 *   - 记忆校验通过后 → 带 org 头的请求发生在该无头请求**之后**（无中间窗口）
 *   - 记忆失效 → 回落个人空间，请求带 personalOrgId（AC3/AC4）
 *   - 换账号后 → 两 id 为 null，首个请求不带 X-Org-Id（AC1/AC5）
 *
 * 注：不改 api/client.ts；请求头来源保持 `activeOrgId ?? personalOrgId`（PRD 硬约束）。
 * 新断言刻意放在本独立文件，避免与并行维护的 api/client.test.ts 冲突。
 */

interface Captured {
  url: string
  headers: Record<string, string>
}

let calls: Captured[]
let responder: (req: Request, index: number) => Response | Promise<Response>

const ORGS_URL = 'http://localhost:8080/api/v1/organizations'
const PROJECTS_URL = 'http://localhost:8080/api/v1/projects'

const ENTERPRISE_ORGS = [
  { id: 'org-p-1', kind: 'personal' as const },
  { id: 'org-e-1', kind: 'enterprise' as const },
]

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  })
}

function mkUser(id: string): User {
  return { id, name: id, email: `${id}@example.com` } as unknown as User
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
    calls.push({ url: req.url, headers })
    return responder(req, index)
  })
  // 冷启动态：refreshToken 存在（已登录），但两个运行时 org id 均为 null（persist.merge 保证）。
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

describe('INV-3：租户校验请求天然无头', () => {
  it('冷启动两 id 均为 null → GET /organizations 不带 X-Org-Id', async () => {
    useAuthStore.setState({
      workspaceMemory: { userId: 'u-1', kind: 'enterprise', orgId: 'org-e-1' } as WorkspaceMemory,
    })

    await apiClient.get('organizations')

    expect(calls).toHaveLength(1)
    expect(calls[0].url).toBe(ORGS_URL)
    expect(calls[0].headers['x-org-id']).toBeUndefined()
    expect(calls[0].headers['authorization']).toBe('Bearer tok-1')
  })

  it('记忆校验通过后，带 org 头的请求发生在无头校验请求之后（无中间窗口）', async () => {
    const memory = { userId: 'u-1', kind: 'enterprise', orgId: 'org-e-1' } as WorkspaceMemory
    useAuthStore.setState({ workspaceMemory: memory })

    // ① 首轮：无头校验请求（t=0 两 id 皆 null）
    await apiClient.get('organizations')

    // ② 校准 effect（MainLayout）：同一 tick 内先写个人、再写已校验企业 id
    const nextPersonal = 'org-p-1'
    const nextActive = resolveWorkspaceTarget(memory, 'u-1', ENTERPRISE_ORGS)
    expect(nextActive).toBe('org-e-1')
    useAuthStore.getState().setPersonalOrgId(nextPersonal)
    useAuthStore.getState().setActiveOrg(nextActive)

    // ③ 校准后的重取：此刻读到的是最终（已校验）运行时态
    await apiClient.get('projects')

    const orgCall = calls.find((c) => c.url === ORGS_URL)
    const projCall = calls.find((c) => c.url === PROJECTS_URL)
    expect(orgCall).toBeDefined()
    expect(projCall).toBeDefined()
    // 校验请求无头；其后的重取带已校验企业头
    expect(orgCall!.headers['x-org-id']).toBeUndefined()
    expect(projCall!.headers['x-org-id']).toBe('org-e-1')
    // 顺序：无头校验请求先于任何带 org 头的请求
    expect(calls.indexOf(orgCall!)).toBeLessThan(calls.indexOf(projCall!))
  })

  it('AC4：记忆失效（org 不在本账号 orgs）→ 回落个人空间，请求带 personalOrgId', async () => {
    const stale = { userId: 'u-1', kind: 'enterprise', orgId: 'org-gone' } as WorkspaceMemory
    useAuthStore.setState({ workspaceMemory: stale })

    const orgs = [{ id: 'org-p-1', kind: 'personal' as const }]
    const nextActive = resolveWorkspaceTarget(stale, 'u-1', orgs)
    expect(nextActive).toBeNull() // 记忆失效 → 不恢复
    useAuthStore.getState().setPersonalOrgId('org-p-1')
    useAuthStore.getState().setActiveOrg(nextActive)

    await apiClient.get('projects')

    expect(useAuthStore.getState().activeOrgId).toBeNull()
    expect(calls[0].headers['x-org-id']).toBe('org-p-1')
    expect(calls[0].headers['authorization']).toBe('Bearer tok-1')
  })

  it('AC3：个人空间（activeOrgId 为 null）回落到 personalOrgId 发真头', async () => {
    useAuthStore.setState({ activeOrgId: null, personalOrgId: 'org-p-1' })

    await apiClient.get('projects')

    expect(calls[0].headers['x-org-id']).toBe('org-p-1')
  })

  it('AC1/AC5：换账号后两 id 为 null → 首个请求不带 X-Org-Id', async () => {
    // A 会话（含 A 的记忆与运行时租户）
    useAuthStore.setState({
      activeOrgId: 'org-A-ent',
      personalOrgId: 'org-A-p',
      workspaceMemory: { userId: 'u-A', kind: 'enterprise', orgId: 'org-A-ent' } as WorkspaceMemory,
    })

    // B 登录：setAuth 复位两个运行时 id
    useAuthStore.getState().setAuth('tok-B', 'r-B', mkUser('u-B'))
    expect(useAuthStore.getState().activeOrgId).toBeNull()
    expect(useAuthStore.getState().personalOrgId).toBeNull()

    await apiClient.get('organizations')

    expect(calls[0].url).toBe(ORGS_URL)
    // 不等于任何 A 的 org ⇒ 无头，后端按 user 维度校验 B 自身
    expect(calls[0].headers['x-org-id']).toBeUndefined()
  })
})
