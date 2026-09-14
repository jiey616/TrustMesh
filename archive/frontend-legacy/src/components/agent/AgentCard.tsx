import { Link } from 'react-router-dom'
import { Pencil, Trash2, MoreHorizontal } from 'lucide-react'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { AgentStatusBadge } from '@/components/shared/StatusBadge'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger, DropdownMenuSeparator } from '@/components/ui/dropdown-menu'
import { Avatar } from '@/components/ui/avatar'
import type { Agent } from '@/types'
import { formatRelativeTime } from '@/lib/utils'

const roleLabels: Record<string, string> = {
  pm: 'PM',
  developer: '开发者',
  reviewer: '审核者',
  custom: '自定义',
}

interface AgentCardProps {
  agent: Agent
  onEdit: () => void
  onDelete: () => void
}

function formatUsage(agent: Agent) {
  const parts: string[] = []
  if (agent.usage.project_count > 0) parts.push(`${agent.usage.project_count} 个项目`)
  if (agent.usage.task_count > 0) parts.push(`${agent.usage.task_count} 个任务`)
  if (agent.usage.todo_count > 0) parts.push(`${agent.usage.todo_count} 个 Todo`)
  return parts.join('、')
}

export function AgentCard({ agent, onEdit, onDelete }: AgentCardProps) {
  const usageText = formatUsage(agent)

  return (
    <Card className="transition-all hover:shadow-md">
      <CardContent className="p-4">
        <div className="flex items-start justify-between gap-3">
          <Link to={`/agents/${agent.id}`} className="flex items-center gap-3 min-w-0 flex-1">
            <Avatar
              fallback={agent.name}
              seed={agent.node_id}
              kind="agent"
              role={agent.role}
              size="lg"
            />
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <h3 className="text-sm font-semibold truncate">{agent.name}</h3>
                <AgentStatusBadge status={agent.status} />
              </div>
              <p className="text-xs text-muted-foreground mt-0.5">
                {roleLabels[agent.role]} &middot; {agent.node_id}
              </p>
            </div>
          </Link>

          <DropdownMenu>
            <DropdownMenuTrigger className="p-1 rounded hover:bg-muted shrink-0">
              <MoreHorizontal className="size-4 text-muted-foreground" />
            </DropdownMenuTrigger>
            <DropdownMenuContent>
              <DropdownMenuItem onClick={onEdit}>
                <Pencil className="size-3.5 mr-2" />
                编辑
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                onClick={onDelete}
                className="text-destructive"
                title={agent.usage.in_use ? `该 Agent 正被 ${usageText} 引用，将办理离职` : '删除 Agent'}
              >
                <Trash2 className="size-3.5 mr-2" />
                {agent.usage.in_use ? '离职' : '删除'}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>

        <Link to={`/agents/${agent.id}`} className="block">
          {agent.description && (
            <p className="text-sm text-muted-foreground mt-3 line-clamp-2">{agent.description}</p>
          )}

          <div className="flex flex-wrap gap-1 mt-3">
            {agent.capabilities.map((cap) => (
              <Badge key={cap} variant="secondary" className="text-xs">
                {cap}
              </Badge>
            ))}
          </div>

          <div className="mt-3 flex items-center gap-2">
            <Badge variant={agent.usage.in_use ? 'destructive' : 'secondary'} className="text-xs">
              {agent.usage.in_use ? '已被引用' : '可删除'}
            </Badge>
            {agent.usage.in_use && (
              <p className="text-xs text-muted-foreground truncate" title={`被 ${usageText} 引用`}>
                被 {usageText} 引用
              </p>
            )}
          </div>

          {agent.last_seen_at && (
            <p className="text-xs text-muted-foreground mt-3">
              最近在线: {formatRelativeTime(agent.last_seen_at)}
            </p>
          )}
        </Link>
      </CardContent>
    </Card>
  )
}
