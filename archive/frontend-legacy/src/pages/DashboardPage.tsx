import { PageContainer } from '@/components/layout/PageContainer'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { AgentStatusCard } from '@/components/dashboard/AgentStatusCard'
import { NodeStatusIndicator } from '@/components/dashboard/NodeStatusIndicator'
import { StatsCard } from '@/components/dashboard/StatsCard'
import { RecentTasksList } from '@/components/dashboard/RecentTasksList'
import { EventTimeline } from '@/components/shared/EventTimeline'
import { useAgents } from '@/hooks/useAgents'
import { useDashboardStats, useDashboardEvents, useDashboardTasks } from '@/hooks/useDashboard'

export function DashboardPage() {
  const { data: agents } = useAgents()
  const { data: stats } = useDashboardStats()
  const { data: events, isLoading: eventsLoading } = useDashboardEvents()
  const { data: tasks, isLoading: tasksLoading } = useDashboardTasks()

  return (
    <PageContainer className="flex flex-col h-full gap-6">
      <div className="shrink-0 flex items-center justify-between">
        <h1 className="text-2xl font-bold">仪表盘</h1>
        <NodeStatusIndicator />
      </div>

      {/* Agent 状态卡片 + 最近活动 */}
      {agents && agents.length > 0 && (
        <div className="shrink-0">
          <h2 className="text-xs font-medium text-muted-foreground uppercase tracking-wider mb-3">
            智能体
          </h2>
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3">
            {agents.map((agent) => (
              <AgentStatusCard key={agent.id} agent={agent} />
            ))}
          </div>
        </div>
      )}

      {/* 统计卡片 */}
      {stats && (
        <div className="shrink-0 grid grid-cols-2 md:grid-cols-4 gap-3">
          <StatsCard
            label="Agent"
            value={`${stats.agents_online} / ${stats.agents_total}`}
            sub="在线"
          />
          <StatsCard
            label="进行中任务"
            value={stats.tasks_in_progress}
            sub={`共 ${stats.tasks_total} 个`}
          />
          <StatsCard
            label="已完成"
            value={stats.tasks_done_count}
          />
          <StatsCard
            label="成功率"
            value={stats.success_rate > 0 ? `${stats.success_rate.toFixed(0)}%` : '-'}
          />
        </div>
      )}

      {/* 最近活动 + 最近任务：填满剩余高度，内部滚动 */}
      <div className="flex-1 min-h-0 grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card className="flex flex-col min-h-0 overflow-hidden">
          <CardHeader className="pb-3 shrink-0">
            <CardTitle className="text-base">最近活动</CardTitle>
          </CardHeader>
          <CardContent className="flex-1 min-h-0 overflow-auto">
            <EventTimeline
              events={events ?? []}
              loading={eventsLoading}
              showActorName
              emptyText="暂无活动记录"
            />
          </CardContent>
        </Card>

        <Card className="flex flex-col min-h-0 overflow-hidden">
          <CardHeader className="pb-3 shrink-0">
            <CardTitle className="text-base">最近任务</CardTitle>
          </CardHeader>
          <CardContent className="flex-1 min-h-0 overflow-auto">
            <RecentTasksList tasks={tasks ?? []} loading={tasksLoading} />
          </CardContent>
        </Card>
      </div>
    </PageContainer>
  )
}
