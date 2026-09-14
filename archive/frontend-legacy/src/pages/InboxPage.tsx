import { useState } from 'react'
import { PageContainer } from '@/components/layout/PageContainer'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { NotificationGroup } from '@/components/inbox/NotificationGroup'
import { groupNotificationsByDate } from '@/lib/notifications'
import { useNotifications, useMarkNotificationRead, useMarkAllRead } from '@/hooks/useNotifications'
import { toast } from 'sonner'

const filters = [
  { label: '最近', value: 'recent' },
  { label: '未读', value: 'unread' },
  { label: '全部', value: 'all' },
] as const

export function InboxPage() {
  const [filter, setFilter] = useState<string>('recent')
  const { data: notifications, isLoading } = useNotifications(filter)
  const markRead = useMarkNotificationRead()
  const markAllRead = useMarkAllRead()

  const groups = notifications ? groupNotificationsByDate(notifications) : []
  const hasUnread = notifications?.some((n) => !n.is_read) ?? false

  return (
    <PageContainer className="flex flex-col h-full gap-3">
      <div className="shrink-0 flex items-center justify-between">
        <h1 className="text-2xl font-bold">收件箱</h1>
        <div className="flex items-center gap-2">
          {hasUnread && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => markAllRead.mutate(undefined, { onSuccess: () => toast.success('全部通知已标记为已读') })}
              disabled={markAllRead.isPending}
            >
              全部已读
            </Button>
          )}
        </div>
      </div>

      <div className="shrink-0 flex items-center gap-1">
        {filters.map((f) => (
          <Button
            key={f.value}
            variant={filter === f.value ? 'default' : 'ghost'}
            size="sm"
            className="text-xs h-7"
            onClick={() => setFilter(f.value)}
          >
            {f.label}
          </Button>
        ))}
      </div>

      {isLoading && (
        <div className="py-12 text-center text-sm text-muted-foreground">加载中...</div>
      )}

      {!isLoading && groups.length === 0 && (
        <div className="py-12 text-center text-sm text-muted-foreground">
          {filter === 'unread' ? '没有未读通知' : '暂无通知'}
        </div>
      )}

      {!isLoading && groups.length > 0 && (
        <div className="flex-1 min-h-0 border rounded-lg overflow-hidden">
          <ScrollArea className="h-full">
            {groups.map((group) => (
              <NotificationGroup
                key={group.label}
                label={group.label}
                notifications={group.items}
                onMarkRead={(id) => markRead.mutate(id)}
              />
            ))}
          </ScrollArea>
        </div>
      )}
    </PageContainer>
  )
}
