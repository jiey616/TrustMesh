import { useParams, useNavigate } from 'react-router-dom'
import { Plus, Pencil, MoreHorizontal, Trash2, Archive, Copy, Check } from 'lucide-react'
import { toast } from 'sonner'
import { PageContainer } from '@/components/layout/PageContainer'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger, DropdownMenuSeparator } from '@/components/ui/dropdown-menu'
import { AgentStatusBadge } from '@/components/shared/StatusBadge'
import { EventTimeline } from '@/components/shared/EventTimeline'
import { useAgent, useAgentStats, useDeleteAgent } from '@/hooks/useAgents'
import { useAgentInsights } from '@/hooks/useAgentInsights'
import { useAgentEvents } from '@/hooks/useDashboard'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'
import { ApiRequestError } from '@/api/client'
import { formatRelativeTime, truncateNodeId } from '@/lib/utils'
import { useCallback, useRef, useState } from 'react'
import {
  AgentMetricCards,
  AgentDailyChart,
  AgentInsightPanels,
  AgentWorkload,
} from '@/components/agent/AgentStatsPanel'
import { AgentTaskList } from '@/components/agent/AgentTaskList'
import { AgentConfigDialog } from '@/components/agent/AgentConfigDialog'
import { ArchiveAgentDialog } from '@/components/agent/ArchiveAgentDialog'
import { AgentChatPanel } from '@/components/agent/AgentChatPanel'
import { HermesJobsTab, HermesSkillsTab, HermesModelsTab } from '@/components/agent/HermesCapabilityTab'
import { CreateTaskDialog } from '@/components/task/CreateTaskDialog'
import { Avatar } from '@/components/ui/avatar'
import type { EventType } from '@/types'

function CopyIcon({ value }: { value: string }) {
  const { copied, copy } = useCopyToClipboard()

  return (
    <button
      type="button"
      onClick={() => copy(value)}
      className="inline-flex items-center text-muted-foreground/50 hover:text-foreground transition-colors cursor-pointer"
      title="复制"
    >
      {copied ? <Check className="size-3" /> : <Copy className="size-3" />}
    </button>
  )
}

// 产品标识徽章：显示 Agent 的产品归属（trustmesh 平台原生 / hermes / opc / 其他）
const productBadgeStyle: Record<string, 'secondary' | 'info' | 'success' | 'warning' | 'outline'> = {
  trustmesh: 'secondary',
  hermes: 'info',
  opc: 'success',
  openclaw: 'warning',
}

function ProductBadge({ product }: { product: string }) {
  const value = product || 'trustmesh'
  const variant = productBadgeStyle[value] ?? 'outline'
  return (
    <Badge variant={variant} className="text-xs" title={`产品标识: ${value}`}>
      {value}
    </Badge>
  )
}

const eventTypeFilters: { label: string; value: EventType | 'all' }[] = [
  { label: '全部', value: 'all' },
  { label: '任务', value: 'task_created' },
  { label: 'Todo', value: 'todo_completed' },
  { label: '失败', value: 'todo_failed' },
  { label: '进度', value: 'todo_progress' },
]

export function AgentDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { data: agent, isLoading: agentLoading } = useAgent(id)
  const { data: stats } = useAgentStats(id)
  const { data: insights, isLoading: insightsLoading } = useAgentInsights(id)
  const { data: events, isLoading: eventsLoading } = useAgentEvents(id)
  const navigate = useNavigate()
  const deleteAgent = useDeleteAgent()
  const [filter, setFilter] = useState<EventType | 'all'>('all')
  const [createTaskOpen, setCreateTaskOpen] = useState(false)
  const [editOpen, setEditOpen] = useState(false)
  const [archiveOpen, setArchiveOpen] = useState(false)
  const [activeTab, setActiveTab] = useState('overview')
  const containerRef = useRef<HTMLDivElement>(null)
  const headerRef = useCallback((node: HTMLDivElement | null) => {
    if (!node) return
    const observer = new ResizeObserver(() => {
      containerRef.current?.style.setProperty('--agent-header-h', `${node.offsetHeight}px`)
    })
    observer.observe(node)
  }, [])

  if (agentLoading) {
    return <div className="p-6 text-sm text-muted-foreground">加载中...</div>
  }

  if (!agent) {
    return <div className="p-6 text-sm text-muted-foreground">Agent 未找到</div>
  }

  const handleDelete = async () => {
    if (agent.usage.in_use) {
      setArchiveOpen(true)
      return
    }
    if (confirm(`确定要删除 Agent "${agent.name}" 吗？`)) {
      try {
        await deleteAgent.mutateAsync(agent.id)
        toast.success(`Agent "${agent.name}" 已删除`)
        navigate('/dashboard')
      } catch (err) {
        const message = err instanceof ApiRequestError ? err.message : '删除失败'
        toast.error(message)
      }
    }
  }

  const filteredEvents = filter === 'all'
    ? (events ?? [])
    : (events ?? []).filter((e) => e.event_type === filter)

  return (
    <PageContainer ref={containerRef} className="flex h-full min-h-0 flex-col overflow-hidden p-0 [--agent-header-h:4.5rem]">
      <Tabs value={activeTab} onValueChange={setActiveTab} className="flex min-h-0 flex-1 flex-col bg-background">
        {/* Sticky header with integrated tab navigation */}
        <div ref={headerRef} className="sticky top-0 z-20 border-b bg-background/95 px-6 py-3 supports-backdrop-filter:backdrop-blur-xs">
          <div className="relative flex items-center gap-3">
            <Avatar
              fallback={agent.name}
              seed={agent.node_id}
              kind="agent"
              role={agent.role}
              size="lg"
            />
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <h1 className="text-lg font-bold truncate">{agent.name}</h1>
                <AgentStatusBadge status={agent.status} />
                <ProductBadge product={agent.product} />
                {agent.archived && (
                  <Badge variant="secondary" className="text-xs">已离职</Badge>
                )}
                <span className="text-xs text-muted-foreground">
                  <span>{({ pm: 'PM', developer: '开发者', reviewer: '审核者', custom: '自定义' })[agent.role] ?? agent.role}</span>
                  <span> · </span>
                  <span className="font-mono" title={agent.node_id}>{truncateNodeId(agent.node_id)}</span>
                  <span className="ml-0.5"><CopyIcon value={agent.node_id} /></span>
                  {agent.last_seen_at && (
                    <span> · {formatRelativeTime(agent.last_seen_at)}</span>
                  )}
                </span>
              </div>
              <div className="flex items-center gap-2 mt-0.5">
                {agent.description && (
                  <p className="text-xs text-muted-foreground truncate">{agent.description}</p>
                )}
                {agent.description && agent.capabilities?.length > 0 && (
                  <span className="text-xs text-muted-foreground shrink-0">·</span>
                )}
                {agent.capabilities?.length > 0 && (
                  <div className="flex items-center gap-1 shrink-0">
                    {agent.capabilities.map((cap) => (
                      <Badge key={cap} variant="secondary" className="text-[11px] px-1.5 py-0">
                        {cap}
                      </Badge>
                    ))}
                  </div>
                )}
              </div>
            </div>
            <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
              <TabsList variant="line" className="pointer-events-auto">
                <TabsTrigger value="overview">概览</TabsTrigger>
                <TabsTrigger value="chat">对话</TabsTrigger>
                <TabsTrigger value="tasks">工作记录</TabsTrigger>
                <TabsTrigger value="activity">活动日志</TabsTrigger>
                {agent.product === 'hermes' && (
                  <>
                    <TabsTrigger value="capability-skills">技能</TabsTrigger>
                    <TabsTrigger value="capability-models">模型</TabsTrigger>
                    <TabsTrigger value="capability-jobs">定时任务</TabsTrigger>
                  </>
                )}
              </TabsList>
            </div>
            <div className="flex items-center gap-2 shrink-0">
              {agent.role !== 'pm' && !agent.archived && (
                <Button size="sm" variant="outline" onClick={() => setCreateTaskOpen(true)}>
                  <Plus className="size-4 mr-1.5" />
                  创建任务
                </Button>
              )}
              {!agent.archived && (
                <DropdownMenu>
                  <DropdownMenuTrigger className="inline-flex items-center justify-center rounded-md hover:bg-muted size-8">
                    <MoreHorizontal className="size-4" />
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem onClick={() => setEditOpen(true)}>
                      <Pencil className="size-3.5 mr-2" />
                      编辑
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem onClick={handleDelete} className="text-destructive">
                      {agent.usage.in_use ? (
                        <><Archive className="size-3.5 mr-2" />离职</>
                      ) : (
                        <><Trash2 className="size-3.5 mr-2" />删除</>
                      )}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              )}
            </div>
          </div>
        </div>

        {agent.archived && (
          <div className="mx-6 mt-4 flex items-center gap-3 rounded-lg border border-amber-500/20 bg-amber-500/5 px-4 py-3">
            <Archive className="size-4 text-amber-600 shrink-0" />
            <p className="text-sm text-amber-700 dark:text-amber-400">
              该 Agent 已离职，不再接收新任务
            </p>
          </div>
        )}

        <div className="flex min-h-0 flex-1 flex-col overflow-hidden px-6 py-4">
          {/* Tab: 概览 */}
          <TabsContent value="overview" className="min-h-0 flex-1 overflow-y-auto">
            <div className="flex flex-col gap-4 pb-2">
              {stats && <AgentMetricCards stats={stats} />}
              {stats && <AgentDailyChart stats={stats} />}
              {stats && (
                <AgentInsightPanels
                  stats={stats}
                  insights={insights ?? null}
                  loading={insightsLoading}
                />
              )}
              {stats && <AgentWorkload stats={stats} />}
            </div>
          </TabsContent>

          <TabsContent value="chat" className="min-h-0 flex-1 overflow-hidden">
            <AgentChatPanel agent={agent} />
          </TabsContent>

          {/* Tab: 工作记录 */}
          <TabsContent value="tasks" className="min-h-0 flex-1 overflow-y-auto">
            {id && <AgentTaskList agentId={id} />}
          </TabsContent>

          {/* Tab: 活动日志 */}
          <TabsContent value="activity" className="min-h-0 flex-1 overflow-y-auto">
            <div className="flex flex-wrap gap-1 mb-4">
              {eventTypeFilters.map((f) => (
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
            <EventTimeline
              events={filteredEvents}
              loading={eventsLoading}
              emptyText="暂无活动记录"
            />
          </TabsContent>

          {/* Tab: Hermes 定时任务 / 技能 / 模型（仅 hermes 产品节点） */}
          <TabsContent value="capability-jobs" className="min-h-0 flex-1 overflow-y-auto">
            {id && <HermesJobsTab agentId={id} />}
          </TabsContent>
          <TabsContent value="capability-skills" className="min-h-0 flex-1 overflow-y-auto">
            {id && <HermesSkillsTab agentId={id} />}
          </TabsContent>
          <TabsContent value="capability-models" className="min-h-0 flex-1 overflow-y-auto">
            {id && <HermesModelsTab agentId={id} />}
          </TabsContent>
        </div>
      </Tabs>
      {id && (
        <CreateTaskDialog
          open={createTaskOpen}
          onOpenChange={setCreateTaskOpen}
          defaultAgentId={id}
        />
      )}
      <AgentConfigDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        agent={agent}
      />
      <ArchiveAgentDialog
        open={archiveOpen}
        onOpenChange={setArchiveOpen}
        agent={agent}
        onArchived={() => navigate('/dashboard')}
      />
    </PageContainer>
  )
}
