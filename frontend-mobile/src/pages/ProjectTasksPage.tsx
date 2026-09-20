import { useNavigate, useParams } from 'react-router-dom'
import { PageScaffold } from '@/components/PageScaffold'
import { StatusTag } from '@/components/StatusTag'
import { EmptyState } from '@/components/EmptyState'
import { useProjectTasks, useProjects } from '@/hooks/useTasks'

export function ProjectTasksPage() {
  const { projectId = '' } = useParams()
  const navigate = useNavigate()
  const { data: projects = [] } = useProjects()
  const { data: tasks = [], isLoading } = useProjectTasks(projectId)

  const project = projects.find((p) => p.id === projectId)

  return (
    <PageScaffold
      title={project?.name ?? '任务'}
      extra={
        <button type="button" className="text-[13px] text-[var(--tm-text-2)]" onClick={() => navigate('/tasks')}>
          切换
        </button>
      }
    >
      {isLoading ? (
        <div className="h-[56px] animate-pulse rounded-[var(--tm-radius-card)] bg-white" />
      ) : tasks.length === 0 ? (
        <EmptyState title="该项目暂无任务" />
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
              <p className="mt-1 text-[12px] text-[var(--tm-text-3)]">
                已完成 {t.completed_todo_count}/{t.todo_count}
                {t.failed_todo_count > 0 ? ` · 失败 ${t.failed_todo_count}` : ''}
              </p>
            </button>
          ))}
        </div>
      )}
    </PageScaffold>
  )
}
