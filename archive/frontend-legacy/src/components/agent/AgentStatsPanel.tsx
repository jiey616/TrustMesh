import { Link } from 'react-router-dom'
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  CartesianGrid,
  Legend,
} from 'recharts'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { formatRelativeTime } from '@/lib/utils'
import type { AgentInsights, AgentStats } from '@/types'
import {
  CheckCircle2,
  Clock,
  Zap,
  ListTodo,
  FolderKanban,
  ClipboardList,
  MessageSquare,
  AlertTriangle,
  Layers3,
  ShieldAlert,
  BriefcaseBusiness,
} from 'lucide-react'

function formatDuration(ms: number | null): string {
  if (ms === null) return '-'
  if (ms < 1000) return `${Math.round(ms)} ms`
  const sec = ms / 1000
  if (sec < 60) return `${sec.toFixed(1)} 秒`
  const min = sec / 60
  if (min < 60) return `${min.toFixed(1)} 分钟`
  const hr = min / 60
  return `${hr.toFixed(1)} 小时`
}

function formatDateLabel(dateStr: string): string {
  const parts = dateStr.split('-')
  return `${parts[1]}/${parts[2]}`
}

function formatDurationShort(ms: number | null): string {
  if (ms === null) return '-'
  const hour = 60 * 60 * 1000
  const day = 24 * hour
  if (ms < hour) {
    return `${Math.max(1, Math.round(ms / 60000))} 分钟`
  }
  if (ms < day) {
    return `${(ms / hour).toFixed(1)} 小时`
  }
  return `${(ms / day).toFixed(1)} 天`
}

function formatPercent(value: number): string {
  return `${value.toFixed(1)}%`
}

function getStatusMeta(status: 'pending' | 'in_progress') {
  if (status === 'in_progress') {
    return {
      label: '执行中',
      badge: 'secondary' as const,
    }
  }

  return {
    label: '待处理',
    badge: 'outline' as const,
  }
}

/** 指标卡片 — 根据角色显示不同内容 */
export function AgentMetricCards({ stats }: { stats: AgentStats }) {
  if (stats.role === 'pm') {
    return <PMMetricCards stats={stats} />
  }
  return <ExecutorMetricCards stats={stats} />
}

function PMMetricCards({ stats }: { stats: AgentStats }) {
  const hasTask = stats.tasks_created > 0
  return (
    <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
      <Card>
        <CardContent className="p-4">
          <div className="flex items-center gap-2 text-muted-foreground mb-1">
            <FolderKanban className="size-3.5" />
            <span className="text-xs">管理项目</span>
          </div>
          <div className="text-2xl font-bold">{stats.projects_managed}</div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="p-4">
          <div className="flex items-center gap-2 text-muted-foreground mb-1">
            <ClipboardList className="size-3.5" />
            <span className="text-xs">创建任务</span>
          </div>
          <div className="text-2xl font-bold">{stats.tasks_created}</div>
          <div className="flex gap-1.5 mt-1 flex-wrap">
            {stats.tasks_in_progress > 0 && (
              <Badge variant="secondary" className="text-xs px-1.5 py-0">
                {stats.tasks_in_progress} 进行中
              </Badge>
            )}
            {stats.tasks_pending > 0 && (
              <Badge variant="outline" className="text-xs px-1.5 py-0">
                {stats.tasks_pending} 待处理
              </Badge>
            )}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="p-4">
          <div className="flex items-center gap-2 text-muted-foreground mb-1">
            <CheckCircle2 className="size-3.5" />
            <span className="text-xs">任务完成率</span>
          </div>
          <div className="text-2xl font-bold">
            {hasTask ? `${stats.task_success_rate.toFixed(1)}%` : '-'}
          </div>
          <div className="text-xs text-muted-foreground mt-0.5">
            {stats.tasks_done} 完成 / {stats.tasks_failed} 失败
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="p-4">
          <div className="flex items-center gap-2 text-muted-foreground mb-1">
            <MessageSquare className="size-3.5" />
            <span className="text-xs">规划回复</span>
          </div>
          <div className="text-2xl font-bold">{stats.planning_replies}</div>
        </CardContent>
      </Card>
    </div>
  )
}

function ExecutorMetricCards({ stats }: { stats: AgentStats }) {
  const hasActivity = stats.todos_total > 0
  return (
    <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
      <Card>
        <CardContent className="p-4">
          <div className="flex items-center gap-2 text-muted-foreground mb-1">
            <CheckCircle2 className="size-3.5" />
            <span className="text-xs">完成率</span>
          </div>
          <div className="text-2xl font-bold">
            {hasActivity ? `${stats.success_rate.toFixed(1)}%` : '-'}
          </div>
          <div className="text-xs text-muted-foreground mt-0.5">
            {stats.todos_done} 完成 / {stats.todos_failed} 失败
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="p-4">
          <div className="flex items-center gap-2 text-muted-foreground mb-1">
            <ListTodo className="size-3.5" />
            <span className="text-xs">Todo 总览</span>
          </div>
          <div className="text-2xl font-bold">{stats.todos_total}</div>
          <div className="flex gap-1.5 mt-1 flex-wrap">
            {stats.todos_in_progress > 0 && (
              <Badge variant="secondary" className="text-xs px-1.5 py-0">
                {stats.todos_in_progress} 进行中
              </Badge>
            )}
            {stats.todos_pending > 0 && (
              <Badge variant="outline" className="text-xs px-1.5 py-0">
                {stats.todos_pending} 待处理
              </Badge>
            )}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="p-4">
          <div className="flex items-center gap-2 text-muted-foreground mb-1">
            <Zap className="size-3.5" />
            <span className="text-xs">平均响应时间</span>
          </div>
          <div className="text-2xl font-bold">
            {formatDuration(stats.avg_response_time_ms)}
          </div>
          <div className="text-xs text-muted-foreground mt-0.5">
            接收到开始执行
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="p-4">
          <div className="flex items-center gap-2 text-muted-foreground mb-1">
            <Clock className="size-3.5" />
            <span className="text-xs">平均完成时间</span>
          </div>
          <div className="text-2xl font-bold">
            {formatDuration(stats.avg_completion_time_ms)}
          </div>
          <div className="text-xs text-muted-foreground mt-0.5">
            开始到完成
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

/** 每日工作量图表 — 根据角色显示不同数据 */
export function AgentDailyChart({ stats }: { stats: AgentStats }) {
  const isPM = stats.role === 'pm'
  const hasActivity = isPM ? stats.tasks_created > 0 : stats.todos_total > 0

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">
          {isPM ? '每日任务动态（近 30 天）' : '每日工作量（近 30 天）'}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {hasActivity ? (
          <ResponsiveContainer width="100%" height={220}>
            <BarChart
              data={stats.daily_activity}
              margin={{ top: 4, right: 4, bottom: 0, left: -20 }}
            >
              <CartesianGrid
                strokeDasharray="3 3"
                vertical={false}
                stroke="var(--color-border)"
              />
              <XAxis
                dataKey="date"
                tickFormatter={formatDateLabel}
                tick={{ fontSize: 11, fill: 'var(--color-muted-foreground)' }}
                interval={4}
                axisLine={false}
                tickLine={false}
              />
              <YAxis
                allowDecimals={false}
                tick={{ fontSize: 11, fill: 'var(--color-muted-foreground)' }}
                axisLine={false}
                tickLine={false}
              />
              <Tooltip
                labelFormatter={(label) => `日期: ${String(label)}`}
                contentStyle={{
                  backgroundColor: 'var(--color-popover)',
                  border: '1px solid var(--color-border)',
                  borderRadius: 'var(--radius-md)',
                  fontSize: 12,
                  color: 'var(--color-popover-foreground)',
                }}
              />
              <Legend
                wrapperStyle={{ fontSize: 12 }}
              />
              {isPM && (
                <Bar
                  dataKey="created"
                  name="创建"
                  fill="var(--color-primary)"
                  radius={[2, 2, 0, 0]}
                />
              )}
              <Bar
                dataKey="completed"
                name="完成"
                stackId="result"
                fill="var(--color-success)"
                radius={[0, 0, 0, 0]}
              />
              <Bar
                dataKey="failed"
                name="失败"
                stackId="result"
                fill="var(--color-destructive)"
                radius={[2, 2, 0, 0]}
              />
            </BarChart>
          </ResponsiveContainer>
        ) : (
          <div className="h-[220px] flex items-center justify-center text-sm text-muted-foreground">
            暂无活动数据
          </div>
        )}
      </CardContent>
    </Card>
  )
}

/** 当前工作负载 */
export function AgentWorkload({ stats }: { stats: AgentStats }) {
  if (stats.current_workload.length === 0) return null
  const isPM = stats.role === 'pm'
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base flex items-center gap-2">
          {isPM ? '进行中的任务' : '当前工作负载'}
          <Badge variant="secondary" className="text-xs">
            {stats.current_workload.length}
          </Badge>
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="divide-y divide-border">
          {stats.current_workload.map((item) => (
            <div
              key={item.todo_id || item.task_id}
              className="py-2.5 first:pt-0 last:pb-0 flex items-center justify-between gap-3"
            >
              <div className="min-w-0 flex-1">
                <div className="text-sm font-medium truncate">
                  {isPM ? item.task_title : item.todo_title}
                </div>
                {!isPM && item.task_title && (
                  <div className="text-xs text-muted-foreground mt-0.5">
                    <Link
                      to={`/projects/${item.project_id}/tasks/${item.task_id}`}
                      className="hover:underline"
                    >
                      {item.task_title}
                    </Link>
                  </div>
                )}
              </div>
              <div className="text-xs text-muted-foreground shrink-0">
                {formatRelativeTime(item.started_at)}
              </div>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}

function InsightsCardSkeleton() {
  return (
    <Card>
      <CardHeader className="pb-3">
        <Skeleton className="h-5 w-28" />
      </CardHeader>
      <CardContent className="space-y-3">
        <Skeleton className="h-4 w-full" />
        <Skeleton className="h-4 w-5/6" />
        <Skeleton className="h-4 w-2/3" />
      </CardContent>
    </Card>
  )
}

function EmptyInsightCard({
  title,
  description,
  icon: Icon,
}: {
  title: string
  description: string
  icon: typeof Layers3
}) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base flex items-center gap-2">
          <Icon className="size-4 text-muted-foreground" />
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="h-[140px] flex items-center justify-center text-sm text-muted-foreground text-center">
          {description}
        </div>
      </CardContent>
    </Card>
  )
}

function AgingDistributionCard({ insights }: { insights: AgentInsights }) {
  const max = Math.max(...insights.aging.map((item) => item.count), 1)

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base flex items-center gap-2">
          <Layers3 className="size-4 text-muted-foreground" />
          积压老化分布
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {insights.aging.some((item) => item.count > 0) ? (
          insights.aging.map((item) => (
            <div key={item.label} className="space-y-1.5">
              <div className="flex items-center justify-between text-sm">
                <span>{item.label}</span>
                <span className="text-muted-foreground">{item.count}</span>
              </div>
              <div className="h-2 rounded-full bg-muted overflow-hidden">
                <div
                  className="h-full rounded-full bg-primary"
                  style={{ width: `${(item.count / max) * 100}%` }}
                />
              </div>
            </div>
          ))
        ) : (
          <div className="h-[140px] flex items-center justify-center text-sm text-muted-foreground">
            当前没有积压项
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function PriorityBreakdownCard({ insights, isPM }: { insights: AgentInsights; isPM: boolean }) {
  if (insights.priority_breakdown.length === 0) {
    return (
      <EmptyInsightCard
        title="按优先级完成情况"
        description={`暂无${isPM ? '任务' : 'Todo'}优先级数据`}
        icon={ShieldAlert}
      />
    )
  }

  const max = Math.max(...insights.priority_breakdown.map((item) => item.total), 1)

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base flex items-center gap-2">
          <ShieldAlert className="size-4 text-muted-foreground" />
          按优先级完成情况
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {insights.priority_breakdown.map((item) => (
          <div key={item.priority} className="space-y-1.5">
            <div className="flex items-center justify-between text-sm">
              <div className="flex items-center gap-2">
                <span>{item.label}</span>
                <Badge variant="outline" className="px-1.5 py-0">
                  {item.total}
                </Badge>
              </div>
              <span className="text-muted-foreground">
                {item.done} 完成 / {item.failed} 失败
              </span>
            </div>
            <div className="h-2 rounded-full bg-muted overflow-hidden">
              <div
                className="h-full rounded-full bg-success"
                style={{ width: `${(item.total / max) * 100}%` }}
              />
            </div>
            <div className="flex items-center justify-between text-xs text-muted-foreground">
              <span>{item.pending} 待处理 · {item.in_progress} 执行中</span>
              <span>{formatPercent(item.completion_rate)}</span>
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  )
}

function ProjectContributionCard({ insights, isPM }: { insights: AgentInsights; isPM: boolean }) {
  if (insights.project_contribution.length === 0) {
    return (
      <EmptyInsightCard
        title="项目贡献排行"
        description={`暂无${isPM ? '项目任务' : '项目协作'}数据`}
        icon={BriefcaseBusiness}
      />
    )
  }

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base flex items-center gap-2">
          <BriefcaseBusiness className="size-4 text-muted-foreground" />
          项目贡献排行
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {insights.project_contribution.map((item) => (
          <div key={item.project_id} className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="text-sm font-medium truncate">{item.project_name}</div>
              <div className="text-xs text-muted-foreground mt-0.5">
                {item.total} 项 · {item.done} 完成 · {item.failed} 失败
              </div>
            </div>
            <div className="text-right shrink-0">
              <div className="text-sm font-medium">{formatPercent(item.completion_rate)}</div>
              <div className="text-xs text-muted-foreground">
                {item.pending} 待处理 / {item.in_progress} 执行中
              </div>
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  )
}

function RiskSummaryCard({ insights, isPM }: { insights: AgentInsights; isPM: boolean }) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base flex items-center gap-2">
          <AlertTriangle className="size-4 text-muted-foreground" />
          风险摘要
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid grid-cols-2 gap-3">
          <div className="rounded-lg border p-3">
            <div className="text-xs text-muted-foreground">最老待处理</div>
            <div className="mt-1 text-lg font-semibold">
              {formatDurationShort(insights.oldest_pending_ms)}
            </div>
          </div>
          <div className="rounded-lg border p-3">
            <div className="text-xs text-muted-foreground">最长执行中</div>
            <div className="mt-1 text-lg font-semibold">
              {formatDurationShort(insights.longest_in_progress_ms)}
            </div>
          </div>
          <div className="rounded-lg border p-3">
            <div className="text-xs text-muted-foreground">24 小时未闭环</div>
            <div className="mt-1 text-lg font-semibold">{insights.pending_over_24h}</div>
          </div>
          <div className="rounded-lg border p-3">
            <div className="text-xs text-muted-foreground">近 7 天失败</div>
            <div className="mt-1 text-lg font-semibold">{insights.failures_last_7d}</div>
          </div>
        </div>

        {!isPM && (insights.response_p90_ms !== null || insights.completion_p90_ms !== null) && (
          <div className="grid grid-cols-2 gap-3 text-sm">
            <div className="rounded-lg bg-muted/40 p-3">
              <div className="text-xs text-muted-foreground">P90 响应时长</div>
              <div className="mt-1 font-medium">{formatDuration(insights.response_p90_ms)}</div>
            </div>
            <div className="rounded-lg bg-muted/40 p-3">
              <div className="text-xs text-muted-foreground">P90 完成时长</div>
              <div className="mt-1 font-medium">{formatDuration(insights.completion_p90_ms)}</div>
            </div>
          </div>
        )}

        {insights.risk_items.length > 0 ? (
          <div className="space-y-2">
            {insights.risk_items.map((item) => {
              const meta = getStatusMeta(item.status)
              return (
                <div key={item.id} className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="text-sm font-medium truncate">{item.title}</div>
                    <div className="text-xs text-muted-foreground mt-0.5 truncate">
                      {item.project_name} · {item.subtitle}
                    </div>
                  </div>
                  <div className="text-right shrink-0">
                    <Badge variant={meta.badge} className="mb-1">
                      {meta.label}
                    </Badge>
                    <div className="text-xs text-muted-foreground">
                      {formatDurationShort(item.age_ms)}
                    </div>
                  </div>
                </div>
              )
            })}
          </div>
        ) : (
          <div className="text-sm text-muted-foreground">
            当前没有需要关注的{isPM ? '任务' : 'Todo'}风险项。
          </div>
        )}
      </CardContent>
    </Card>
  )
}

export function AgentInsightPanels({
  stats,
  insights,
  loading,
}: {
  stats: AgentStats
  insights: AgentInsights | null
  loading: boolean
}) {
  if (loading) {
    return (
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <InsightsCardSkeleton />
        <InsightsCardSkeleton />
        <InsightsCardSkeleton />
        <InsightsCardSkeleton />
      </div>
    )
  }

  if (!insights || insights.total_items === 0) {
    return (
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <EmptyInsightCard
          title="积压老化分布"
          description="当前 Agent 还没有可分析的历史工作项。"
          icon={Layers3}
        />
        <EmptyInsightCard
          title="按优先级完成情况"
          description="后续有任务流转后，这里会显示老板视角的交付结构。"
          icon={ShieldAlert}
        />
      </div>
    )
  }

  const isPM = stats.role === 'pm'

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
      <AgingDistributionCard insights={insights} />
      <PriorityBreakdownCard insights={insights} isPM={isPM} />
      <ProjectContributionCard insights={insights} isPM={isPM} />
      <RiskSummaryCard insights={insights} isPM={isPM} />
    </div>
  )
}
