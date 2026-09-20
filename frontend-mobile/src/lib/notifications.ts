import type { Notification } from '@/types'

export const categoryLabel: Record<string, string> = {
  task: '任务',
  todo: 'Todo',
  agent: '数字员工',
  system: '系统',
  meeting: '会议',
}

/** 通知分类色（与桌面端同口径，移动端只用于小徽标） */
export const categoryColor: Record<string, string> = {
  task: '#185fa5',
  todo: '#0f6e56',
  agent: '#6d5ff5',
  system: '#5f5e5a',
  meeting: '#ba7517',
}

function dayKey(iso: string): string {
  return iso.slice(0, 10)
}

function dayLabel(key: string, today: string, yesterday: string): string {
  if (key === today) return '今天'
  if (key === yesterday) return '昨天'
  return key.slice(5).replace('-', '月') + '日'
}

/**
 * 按日期分组（今天 / 昨天 / 具体日期）。
 * 移动端不做"消息 / @提及 / 系统通知"三分：屏幕窄，`category` 徽标已足够区分，
 * 再叠一层筛选反而增加点击成本。
 */
export function groupNotificationsByDate(
  items: Notification[],
  now: Date = new Date(),
): Array<{ key: string; label: string; items: Notification[] }> {
  const today = now.toISOString().slice(0, 10)
  const y = new Date(now.getTime() - 86_400_000).toISOString().slice(0, 10)

  const buckets = new Map<string, Notification[]>()
  for (const n of items) {
    const k = dayKey(n.created_at)
    const list = buckets.get(k)
    if (list) list.push(n)
    else buckets.set(k, [n])
  }
  return [...buckets.entries()]
    .sort((a, b) => b[0].localeCompare(a[0]))
    .map(([key, list]) => ({ key, label: dayLabel(key, today, y), items: list }))
}
