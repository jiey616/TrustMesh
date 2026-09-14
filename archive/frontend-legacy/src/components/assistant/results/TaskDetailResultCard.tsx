import { useNavigate } from 'react-router-dom'
import { ArrowRight, ListTodo } from 'lucide-react'
import { TaskStatusBadge, PriorityBadge } from '@/components/shared/StatusBadge'
import { normalizeEscapedText } from '@/lib/utils'
import type { TaskDetail } from '@/types'

interface Props {
  task: TaskDetail
}

export function TaskDetailResultCard({ task }: Props) {
  const navigate = useNavigate()
  const description = normalizeEscapedText(task.description)
  const summary = normalizeEscapedText(task.result?.summary)

  return (
    <div className="rounded-xl border bg-card p-3 space-y-3 text-sm">
      <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
        <ListTodo className="size-3.5" />
        任务详情
      </div>

      <div className="space-y-2">
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0">
            <p className="font-medium text-sm">{task.title}</p>
            <p className="text-xs text-muted-foreground mt-0.5">ID: {task.id}</p>
          </div>
          <div className="flex items-center gap-1 shrink-0">
            <TaskStatusBadge status={task.status} />
            <PriorityBadge priority={task.priority} />
          </div>
        </div>

        {description && (
          <div className="rounded-lg bg-muted/50 p-2.5">
            <p className="text-[11px] font-medium text-muted-foreground mb-1">任务描述</p>
            <p className="text-xs whitespace-pre-wrap wrap-break-word">{description}</p>
          </div>
        )}

        {summary && (
          <div className="rounded-lg bg-muted/50 p-2.5">
            <p className="text-[11px] font-medium text-muted-foreground mb-1">结果摘要</p>
            <p className="text-xs whitespace-pre-wrap wrap-break-word">{summary}</p>
          </div>
        )}
      </div>

      <button
        type="button"
        className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
        onClick={() => navigate(`/projects/${task.project_id}`)}
      >
        查看任务所在项目
        <ArrowRight className="size-3.5" />
      </button>
    </div>
  )
}
