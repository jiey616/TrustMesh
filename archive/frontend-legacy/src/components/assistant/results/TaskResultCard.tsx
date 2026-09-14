import type { TaskListItem } from '@/types'
import { normalizeEscapedText } from '@/lib/utils'
import { ListTodo } from 'lucide-react'
import { useNavigate } from 'react-router-dom'

const STATUS_LABELS: Record<string, { label: string; className: string }> = {
  pending: { label: '待处理', className: 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300' },
  in_progress: { label: '进行中', className: 'bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300' },
  done: { label: '已完成', className: 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300' },
  failed: { label: '失败', className: 'bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300' },
  canceled: { label: '已取消', className: 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300' },
}

interface Props {
  items: TaskListItem[]
}

export function TaskResultCard({ items }: Props) {
  const navigate = useNavigate()

  if (items.length === 0) return null

  return (
    <div className="rounded-xl border bg-card p-3 space-y-2 text-sm">
      <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
        <ListTodo className="size-3.5" />
        任务结果 ({items.length})
      </div>
      <div className="space-y-1.5">
        {items.slice(0, 5).map((task) => {
          const status = STATUS_LABELS[task.status] ?? STATUS_LABELS.pending
          return (
            <button
              key={task.id}
              className="w-full text-left rounded-lg bg-muted/50 p-2.5 hover:bg-muted transition-colors"
              onClick={() => navigate(`/projects/${task.project_id}`)}
            >
              <div className="flex items-center gap-2">
                <span className="flex-1 font-medium text-xs truncate">{task.title}</span>
                <span className={`shrink-0 text-[10px] px-1.5 py-0.5 rounded-full ${status.className}`}>
                  {status.label}
                </span>
              </div>
              {task.description && (
                <p className="text-xs text-muted-foreground mt-0.5 line-clamp-1">
                  {normalizeEscapedText(task.description)}
                </p>
              )}
            </button>
          )
        })}
        {items.length > 5 && (
          <p className="text-xs text-muted-foreground text-center">
            还有 {items.length - 5} 个任务
          </p>
        )}
      </div>
    </div>
  )
}
