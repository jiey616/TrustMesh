import { describe, expect, it } from 'vitest'
import { notificationTarget } from './notifications'

// 站内通知 → 点击跳转路由的唯一实现（收件箱与桌面端系统通知横幅共用）。
describe('notificationTarget', () => {
  it('入职申请（agent 类且文案含「入职」「申请加入」）→ 协作邀请页', () => {
    expect(notificationTarget({ category: 'agent', title: '张三申请入职' })).toBe('/agent-invite')
    expect(notificationTarget({ category: 'agent', body: '李四申请加入团队' })).toBe('/agent-invite')
  })

  it('agent 类且有 actor_id → 该数字员工详情', () => {
    expect(notificationTarget({ category: 'agent', actor_id: 'a1', title: '数字员工已上线' })).toBe('/agents/a1')
  })

  it('有 task_id → 直达任务工作台（?task= 深链，ProjectBoardPage 支持）', () => {
    expect(notificationTarget({ category: 'todo', task_id: 't1', project_id: 'p1' })).toBe(
      '/projects/p1?task=t1',
    )
  })

  it('只有 project_id → 项目看板', () => {
    expect(notificationTarget({ category: 'task', project_id: 'p1' })).toBe('/projects/p1')
  })

  it('优先级顺序：agent 详情 优先于 task_id', () => {
    // 同一条通知可能既有 actor_id 又有 task_id，规则必须与既有收件箱行为一致（agent 优先）。
    expect(
      notificationTarget({ category: 'agent', actor_id: 'a1', task_id: 't1', project_id: 'p1' }),
    ).toBe('/agents/a1')
  })

  it('无可用目标 → null（调用方据此决定不跳或兜底到收件箱）', () => {
    expect(notificationTarget({})).toBeNull()
    expect(notificationTarget({ category: 'system', title: '系统维护通知' })).toBeNull()
  })

  it('🔴 SSE payload 缺字段时不能抛异常（JSON 里缺字段是 undefined，不是空串）', () => {
    expect(() => notificationTarget({ category: 'agent' })).not.toThrow()
    expect(notificationTarget({ category: 'agent' })).toBeNull()
    expect(notificationTarget({ category: 'agent', actor_id: 'a1' })).toBe('/agents/a1')
  })
})
