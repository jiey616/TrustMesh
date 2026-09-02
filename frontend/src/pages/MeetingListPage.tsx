import { useState, useMemo } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Plus, MessageSquare, Clock, CheckCircle2, Loader2, Users, FileText } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Avatar } from '@/components/ui/avatar'
import { useMeetings } from '@/hooks/useMeetings'
import { CreateMeetingDialog } from '@/components/meeting/CreateMeetingDialog'
import type { Meeting } from '@/types/meeting'

function formatRelativeTime(dateStr: string): string {
  const now = Date.now()
  const date = new Date(dateStr).getTime()
  const diff = now - date
  const mins = Math.floor(diff / 60000)
  if (mins < 1) return '刚刚'
  if (mins < 60) return `${mins}分钟前`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}小时前`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}天前`
  return new Date(dateStr).toLocaleDateString('zh-CN')
}

const statusConfig = {
  waiting: { label: '待开始', icon: Clock, color: 'text-yellow-600', bg: 'bg-yellow-100 dark:bg-yellow-950/30', border: 'border-l-yellow-400' },
  in_progress: { label: '进行中', icon: MessageSquare, color: 'text-green-600', bg: 'bg-green-100 dark:bg-green-950/30', border: 'border-l-green-400' },
  completed: { label: '已结束', icon: CheckCircle2, color: 'text-muted-foreground', bg: 'bg-muted', border: 'border-l-muted-foreground/20' },
}

export default function MeetingListPage() {
  const { projectId } = useParams<{ projectId: string }>()
  const navigate = useNavigate()
  const { data: meetings, isLoading } = useMeetings(projectId!)
  const [createOpen, setCreateOpen] = useState(false)

  const grouped = useMemo(() => {
    const groups: Record<string, Meeting[]> = { waiting: [], in_progress: [], completed: [] }
    meetings?.forEach(m => {
      const key = m.status as keyof typeof groups
      if (groups[key]) groups[key].push(m)
    })
    // Sort: newest first within each group
    Object.values(groups).forEach(list => list.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime()))
    return groups
  }, [meetings])

  const totalCount = meetings?.length ?? 0

  return (
    <div className="h-full flex flex-col">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b shrink-0 bg-card">
        <div className="flex items-center gap-2">
          <h2 className="font-semibold">会议室</h2>
          {totalCount > 0 && (
            <span className="text-xs text-muted-foreground bg-muted rounded-full px-2 py-0.5">
              {totalCount}
            </span>
          )}
        </div>
        <Button size="sm" className="h-8 text-xs" onClick={() => setCreateOpen(true)}>
          <Plus className="size-3.5 mr-1" /> 新建会议
        </Button>
      </div>

      {/* List */}
      <div className="flex-1 overflow-y-auto">
        {isLoading ? (
          <div className="flex items-center justify-center h-32">
            <Loader2 className="size-5 animate-spin text-muted-foreground" />
          </div>
        ) : !meetings || meetings.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-muted-foreground gap-2">
            <div className="rounded-2xl bg-primary/10 p-4 inline-flex">
              <MessageSquare className="size-8 text-primary/60" />
            </div>
            <p className="text-sm font-medium">暂无会议</p>
            <p className="text-xs text-muted-foreground">创建会议后自动展示在这里</p>
            <Button variant="outline" size="sm" className="mt-1" onClick={() => setCreateOpen(true)}>
              <Plus className="size-3.5 mr-1" /> 创建第一个会议
            </Button>
          </div>
        ) : (
          <div className="p-4 space-y-6">
            {/* Active meetings first */}
            {(['in_progress', 'waiting', 'completed'] as const).map(status => {
              const list = grouped[status]
              const cfg = statusConfig[status]
              const StatusIcon = cfg.icon
              if (list.length === 0) return null

              return (
                <div key={status}>
                  {/* Group header */}
                  <div className="flex items-center gap-2 mb-2 px-1">
                    <StatusIcon className={cn('size-3.5', status === 'completed' ? 'text-muted-foreground/60' : cfg.color)} />
                    <span className="text-xs font-medium text-muted-foreground">{cfg.label}</span>
                    <span className="text-[10px] text-muted-foreground/50">{list.length}</span>
                  </div>

                  {/* Meeting cards */}
                  <div className="space-y-2">
                    {list.map(m => (
                      <div
                        key={m.id}
                        className={cn(
                          'rounded-lg border border-l-4 bg-card hover:bg-accent/50 cursor-pointer transition-all',
                          'hover:shadow-sm active:scale-[0.99]',
                          cfg.border,
                        )}
                        onClick={() => navigate(`/projects/${projectId}/meetings/${m.id}`)}
                      >
                        <div className="p-3.5">
                          {/* Title row */}
                          <div className="flex items-start justify-between gap-2">
                            <div className="flex items-center gap-2 min-w-0">
                              <span className="text-sm font-medium truncate">{m.title}</span>
                              <Badge variant="outline" className={cn('text-[10px] h-4 px-1.5 font-normal shrink-0', cfg.bg)}>
                                {cfg.label}
                              </Badge>
                            </div>
                          </div>

                          {/* Agenda */}
                          {m.agenda && (
                            <p className="text-xs text-muted-foreground mt-1.5 line-clamp-2">{m.agenda}</p>
                          )}

                          {/* Meta row */}
                          <div className="flex items-center gap-3 mt-2.5 text-[11px] text-muted-foreground">
                            {/* Participants */}
                            <span className="flex items-center gap-1">
                              <Users className="size-3" />
                              {m.participants.length} 位
                            </span>

                            {/* Files count */}
                            {m.attached_files && m.attached_files.length > 0 && (
                              <span className="flex items-center gap-1">
                                <FileText className="size-3" />
                                {m.attached_files.length} 个文件
                              </span>
                            )}

                            {/* Time */}
                            <span className="ml-auto">{formatRelativeTime(m.created_at)}</span>
                          </div>

                          {/* Participant avatars */}
                          <div className="flex items-center gap-1 mt-2.5">
                            {m.participants.slice(0, 5).map(p => (
                              <Avatar
                                key={p.agent_id}
                                seed={p.node_id}
                                kind="agent"
                                size="sm"
                                role={p.agent_id === m.host_agent_id ? 'pm' : undefined}
                                fallback={p.agent_name?.charAt(0) ?? '?'}
                              />
                            ))}
                            {m.participants.length > 5 && (
                              <span className="text-[10px] text-muted-foreground ml-0.5">
                                +{m.participants.length - 5}
                              </span>
                            )}
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>

      <CreateMeetingDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        projectId={projectId!}
      />
    </div>
  )
}
