import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ClipboardList, Circle, CircleDot, CheckCircle2, XCircle, CircleSlash2, MessageSquareMore } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { PriorityBadge } from '@/components/shared/StatusBadge'
import { EmptyState } from '@/components/shared/EmptyState'
import { Badge } from '@/components/ui/badge'
import { useAgentTasks } from '@/hooks/useAgents'
import { cn, formatRelativeTime } from '@/lib/utils'
import type { AgentTaskItem, TaskStatus } from '@/types'

const statusFilters: { label: string; value: TaskStatus | 'all' }[] = [
  { label: '全部', value: 'all' },
  { label: '规划中', value: 'planning' },
  { label: '进行中', value: 'in_progress' },
  { label: '待处理', value: 'pending' },
  { label: '已完成', value: 'done' },
  { label: '失败', value: 'failed' },
  { label: '已取消', value: 'canceled' },
]

const statusIcon: Record<TaskStatus, { icon: typeof Circle; className: string }> = {
  planning: { icon: MessageSquareMore, className: 'text-info' },
  review: { icon: MessageSquareMore, className: 'text-warning' },
  pending: { icon: Circle, className: 'text-muted-foreground' },
  in_progress: { icon: CircleDot, className: 'text-info animate-pulse' },
  awaiting_review: { icon: MessageSquareMore, className: 'text-warning' },
  waiting_user: { icon: MessageSquareMore, className: 'text-warning animate-pulse' },
  done: { icon: CheckCircle2, className: 'text-success' },
  failed: { icon: XCircle, className: 'text-destructive' },
  canceled: { icon: CircleSlash2, className: 'text-muted-foreground' },
}

interface AgentTaskListProps {
  agentId: string
}

export function AgentTaskList({ agentId }: AgentTaskListProps) {
  const [filter, setFilter] = useState<TaskStatus | 'all'>('all')
  const queryStatus = filter === 'all' ? undefined : filter
  const { data: tasks, isLoading } = useAgentTasks(agentId, queryStatus)
  const navigate = useNavigate()

  return (
    <div className="flex flex-col gap-4">
      {/* Filter bar */}
      <div className="flex flex-wrap gap-1">
        {statusFilters.map((f) => (
          <Button
            key={f.value}
            variant={filter === f.value ? 'default' : 'ghost'}
            size="sm"
            className="h-7 text-xs"
            onClick={() => setFilter(f.value)}
          >
            {f.label}
          </Button>
        ))}
      </div>

      {/* Table header */}
      <div className="grid grid-cols-[minmax(0,1fr)_120px_80px_100px_80px] items-center gap-3 px-3 py-2 border-b text-xs font-medium text-muted-foreground uppercase tracking-wider">
        <span>任务名称</span>
        <span>项目</span>
        <span className="text-center">优先级</span>
        <span>进度</span>
        <span className="text-right">更新</span>
      </div>

      {/* Loading */}
      {isLoading && (
        <div className="py-12 text-center text-sm text-muted-foreground">加载中...</div>
      )}

      {/* Empty */}
      {!isLoading && (!tasks || tasks.length === 0) && (
        <EmptyState
          icon={ClipboardList}
          title="暂无工作记录"
          description={filter !== 'all' ? '该状态下暂无任务' : '该 Agent 尚未参与任何任务'}
        />
      )}

      {/* Task rows */}
      {!isLoading && tasks && tasks.length > 0 && (
        <div className="flex flex-col">
          {tasks.map((task) => (
            <AgentTaskRow
              key={task.id}
              task={task}
              onClick={() => navigate(`/projects/${task.project_id}?task=${task.id}`)}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function AgentTaskRow({ task, onClick }: { task: AgentTaskItem; onClick: () => void }) {
  const Icon = statusIcon[task.status].icon
  const iconClass = statusIcon[task.status].className
  const progress = task.todo_count > 0
    ? Math.round((task.completed_todo_count / task.todo_count) * 100)
    : 0

  return (
    <div
      className="group grid grid-cols-[minmax(0,1fr)_120px_80px_100px_80px] items-center gap-3 px-3 py-2.5 cursor-pointer rounded-md transition-colors hover:bg-muted/50"
      onClick={onClick}
    >
      {/* Name + status icon */}
      <div className="flex min-w-0 items-center gap-2.5">
        <Icon className={cn('size-4 shrink-0', iconClass)} />
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-medium">{task.title}</div>
          {task.description && (
            <div className="text-xs text-muted-foreground truncate mt-0.5">{task.description}</div>
          )}
        </div>
        {task.relation === 'pm' && (
          <Badge variant="outline" className="text-[10px] px-1.5 py-0 shrink-0">PM</Badge>
        )}
      </div>

      {/* Project */}
      <div className="text-xs text-muted-foreground truncate">{task.project_name}</div>

      {/* Priority */}
      <div className="flex justify-center">
        <PriorityBadge priority={task.priority} />
      </div>

      {/* Progress */}
      <div className="flex items-center gap-2">
        {task.todo_count > 0 ? (
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
          </>
        ) : (
          <span className="text-xs text-muted-foreground">-</span>
        )}
      </div>

      {/* Time */}
      <div className="text-xs text-muted-foreground text-right whitespace-nowrap">
        {formatRelativeTime(task.updated_at)}
      </div>
    </div>
  )
}
