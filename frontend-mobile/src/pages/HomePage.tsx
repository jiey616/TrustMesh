import { useNavigate } from 'react-router-dom'
import { PageScaffold } from '@/components/PageScaffold'
import { PendingCard } from '@/components/PendingCard'
import { StatusTag } from '@/components/StatusTag'
import { EmptyState } from '@/components/EmptyState'
import { usePendingFeed } from '@/hooks/useTasks'
import { useAuthStore } from '@/stores/authStore'

export function HomePage() {
  const navigate = useNavigate()
  const user = useAuthStore((s) => s.user)
  const { isLoading, items, activeTasks } = usePendingFeed()

  return (
    <PageScaffold title="工作台">
      <p className="mb-3 text-[13px] text-[var(--tm-text-2)]">
        {user?.name ? `${user.name}，` : ''}共 {items.length} 项待你处理
      </p>

      <h2 className="mb-2 text-[13px] text-[var(--tm-text-2)]">待我确认</h2>
      {isLoading ? (
        <div className="flex flex-col gap-3">
          <div className="h-[92px] animate-pulse rounded-[var(--tm-radius-card)] bg-white" />
          <div className="h-[92px] animate-pulse rounded-[var(--tm-radius-card)] bg-white" />
        </div>
      ) : items.length === 0 ? (
        <div className="rounded-[var(--tm-radius-card)] border border-[var(--tm-line)] bg-white">
          <EmptyState title="没有待确认事项" description="数字员工正在推进，需要你拍板时会出现在这里。" />
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          {items.map((item) => (
            <PendingCard key={item.id} item={item} />
          ))}
        </div>
      )}

      <h2 className="mb-2 mt-5 text-[13px] text-[var(--tm-text-2)]">进行中的任务</h2>
      {activeTasks.length === 0 ? (
        <div className="rounded-[var(--tm-radius-card)] border border-[var(--tm-line)] bg-white">
          <EmptyState title="暂无进行中的任务" />
        </div>
      ) : (
        <div className="overflow-hidden rounded-[var(--tm-radius-card)] border border-[var(--tm-line)] bg-white">
          {activeTasks.map((t) => (
            <button
              key={t.id}
              type="button"
              onClick={() => navigate(`/tasks/detail/${t.id}`)}
              className="flex w-full items-center justify-between gap-3 border-b border-[var(--tm-line)] px-3 py-3 text-left last:border-b-0"
            >
              <span className="min-w-0 flex-1">
                <span className="block truncate text-[15px]">{t.title}</span>
                <span className="mt-[2px] block text-[12px] text-[var(--tm-text-3)]">
                  已完成 {t.completed_todo_count}/{t.todo_count}
                  {t.failed_todo_count > 0 ? ` · 失败 ${t.failed_todo_count}` : ''}
                </span>
              </span>
              <StatusTag status={t.status} />
            </button>
          ))}
        </div>
      )}
    </PageScaffold>
  )
}
