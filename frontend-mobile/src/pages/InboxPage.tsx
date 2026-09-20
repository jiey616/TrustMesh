import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Segmented, Toast } from 'antd-mobile'
import { PageScaffold } from '@/components/PageScaffold'
import { EmptyState } from '@/components/EmptyState'
import { useMarkAllRead, useMarkNotificationRead, useNotifications } from '@/hooks/useNotifications'
import { categoryColor, categoryLabel, groupNotificationsByDate } from '@/lib/notifications'
import type { Notification } from '@/types'

export function InboxPage() {
  const navigate = useNavigate()
  const [filter, setFilter] = useState<'recent' | 'unread'>('recent')
  const { data: items = [], isLoading } = useNotifications(filter)
  const markRead = useMarkNotificationRead()
  const markAllRead = useMarkAllRead()

  const hasUnread = items.some((n) => !n.is_read)
  const groups = groupNotificationsByDate(items)

  async function open(item: Notification) {
    if (!item.is_read) {
      try {
        await markRead.mutateAsync(item.id)
      } catch {
        // 标记已读失败不阻断跳转
      }
    }
    if (item.task_id) {
      navigate(`/tasks/detail/${item.task_id}`)
      return
    }
    if (item.project_id) {
      navigate(`/tasks/project/${item.project_id}`)
    }
  }

  return (
    <PageScaffold
      title="收件箱"
      extra={
        hasUnread ? (
          <button
            type="button"
            className="text-[13px] text-[var(--tm-brand)]"
            disabled={markAllRead.isPending}
            onClick={() => {
              markAllRead.mutate(undefined, {
                onError: (err) =>
                  Toast.show({ content: err instanceof Error ? err.message : '操作失败' }),
              })
            }}
          >
            全部已读
          </button>
        ) : null
      }
    >
      <Segmented
        value={filter}
        onChange={(v) => setFilter(v as 'recent' | 'unread')}
        options={[
          { label: '最近', value: 'recent' },
          { label: '未读', value: 'unread' },
        ]}
      />

      <div className="mt-3">
        {isLoading ? (
          <div className="h-[80px] animate-pulse rounded-[var(--tm-radius-card)] bg-white" />
        ) : items.length === 0 ? (
          <div className="rounded-[var(--tm-radius-card)] border border-[var(--tm-line)] bg-white">
            <EmptyState
              title={filter === 'unread' ? '没有未读通知' : '暂无通知'}
              description="任务进展、@提及与系统消息会集中在这里。"
            />
          </div>
        ) : (
          groups.map((g) => (
            <div key={g.key} className="mb-3">
              <p className="mb-2 text-[12px] text-[var(--tm-text-3)]">{g.label}</p>
              <div className="overflow-hidden rounded-[var(--tm-radius-card)] border border-[var(--tm-line)] bg-white">
                {g.items.map((item) => (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => void open(item)}
                    className="flex w-full gap-2 border-b border-[var(--tm-line)] px-3 py-3 text-left last:border-b-0"
                  >
                    <span
                      className="mt-[5px] h-[8px] w-[8px] shrink-0 rounded-full"
                      style={{ background: item.is_read ? 'transparent' : 'var(--tm-danger)' }}
                    />
                    <span className="min-w-0 flex-1">
                      <span className="flex items-center gap-2">
                        <span
                          className="shrink-0 rounded-full px-[6px] py-[1px] text-[11px]"
                          style={{
                            color: categoryColor[item.category] ?? 'var(--tm-text-3)',
                            background: 'var(--tm-surface)',
                          }}
                        >
                          {categoryLabel[item.category] ?? item.category}
                        </span>
                        <span className="min-w-0 flex-1 truncate text-[15px] font-medium">
                          {item.title}
                        </span>
                      </span>
                      <span className="mt-1 block text-[13px] leading-relaxed text-[var(--tm-text-2)]">
                        {item.body}
                      </span>
                    </span>
                  </button>
                ))}
              </div>
            </div>
          ))
        )}
      </div>
    </PageScaffold>
  )
}
