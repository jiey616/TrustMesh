import { describe, expect, it } from 'vitest'
import { DESKTOP_NOTIFY_SOURCES, shouldNotifyDesktop } from './notifications'

// 桌面端系统通知的白名单筛选。
//
// 用户口径：只要「需要人工确认的」与「任务完成/失败」两类弹系统横幅，其余通知照常进收件箱、
// 只是不打扰人。判据必须是后端的 `source_event`（机器可读），**不能**用标题文本 ——
// 标题改文案就静默失配，不报错、只是从此不弹。
describe('shouldNotifyDesktop', () => {
  it('白名单内的 8 个来源键都弹', () => {
    const sources = [
      'task_status_changed.done', // 任务已完成
      'task_status_changed.failed', // 任务执行失败
      'todo_hard_deadline_failed', // 硬超时判失败（此路径不发 task_status_changed，必须单独列入）
      'todo_awaiting_review', // 产出已提交，等待人工确认
      'todo_ask_received', // 数字员工向你提问
      'task_plan_ready', // 规划完成，请确认后开始执行
      'todo_rework_exhausted', // 产出多次重做仍未通过，请人工介入
      'todo_remind_escalated', // 多次提醒无响应，疑似执行智能体卡死
    ]
    for (const source_event of sources) {
      expect(shouldNotifyDesktop({ source_event, title: '随便什么标题' }), source_event).toBe(true)
    }
    // 与常量逐字对齐：这里硬编码的列表漏一个、常量多一个，都要红。
    expect(sources.slice().sort()).toEqual([...DESKTOP_NOTIFY_SOURCES].sort())
  })

  it('白名单外的一律不弹（含同族的 canceled、派发失败、评论、状态变化等）', () => {
    const sources = [
      'task_status_changed.canceled',
      'agent_status_changed',
      'join_request_received',
      'planning_reply',
      'task_comment',
      'todo_failed', // 普通 todo 失败：紧随其后会发 task_status_changed.failed，无需重复弹
      'todo_dispatch_failed', // 派发失败会自动重试，不需要打扰人
      'todo_run_budget_exhausted',
      'todo_reopened',
      'todo_resumed',
      'todo_review_approved',
      'todo_rework_requested',
    ]
    for (const source_event of sources) {
      expect(shouldNotifyDesktop({ source_event, title: '任务已完成' }), source_event).toBe(false)
    }
  })

  it('🔴 缺 source_event 时不弹，且绝不退回按标题猜（老后端 / 老桌面壳）', () => {
    // 标题恰好是白名单类目的文案，但没有机器可读键 ⇒ 仍然不弹。
    // 这条断言的作用是钉死"判据只有 source_event 一个"，防止日后有人好心加一条
    // 「没有 source_event 就按标题兜底」的分支，把那套跨仓耦合并回来。
    expect(shouldNotifyDesktop({ title: '待人工确认', category: 'task' })).toBe(false)
    expect(shouldNotifyDesktop({ title: '任务已完成', category: 'task' })).toBe(false)
    expect(shouldNotifyDesktop({ source_event: '' })).toBe(false)
  })

  it('空值安全（SSE payload 字段可能缺失）', () => {
    expect(shouldNotifyDesktop(null)).toBe(false)
    expect(shouldNotifyDesktop(undefined)).toBe(false)
    expect(shouldNotifyDesktop({})).toBe(false)
  })

  it('白名单恰好 8 项且无重复（与后端 TestNotificationSourceEvent 配对）', () => {
    expect(DESKTOP_NOTIFY_SOURCES).toHaveLength(8)
    expect(new Set(DESKTOP_NOTIFY_SOURCES).size).toBe(8)
    // 「任务完成」类只认 done / failed —— canceled 不该混进来
    expect(DESKTOP_NOTIFY_SOURCES).toContain('task_status_changed.done')
    expect(DESKTOP_NOTIFY_SOURCES).toContain('task_status_changed.failed')
    expect(DESKTOP_NOTIFY_SOURCES).not.toContain('task_status_changed.canceled')
    // 硬超时那条是"任务失败"的唯一信号，必须在内
    expect(DESKTOP_NOTIFY_SOURCES).toContain('todo_hard_deadline_failed')
  })
})
