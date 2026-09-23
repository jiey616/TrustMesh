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
