import { beforeEach, describe, expect, it } from 'vitest'

import { resolveWorkspaceTarget } from '@/lib/workspaceMemory'
import { useAuthStore } from '@/stores/authStore'
import type { User } from '@/types'

/**
 * 租户上下文的生命周期（T1.5 扩充）。
 *
 * 跨租户串数据的根因有两类：请求头发错租户，或缓存跨租户存活。
 * 这里覆盖第一类的状态侧：登出必须把租户上下文彻底清空，
 * 否则下一次登录会在 personalOrgId/activeOrgId 上继承上一账号的值。
 *
 * T2.5「记住上次选中的空间」在此追加「记忆 vs 运行时态分离」的行为断言：
 *   - AC2：登出清空两个运行时租户 id（回归既有契约）
 *   - AC1 / AC6：换账号不继承，且解析出的工作区目标不读取他人记忆
 *   - 记忆（workspaceMemory）按 userId 隔离、写入带守门
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
  workspaceCalibrated: false,
}

describe('authStore 租户上下文', () => {
  beforeEach(() => {
    useAuthStore.setState(resetState)
  })

  it('登出清空令牌与两个租户 id', () => {
    useAuthStore.setState({
      accessToken: 'a',
      refreshToken: 'r',
      user: userA,
      activeOrgId: 'org-enterprise',
      personalOrgId: 'org-personal',
    })

    useAuthStore.getState().logout()

    const state = useAuthStore.getState()
    expect(state.accessToken).toBeNull()
    expect(state.refreshToken).toBeNull()
    expect(state.user).toBeNull()
    expect(state.activeOrgId).toBeNull()
    expect(state.personalOrgId).toBeNull()
    expect(state.isAuthenticated()).toBe(false)
  })

  it('切换工作区后 activeOrgId 生效，个人租户 id 保留以便回落', () => {
    useAuthStore.getState().setPersonalOrgId('org-personal')
    useAuthStore.getState().setActiveOrg('org-enterprise')

    const state = useAuthStore.getState()
    expect(state.activeOrgId).toBe('org-enterprise')
    // 个人租户 id 不能被切换覆盖，否则切回个人空间时会退化成无租户头
    expect(state.personalOrgId).toBe('org-personal')

    useAuthStore.getState().setActiveOrg(null)
    expect(useAuthStore.getState().activeOrgId).toBeNull()
    expect(useAuthStore.getState().personalOrgId).toBe('org-personal')
  })

  it('partialize 白名单只含 refreshToken/user/workspaceMemory（运行时两 id 不落盘）', () => {
    useAuthStore.setState({
      accessToken: 'a',
      refreshToken: 'r',
      user: userA,
      activeOrgId: 'org-enterprise',
      personalOrgId: 'org-personal',
      workspaceMemory: { userId: 'u-A', kind: 'enterprise', orgId: 'org-enterprise' },
    })

    const persisted = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}') as {
      state: Record<string, unknown>
    }
    // INV-1：accessToken 与两个运行时 id 一律不落盘
    expect(persisted.state.accessToken).toBeUndefined()
    expect(persisted.state.activeOrgId).toBeUndefined()
    expect(persisted.state.personalOrgId).toBeUndefined()
    // F2 门控信号非持久化，同样绝不落盘
    expect(persisted.state.workspaceCalibrated).toBeUndefined()
    expect(Object.keys(persisted.state).sort()).toEqual(['refreshToken', 'user', 'workspaceMemory'])
  })
})

describe('authStore 工作区记忆与跨账号隔离（T2.5）', () => {
  beforeEach(() => {
    useAuthStore.setState(resetState)
  })

  it('R1：setAuth（换账号）复位两个运行时 id，不继承上一会话', () => {
    useAuthStore.setState({ activeOrgId: 'org-A-ent', personalOrgId: 'org-A-personal' })

    useAuthStore.getState().setAuth('tok-B', 'ref-B', userB)

    const state = useAuthStore.getState()
    expect(state.activeOrgId).toBeNull()
    expect(state.personalOrgId).toBeNull()
    expect(state.user?.id).toBe('u-B')
  })

  it('AC1/AC6：A 会话 → logout → B 登录，两 id 为 null 且不读取 A 的记忆', () => {
    // A 会话：选中企业空间并留下记忆
    useAuthStore.getState().setAuth('tok-A', 'ref-A', userA)
    useAuthStore.getState().setActiveOrg('org-A-ent')
    useAuthStore.getState().setPersonalOrgId('org-A-personal')
    useAuthStore.getState().rememberWorkspace('enterprise', 'org-A-ent')

    useAuthStore.getState().logout()

    // B 登录（复用同一 store 实例，模拟换账号）
    useAuthStore.getState().setAuth('tok-B', 'ref-B', userB)

    const state = useAuthStore.getState()
    expect(state.activeOrgId).toBeNull()
    expect(state.personalOrgId).toBeNull()

    // 即便 A 的记忆仍在存储里，解析目标也因 userId 守门而回落个人空间
    const orgs = [{ id: 'org-A-ent', kind: 'enterprise' as const }]
    expect(resolveWorkspaceTarget(state.workspaceMemory, state.user?.id, orgs)).toBeNull()
  })

  it('R2：logout 保留 workspaceMemory（记忆是经校验的提示数据，非运行时态）', () => {
    useAuthStore.getState().setAuth('tok-A', 'ref-A', userA)
    useAuthStore.getState().rememberWorkspace('enterprise', 'org-A-ent')

    useAuthStore.getState().logout()

    const state = useAuthStore.getState()
    expect(state.activeOrgId).toBeNull()
    expect(state.personalOrgId).toBeNull()
    expect(state.workspaceMemory).toEqual({
      userId: 'u-A',
      kind: 'enterprise',
      orgId: 'org-A-ent',
    })
  })

  it('rememberWorkspace 带 userId 守门：未登录不写记忆', () => {
    // beforeEach 已把 user 置 null
    useAuthStore.getState().rememberWorkspace('enterprise', 'org-x')
    expect(useAuthStore.getState().workspaceMemory).toBeNull()
  })

  it('rememberWorkspace 记录企业空间：需带 orgId，否则忽略', () => {
    useAuthStore.getState().setAuth('tok-A', 'ref-A', userA)

    // 企业分支缺 orgId → 忽略
    useAuthStore.getState().rememberWorkspace('enterprise')
    expect(useAuthStore.getState().workspaceMemory).toBeNull()

    useAuthStore.getState().rememberWorkspace('enterprise', 'org-A-ent')
    expect(useAuthStore.getState().workspaceMemory).toEqual({
      userId: 'u-A',
      kind: 'enterprise',
      orgId: 'org-A-ent',
    })
  })

  it('rememberWorkspace 记录个人空间：kind=personal（不带 orgId）', () => {
    useAuthStore.getState().setAuth('tok-A', 'ref-A', userA)

    useAuthStore.getState().rememberWorkspace('personal')
    expect(useAuthStore.getState().workspaceMemory).toEqual({ userId: 'u-A', kind: 'personal' })
  })

  it('记忆随 setAuth 切换账号后按新用户重写（不残留旧 userId）', () => {
    useAuthStore.getState().setAuth('tok-A', 'ref-A', userA)
    useAuthStore.getState().rememberWorkspace('personal')

    useAuthStore.getState().setAuth('tok-B', 'ref-B', userB)
    useAuthStore.getState().rememberWorkspace('enterprise', 'org-B-ent')

    expect(useAuthStore.getState().workspaceMemory).toEqual({
      userId: 'u-B',
      kind: 'enterprise',
      orgId: 'org-B-ent',
    })
  })
})

describe('authStore F2 门控信号 workspaceCalibrated', () => {
  beforeEach(() => {
    useAuthStore.setState(resetState)
  })

  it('R-init：初值为 false（冷启动默认关闸）', () => {
    expect(useAuthStore.getState().workspaceCalibrated).toBe(false)
  })

  it('setWorkspaceCalibrated 可开闸 / 手动关闸', () => {
    useAuthStore.getState().setWorkspaceCalibrated(true)
    expect(useAuthStore.getState().workspaceCalibrated).toBe(true)

    useAuthStore.getState().setWorkspaceCalibrated(false)
    expect(useAuthStore.getState().workspaceCalibrated).toBe(false)
  })

  it('R-reset：setAuth（换账号）把已开闸的门控复位为 false', () => {
    useAuthStore.getState().setWorkspaceCalibrated(true)

    useAuthStore.getState().setAuth('tok-B', 'ref-B', userB)

    expect(useAuthStore.getState().workspaceCalibrated).toBe(false)
  })

  it('R-reset：logout 把已开闸的门控复位为 false', () => {
    useAuthStore.getState().setAuth('tok-A', 'ref-A', userA)
    useAuthStore.getState().setWorkspaceCalibrated(true)

    useAuthStore.getState().logout()

    expect(useAuthStore.getState().workspaceCalibrated).toBe(false)
  })
})
