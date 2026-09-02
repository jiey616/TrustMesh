import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { CircleCheck, CircleOff, Cog } from 'lucide-react'
import type { TaskStatus, TaskPriority, AgentStatus, ProjectStatus, ProjectWorkStatus } from '@/types'

const taskStatusConfig: Record<TaskStatus, { label: string; variant: 'secondary' | 'info' | 'success' | 'destructive' | 'warning' }> = {
  planning: { label: '规划中', variant: 'info' },
  review: { label: '待确认', variant: 'warning' },
  pending: { label: '待处理', variant: 'secondary' },
  in_progress: { label: '进行中', variant: 'info' },
  awaiting_review: { label: '待人工确认', variant: 'warning' },
  waiting_user: { label: '待用户输入', variant: 'warning' },
  done: { label: '已完成', variant: 'success' },
  failed: { label: '失败', variant: 'destructive' },
  canceled: { label: '已取消', variant: 'secondary' },
}

const priorityConfig: Record<TaskPriority, { label: string; className: string }> = {
  low: { label: '低', className: 'bg-priority-low/15 text-priority-low' },
  medium: { label: '中', className: 'bg-priority-medium/15 text-priority-medium' },
  high: { label: '高', className: 'bg-priority-high/15 text-priority-high' },
  urgent: { label: '紧急', className: 'bg-priority-urgent/15 text-priority-urgent' },
}

const agentStatusConfig: Record<AgentStatus, { label: string; className: string }> = {
  online: { label: '在线', className: 'bg-status-online/15 text-status-online' },
  offline: { label: '离线', className: 'bg-status-offline/15 text-status-offline' },
  busy: { label: '忙碌', className: 'bg-status-busy/15 text-status-busy' },
}

const projectStatusConfig: Record<ProjectStatus, { label: string; variant: 'outline' | 'secondary' }> = {
  active: { label: '开放中', variant: 'outline' },
  archived: { label: '已归档', variant: 'secondary' },
}

const projectWorkStatusConfig: Record<ProjectWorkStatus, { label: string; variant: 'secondary' | 'info' | 'success' | 'warning' | 'destructive' }> = {
  empty: { label: '暂无任务', variant: 'secondary' },
  idle: { label: '空闲', variant: 'success' },
  queued: { label: '待处理', variant: 'warning' },
  running: { label: '执行中', variant: 'info' },
  attention: { label: '需关注', variant: 'destructive' },
  archived: { label: '已归档', variant: 'secondary' },
}

export function TaskStatusBadge({ status }: { status: TaskStatus }) {
  const config = taskStatusConfig[status] ?? { label: status ?? '未知', variant: 'secondary' as const }
  return <Badge variant={config.variant}>{config.label}</Badge>
}

export function PriorityBadge({ priority }: { priority: TaskPriority }) {
  const config = priorityConfig[priority]
  return (
    <Badge className={config.className} variant="outline">
      {config.label}
    </Badge>
  )
}

export function AgentStatusBadge({ status }: { status: AgentStatus }) {
  const config = agentStatusConfig[status]
  return <Badge className={config.className}>{config.label}</Badge>
}

export function ProjectStatusBadge({ status }: { status: ProjectStatus }) {
  const config = projectStatusConfig[status]
  return <Badge variant={config.variant}>{config.label}</Badge>
}

export function ProjectWorkStatusBadge({ status }: { status: ProjectWorkStatus }) {
  const config = projectWorkStatusConfig[status]
  return <Badge variant={config.variant}>{config.label}</Badge>
}

export function ProjectWorkStatusDot({ status }: { status: ProjectWorkStatus }) {
  const colorClass =
    status === 'running' ? 'bg-info' :
    status === 'attention' ? 'bg-destructive' :
    status === 'queued' ? 'bg-warning' :
    status === 'idle' ? 'bg-success' : 'bg-muted-foreground/40'
  return <span className={cn('inline-block size-2 rounded-full', colorClass)} />
}

export function AgentStatusDot({ status }: { status: AgentStatus }) {
  const colorClass =
    status === 'online' ? 'bg-status-online' :
    status === 'busy' ? 'bg-status-busy' : 'bg-status-offline'
  return <span className={cn('inline-block size-2 rounded-full', colorClass)} />
}

export function AgentStatusIcon({ status, className }: { status: AgentStatus; className?: string }) {
  const size = cn('size-4', className)
  switch (status) {
    case 'online':
      return <CircleCheck className={cn(size, 'text-status-online')} />
    case 'busy':
      return <Cog className={cn(size, 'text-status-busy animate-spin')} />
    case 'offline':
      return <CircleOff className={cn(size, 'text-status-offline')} />
  }
}
