import { Card, Col, Row, Progress, Tag, Empty, Skeleton, Typography } from 'antd'
import {
  ProjectOutlined,
  ProfileOutlined,
  CheckCircleOutlined,
  ThunderboltOutlined,
  ClockCircleOutlined,
  AppstoreOutlined,
  AlertOutlined,
  BarChartOutlined,
} from '@ant-design/icons'
import dayjs from 'dayjs'
import type { AgentInsights, AgentStats } from '@/types'

function formatDuration(ms: number | null): string {
  if (ms === null) return '-'
  if (ms < 1000) return `${Math.round(ms)} ms`
  const sec = ms / 1000
  if (sec < 60) return `${sec.toFixed(1)} 秒`
  const min = sec / 60
  if (min < 60) return `${min.toFixed(1)} 分钟`
  return `${(min / 60).toFixed(1)} 小时`
}

function formatDurationShort(ms: number | null): string {
  if (ms === null) return '-'
  const hour = 60 * 60 * 1000
  const day = 24 * hour
  if (ms < hour) return `${Math.max(1, Math.round(ms / 60000))} 分钟`
  if (ms < day) return `${(ms / hour).toFixed(1)} 小时`
  return `${(ms / day).toFixed(1)} 天`
}

function formatDateLabel(dateStr: string): string {
  const parts = dateStr.split('-')
  return `${parts[1]}/${parts[2]}`
}

function formatPercent(value: number): string {
  return `${value.toFixed(1)}%`
}

/* ---------- 指标卡片 ---------- */
function AgentMetricCards({ stats }: { stats: AgentStats }) {
  if (stats.role === 'pm') {
    return (
      <Row gutter={[16, 16]}>
        <Col xs={12} lg={6}>
          <Card bordered={false} className="!bg-white/[0.03]">
            <div className="mb-1 flex items-center gap-2 text-white/50">
              <ProjectOutlined /> <span className="text-xs">管理项目</span>
            </div>
            <div className="text-2xl font-bold text-white">{stats.projects_managed}</div>
          </Card>
        </Col>
        <Col xs={12} lg={6}>
          <Card bordered={false} className="!bg-white/[0.03]">
            <div className="mb-1 flex items-center gap-2 text-white/50">
              <ProfileOutlined /> <span className="text-xs">创建任务</span>
            </div>
            <div className="text-2xl font-bold text-white">{stats.tasks_created}</div>
            <div className="mt-1 flex gap-1.5 flex-wrap">
              {stats.tasks_in_progress > 0 && <Tag className="!text-[11px]" color="blue">{stats.tasks_in_progress} 进行中</Tag>}
              {stats.tasks_pending > 0 && <Tag className="!text-[11px]" color="default">{stats.tasks_pending} 待处理</Tag>}
            </div>
          </Card>
        </Col>
        <Col xs={12} lg={6}>
          <Card bordered={false} className="!bg-white/[0.03]">
            <div className="mb-1 flex items-center gap-2 text-white/50">
              <CheckCircleOutlined /> <span className="text-xs">任务完成率</span>
            </div>
            <div className="text-2xl font-bold text-white">
              {stats.tasks_created > 0 && stats.task_success_rate != null ? `${stats.task_success_rate.toFixed(1)}%` : '-'}
            </div>
            <div className="mt-0.5 text-xs text-white/50">{stats.tasks_done} 完成 / {stats.tasks_failed} 失败</div>
          </Card>
        </Col>
        <Col xs={12} lg={6}>
          <Card bordered={false} className="!bg-white/[0.03]">
            <div className="mb-1 flex items-center gap-2 text-white/50">
              <ThunderboltOutlined /> <span className="text-xs">规划回复</span>
            </div>
            <div className="text-2xl font-bold text-white">{stats.planning_replies}</div>
          </Card>
        </Col>
      </Row>
    )
  }
  const hasActivity = stats.todos_total > 0
  return (
    <Row gutter={[16, 16]}>
      <Col xs={12} lg={6}>
        <Card bordered={false} className="!bg-white/[0.03]">
          <div className="mb-1 flex items-center gap-2 text-white/50">
            <CheckCircleOutlined /> <span className="text-xs">完成率</span>
          </div>
          <div className="text-2xl font-bold text-white">
            {hasActivity && stats.success_rate != null ? `${stats.success_rate.toFixed(1)}%` : '-'}
          </div>
          <div className="mt-0.5 text-xs text-white/50">{stats.todos_done} 完成 / {stats.todos_failed} 失败</div>
        </Card>
      </Col>
      <Col xs={12} lg={6}>
        <Card bordered={false} className="!bg-white/[0.03]">
          <div className="mb-1 flex items-center gap-2 text-white/50">
            <ProfileOutlined /> <span className="text-xs">Todo 总览</span>
          </div>
          <div className="text-2xl font-bold text-white">{stats.todos_total}</div>
          <div className="mt-1 flex gap-1.5 flex-wrap">
            {stats.todos_in_progress > 0 && <Tag className="!text-[11px]" color="blue">{stats.todos_in_progress} 进行中</Tag>}
            {stats.todos_pending > 0 && <Tag className="!text-[11px]" color="default">{stats.todos_pending} 待处理</Tag>}
          </div>
        </Card>
      </Col>
      <Col xs={12} lg={6}>
        <Card bordered={false} className="!bg-white/[0.03]">
          <div className="mb-1 flex items-center gap-2 text-white/50">
            <ThunderboltOutlined /> <span className="text-xs">平均响应时间</span>
          </div>
          <div className="text-2xl font-bold text-white">{formatDuration(stats.avg_response_time_ms)}</div>
          <div className="mt-0.5 text-xs text-white/50">接收到开始执行</div>
        </Card>
      </Col>
      <Col xs={12} lg={6}>
        <Card bordered={false} className="!bg-white/[0.03]">
          <div className="mb-1 flex items-center gap-2 text-white/50">
            <ClockCircleOutlined /> <span className="text-xs">平均完成时间</span>
          </div>
          <div className="text-2xl font-bold text-white">{formatDuration(stats.avg_completion_time_ms)}</div>
          <div className="mt-0.5 text-xs text-white/50">开始到完成</div>
        </Card>
      </Col>
    </Row>
  )
}

/* ---------- 每日工作量柱状图（纯 CSS，无需图表库） ---------- */
function AgentDailyChart({ stats }: { stats: AgentStats }) {
  const isPM = stats.role === 'pm'
  const hasActivity = isPM ? stats.tasks_created > 0 : stats.todos_total > 0
  const data = stats.daily_activity ?? []

  if (!hasActivity || data.length === 0) {
    return (
      <Card bordered={false} className="!bg-white/[0.03]">
        <div className="mb-2 text-base font-medium text-white/90">
          {isPM ? '每日任务动态（近 30 天）' : '每日工作量（近 30 天）'}
        </div>
        <div className="flex h-[220px] items-center justify-center text-sm text-white/50">暂无活动数据</div>
      </Card>
    )
  }

  const maxVal = Math.max(
    1,
    ...data.map((d) => Math.max(d.completed + d.failed, isPM ? d.created : 0)),
  )

  return (
    <Card bordered={false} className="!bg-white/[0.03]">
      <div className="mb-3 flex items-center justify-between">
        <div className="text-base font-medium text-white/90">
          {isPM ? '每日任务动态（近 30 天）' : '每日工作量（近 30 天）'}
        </div>
        <div className="flex items-center gap-3 text-xs text-white/50">
          {isPM && <span className="flex items-center gap-1"><i className="inline-block h-2 w-2 rounded-sm bg-[#6d5ff5]" />创建</span>}
          <span className="flex items-center gap-1"><i className="inline-block h-2 w-2 rounded-sm bg-[#10b981]" />完成</span>
          <span className="flex items-center gap-1"><i className="inline-block h-2 w-2 rounded-sm bg-[#ef4444]" />失败</span>
        </div>
      </div>
      <div className="flex h-[220px] items-end gap-[3px] overflow-x-auto pb-1">
        {data.map((d) => {
          const createdH = isPM ? (d.created / maxVal) * 100 : 0
          const completedH = (d.completed / maxVal) * 100
          const failedH = (d.failed / maxVal) * 100
          return (
            <div key={d.date} className="group flex min-w-[6px] flex-1 flex-col items-center justify-end gap-[1px]" title={`${d.date}: 完成 ${d.completed} / 失败 ${d.failed}${isPM ? ` / 创建 ${d.created}` : ''}`}>
              <div className="flex w-full items-end justify-center gap-[1px]">
                {isPM && <div className="w-[3px] rounded-sm bg-[#6d5ff5]" style={{ height: `${createdH * 2}px` }} />}
                <div className="w-[3px] rounded-sm bg-[#10b981]" style={{ height: `${completedH * 2}px` }} />
                <div className="w-[3px] rounded-sm bg-[#ef4444]" style={{ height: `${failedH * 2}px` }} />
              </div>
            </div>
          )
        })}
      </div>
      <div className="mt-1 flex justify-between text-[10px] text-white/40">
        <span>{data[0] ? formatDateLabel(data[0].date) : ''}</span>
        <span>{data[data.length - 1] ? formatDateLabel(data[data.length - 1].date) : ''}</span>
      </div>
    </Card>
  )
}

/* ---------- 工作负载 ---------- */
function AgentWorkload({ stats }: { stats: AgentStats }) {
  if (!stats.current_workload || stats.current_workload.length === 0) return null
  const isPM = stats.role === 'pm'
  return (
    <Card bordered={false} className="!bg-white/[0.03]">
      <div className="mb-2 flex items-center gap-2 text-base font-medium text-white/90">
        {isPM ? '进行中的任务' : '当前工作负载'}
        <Tag className="!text-xs" color="blue">{stats.current_workload.length}</Tag>
      </div>
      <div className="divide-y divide-white/[0.05]">
        {stats.current_workload.map((item) => (
          <div key={item.todo_id || item.task_id} className="flex items-center justify-between gap-3 py-2.5 first:pt-0 last:pb-0">
            <div className="min-w-0 flex-1">
              <div className="truncate text-sm text-white/90">{isPM ? item.task_title : item.todo_title}</div>
              {!isPM && item.task_title && (
                <div className="mt-0.5 truncate text-xs text-white/50">{item.task_title}</div>
              )}
            </div>
            <div className="shrink-0 text-xs text-white/40">{dayjs(item.started_at).fromNow()}</div>
          </div>
        ))}
      </div>
    </Card>
  )
}

/* ---------- 洞察面板 ---------- */
function EmptyInsightCard({ title, description }: { title: string; description: string }) {
  return (
    <Card bordered={false} className="!bg-white/[0.03]">
      <div className="mb-2 text-base font-medium text-white/90">{title}</div>
      <div className="flex h-[140px] items-center justify-center text-center text-sm text-white/50">{description}</div>
    </Card>
  )
}

function AgingDistributionCard({ insights }: { insights: AgentInsights }) {
  const aging = insights.aging ?? []
  const max = Math.max(...aging.map((i) => i.count), 1)
  return (
    <Card bordered={false} className="!bg-white/[0.03]">
      <div className="mb-2 flex items-center gap-2 text-base font-medium text-white/90">
        <AppstoreOutlined /> 积压老化分布
      </div>
      {aging.some((i) => i.count > 0) ? (
        <div className="space-y-3">
          {aging.map((i) => (
            <div key={i.label} className="space-y-1.5">
              <div className="flex items-center justify-between text-sm text-white/80">
                <span>{i.label}</span>
                <span className="text-white/50">{i.count}</span>
              </div>
              <Progress percent={Math.round((i.count / max) * 100)} showInfo={false} strokeColor="#6d5ff5" />
            </div>
          ))}
        </div>
      ) : (
        <div className="flex h-[140px] items-center justify-center text-sm text-white/50">当前没有积压项</div>
      )}
    </Card>
  )
}

function PriorityBreakdownCard({ insights, isPM }: { insights: AgentInsights; isPM: boolean }) {
  const priorityBreakdown = insights.priority_breakdown ?? []
  if (priorityBreakdown.length === 0) {
    return <EmptyInsightCard title="按优先级完成情况" description={`暂无${isPM ? '任务' : 'Todo'}优先级数据`} />
  }
  const max = Math.max(...priorityBreakdown.map((i) => i.total), 1)
  return (
    <Card bordered={false} className="!bg-white/[0.03]">
      <div className="mb-2 flex items-center gap-2 text-base font-medium text-white/90">
        <AlertOutlined /> 按优先级完成情况
      </div>
      <div className="space-y-3">
        {priorityBreakdown.map((i) => (
          <div key={i.priority} className="space-y-1.5">
            <div className="flex items-center justify-between text-sm text-white/80">
              <span>{i.label}</span>
              <span className="text-white/50">{i.done} 完成 / {i.failed} 失败</span>
            </div>
            <Progress percent={Math.round((i.total / max) * 100)} showInfo={false} strokeColor="#22d3ee" />
            <div className="flex items-center justify-between text-xs text-white/40">
              <span>{i.pending} 待处理 · {i.in_progress} 执行中</span>
              <span>{formatPercent(i.completion_rate ?? 0)}</span>
            </div>
          </div>
        ))}
      </div>
    </Card>
  )
}

function ProjectContributionCard({ insights, isPM }: { insights: AgentInsights; isPM: boolean }) {
  const projectContribution = insights.project_contribution ?? []
  if (projectContribution.length === 0) {
    return <EmptyInsightCard title="项目贡献排行" description={`暂无${isPM ? '项目任务' : '项目协作'}数据`} />
  }
  return (
    <Card bordered={false} className="!bg-white/[0.03]">
      <div className="mb-2 flex items-center gap-2 text-base font-medium text-white/90">
        <BarChartOutlined /> 项目贡献排行
      </div>
      <div className="space-y-3">
        {projectContribution.map((i) => (
          <div key={i.project_id} className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="truncate text-sm text-white/90">{i.project_name}</div>
              <div className="mt-0.5 text-xs text-white/50">{i.total} 项 · {i.done} 完成 · {i.failed} 失败</div>
            </div>
            <div className="shrink-0 text-right">
              <div className="text-sm font-medium text-white">{formatPercent(i.completion_rate ?? 0)}</div>
              <div className="text-xs text-white/50">{i.pending} 待处理 / {i.in_progress} 执行中</div>
            </div>
          </div>
        ))}
      </div>
    </Card>
  )
}

function RiskSummaryCard({ insights, isPM }: { insights: AgentInsights; isPM: boolean }) {
  return (
    <Card bordered={false} className="!bg-white/[0.03]">
      <div className="mb-2 flex items-center gap-2 text-base font-medium text-white/90">
        <AlertOutlined /> 风险摘要
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div className="rounded-lg border border-white/[0.08] p-3">
          <div className="text-xs text-white/50">最老待处理</div>
          <div className="mt-1 text-lg font-semibold text-white">{formatDurationShort(insights.oldest_pending_ms)}</div>
        </div>
        <div className="rounded-lg border border-white/[0.08] p-3">
          <div className="text-xs text-white/50">最长执行中</div>
          <div className="mt-1 text-lg font-semibold text-white">{formatDurationShort(insights.longest_in_progress_ms)}</div>
        </div>
        <div className="rounded-lg border border-white/[0.08] p-3">
          <div className="text-xs text-white/50">24 小时未闭环</div>
          <div className="mt-1 text-lg font-semibold text-white">{insights.pending_over_24h}</div>
        </div>
        <div className="rounded-lg border border-white/[0.08] p-3">
          <div className="text-xs text-white/50">近 7 天失败</div>
          <div className="mt-1 text-lg font-semibold text-white">{insights.failures_last_7d}</div>
        </div>
      </div>
      {!isPM && (insights.response_p90_ms !== null || insights.completion_p90_ms !== null) && (
        <div className="mt-3 grid grid-cols-2 gap-3 text-sm">
          <div className="rounded-lg bg-white/[0.04] p-3">
            <div className="text-xs text-white/50">P90 响应时长</div>
            <div className="mt-1 font-medium text-white">{formatDuration(insights.response_p90_ms)}</div>
          </div>
          <div className="rounded-lg bg-white/[0.04] p-3">
            <div className="text-xs text-white/50">P90 完成时长</div>
            <div className="mt-1 font-medium text-white">{formatDuration(insights.completion_p90_ms)}</div>
          </div>
        </div>
      )}
      {insights.risk_items?.length > 0 ? (
        <div className="mt-3 space-y-2">
          {(insights.risk_items ?? []).map((item) => (
            <div key={item.id} className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="truncate text-sm text-white/90">{item.title}</div>
                <div className="mt-0.5 truncate text-xs text-white/50">{item.project_name} · {item.subtitle}</div>
              </div>
              <div className="shrink-0 text-right">
                <Tag className="!text-[10px]" color={item.status === 'in_progress' ? 'blue' : 'default'}>
                  {item.status === 'in_progress' ? '执行中' : '待处理'}
                </Tag>
                <div className="mt-0.5 text-xs text-white/40">{formatDurationShort(item.age_ms)}</div>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="mt-3 text-sm text-white/50">当前没有需要关注的{isPM ? '任务' : 'Todo'}风险项。</div>
      )}
    </Card>
  )
}

function InsightsSummaryCard({ insights }: { insights: AgentInsights }) {
  if (!insights.summary && (!insights.strengths || insights.strengths.length === 0) && (!insights.weaknesses || insights.weaknesses.length === 0)) {
    return null
  }
  return (
    <Card bordered={false} className="!bg-white/[0.03]">
      <div className="mb-2 text-base font-medium text-white/90">Insights</div>
      {insights.summary && <p className="mb-3 text-sm text-white/80">{insights.summary}</p>}
      <Row gutter={[16, 16]}>
        <Col xs={24} md={12}>
          <Typography.Text strong style={{ color: '#10b981' }}>优势</Typography.Text>
          <ul style={{ color: 'rgba(255,255,255,0.8)', paddingLeft: 18 }}>
            {(insights.strengths ?? []).map((s, i) => <li key={i}>{s}</li>)}
          </ul>
        </Col>
        <Col xs={24} md={12}>
          <Typography.Text strong style={{ color: '#f59e0b' }}>改进空间</Typography.Text>
          <ul style={{ color: 'rgba(255,255,255,0.8)', paddingLeft: 18 }}>
            {(insights.weaknesses ?? []).map((s, i) => <li key={i}>{s}</li>)}
          </ul>
        </Col>
      </Row>
    </Card>
  )
}

export function AgentOverview({
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
      <div className="flex flex-col gap-4">
        <Skeleton active paragraph={{ rows: 4 }} />
        <Skeleton active paragraph={{ rows: 4 }} />
      </div>
    )
  }

  const isPM = stats.role === 'pm'
  const showInsights = insights && insights.total_items > 0

  return (
    <div className="flex flex-col gap-4 pb-2">
      <AgentMetricCards stats={stats} />
      <AgentDailyChart stats={stats} />
      <AgentWorkload stats={stats} />

      {insights && <InsightsSummaryCard insights={insights} />}

      {!showInsights ? (
        <Empty description="暂无洞察数据" />
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <AgingDistributionCard insights={insights} />
          <PriorityBreakdownCard insights={insights} isPM={isPM} />
          <ProjectContributionCard insights={insights} isPM={isPM} />
          <RiskSummaryCard insights={insights} isPM={isPM} />
        </div>
      )}
    </div>
  )
}
