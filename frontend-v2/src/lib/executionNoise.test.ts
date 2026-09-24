import { describe, expect, it } from 'vitest'
import { EXECUTION_NOISE_EVENTS, isExecutionNoise } from './executionNoise'

// 任务「执行过程」的默认折叠筛选。
//
// 用户口径：执行总控反复派发、执行侧的进度上报这类**机械动作**不要占主视图，
// 但它们是唯一的审计轨迹 ⇒ 前端只能默认收起、不能删。判据是事件类型字符串。
describe('isExecutionNoise', () => {
  it('机械噪声事件全部判为噪声', () => {
    for (const t of EXECUTION_NOISE_EVENTS) {
      expect(isExecutionNoise(t), t).toBe(true)
    }
  })

  it('非字符串 / 空值一律不判为噪声（fail-safe：宁可多显示）', () => {
    expect(isExecutionNoise(null)).toBe(false)
    expect(isExecutionNoise(undefined)).toBe(false)
    expect(isExecutionNoise('')).toBe(false)
    expect(isExecutionNoise('todo_progress_v2')).toBe(false) // 前缀相似但不命中
  })

  it('🔴 承载故事线的骨架事件绝不能被折进噪声', () => {
    // 每个步骤的 分配→开始→完成 是执行过程的主干，第一眼就要看到。
    const skeleton = ['todo_assigned', 'todo_started', 'todo_completed']
    for (const t of skeleton) {
      expect(isExecutionNoise(t), t).toBe(false)
    }
  })

  it('🔴 失败 / 待确认 / 提问类事件绝不能被折进噪声', () => {
    // 这些是「要人做决定」的信号，必须留在主视图最显眼处。
    const mustSee = [
      'todo_failed',
      'todo_hard_deadline_failed',
      'todo_awaiting_review',
      'todo_ask_received',
      'todo_rework_exhausted',
      'todo_remind_escalated',
      'task_plan_ready',
      'task_status_changed',
    ]
    for (const t of mustSee) {
      expect(isExecutionNoise(t), t).toBe(false)
    }
  })

  it('噪声清单与「必须看见」清单严格互斥（防有人顺手加错）', () => {
    const mustSee = [
      'todo_assigned',
      'todo_started',
      'todo_completed',
      'todo_failed',
      'todo_hard_deadline_failed',
      'todo_awaiting_review',
      'todo_ask_received',
      'todo_rework_exhausted',
      'todo_remind_escalated',
      'task_plan_ready',
      'task_status_changed',
    ]
    for (const t of mustSee) {
      expect(EXECUTION_NOISE_EVENTS, t).not.toContain(t)
    }
  })

  it('清单本身无重复项', () => {
    expect(new Set(EXECUTION_NOISE_EVENTS).size).toBe(EXECUTION_NOISE_EVENTS.length)
  })
})
