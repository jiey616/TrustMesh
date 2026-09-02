import { ChevronDown, ChevronRight } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { cn } from '@/lib/utils'
import { TaskListRow } from './TaskListRow'
import type { TaskListItem, TaskStatus } from '@/types'

const statusGroups: { status: TaskStatus; label: string }[] = [
  { status: 'planning', label: '规划中' },
  { status: 'review', label: '待确认' },
  { status: 'in_progress', label: '进行中' },
  { status: 'awaiting_review', label: '待人工确认' },
  { status: 'waiting_user', label: '待用户输入' },
  { status: 'pending', label: '待处理' },
  { status: 'done', label: '已完成' },
  { status: 'failed', label: '失败' },
  { status: 'canceled', label: '已取消' },
]

interface TaskListViewProps {
  tasks: TaskListItem[]
  selectedTaskId: string | null
  onTaskClick: (taskId: string) => void
}

export function TaskListView({ tasks, selectedTaskId, onTaskClick }: TaskListViewProps) {
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({})

  const scrollRef = useRef<HTMLDivElement>(null)
  const prevSelectedRef = useRef<string | null>(null)

  // Derive effective collapsed state: force-expand group containing selected task
  const selectedTask = selectedTaskId ? tasks.find((t) => t.id === selectedTaskId) : undefined
  const effectiveCollapsed = selectedTask && collapsed[selectedTask.status]
    ? { ...collapsed, [selectedTask.status]: false }
    : collapsed

  // Scroll to selected task when selection changes
  useEffect(() => {
    if (!selectedTaskId || selectedTaskId === prevSelectedRef.current) return
    prevSelectedRef.current = selectedTaskId
    requestAnimationFrame(() => {
      const row = scrollRef.current?.querySelector(`[data-task-id="${selectedTaskId}"]`)
      row?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
    })
  }, [selectedTaskId])

  const groups = statusGroups
    .map((g) => ({
      ...g,
      tasks: tasks.filter((t) => t.status === g.status),
    }))
    .filter((g) => g.tasks.length > 0)

  const toggle = (status: string) =>
    setCollapsed((prev) => ({ ...prev, [status]: !prev[status] }))

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="grid grid-cols-[1fr_80px_140px_80px] items-center gap-3 px-3 py-2 border-b text-xs font-medium text-muted-foreground uppercase tracking-wider">
        <span>名称</span>
        <span className="text-center">优先级</span>
        <span>进度</span>
        <span className="text-right">更新</span>
      </div>

      {/* Groups */}
      <div ref={scrollRef} className="flex-1 overflow-y-auto py-1">
        {groups.map((group) => {
          const isCollapsed = effectiveCollapsed[group.status]
          return (
            <div key={group.status} className="mb-1">
              {/* Section header */}
              <div
                className="flex items-center gap-1.5 px-3 py-1.5 cursor-pointer select-none hover:bg-muted/30 rounded-md"
                onClick={() => toggle(group.status)}
              >
                {isCollapsed
                  ? <ChevronRight className="size-3.5 text-muted-foreground" />
                  : <ChevronDown className="size-3.5 text-muted-foreground" />
                }
                <span className="text-xs font-semibold">{group.label}</span>
                <span className={cn(
                  'flex h-4 min-w-[16px] items-center justify-center rounded-full px-1 text-[10px] font-medium',
                  'bg-muted text-muted-foreground',
                )}>
                  {group.tasks.length}
                </span>
              </div>

              {/* Rows */}
              {!isCollapsed && group.tasks.map((task) => (
                <TaskListRow
                  key={task.id}
                  task={task}
                  isSelected={task.id === selectedTaskId}
                  onClick={() => onTaskClick(task.id)}
                />
              ))}
            </div>
          )
        })}

        {groups.length === 0 && (
          <div className="py-12 text-center text-sm text-muted-foreground">
            暂无任务
          </div>
        )}
      </div>
    </div>
  )
}
