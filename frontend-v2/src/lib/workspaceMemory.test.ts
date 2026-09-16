import { describe, expect, it } from 'vitest'

import {
  migratePersistedAuth,
  parseWorkspaceMemory,
  resolveWorkspaceTarget,
  type WorkspaceOrgRef,
} from '@/lib/workspaceMemory'
import type { WorkspaceMemory } from '@/types'

/**
 * 工作区记忆纯函数单测（T01）。
 *
 * 覆盖设计验收映射：
 *   - AC6 / R3：记忆按 userId 维度隔离，绝不读取他人记忆
 *   - AC4 / R5：记忆中的企业 org 不在本账号 orgs 时安全回落（不抛、不弹窗）
 *   - R10：结构损坏、迁移异常一律降级为「无记忆」，绝不抛
 *   - 迁移合成：旧 activeOrgId / personalOrgId → 按 userId 维度的记忆
 */

const ORGS: WorkspaceOrgRef[] = [
  { id: 'org-personal-1', kind: 'personal' },
  { id: 'org-enterprise-1', kind: 'enterprise' },
  { id: 'org-enterprise-2', kind: 'enterprise' },
]

describe('resolveWorkspaceTarget —— 记忆解析与安全回落', () => {
  const enterpriseMemory: WorkspaceMemory = {
    userId: 'u-1',
    kind: 'enterprise',
    orgId: 'org-enterprise-1',
  }

  it('记忆=企业且 org ∈ 本账号 orgs → 返回该企业 org id（正向恢复 R8）', () => {
    expect(resolveWorkspaceTarget(enterpriseMemory, 'u-1', ORGS)).toBe('org-enterprise-1')
  })

  it('无记忆 → null', () => {
    expect(resolveWorkspaceTarget(null, 'u-1', ORGS)).toBeNull()
    expect(resolveWorkspaceTarget(undefined, 'u-1', ORGS)).toBeNull()
  })

  it('未登录（currentUserId 为空）→ null', () => {
    expect(resolveWorkspaceTarget(enterpriseMemory, null, ORGS)).toBeNull()
    expect(resolveWorkspaceTarget(enterpriseMemory, undefined, ORGS)).toBeNull()
    expect(resolveWorkspaceTarget(enterpriseMemory, '', ORGS)).toBeNull()
  })

  it('R3/AC6：记忆属于他人（userId 不匹配）→ 绝不读取 → null', () => {
    expect(resolveWorkspaceTarget(enterpriseMemory, 'u-2', ORGS)).toBeNull()
  })

  it('记忆=个人空间 → null（回落个人空间）', () => {
    const personal: WorkspaceMemory = { userId: 'u-1', kind: 'personal' }
    expect(resolveWorkspaceTarget(personal, 'u-1', ORGS)).toBeNull()
  })

  it('企业记忆但缺失 orgId → null', () => {
    const broken: WorkspaceMemory = { userId: 'u-1', kind: 'enterprise' }
    expect(resolveWorkspaceTarget(broken, 'u-1', ORGS)).toBeNull()
  })

  it('R5/AC4：记忆 org 不在本账号 orgs（被移出 / 企业被删）→ null', () => {
    const stale: WorkspaceMemory = { userId: 'u-1', kind: 'enterprise', orgId: 'org-gone' }
    expect(resolveWorkspaceTarget(stale, 'u-1', ORGS)).toBeNull()
  })

  it('R5：记忆 org 命中 id 但 kind 非企业（数据错配）→ null', () => {
    const mismatch: WorkspaceMemory = {
      userId: 'u-1',
      kind: 'enterprise',
      orgId: 'org-personal-1',
    }
    expect(resolveWorkspaceTarget(mismatch, 'u-1', ORGS)).toBeNull()
  })

  it('orgs 未就绪（null / 空数组）→ null（保守回落）', () => {
    expect(resolveWorkspaceTarget(enterpriseMemory, 'u-1', null)).toBeNull()
    expect(resolveWorkspaceTarget(enterpriseMemory, 'u-1', undefined)).toBeNull()
    expect(resolveWorkspaceTarget(enterpriseMemory, 'u-1', [])).toBeNull()
  })
})

describe('parseWorkspaceMemory —— 结构归一化与损坏降级', () => {
  it('合法企业记忆 → 规范形状', () => {
    expect(parseWorkspaceMemory({ userId: 'u-1', kind: 'enterprise', orgId: 'org-e' })).toEqual({
      userId: 'u-1',
      kind: 'enterprise',
      orgId: 'org-e',
    })
  })

  it('合法个人记忆 → 规范形状（裁剪多余 orgId）', () => {
    expect(parseWorkspaceMemory({ userId: 'u-1', kind: 'personal', orgId: 'x' })).toEqual({
      userId: 'u-1',
      kind: 'personal',
    })
  })

  it('非对象 / 空值 → null', () => {
    expect(parseWorkspaceMemory(null)).toBeNull()
    expect(parseWorkspaceMemory(undefined)).toBeNull()
    expect(parseWorkspaceMemory('personal')).toBeNull()
    expect(parseWorkspaceMemory(42)).toBeNull()
    expect(parseWorkspaceMemory([])).toBeNull()
  })

  it('userId 缺失 / 非字符串 / 空串 → null', () => {
    expect(parseWorkspaceMemory({ kind: 'personal' })).toBeNull()
    expect(parseWorkspaceMemory({ userId: 123, kind: 'personal' })).toBeNull()
    expect(parseWorkspaceMemory({ userId: '', kind: 'personal' })).toBeNull()
  })

  it('kind 非法 → null', () => {
    expect(parseWorkspaceMemory({ userId: 'u-1', kind: 'team' })).toBeNull()
    expect(parseWorkspaceMemory({ userId: 'u-1' })).toBeNull()
  })

  it('企业记忆缺失 / 空 orgId → null', () => {
    expect(parseWorkspaceMemory({ userId: 'u-1', kind: 'enterprise' })).toBeNull()
    expect(parseWorkspaceMemory({ userId: 'u-1', kind: 'enterprise', orgId: '' })).toBeNull()
    expect(parseWorkspaceMemory({ userId: 'u-1', kind: 'enterprise', orgId: 5 })).toBeNull()
  })

  it('全部为非总（绝不抛）', () => {
    expect(() => parseWorkspaceMemory(Symbol('x'))).not.toThrow()
    expect(parseWorkspaceMemory(Symbol('x'))).toBeNull()
  })
})

describe('migratePersistedAuth —— 存量迁移与结构性剥离', () => {
  it('v0 含 activeOrgId + user.id → 合成企业记忆 + 剥离旧字段', () => {
    const result = migratePersistedAuth(
      {
        refreshToken: 'r-1',
        user: { id: 'u-1', email: 'a@example.com' },
        activeOrgId: 'org-enterprise-1',
        personalOrgId: 'org-personal-1',
      },
      0,
    )

    expect(result.refreshToken).toBe('r-1')
    expect(result.user).toEqual({ id: 'u-1', email: 'a@example.com' })
    expect(result.workspaceMemory).toEqual({
      userId: 'u-1',
      kind: 'enterprise',
      orgId: 'org-enterprise-1',
    })
    // 结构性剥离：返回体绝不含旧运行时字段
    expect(result).not.toHaveProperty('activeOrgId')
    expect(result).not.toHaveProperty('personalOrgId')
  })

  it('v0 仅含 personalOrgId（无 activeOrgId）→ 合成个人记忆', () => {
    const result = migratePersistedAuth(
      { refreshToken: 'r-1', user: { id: 'u-1' }, activeOrgId: null, personalOrgId: 'org-personal-1' },
      0,
    )
    expect(result.workspaceMemory).toEqual({ userId: 'u-1', kind: 'personal' })
  })

  it('v0 无 user.id → 不合成记忆，但保留 refreshToken', () => {
    const result = migratePersistedAuth({ refreshToken: 'r-1', user: null, activeOrgId: 'org-x' }, 0)
    expect(result.refreshToken).toBe('r-1')
    expect(result.workspaceMemory).toBeNull()
  })

  it('v0 activeOrgId 为空串 → 回落到 personalOrgId 合成个人记忆', () => {
    const result = migratePersistedAuth(
      { user: { id: 'u-1' }, activeOrgId: '', personalOrgId: 'org-personal-1' },
      0,
    )
    expect(result.workspaceMemory).toEqual({ userId: 'u-1', kind: 'personal' })
  })

  it('空持久化值 → 全空（不抛）', () => {
    expect(migratePersistedAuth(undefined, 0)).toEqual({
      refreshToken: null,
      user: null,
      workspaceMemory: null,
    })
    expect(migratePersistedAuth(null, 0)).toEqual({
      refreshToken: null,
      user: null,
      workspaceMemory: null,
    })
  })

  it('非对象输入（字符串 / 数字 / 数组）→ 全空（不抛）', () => {
    const empty = { refreshToken: null, user: null, workspaceMemory: null }
    expect(migratePersistedAuth('corrupted', 0)).toEqual(empty)
    expect(migratePersistedAuth(7, 0)).toEqual(empty)
    expect(migratePersistedAuth([], 0)).toEqual(empty)
  })

  it('R10：属性访问抛错时被 try/catch 兜住 → 全空（绝不抛）', () => {
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

  it('refreshToken 非字符串 → 归一为 null', () => {
    expect(migratePersistedAuth({ refreshToken: 123, user: { id: 'u-1' } }, 0).refreshToken).toBeNull()
  })
})
