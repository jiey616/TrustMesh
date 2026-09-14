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
