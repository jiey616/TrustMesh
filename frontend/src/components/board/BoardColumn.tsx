import { cn } from '@/lib/utils'
import type { TaskListItem, TaskStatus } from '@/types'
import { TaskCard } from './TaskCard'

const columnConfig: Record<TaskStatus, { title: string; accent: string }> = {
  planning: { title: '规划中', accent: 'border-t-info' },
  review: { title: '待确认', accent: 'border-t-warning' },
  pending: { title: '待处理', accent: 'border-t-muted-foreground' },
  in_progress: { title: '进行中', accent: 'border-t-info' },
  awaiting_review: { title: '待人工确认', accent: 'border-t-warning' },
  waiting_user: { title: '待用户输入', accent: 'border-t-violet-400' },
  done: { title: '已完成', accent: 'border-t-success' },
  failed: { title: '失败', accent: 'border-t-destructive' },
  canceled: { title: '已取消', accent: 'border-t-slate-400' },
}

interface BoardColumnProps {
  status: TaskStatus
  tasks: TaskListItem[]
  onTaskClick: (taskId: string) => void
}

export function BoardColumn({ status, tasks, onTaskClick }: BoardColumnProps) {
  const config = columnConfig[status]

  return (
    <div className="flex flex-col min-w-[280px] w-[280px]">
      <div className={cn('rounded-t-lg border-t-2 px-3 py-2.5 flex items-center justify-between', config.accent)}>
        <span className="text-sm font-medium">{config.title}</span>
        <span className="flex h-5 min-w-5 items-center justify-center rounded-full bg-muted px-1.5 text-xs font-medium text-muted-foreground">
          {tasks.length}
        </span>
      </div>
      <div className="flex flex-1 flex-col gap-2 p-1 overflow-y-auto">
        {tasks.map((task) => (
          <TaskCard key={task.id} task={task} onClick={() => onTaskClick(task.id)} />
        ))}
      </div>
    </div>
  )
}
