import { beforeEach, describe, expect, it, vi } from 'vitest'

/**
 * authStore 持久化 / 迁移 / merge 集成测试（T02）。
 *
 * 用真实 `localStorage`（jsdom）+ 重导入 store 的方式，验证冷启动 rehydrate 的端到端行为：
 *   - INV-1：`merge` 无条件把运行时 `activeOrgId`/`personalOrgId` 置 null（任何来源都无法污染）
 *   - 迁移：v0（隐式）→ v1（显式）结构性剥离旧字段 + 合成按 userId 的记忆
 *   - merge：只保留 refreshToken/user/workspaceMemory，记忆经 parseWorkspaceMemory 归一
 *   - R10：JSON 损坏 / 迁移异常 → 静默降级为全空（绝不抛、不白屏）
 *
 * zustand persist 对同步 storage（localStorage）是**同步 rehydrate**（在 store 创建时即完成），
 * 故 `await import(...)` 返回时状态已就绪。
 */

const STORAGE_KEY = 'trustmesh-v2:auth'

/** 清空存储、可选注入原始条目、重置模块后重新导入 authStore，返回其当时的状态快照。 */
async function rehydrateStore(rawEntry?: string | Record<string, unknown>) {
  vi.resetModules()
  localStorage.clear()
  if (rawEntry !== undefined) {
    localStorage.setItem(STORAGE_KEY, typeof rawEntry === 'string' ? rawEntry : JSON.stringify(rawEntry))
  }
  const mod = await import('@/stores/authStore')
  return { store: mod.useAuthStore, state: mod.useAuthStore.getState() }
}

/** 读取当前落盘的持久化条目。 */
function readPersisted(): { state: Record<string, unknown>; version?: number } {
  return JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}') as {
    state: Record<string, unknown>
    version?: number
  }
}

describe('authStore 持久化 rehydrate', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('无任何落盘数据 → 全空初始态', async () => {
    const { state } = await rehydrateStore()
    expect(state.refreshToken).toBeNull()
    expect(state.user).toBeNull()
    expect(state.activeOrgId).toBeNull()
    expect(state.personalOrgId).toBeNull()
    expect(state.workspaceMemory).toBeNull()
    // F2 门控信号非持久化 → 冷启动必为 false（关闸）
    expect(state.workspaceCalibrated).toBe(false)
  })

  it('v0 旧结构（含两个 org id）→ 剥离运行时态 + 合成企业记忆', async () => {
    const { state } = await rehydrateStore({
      state: {
        refreshToken: 'r-1',
        user: { id: 'u-1', email: 'a@example.com' },
        activeOrgId: 'org-enterprise-1',
        personalOrgId: 'org-personal-1',
      },
      version: 0,
    })

    // INV-1：运行时两 id 恒为 null（不从持久化恢复）
    expect(state.activeOrgId).toBeNull()
    expect(state.personalOrgId).toBeNull()
    // 会话凭证保留
    expect(state.refreshToken).toBe('r-1')
    expect((state.user as { id?: string } | null)?.id).toBe('u-1')
    // 记忆合成（按 userId 维度）
    expect(state.workspaceMemory).toEqual({
      userId: 'u-1',
      kind: 'enterprise',
      orgId: 'org-enterprise-1',
    })
  })

  it('v0 仅含 personalOrgId → 合成个人记忆', async () => {
    const { state } = await rehydrateStore({
      state: {
        refreshToken: 'r-1',
        user: { id: 'u-1' },
        activeOrgId: null,
        personalOrgId: 'org-personal-1',
      },
      version: 0,
    })

    expect(state.activeOrgId).toBeNull()
    expect(state.personalOrgId).toBeNull()
    expect(state.workspaceMemory).toEqual({ userId: 'u-1', kind: 'personal' })
  })

  it('v0 无 user.id → 不合成记忆', async () => {
    const { state } = await rehydrateStore({
      state: { refreshToken: 'r-1', user: null, activeOrgId: 'org-x', personalOrgId: 'org-p' },
      version: 0,
    })
    expect(state.refreshToken).toBe('r-1')
    expect(state.workspaceMemory).toBeNull()
  })

  it('v0 迁移后被回写为 v1（且不含两个 org id）', async () => {
    await rehydrateStore({
      state: {
        refreshToken: 'r-1',
        user: { id: 'u-1' },
        activeOrgId: 'org-enterprise-1',
        personalOrgId: 'org-personal-1',
      },
      version: 0,
    })

    const persisted = readPersisted()
    expect(persisted.version).toBe(1)
    expect(persisted.state.activeOrgId).toBeUndefined()
    expect(persisted.state.personalOrgId).toBeUndefined()
    expect(Object.keys(persisted.state).sort()).toEqual(['refreshToken', 'user', 'workspaceMemory'])
    expect(persisted.state.workspaceMemory).toEqual({
      userId: 'u-1',
      kind: 'enterprise',
      orgId: 'org-enterprise-1',
    })
  })

  it('v1 正常数据 → 记忆经 parse 归一保留，运行时两 id 仍为 null', async () => {
    const { state } = await rehydrateStore({
      state: {
        refreshToken: 'r-1',
        user: { id: 'u-1' },
        workspaceMemory: { userId: 'u-1', kind: 'enterprise', orgId: 'org-enterprise-1' },
      },
      version: 1,
    })

    expect(state.workspaceMemory).toEqual({
      userId: 'u-1',
      kind: 'enterprise',
      orgId: 'org-enterprise-1',
    })
    expect(state.activeOrgId).toBeNull()
    expect(state.personalOrgId).toBeNull()
  })

  it('v1 被篡改（含旧两个 org id 与脏记忆）→ 运行时态仍被硬置 null', async () => {
    const { state } = await rehydrateStore({
      state: {
        refreshToken: 'r-1',
        user: { id: 'u-1' },
        activeOrgId: 'org-hacked',
        personalOrgId: 'org-hacked-personal',
        workspaceMemory: { userId: 123, kind: 'bogus' },
        // 篡改者试图伪造门控已开：merge 不读该字段 → 仍取 current 的 false
        workspaceCalibrated: true,
      },
      version: 1,
    })

    // INV-1：任何来源（含手工篡改）都无法让运行时态非空
    expect(state.activeOrgId).toBeNull()
    expect(state.personalOrgId).toBeNull()
    // 非法记忆 → parse 归一为 null
    expect(state.workspaceMemory).toBeNull()
    // F2 门控非持久化：伪造的 true 不会穿过 merge，冷启动仍关闸
    expect(state.workspaceCalibrated).toBe(false)
  })

  it('R10：JSON 损坏 → 静默降级为全空（不抛、不白屏）', async () => {
    await expect(rehydrateStore('{ this is not valid json')).resolves.toBeDefined()

    const { state } = await rehydrateStore('{ this is not valid json')
    expect(state.refreshToken).toBeNull()
    expect(state.user).toBeNull()
    expect(state.activeOrgId).toBeNull()
    expect(state.personalOrgId).toBeNull()
    expect(state.workspaceMemory).toBeNull()
  })
})
