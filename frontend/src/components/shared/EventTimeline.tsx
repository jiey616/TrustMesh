import { Link } from 'react-router-dom'
import {
  CheckCircle2,
  Circle,
  AlertCircle,
  PlayCircle,
  UserCircle,
  Cog,
  MessageSquare,
  Radio,
  ArrowRight,
  FileText,
  Paperclip,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Avatar } from '@/components/ui/avatar'
import { cn } from '@/lib/utils'
import { formatRelativeTime } from '@/lib/utils'
import type { Event, EventType } from '@/types'

const eventConfig: Record<EventType, { icon: typeof Circle; color: string; label: string }> = {
  task_created: { icon: Circle, color: 'text-info', label: '任务创建' },
  task_plan_ready: { icon: Cog, color: 'text-info', label: '规划完成' },
  task_status_changed: { icon: Cog, color: 'text-warning', label: '状态变更' },
  todo_assigned: { icon: UserCircle, color: 'text-info', label: '分配 Todo' },
  todo_started: { icon: PlayCircle, color: 'text-info', label: '开始执行' },
  todo_progress: { icon: Cog, color: 'text-muted-foreground', label: '执行中' },
  todo_completed: { icon: CheckCircle2, color: 'text-success', label: 'Todo 完成' },
  todo_failed: { icon: AlertCircle, color: 'text-destructive', label: 'Todo 失败' },
  task_comment: { icon: MessageSquare, color: 'text-muted-foreground', label: '评论' },
  planning_reply: { icon: MessageSquare, color: 'text-info', label: 'PM 规划回复' },
  agent_status_changed: { icon: Radio, color: 'text-warning', label: 'Agent 状态' },
  artifact_received: { icon: Paperclip, color: 'text-info', label: '上传了文件' },
  todo_ask_received: { icon: MessageSquare, color: 'text-warning', label: '请求确认' },
  todo_answer_received: { icon: CheckCircle2, color: 'text-success', label: '用户已回复' },
}

const taskStatusBadge: Record<string, { label: string; variant: 'secondary' | 'info' | 'success' | 'destructive' }> = {
  planning: { label: '规划中', variant: 'info' },
  pending: { label: '待处理', variant: 'secondary' },
  in_progress: { label: '进行中', variant: 'info' },
  done: { label: '已完成', variant: 'success' },
  failed: { label: '失败', variant: 'destructive' },
  canceled: { label: '已取消', variant: 'secondary' },
}

const agentStatusBadge: Record<string, { label: string; className: string }> = {
  online: { label: '在线', className: 'bg-status-online/15 text-status-online' },
  offline: { label: '离线', className: 'bg-status-offline/15 text-status-offline' },
  busy: { label: '忙碌', className: 'bg-status-busy/15 text-status-busy' },
}

function TaskContext({ event }: { event: Event }) {
  const taskTitle = event.metadata.task_title as string | undefined
  const todoTitle = event.metadata.todo_title as string | undefined
  if (!taskTitle) return null

  return (
    <div className="flex items-center gap-1 text-xs text-muted-foreground mt-0.5 min-w-0">
      <FileText className="size-3 shrink-0" />
      <span className="truncate">{taskTitle}</span>
      {todoTitle && (
        <>
          <span className="shrink-0">›</span>
          <span className="truncate">{todoTitle}</span>
        </>
      )}
    </div>
  )
}

function StatusTransition({ from, to }: { from: string; to: string }) {
  const fromCfg = taskStatusBadge[from]
  const toCfg = taskStatusBadge[to]
  if (!fromCfg || !toCfg) return null

  return (
    <div className="flex items-center gap-1.5 mt-1">
      <Badge variant={fromCfg.variant} className="text-[10px] px-1.5 py-0">
        {fromCfg.label}
      </Badge>
      <ArrowRight className="size-3 text-muted-foreground" />
      <Badge variant={toCfg.variant} className="text-[10px] px-1.5 py-0">
        {toCfg.label}
      </Badge>
    </div>
  )
}

function AgentStatusTransition({ from, to }: { from: string; to: string }) {
  const fromCfg = agentStatusBadge[from]
  const toCfg = agentStatusBadge[to]
  if (!fromCfg || !toCfg) return null

  return (
    <div className="flex items-center gap-1.5 mt-1">
      <Badge className={cn('text-[10px] px-1.5 py-0', fromCfg.className)} variant="outline">
        {fromCfg.label}
      </Badge>
      <ArrowRight className="size-3 text-muted-foreground" />
      <Badge className={cn('text-[10px] px-1.5 py-0', toCfg.className)} variant="outline">
        {toCfg.label}
      </Badge>
    </div>
  )
}

function EventDetail({ event }: { event: Event }) {
  if (event.event_type === 'task_status_changed') {
    const from = event.metadata.from as string | undefined
    const to = event.metadata.to as string | undefined
    if (from && to) return <StatusTransition from={from} to={to} />
  }

  if (event.event_type === 'agent_status_changed') {
    const from = event.metadata.prev_status as string | undefined
    const to = event.metadata.new_status as string | undefined
    if (from && to) return <AgentStatusTransition from={from} to={to} />
  }

  if (event.event_type === 'todo_failed') {
    const error = event.metadata.error as string | undefined
    if (error) {
      return (
        <p className="text-xs text-destructive mt-0.5 truncate">
          {error}
        </p>
      )
    }
  }

  if (event.event_type === 'task_comment' && event.content) {
    return (
      <div
        className={cn(
          'mt-1 rounded-md px-3 py-2 text-sm whitespace-pre-wrap line-clamp-3',
          event.actor_type === 'agent' ? 'bg-muted' : 'bg-primary/5'
        )}
      >
        {event.content}
      </div>
    )
  }

  if (event.content && event.event_type === 'planning_reply') {
    return <p className="text-xs text-muted-foreground mt-0.5 truncate">{event.content}</p>
  }

  return null
}

function buildEventLink(event: Event): string | null {
  if (event.project_id && event.task_id) {
    return `/projects/${event.project_id}`
  }
  return null
}

function ActorAvatar({ event }: { event: Event }) {
  if (event.actor_type === 'system') {
    return (
      <div className="flex size-6 items-center justify-center rounded-full bg-muted text-muted-foreground shrink-0">
        <Cog className="size-3.5" />
      </div>
    )
  }

  return (
    <Avatar
      fallback={event.actor_name || (event.actor_type === 'agent' ? 'Agent' : '用户')}
      seed={event.actor_id || event.actor_name}
      kind={event.actor_type === 'agent' ? 'agent' : 'user'}
      role={event.actor_type === 'agent' ? 'custom' : undefined}
      size="sm"
    />
  )
}

interface EventTimelineProps {
  events: Event[]
  loading?: boolean
  emptyText?: string
  showActorName?: boolean
}

export function EventTimeline({
  events,
  loading,
  emptyText = '暂无事件',
  showActorName = false,
}: EventTimelineProps) {
  if (loading) {
    return <div className="py-8 text-center text-sm text-muted-foreground">加载中...</div>
  }

  if (!events || events.length === 0) {
    return <div className="py-8 text-center text-sm text-muted-foreground">{emptyText}</div>
  }

  return (
    <div className="relative">
      {events.map((event, index) => {
        const config = eventConfig[event.event_type] ?? {
          icon: Circle,
          color: 'text-muted-foreground',
          label: event.event_type,
        }
        const Icon = config.icon
        const isLast = index === events.length - 1
        const link = buildEventLink(event)

        const content = (
          <div className="flex gap-3">
            <div className="flex flex-col items-center">
              <div
                className={cn(
                  'flex size-7 items-center justify-center rounded-full bg-muted',
                  config.color
                )}
              >
                <Icon className="size-3.5" />
              </div>
              {!isLast && <div className="w-px flex-1 bg-border" />}
            </div>
            <div className={cn('pb-4 flex-1 min-w-0', isLast && 'pb-0')}>
              <div className="flex items-center gap-1.5">
                <ActorAvatar event={event} />
                {showActorName && event.actor_name && (
                  <span className="text-xs text-muted-foreground">{event.actor_name}</span>
                )}
                <span className="text-sm font-medium">{config.label}</span>
                <span className="text-xs text-muted-foreground ml-auto shrink-0">
                  {formatRelativeTime(event.created_at)}
                </span>
              </div>
              <TaskContext event={event} />
              <EventDetail event={event} />
            </div>
          </div>
        )

        if (link) {
          return (
            <Link
              key={event.id}
              to={link}
              className="block rounded-md -mx-1 px-1 hover:bg-accent/50 transition-colors"
            >
              {content}
            </Link>
          )
        }

        return <div key={event.id}>{content}</div>
      })}
    </div>
  )
}
