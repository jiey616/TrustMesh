import { Card, CardContent } from '@/components/ui/card'
import { PriorityBadge } from '@/components/shared/StatusBadge'
import { Avatar } from '@/components/ui/avatar'
import type { TaskListItem } from '@/types'

interface TaskCardProps {
  task: TaskListItem
  onClick: () => void
}

export function TaskCard({ task, onClick }: TaskCardProps) {
  const progress = task.todo_count > 0
    ? Math.round((task.completed_todo_count / task.todo_count) * 100)
    : 0

  return (
    <Card
      className="cursor-pointer transition-all hover:shadow-md hover:border-primary/20 active:scale-[0.98]"
      onClick={onClick}
    >
      <CardContent className="flex flex-col gap-2.5 p-3">
        <div className="flex items-start justify-between gap-2">
          <h4 className="text-sm font-medium leading-snug line-clamp-2">{task.title}</h4>
          <PriorityBadge priority={task.priority} />
        </div>

        {task.todo_count > 0 && (
          <div className="flex flex-col gap-1">
            <div className="h-1.5 rounded-full bg-muted overflow-hidden">
              <div
                className="h-full rounded-full bg-primary transition-all duration-300"
                style={{ width: `${progress}%` }}
              />
            </div>
            <div className="flex items-center justify-between text-xs text-muted-foreground">
              <span>
                {task.completed_todo_count}/{task.todo_count} Todo
              </span>
              {task.failed_todo_count > 0 && (
                <span className="text-destructive">{task.failed_todo_count} 失败</span>
              )}
            </div>
          </div>
        )}

        <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
          {task.pm_agent ? (
            <>
              <Avatar
                fallback={task.pm_agent.name}
                seed={task.pm_agent.node_id}
                kind="agent"
                role="pm"
                size="sm"
              />
              <span className="truncate">{task.pm_agent.name}</span>
            </>
          ) : (
            <>
              <Avatar fallback="用户" seed="task-user" kind="user" size="sm" />
              <span className="truncate">用户创建</span>
            </>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
