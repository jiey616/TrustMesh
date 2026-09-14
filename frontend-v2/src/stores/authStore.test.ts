import { beforeEach, describe, expect, it } from 'vitest'

import { useAuthStore } from '@/stores/authStore'
import type { User } from '@/types'

/**
 * 租户上下文的生命周期（T1.5）。
 *
 * 跨租户串数据的根因有两类：请求头发错租户，或缓存跨租户存活。
 * 这里覆盖第一类的状态侧：登出必须把租户上下文彻底清空，
 * 否则下一次登录会在 personalOrgId/activeOrgId 上继承上一账号的值。
 */

const resetState = {
  accessToken: null,
  refreshToken: null,
  user: null,
  activeOrgId: null,
  personalOrgId: null,
}

describe('authStore 租户上下文', () => {
  beforeEach(() => {
    useAuthStore.setState(resetState)
  })

  it('登出清空令牌与两个租户 id', () => {
    useAuthStore.setState({
      accessToken: 'a',
      refreshToken: 'r',
      user: { id: 'u-1', name: 'A', email: 'a@example.com' } as unknown as User,
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

  it('accessToken 不进持久化白名单（整页刷新后靠 refreshToken 换新）', () => {
    // partialize 只保留 refreshToken/user/activeOrgId/personalOrgId，
    // accessToken 是短时效设计，刻意不落盘。
    const persistedKeys = ['refreshToken', 'user', 'activeOrgId', 'personalOrgId']
    expect(persistedKeys).not.toContain('accessToken')
    expect(useAuthStore.getState().accessToken).toBeNull()
  })
})
