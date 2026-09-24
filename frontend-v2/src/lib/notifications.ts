import type { Notification } from '@/types'

export function groupNotificationsByDate(notifications: Notification[]) {
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  const yesterday = new Date(today)
  yesterday.setDate(yesterday.getDate() - 1)

  const groups: { label: string; items: Notification[] }[] = [
    { label: '今天', items: [] },
    { label: '昨天', items: [] },
    { label: '更早', items: [] },
  ]

  for (const n of notifications) {
    const d = new Date(n.created_at)
    d.setHours(0, 0, 0, 0)
    if (d.getTime() >= today.getTime()) {
      groups[0].items.push(n)
    } else if (d.getTime() >= yesterday.getTime()) {
      groups[1].items.push(n)
    } else {
      groups[2].items.push(n)
    }
  }

  return groups.filter((g) => g.items.length > 0)
}

/**
 * 站内通知 → 点击后应跳转的路由；无可用目标返回 null。
 *
 * 🔴 必须只有这一份实现：收件箱列表（`InboxPage`）与**桌面端系统通知的点击跳转**
 * （`MainLayout` 订阅 `onNotificationClicked`）都走它。两处各写一遍的话，同一条通知
 * 「点列表」与「点系统横幅」会落到不同页面 —— 而且这种漂移不会触发任何编译或类型错误，
 * 只能在真机上点出来。
 *
 * 规则沿用 `InboxPage` 原有实现，行为不变，仅做提取：
 *  1. 入职申请（agent 类且文案含「入职」「申请加入」）→ 协作邀请页；
 *  2. agent 类且有 actor_id → 该数字员工详情；
 *  3. 有 task_id → 直达任务工作台（`?task=` 深链，ProjectBoardPage 已支持）；
 *  4. 只有 project_id → 项目看板。
 *
 * 字段全部声明为可选：本函数也被 SSE payload 直接喂（`notification.created` 的事件体
 * 是 JSON，缺字段时是 undefined 而不是空串），用 `Notification` 这种"字段都必填"的
 * 类型去接会让调用方误以为无需兜底。
 */
export interface NotificationTargetSource {
  category?: string
  title?: string
  body?: string
  actor_id?: string
  task_id?: string
  project_id?: string
  /** 后端 `model.Notification.SourceEvent`：机器可读来源键，见 `shouldNotifyDesktop` */
  source_event?: string
}

export function notificationTarget(item: NotificationTargetSource): string | null {
  const title = item.title ?? ''
  const body = item.body ?? ''
  if (item.category === 'agent' && (title.includes('入职') || body.includes('申请加入'))) {
    return '/agent-invite'
  }
  if (item.category === 'agent' && item.actor_id) {
    return `/agents/${item.actor_id}`
  }
  if (item.task_id) {
    return `/projects/${item.project_id ?? ''}?task=${item.task_id}`
  }
  if (item.project_id) {
    return `/projects/${item.project_id}`
  }
  return null
}

/**
 * 桌面端**系统通知**（Windows 通知中心横幅）只弹这些来源 —— 其余通知照常进收件箱，
 * 只是不打扰人。用户口径：只要「需要人工确认的」与「任务完成/失败」两类。
 *
 * 🔴 必须用后端的 `source_event` 判类，**绝不**用 `title` 文本匹配：
 *    标题是写给人看的，后端改一次文案，这里就会**静默**失配 —— 不报错、不告警，
 *    只是从此再也不弹，属于最难排查的那类故障（跨仓耦合也没有编译期能拦）。
 *
 * ⚠️ 与后端 `store_notification_test.go` 的 `TestNotificationSourceEvent` 是**配对契约**：
 *    两侧任一改动都必须同时改，否则客户端筛出来的集合会与后端实际产生的键错位。
 *
 * 键的取值来自后端 `maybeCreateNotificationUnsafe`：
 *  - `task_status_changed.done`    任务已完成
 *  - `task_status_changed.failed`  任务执行失败（用户明确要求把失败一并纳入）
 *  - `todo_hard_deadline_failed`   任务失败的另一条路径 —— 🔴 原因见下方"硬超时"说明
 *  - `todo_awaiting_review`        产出已提交，等待人工确认
 *  - `todo_ask_received`           数字员工向你提问
 *  - `task_plan_ready`             规划完成，请确认后开始执行
 *  - `todo_rework_exhausted`       产出多次重做仍未通过，请人工介入
 *  - `todo_remind_escalated`       多次提醒无响应，疑似执行智能体卡死
 *  - `planning_reply.needs_input`  规划阶段 PM 抛了澄清卡，等你填
 *
 * 🔴 为什么 `planning_reply.needs_input` 是**带后缀**的键：
 *    后端 switch 是按**事件类型**分支的，而 `planning_reply` 是**条件通知**
 *    （纯文字回复"收到，我来分析"不通知，挂了澄清卡才通知）。所以后端在同一 case 内
 *    用 metadata 的 `needs_user_input` 分流，并给来源键加后缀区分场景。
 *    这样本白名单的基类型 `planning_reply` 仍然对得上后端 switch 的既有 case ——
 *    下面那条跨仓契约测试（断言"白名单键的基类型必须能挂到 Go case 上"）才能继续生效。
 *
 * 🔴 为什么必须带上 `todo_hard_deadline_failed`（否则「任务失败」会静默漏报）：
 *    绝大多数 todo 失败都走 `workflow.go` 的 `todo_failed`，紧随其后会调
 *    `updateTaskStatusUnsafe` ⇒ 聚合出任务级 `task_status_changed.failed`，链路完整。
 *    **唯独硬超时这条不同**：`timeout_monitor.go` 判超时后**直接** `task.Status = newStatus`
 *    （约 336 行），**没有**走 `updateTaskStatusUnsafe` ⇒ 任务状态变了但
 *    **不发 `task_status_changed` 事件**。此时唯一能代表"任务失败"的键就是
 *    `todo_hard_deadline_failed`。少了它，硬超时失败的任务对用户完全静默。
 *    （该处还顺带漏了 SSE 刷新 —— 前端要等轮询才发现任务已失败，属另一个待修问题。）
 *
 * 「请人工介入」那两条虽然标题措辞不是「确认」，但同属"卡住了在等人"，用户已确认纳入。
 */
export const DESKTOP_NOTIFY_SOURCES: readonly string[] = [
  'task_status_changed.done',
  'task_status_changed.failed',
  'todo_hard_deadline_failed',
  'todo_awaiting_review',
  'todo_ask_received',
  'task_plan_ready',
  'todo_rework_exhausted',
  'todo_remind_escalated',
  'planning_reply.needs_input',
]

/**
 * 该条通知要不要弹桌面端系统横幅。
 *
 * 缺 `source_event` 时返回 false（**不弹**）：老后端 / 更早的桌面壳不带这个字段，
 * 此时拿不到判据 —— 按「宁可不打扰」处理，而不是退回按标题猜。收件箱不受影响。
 */
export function shouldNotifyDesktop(item: NotificationTargetSource | null | undefined): boolean {
  const src = item?.source_event
  return typeof src === 'string' && src !== '' && DESKTOP_NOTIFY_SOURCES.includes(src)
}
