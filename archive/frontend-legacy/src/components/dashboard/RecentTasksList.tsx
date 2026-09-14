import { Link } from 'react-router-dom'
import { TaskStatusBadge } from '@/components/shared/StatusBadge'
import { Avatar } from '@/components/ui/avatar'
import { formatRelativeTime } from '@/lib/utils'
import type { TaskListItem } from '@/types'

interface RecentTasksListProps {
  tasks: TaskListItem[]
  loading?: boolean
}

export function RecentTasksList({ tasks, loading }: RecentTasksListProps) {
  if (loading) {
    return <div className="py-8 text-center text-sm text-muted-foreground">加载中...</div>
  }

  if (tasks.length === 0) {
    return <div className="py-8 text-center text-sm text-muted-foreground">暂无任务</div>
  }

  return (
    <div className="flex flex-col gap-2">
      {tasks.map((task) => (
        <Link
          key={task.id}
          to={`/projects/${task.project_id}?task=${task.id}`}
          className="flex items-start gap-3 rounded-lg p-2 hover:bg-accent/50 transition-colors"
        >
          <TaskStatusBadge status={task.status} />
          <div className="flex-1 min-w-0">
            <div className="text-sm font-medium truncate">{task.title}</div>
            <div className="mt-0.5 flex items-center gap-1.5 text-xs text-muted-foreground">
              {task.pm_agent ? (
                <Avatar
                  fallback={task.pm_agent.name}
                  seed={task.pm_agent.node_id}
                  kind="agent"
                  role="pm"
                  size="sm"
                />
              ) : (
                <Avatar fallback="用户" seed="recent-task-user" kind="user" size="sm" />
              )}
              <span className="truncate">{task.pm_agent?.name || '用户创建'}</span>
              <span>· {formatRelativeTime(task.updated_at)}</span>
              {task.todo_count > 0 && (
                <span className="ml-1">
                  · {task.completed_todo_count}/{task.todo_count} Todo
                </span>
              )}
            </div>
          </div>
        </Link>
      ))}
    </div>
  )
}
