import { useState } from 'react'
import { ChevronDown, ChevronRight, Circle, CheckCircle2, Loader2, XCircle, CircleSlash2, MessageSquareMore } from 'lucide-react'
import { useTask } from '@/hooks/useTasks'
import { TaskTodoPanel } from './TaskTodoPanel'
import { cn, normalizeEscapedText } from '@/lib/utils'
import { formatRelativeTime } from '@/lib/utils'
import { PriorityBadge } from '@/components/shared/StatusBadge'
import type { TaskListItem, TaskStatus } from '@/types'


const statusIcon: Record<TaskStatus, { icon: typeof Circle; className: string }> = {
  planning: { icon: MessageSquareMore, className: 'text-info' },
  review: { icon: MessageSquareMore, className: 'text-warning' },
  pending: { icon: Circle, className: 'text-muted-foreground' },
  in_progress: { icon: Loader2, className: 'text-info animate-spin' },
  done: { icon: CheckCircle2, className: 'text-success' },
  failed: { icon: XCircle, className: 'text-destructive' },
  canceled: { icon: CircleSlash2, className: 'text-muted-foreground' },
}

interface TaskListRowProps {
  task: TaskListItem
  isSelected: boolean
  onClick: () => void
}

export function TaskListRow({ task, isSelected, onClick }: TaskListRowProps) {
  const [todosExpanded, setTodosExpanded] = useState(task.status === 'in_progress')
  const [prevStatus, setPrevStatus] = useState(task.status)

  // 任务状态变化时同步展开（在 render 阶段处理，避免 effect 中 setState）
  if (prevStatus !== task.status) {
    setPrevStatus(task.status)
    if (task.status === 'in_progress') {
      setTodosExpanded(true)
    }
  }

  const { data: taskDetail, isLoading } = useTask(todosExpanded ? task.id : undefined)
  const progress = task.todo_count > 0
    ? Math.round((task.completed_todo_count / task.todo_count) * 100)
    : 0
  const Icon = statusIcon[task.status].icon
  const iconClass = statusIcon[task.status].className
  const canExpandTodos = task.todo_count > 0 || task.status === 'pending' || task.status === 'in_progress'
  const isPlanning = task.status === 'planning'

  return (
    <div data-task-id={task.id}>
      <div
        className={cn(
          'group grid grid-cols-[minmax(0,1fr)_auto_140px_80px] items-center gap-3 px-3 py-2.5 cursor-pointer rounded-md transition-colors',
          'hover:bg-muted/50',
          isSelected && 'bg-muted/70',
        )}
        onClick={onClick}
      >
        {/* Name */}
        <div className="flex min-w-0 items-start gap-2.5">
          {canExpandTodos ? (
            <button
              type="button"
              className="mt-0.5 rounded-sm p-0.5 text-muted-foreground hover:bg-muted"
              onClick={(e) => {
                e.stopPropagation()
                setTodosExpanded((current) => !current)
              }}
              aria-label={todosExpanded ? '收起执行清单' : '展开执行清单'}
            >
              {todosExpanded ? (
                <ChevronDown className="size-4" />
              ) : (
                <ChevronRight className="size-4" />
              )}
            </button>
          ) : (
            <span className="mt-0.5 size-5 shrink-0" />
          )}
          <Icon className={cn('mt-0.5 size-4 shrink-0', iconClass)} />
          <div className="min-w-0">
            <div className="truncate text-sm font-medium">{task.title}</div>
            {task.description && (
              <div className="text-xs text-muted-foreground truncate mt-0.5">{normalizeEscapedText(task.description)}</div>
            )}
          </div>
        </div>

        {/* Meta */}
        <div className="flex items-center justify-end gap-2 whitespace-nowrap">
          <span className="text-xs text-muted-foreground truncate max-w-[88px]">
            {task.pm_agent?.name || '用户创建'}
          </span>
          <PriorityBadge priority={task.priority} />
        </div>

        {/* Progress */}
        <div className="flex items-center gap-2">
          {isPlanning ? (
            <span className="text-xs text-info">等待 PM 规划</span>
          ) : task.todo_count > 0 ? (
            <>
              <div className="flex-1 h-1.5 rounded-full bg-muted overflow-hidden">
                <div
                  className={cn(
                    'h-full rounded-full transition-all duration-300',
                    task.failed_todo_count > 0 ? 'bg-destructive' : 'bg-primary',
                  )}
                  style={{ width: `${progress}%` }}
                />
              </div>
              <span className="text-xs text-muted-foreground whitespace-nowrap">
                {task.completed_todo_count}/{task.todo_count}
              </span>
              {task.failed_todo_count > 0 && (
                <span className="text-xs text-destructive whitespace-nowrap">
                  {task.failed_todo_count}!
                </span>
              )}
            </>
          ) : (
            <span className="text-xs text-muted-foreground">—</span>
          )}
        </div>

        {/* Time */}
        <div className="text-xs text-muted-foreground text-right whitespace-nowrap">
          {formatRelativeTime(task.updated_at)}
        </div>
      </div>

      {todosExpanded && (
        <div className="ml-9 mt-1 pl-4">
          <div className="border-l border-border/70 pl-4">
            {isLoading ? (
              <div className="flex items-center gap-2 py-4 text-sm text-muted-foreground">
                <Loader2 className="size-4 animate-spin" />
                <span>加载执行清单...</span>
              </div>
            ) : taskDetail ? (
              <TaskTodoPanel task={taskDetail} variant="embedded" />
            ) : (
              <div className="py-4 text-sm text-muted-foreground">执行清单加载失败</div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
