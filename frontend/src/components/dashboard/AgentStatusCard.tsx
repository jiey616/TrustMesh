import { Link } from 'react-router-dom'
import { ChevronRight } from 'lucide-react'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { AgentStatusIcon } from '@/components/shared/StatusBadge'
import { EventTimeline } from '@/components/shared/EventTimeline'
import { useAgentEvents } from '@/hooks/useDashboard'
import { formatRelativeTime } from '@/lib/utils'
import { Avatar } from '@/components/ui/avatar'
import type { Agent } from '@/types'

interface AgentStatusCardProps {
  agent: Agent
}

export function AgentStatusCard({ agent }: AgentStatusCardProps) {
  const { data: events, isLoading: eventsLoading } = useAgentEvents(agent.id, 10)

  return (
    <Card className="flex flex-col min-h-0 overflow-hidden">
      <CardHeader className="pb-2 pt-4 px-4 shrink-0">
        <Link
          to={`/agents/${agent.id}`}
          className="flex items-center justify-between group"
        >
          <div className="flex items-center gap-2 min-w-0">
            <Avatar
              fallback={agent.name}
              seed={agent.node_id}
              kind="agent"
              role={agent.role}
              size="sm"
            />
            <span className="text-sm font-medium truncate">{agent.name}</span>
            <AgentStatusIcon status={agent.status} />
          </div>
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground shrink-0">
            <span>{({ pm: 'PM', developer: '开发者', reviewer: '审核者', custom: '自定义' })[agent.role] ?? agent.role}</span>
            {agent.status === 'offline' && agent.last_seen_at ? (
              <span>· {formatRelativeTime(agent.last_seen_at)}</span>
            ) : agent.status !== 'offline' ? (
              <span>
                {agent.usage.project_count > 0 && `· ${agent.usage.project_count} 项目`}
                {agent.usage.todo_count > 0 && ` · ${agent.usage.todo_count} Todo`}
                {agent.usage.total_count === 0 && '· 空闲'}
              </span>
            ) : (
              <span>· 离线</span>
            )}
            <ChevronRight className="size-4 opacity-0 group-hover:opacity-100 transition-opacity" />
          </div>
        </Link>
      </CardHeader>
      <CardContent className="relative max-h-[200px] overflow-hidden px-4 pt-2 pb-0 border-t">
        <EventTimeline
          events={events ?? []}
          loading={eventsLoading}
          emptyText="暂无活动"
        />
        {(events?.length ?? 0) > 3 && (
          <div className="absolute inset-x-0 bottom-0 h-10 bg-linear-to-t from-card to-transparent pointer-events-none" />
        )}
      </CardContent>
    </Card>
  )
}
