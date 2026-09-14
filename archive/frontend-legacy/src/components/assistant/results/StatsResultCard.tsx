import type { DashboardStats } from '@/types'
import { BarChart3 } from 'lucide-react'

interface Props {
  stats: DashboardStats
}

export function StatsResultCard({ stats }: Props) {
  return (
    <div className="rounded-xl border bg-card p-3 space-y-2 text-sm">
      <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
        <BarChart3 className="size-3.5" />
        统计概览
      </div>
      <div className="grid grid-cols-2 gap-2">
        <StatItem label="Agent 在线" value={`${stats.agents_online}/${stats.agents_total}`} />
        <StatItem label="进行中任务" value={stats.tasks_in_progress} />
        <StatItem label="已完成" value={stats.tasks_done_count} />
        <StatItem label="失败" value={stats.tasks_failed_count} />
        <StatItem label="成功率" value={`${(stats.success_rate * 100).toFixed(0)}%`} />
        <StatItem label="待处理 Todo" value={stats.todos_pending} />
      </div>
    </div>
  )
}

function StatItem({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="rounded-lg bg-muted/50 p-2 text-center">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="text-sm font-semibold mt-0.5">{value}</div>
    </div>
  )
}
