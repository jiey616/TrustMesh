import { NotificationItem } from './NotificationItem'
import type { Notification } from '@/types'

interface NotificationGroupProps {
  label: string
  notifications: Notification[]
  onMarkRead?: (id: string) => void
}

export function NotificationGroup({ label, notifications, onMarkRead }: NotificationGroupProps) {
  if (notifications.length === 0) return null

  return (
    <div>
      <div className="text-xs font-medium text-muted-foreground uppercase tracking-wider px-3 py-1.5 bg-muted/30">
        {label}
      </div>
        <div className="divide-y divide-border/50">
          {notifications.map((n) => (
          <NotificationItem key={n.id} notification={n} onMarkRead={onMarkRead} />
          ))}
        </div>
      </div>
  )
}
