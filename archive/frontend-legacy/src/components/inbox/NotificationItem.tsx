import { useNavigate } from 'react-router-dom'
import { ChevronRight } from 'lucide-react'
import { cn } from '@/lib/utils'
import { formatRelativeTime } from '@/lib/utils'
import type { Notification } from '@/types'

function stripMarkdown(body: string): string {
  return body.replace(/[*_~`#>\-[\]()]/g, '').replace(/\n+/g, ' ').trim()
}

const categoryFallback: Record<string, string> = {
  task: 'PM Agent',
  todo: '执行 Agent',
  agent: 'Agent',
}

interface NotificationItemProps {
  notification: Notification
  onMarkRead?: (id: string) => void
}

export function NotificationItem({ notification, onMarkRead }: NotificationItemProps) {
  const navigate = useNavigate()
  const isUnread = !notification.is_read
  const from = notification.actor_name || categoryFallback[notification.category] || '未知'

  const taskLink = (notification.category === 'task' || notification.category === 'todo') && notification.task_id
    ? `/projects/${notification.project_id}`
    : null

  const handleClick = () => {
    if (isUnread && onMarkRead) {
      onMarkRead(notification.id)
    }
    if (taskLink) {
      navigate(taskLink)
    }
  }

  return (
    <div
      className={cn(
        'flex items-center gap-0 px-3 h-10 cursor-pointer transition-colors hover:bg-accent/50 group',
        isUnread && 'bg-accent/20'
      )}
      onClick={handleClick}
    >
      {/* 未读指示器 */}
      <div className="w-5 shrink-0 flex justify-center">
        {isUnread && <span className="size-2 rounded-full bg-primary" />}
      </div>

      {/* 来源 */}
      <div className={cn(
        'w-[100px] shrink-0 truncate text-sm',
        isUnread ? 'font-semibold text-foreground' : 'text-muted-foreground'
      )}>
        {from}
      </div>

      {/* 标题 */}
      <div className={cn(
        'w-[110px] shrink-0 truncate text-sm',
        isUnread ? 'font-semibold text-foreground' : 'text-muted-foreground'
      )}>
        {notification.title}
      </div>

      {/* 分隔符 */}
      <span className="text-muted-foreground/40 mx-1.5 shrink-0">—</span>

      {/* 摘要 (自适应宽度) */}
      <div className="flex-1 min-w-0 truncate text-sm text-muted-foreground">
        {stripMarkdown(notification.body)}
      </div>

      {/* 时间 */}
      <div className="w-[80px] shrink-0 text-right text-xs text-muted-foreground">
        {formatRelativeTime(notification.created_at)}
      </div>

      {/* 跳转指示 */}
      <div className="w-8 shrink-0 flex justify-center">
        {taskLink && (
          <ChevronRight className="size-4 opacity-0 group-hover:opacity-100 transition-opacity text-muted-foreground" />
        )}
      </div>
    </div>
  )
}
