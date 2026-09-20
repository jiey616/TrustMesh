import { useNavigate } from 'react-router-dom'
import { PageScaffold } from '@/components/PageScaffold'
import { StatusTag } from '@/components/StatusTag'
import { EmptyState } from '@/components/EmptyState'
import { useAllTasks, useProjects } from '@/hooks/useTasks'

/** 任务 Tab：全部任务平铺（按更新时间倒序），点击直接进任务详情。 */
export function TasksPage() {
  const navigate = useNavigate()
  const { data: projects = [] } = useProjects()
  const { data: tasks = [], isLoading } = useAllTasks()

  const projectName = new Map(projects.map((p) => [p.id, p.name] as const))

  return (
    <PageScaffold
      title="任务"
      extra={
        <button
          type="button"
          onClick={() => navigate('/tasks/new')}
          className="text-[20px] leading-none text-[var(--tm-brand)]"
          aria-label="新建任务"
        >
          ＋
        </button>
      }
    >
      {projects.length > 0 ? (
        <div className="-mx-4 mb-3 overflow-x-auto px-4 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          <div className="flex w-max gap-2">
            {projects.map((p) => (
              <button
                key={p.id}
                type="button"
                onClick={() => navigate(`/tasks/pipeline/${p.id}`)}
                className="shrink-0 rounded-full border border-[var(--tm-line)] bg-white px-3 py-[6px] text-[13px] text-[var(--tm-text-2)]"
              >
                {p.name} ›
              </button>
            ))}
          </div>
        </div>
      ) : null}
      {isLoading ? (
        <div className="h-[56px] animate-pulse rounded-[var(--tm-radius-card)] bg-white" />
      ) : tasks.length === 0 ? (
        <EmptyState title="暂无任务" description="在桌面端创建任务后，即可在这里查看。" />
      ) : (
        <div className="flex flex-col gap-2">
          {tasks.map((t) => (
            <button
              key={t.id}
              type="button"
              onClick={() => navigate(`/tasks/detail/${t.id}`)}
              className="rounded-[var(--tm-radius-card)] border border-[var(--tm-line)] bg-white p-3 text-left"
            >
              <div className="flex items-start justify-between gap-2">
                <span className="min-w-0 flex-1 truncate text-[15px] font-medium">{t.title}</span>
                <StatusTag status={t.status} />
              </div>
              <p className="mt-1 truncate text-[12px] text-[var(--tm-text-3)]">
                {projectName.get(t.project_id) ?? '未知项目'} · 已完成 {t.completed_todo_count}/
                {t.todo_count}
                {t.failed_todo_count > 0 ? ` · 失败 ${t.failed_todo_count}` : ''}
              </p>
            </button>
          ))}
        </div>
      )}
    </PageScaffold>
  )
}
