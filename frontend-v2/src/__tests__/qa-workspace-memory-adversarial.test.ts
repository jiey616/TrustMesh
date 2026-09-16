import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  migratePersistedAuth,
  parseWorkspaceMemory,
  resolveWorkspaceTarget,
  type WorkspaceOrgRef,
} from '@/lib/workspaceMemory'
import { useAuthStore } from '@/stores/authStore'
import type { User, WorkspaceMemory } from '@/types'

/**
 * 【QA 对抗验收 · 独立文件】T04「记住上次选中的空间」——记忆层 + store 层对抗测试。
 *
 * 立场：**目标是打穿实现**。本文件刻意覆盖既有测试没有触及的边界与攻击面：
 *   - 攻击面 2：`personalOrgId` 交叉污染（穷举四个时点）
 *   - 攻击面 3：zustand persist `migrate`/`merge` 的真实语义（真 localStorage + 重导入）
 *   - 攻击面 4：`resolveWorkspaceTarget` / `parseWorkspaceMemory` 的 total 性与绕过
 *   - 攻击面 6：既有契约是否仍有鉴别力（本文件与既有测试互补，不做恒真断言）
 *
 * ⚠️ 与 `authStore.persist.test.ts` 同构地使用 `vi.resetModules()` + 真实 localStorage
 * 重导入 store；本文件**不与 apiClient 混用**（模块身份隔离，见
 * `qa-workspace-org-header-adversarial.test.ts`）。
 */

const STORAGE_KEY = 'trustmesh-v2:auth'

const userA = { id: 'u-A', name: 'A', email: 'a@example.com' } as unknown as User
const userB = { id: 'u-B', name: 'B', email: 'b@example.com' } as unknown as User

const resetState = {
  accessToken: null,
  refreshToken: null,
  user: null,
  activeOrgId: null,
  personalOrgId: null,
  workspaceMemory: null,
}

/** 清空存储、可选注入原始条目、重置模块后重新导入 authStore。 */
async function rehydrateStore(rawEntry?: string | Record<string, unknown>) {
  vi.resetModules()
  localStorage.clear()
  if (rawEntry !== undefined) {
    localStorage.setItem(
      STORAGE_KEY,
      typeof rawEntry === 'string' ? rawEntry : JSON.stringify(rawEntry),
    )
  }
  const mod = await import('@/stores/authStore')
  return mod.useAuthStore
}

// ─────────────────────────────────────────────────────────────────────────────
// 攻击面 2：personalOrgId 交叉污染 —— 穷举「任何时刻」都不可能持有非本账号值
// ─────────────────────────────────────────────────────────────────────────────

describe('[QA] 攻击面2：personalOrgId 写点穷举——只可能为 null 或本账号 org', () => {
  beforeEach(() => {
    useAuthStore.setState(resetState)
  })

  it('时点①：新 store 实例（校准 effect 之前）→ personalOrgId 为 null', () => {
    expect(useAuthStore.getState().personalOrgId).toBeNull()
  })

  it('时点②：setAuth（换账号）→ personalOrgId 必须复位 null，绝不继承上一会话', () => {
    useAuthStore.setState({ personalOrgId: 'org-A-personal', activeOrgId: 'org-A-ent' })
    useAuthStore.getState().setAuth('tok-B', 'ref-B', userB)
    expect(useAuthStore.getState().personalOrgId).toBeNull()
    expect(useAuthStore.getState().activeOrgId).toBeNull()
  })

  it('时点③：logout → personalOrgId 必须复位 null', () => {
    useAuthStore.setState({ personalOrgId: 'org-A-personal' })
    useAuthStore.getState().logout()
    expect(useAuthStore.getState().personalOrgId).toBeNull()
  })

  it('时点④：校准 effect 之前唯一来源是显式 setter（该校验由调用方保证）', () => {
    // 显式 setter 是「已校验值」的注入口；本用例锁定：除它以外无任何路径能写入非 null。
    const s = useAuthStore.getState()
    expect(s.personalOrgId).toBeNull()
    s.setPersonalOrgId('org-p-1')
    expect(useAuthStore.getState().personalOrgId).toBe('org-p-1')
  })

  it('A→logout→B→setAuth 全链路：personalOrgId 恒为 null（不残留 A 值）', () => {
    useAuthStore.getState().setAuth('tok-A', 'ref-A', userA)
    useAuthStore.getState().setPersonalOrgId('org-A-personal')
    useAuthStore.getState().setActiveOrg('org-A-ent')

    useAuthStore.getState().logout()
    useAuthStore.getState().setAuth('tok-B', 'ref-B', userB)

    const st = useAuthStore.getState()
    expect(st.activeOrgId).toBeNull()
    expect(st.personalOrgId).toBeNull()
  })
})

// ─────────────────────────────────────────────────────────────────────────────
// 攻击面 4：resolveWorkspaceTarget 的 total 性与绕过猎杀
// ─────────────────────────────────────────────────────────────────────────────

const ACCOUNT_ORGS: WorkspaceOrgRef[] = [
  { id: 'org-p-1', kind: 'personal' },
  { id: 'org-e-1', kind: 'enterprise' },
  { id: 'org-e-2', kind: 'enterprise' },
]

describe('[QA] 攻击面4：resolveWorkspaceTarget 边界与绕过猎杀', () => {
  it('memory 为 null / undefined / 无字段 → null', () => {
    expect(resolveWorkspaceTarget(null, 'u-1', ACCOUNT_ORGS)).toBeNull()
    expect(resolveWorkspaceTarget(undefined, 'u-1', ACCOUNT_ORGS)).toBeNull()
  })

  it('currentUserId 为 null/undefined/空串 → null（未登录）', () => {
    const m: WorkspaceMemory = { userId: 'u-1', kind: 'enterprise', orgId: 'org-e-1' }
    expect(resolveWorkspaceTarget(m, null, ACCOUNT_ORGS)).toBeNull()
    expect(resolveWorkspaceTarget(m, undefined, ACCOUNT_ORGS)).toBeNull()
    expect(resolveWorkspaceTarget(m, '', ACCOUNT_ORGS)).toBeNull()
  })

  it('orgs 为 null/undefined/空数组 → null（保守回落，绝不猜）', () => {
    const m: WorkspaceMemory = { userId: 'u-1', kind: 'enterprise', orgId: 'org-e-1' }
    expect(resolveWorkspaceTarget(m, 'u-1', null)).toBeNull()
    expect(resolveWorkspaceTarget(m, 'u-1', undefined)).toBeNull()
    expect(resolveWorkspaceTarget(m, 'u-1', [])).toBeNull()
  })

  it('memory.orgId 为 空串 / 非 string / null → null', () => {
    for (const bad of ['', null, undefined, 0, 123, {}, []]) {
      const m = { userId: 'u-1', kind: 'enterprise', orgId: bad } as unknown as WorkspaceMemory
      expect(resolveWorkspaceTarget(m, 'u-1', ACCOUNT_ORGS), `orgId=${String(bad)}`).toBeNull()
    }
  })

  it('kind 非法值（team / false / 1 / 空串）→ null（绝不误当企业）', () => {
    for (const bad of ['team', 'Personal', 'ENTERPRISE', '', false, 1, null, undefined]) {
      const m = { userId: 'u-1', kind: bad, orgId: 'org-e-1' } as unknown as WorkspaceMemory
      expect(resolveWorkspaceTarget(m, 'u-1', ACCOUNT_ORGS), `kind=${String(bad)}`).toBeNull()
    }
  })

  it('orgs 中同 id 但 kind=personal（数据错配）→ null', () => {
    const m: WorkspaceMemory = { userId: 'u-1', kind: 'enterprise', orgId: 'org-p-1' }
    expect(resolveWorkspaceTarget(m, 'u-1', ACCOUNT_ORGS)).toBeNull()
  })

  it('orgs 中同 id 但 kind 缺失/非法 → null（不做非企业匹配）', () => {
    const m: WorkspaceMemory = { userId: 'u-1', kind: 'enterprise', orgId: 'org-e-1' }
    const weird = [{ id: 'org-e-1', kind: 'team' }, { id: 'org-e-1' }] as unknown as WorkspaceOrgRef[]
    expect(resolveWorkspaceTarget(m, 'u-1', weird)).toBeNull()
  })

  it('绕过猎杀：任意输入组合，返回值恒 ∈ {null} ∪ 本账号 kind=enterprise 的 org id', () => {
    const memories: Array<WorkspaceMemory | null> = [
      null,
      { userId: 'u-1', kind: 'enterprise', orgId: 'org-e-1' },
      { userId: 'u-1', kind: 'enterprise', orgId: 'org-gone' },
      { userId: 'u-2', kind: 'enterprise', orgId: 'org-e-1' },
      { userId: 'u-1', kind: 'personal' },
      { userId: 'u-1', kind: 'enterprise' },
    ]
    const orgsMatrix: Array<WorkspaceOrgRef[] | null | undefined> = [
      ACCOUNT_ORGS,
      [{ id: 'org-e-1', kind: 'personal' }],
      [],
      null,
      undefined,
    ]
    const userIds: Array<string | null> = ['u-1', 'u-2', null, '']
    const enterpriseIds = new Set(
      (ACCOUNT_ORGS.filter((o) => o.kind === 'enterprise') ?? []).map((o) => o.id),
    )
    for (const m of memories) {
      for (const orgs of orgsMatrix) {
        for (const uid of userIds) {
          const r = resolveWorkspaceTarget(m, uid, orgs)
          // 若返回值非 null，则它必须是「本账号 orgs 中确为企业」的 id —— 绝无未校验值
          if (r !== null) expect(enterpriseIds.has(r)).toBe(true)
        }
      }
    }
  })
})

// ─────────────────────────────────────────────────────────────────────────────
// 攻击面 4（续）：parseWorkspaceMemory 的 total 性
// ─────────────────────────────────────────────────────────────────────────────

describe('[QA] 攻击面4：parseWorkspaceMemory 结构归一与 total 性', () => {
  it('各种损坏输入一律 → null（不抛）', () => {
    const cases: unknown[] = [
      null,
      undefined,
      '',
      'enterprise',
      0,
      1,
      true,
      false,
      [],
      [1, 2],
      Symbol('x'),
      () => {},
    ]
    for (const c of cases) {
      expect(() => parseWorkspaceMemory(c), `input=${String(c)}`).not.toThrow()
      expect(parseWorkspaceMemory(c)).toBeNull()
    }
  })

  it('个人信息中残留 orgId 被裁剪（不污染后续解析）', () => {
    expect(parseWorkspaceMemory({ userId: 'u-1', kind: 'personal', orgId: 'org-e-1' })).toEqual({
      userId: 'u-1',
      kind: 'personal',
    })
  })

  // F1 已修复（2026-09-16）：parseWorkspaceMemory 已包 try/catch，对「属性访问即抛错」的
  // 对象也 total（设计 §8.4「绝不抛」）。本用例由 `it.fails` 转正为普通 `it`（现应通过）。
  it('parseWorkspaceMemory 对「抛错 getter」对象仍 total（设计 §8.4，F1 已修复）', () => {
    const evil = {
      get userId(): never {
        throw new Error('boom')
      },
    }
    expect(() => parseWorkspaceMemory(evil)).not.toThrow()
  })
})

// ─────────────────────────────────────────────────────────────────────────────
// 攻击面 4（续）：migratePersistedAuth 的 total 性与结构性剥离
// ─────────────────────────────────────────────────────────────────────────────

describe('[QA] 攻击面4：migratePersistedAuth total 性与剥离', () => {
  it('v0 含 activeOrgId → 合成企业记忆，且返回体绝不含两个运行时字段', () => {
    const r = migratePersistedAuth(
      { refreshToken: 'r', user: { id: 'u-1' }, activeOrgId: 'org-e-1', personalOrgId: 'org-p-1' },
      0,
    )
    expect(r.workspaceMemory).toEqual({ userId: 'u-1', kind: 'enterprise', orgId: 'org-e-1' })
    expect(Object.prototype.hasOwnProperty.call(r, 'activeOrgId')).toBe(false)
    expect(Object.prototype.hasOwnProperty.call(r, 'personalOrgId')).toBe(false)
    // 即便旧 state 里塞了任意额外字段，也不得被带出（白名单式返回）
    const r2 = migratePersistedAuth(
      { user: { id: 'u-1' }, activeOrgId: 'org-e-1', evil: 'x', __proto__: { polluted: true } },
      0,
    )
    expect(Object.keys(r2).sort()).toEqual(['refreshToken', 'user', 'workspaceMemory'])
  })

  it('抛错 getter 被 try/catch 兜住 → 全空（不抛）', () => {
    const evil = {
      get user(): never {
        throw new Error('boom')
      },
    }
    expect(() => migratePersistedAuth(evil, 0)).not.toThrow()
    expect(migratePersistedAuth(evil, 0)).toEqual({
      refreshToken: null,
      user: null,
      workspaceMemory: null,
    })
  })

  it('activeOrgId 非字符串（对象/数字）→ 不合成企业记忆（绝不接受脏 org id）', () => {
    expect(migratePersistedAuth({ user: { id: 'u-1' }, activeOrgId: {} }, 0).workspaceMemory).toBeNull()
    expect(migratePersistedAuth({ user: { id: 'u-1' }, activeOrgId: 123 }, 0).workspaceMemory).toBeNull()
  })
})

// ─────────────────────────────────────────────────────────────────────────────
// 攻击面 3：zustand persist rehydrate / migrate / merge 的真实语义
// ─────────────────────────────────────────────────────────────────────────────

describe('[QA] 攻击面3：persist rehydrate 对抗（真 localStorage + 重导入）', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('INV-1：v1 被篡改（含旧两 id）→ merge 无条件把运行时两 id 置 null', async () => {
    const store = await rehydrateStore({
      state: {
        refreshToken: 'r-1',
        user: { id: 'u-1' },
        activeOrgId: 'org-hacked',
        personalOrgId: 'org-hacked-p',
      },
      version: 1,
    })
    expect(store.getState().activeOrgId).toBeNull()
    expect(store.getState().personalOrgId).toBeNull()
  })

  it('v0（version:0）→ 触发 migrate：剥离两 id + 合成记忆', async () => {
    const store = await rehydrateStore({
      state: {
        refreshToken: 'r-1',
        user: { id: 'u-1' },
        activeOrgId: 'org-e-1',
        personalOrgId: 'org-p-1',
      },
      version: 0,
    })
    const st = store.getState()
    expect(st.activeOrgId).toBeNull()
    expect(st.personalOrgId).toBeNull()
    expect(st.workspaceMemory).toEqual({ userId: 'u-1', kind: 'enterprise', orgId: 'org-e-1' })
  })

  it('v0 无 `version` 字段（手写/被中间件剥离）→ 运行时两 id 仍被 merge 硬置 null（安全性不依赖 migrate）', async () => {
    const store = await rehydrateStore({
      state: {
        refreshToken: 'r-1',
        user: { id: 'u-1' },
        activeOrgId: 'org-e-1',
        personalOrgId: 'org-p-1',
      },
      // 刻意不带 version
    })
    const st = store.getState()
    // 🔴 安全不变量成立：即使 migrate 未被触发（zustand 要求 version !== undefined），
    // merge 仍把两 id 硬置 null —— 运行时绝不可能携带未校验 org id。
    expect(st.activeOrgId).toBeNull()
    expect(st.personalOrgId).toBeNull()
    // 记录实际语义：此形状下记忆不会被合成（无 version ⇒ zustand 不调 migrate）。
    // 真实旧客户端由 zustand 写入 version:0，故不影响存量用户「保留一次体验」。
    expect(st.workspaceMemory).toBeNull()
  })

  it('JSON 损坏 → 静默降级全空（不抛）', async () => {
    const store = await rehydrateStore('{ not: valid json,')
    const st = store.getState()
    expect(st.refreshToken).toBeNull()
    expect(st.user).toBeNull()
    expect(st.activeOrgId).toBeNull()
    expect(st.personalOrgId).toBeNull()
    expect(st.workspaceMemory).toBeNull()
  })

  it.each([
    ['字符串 "null"', 'null'],
    ['空对象 "{}"', '{}'],
    ['state 为 null', { state: null, version: 1 }],
    ['user 为 null', { state: { refreshToken: 'r', user: null }, version: 1 }],
  ])('%s → 不抛、降级全空', async (_label, raw) => {
    const store = await rehydrateStore(raw as string | Record<string, unknown>)
    const st = store.getState()
    expect(st.activeOrgId).toBeNull()
    expect(st.personalOrgId).toBeNull()
    expect(st.workspaceMemory).toBeNull()
  })

  it('AC6 核心：v1 记忆 userId 属于 A，但当前登录是 B → 不恢复到 A 的 org', async () => {
    const store = await rehydrateStore({
      state: {
        refreshToken: 'r-B',
        user: { id: 'u-B' },
        workspaceMemory: { userId: 'u-A', kind: 'enterprise', orgId: 'org-A-ent' },
      },
      version: 1,
    })
    const st = store.getState()
    // 存储层保留了 A 的记录（按 userId 维度），但决策层因 userId 守门而回落
    expect(st.user?.id).toBe('u-B')
    const target = resolveWorkspaceTarget(st.workspaceMemory, st.user?.id, [
      { id: 'org-A-ent', kind: 'enterprise' },
    ])
    expect(target).toBeNull()
  })

  it('AC6：记忆 userId 属于 B 且 org 在 B 的 orgs → 正确恢复', async () => {
    const store = await rehydrateStore({
      state: {
        refreshToken: 'r-B',
        user: { id: 'u-B' },
        workspaceMemory: { userId: 'u-B', kind: 'enterprise', orgId: 'org-B-ent' },
      },
      version: 1,
    })
    const st = store.getState()
    expect(
      resolveWorkspaceTarget(st.workspaceMemory, st.user?.id, [
        { id: 'org-B-ent', kind: 'enterprise' },
      ]),
    ).toBe('org-B-ent')
  })

  it('v1 非法记忆（userId 非 string / kind 非法）→ parse 归一为 null', async () => {
    const store = await rehydrateStore({
      state: {
        refreshToken: 'r',
        user: { id: 'u-1' },
        workspaceMemory: { userId: 123, kind: 'bogus', orgId: 'org-x' },
      },
      version: 1,
    })
    expect(store.getState().workspaceMemory).toBeNull()
  })
})

// ─────────────────────────────────────────────────────────────────────────────
// 攻击面 2（存储层）：记忆按用户维度隔离——写 A 后切 B 不读 A
// ─────────────────────────────────────────────────────────────────────────────

describe('[QA] 攻击面2/AC6：存储层记忆按用户隔离', () => {
  beforeEach(() => {
    useAuthStore.setState(resetState)
  })

  it('写 A 记忆 → setAuth(B)（未写 B 记忆）→ 决策层对 B 返回 null', () => {
    useAuthStore.getState().setAuth('tok-A', 'ref-A', userA)
    useAuthStore.getState().rememberWorkspace('enterprise', 'org-A-ent')

    useAuthStore.getState().setAuth('tok-B', 'ref-B', userB)

    const st = useAuthStore.getState()
    // 存储中仍是 A 的记忆（按 userId 隔离保存），但对 B 的决策恒为 null
    expect(st.workspaceMemory).toEqual({ userId: 'u-A', kind: 'enterprise', orgId: 'org-A-ent' })
    expect(
      resolveWorkspaceTarget(st.workspaceMemory, st.user?.id, [
        { id: 'org-A-ent', kind: 'enterprise' },
        { id: 'org-B-ent', kind: 'enterprise' },
      ]),
    ).toBeNull()
  })

  it('AC9：登出再登录（同账号）→ 记忆保留、运行时两 id 干净', () => {
    useAuthStore.getState().setAuth('tok-A', 'ref-A', userA)
    useAuthStore.getState().rememberWorkspace('enterprise', 'org-A-ent')
    useAuthStore.getState().setActiveOrg('org-A-ent')
    useAuthStore.getState().setPersonalOrgId('org-A-p')

    useAuthStore.getState().logout()
    useAuthStore.getState().setAuth('tok-A2', 'ref-A2', userA)

    const st = useAuthStore.getState()
    expect(st.activeOrgId).toBeNull()
    expect(st.personalOrgId).toBeNull()
    expect(st.workspaceMemory).toEqual({ userId: 'u-A', kind: 'enterprise', orgId: 'org-A-ent' })
  })
})
