import { useState, useEffect, useMemo } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { ArrowLeft, Send, Loader2, Play, Square, FileText, ListTodo, Paperclip } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { toast } from 'sonner'
import { useMeeting, useMeetingMessages } from '@/hooks/useMeetings'
import { sendMeetingMessage, startMeeting, endMeeting } from '@/api/meetings'
import { MeetingMessageList } from '@/components/meeting/MeetingMessageList'
import { MeetingParticipantPanel } from '@/components/meeting/MeetingParticipantPanel'

export default function MeetingRoomPage() {
  const { meetingId } = useParams<{ meetingId: string }>()
  const { projectId } = useParams<{ projectId: string }>()
  const navigate = useNavigate()

  const { data: meeting, isLoading: meetingLoading } = useMeeting(meetingId)
  const { data: messages, isLoading: msgLoading, refetch } = useMeetingMessages(meetingId)

  const [input, setInput] = useState('')
  const [sending, setSending] = useState(false)
  const [actionLoading, setActionLoading] = useState(false)
  const [showAgenda, setShowAgenda] = useState(true)
  const [autoStarted, setAutoStarted] = useState(false)

  // Auto-start meeting when page loads with "waiting" status
  useEffect(() => {
    if (meeting && meeting.status === 'waiting' && !autoStarted && meetingId) {
      setAutoStarted(true)
      startMeeting(meetingId)
        .then(() => {
          refetch()
          toast.success('会议已开始')
        })
        .catch(() => {
          toast.error('自动开始失败，请手动点击')
        })
    }
  }, [meeting?.status, autoStarted])

  // agent_id / node_id → 名称 映射，供会议消息里把主持人点名的 ID 显示为名称
  // ⚠️ 必须在 early return 之前声明，否则违反 React hooks 规则（#310）
  const agentNameMap = useMemo(() => {
    const m: Record<string, string> = {}
    for (const p of meeting?.participants ?? []) {
      if (p.agent_id) m[p.agent_id] = p.agent_name
      if (p.node_id) m[p.node_id] = p.agent_name
    }
    return m
  }, [meeting])

  const handleSend = async () => {
    if (!input.trim() || !meetingId) return
    setSending(true)
    try {
      await sendMeetingMessage(meetingId, input.trim())
      setInput('')
      refetch()
    } catch {
      toast.error('发送失败')
    } finally {
      setSending(false)
    }
  }

  const handleStart = async () => {
    if (!meetingId) return
    setActionLoading(true)
    try {
      await startMeeting(meetingId)
      refetch()
      toast.success('会议已开始')
    } catch {
      toast.error('启动失败')
    } finally {
      setActionLoading(false)
    }
  }

  const handleEnd = async () => {
    if (!meetingId) return
    setActionLoading(true)
    try {
      await endMeeting(meetingId)
      refetch()
      toast.success('会议已结束，纪要已保存')
    } catch {
      toast.error('结束失败')
    } finally {
      setActionLoading(false)
    }
  }

  const handleContinue = async () => {
    if (!meetingId) return
    // Send a message asking the user for feedback to guide further discussion
    setInput('请对我的发言进行点评，让我更清楚后续如何继续讨论')
    // Focus the input
    const textarea = document.querySelector('textarea')
    textarea?.focus()
  }

  if (meetingLoading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader2 className="size-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (!meeting) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground">
        会议不存在
      </div>
    )
  }

  const statusLabel: Record<string, string> = {
    waiting: '等待开始',
    in_progress: '进行中',
    completed: '已结束',
  }
  const statusColor: Record<string, string> = {
    waiting: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400',
    in_progress: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400',
    completed: 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-400',
  }

  const agendaItems = meeting.agenda_items ?? []

  return (
    <div className="h-full flex flex-col">
      {/* Header */}
      <div className="flex items-center gap-3 px-4 py-2.5 border-b shrink-0">
        <Button variant="ghost" size="icon" className="size-8" onClick={() => navigate(`/projects/${projectId}/meetings`)}>
          <ArrowLeft className="size-4" />
        </Button>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <h2 className="font-semibold text-sm truncate">{meeting.title}</h2>
            <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${statusColor[meeting.status]}`}>
              {statusLabel[meeting.status]}
            </span>
          </div>
          {meeting.agenda && (
            <p className="text-xs text-muted-foreground truncate">{meeting.agenda}</p>
          )}
        </div>
        <div className="flex items-center gap-1">
          {meeting.status === 'waiting' && (
            <Button size="sm" variant="outline" className="h-7 text-xs" onClick={handleStart} disabled={actionLoading}>
              <Play className="size-3 mr-1" /> 开始
            </Button>
          )}
          {meeting.status === 'in_progress' && (
            <Button size="sm" variant="outline" className="h-7 text-xs" onClick={handleEnd} disabled={actionLoading}>
              <Square className="size-3 mr-1" /> 结束
            </Button>
          )}
        </div>
      </div>

      {/* Agenda items */}
      {agendaItems.length > 0 && (
        <div className="border-b">
          <button
            className="flex items-center gap-2 px-4 py-2 w-full text-left text-xs font-medium text-muted-foreground hover:bg-muted/30"
            onClick={() => setShowAgenda(!showAgenda)}
          >
            <ListTodo className="size-3.5" />
            议程项 ({agendaItems.length})
            <span className="ml-auto">{showAgenda ? '收起' : '展开'}</span>
          </button>
          {showAgenda && (
            <div className="px-4 pb-2 space-y-2">
              {agendaItems.map((item, i) => (
                <div key={item.id ?? i} className="text-xs">
                  <div className="flex items-center gap-2 py-1">
                    <span className="flex-1">{item.description}</span>
                    <span className="text-[10px] text-muted-foreground shrink-0">总分{item.assignees?.reduce((s, a) => s + a.weight, 0) ?? 0}</span>
                  </div>
                  {item.assignees && item.assignees.length > 0 && (
                    <div className="flex flex-wrap gap-1.5 pl-5 pb-1">
                      {item.assignees.map(as => (
                        <span key={as.agent_id} className="text-[10px] px-1.5 py-0.5 rounded-full bg-muted text-muted-foreground">
                          w{as.weight}
                        </span>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Reference files */}
      {meeting.file_ids && meeting.file_ids.length > 0 && (
        <div className="border-b px-4 py-2 bg-muted/30">
          <div className="flex items-center gap-2 text-xs text-muted-foreground mb-1">
            <Paperclip className="size-3" />
            <span>参考文件 ({meeting.file_ids.length})</span>
          </div>
          <div className="flex flex-wrap gap-2">
            {meeting.file_ids.map(fid => {
              const file = meeting.attached_files?.find(af => af.id === fid)
              const fileName = file?.file_name ?? `文件 ${fid.slice(0, 8)}`
              return (
                <a
                  key={fid}
                  href={`/api/v1/projects/${projectId}/files/${fid}/content`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center gap-1 text-xs bg-background rounded-md px-2 py-1 border hover:bg-muted transition-colors"
                >
                  <FileText className="size-3 shrink-0" />
                  <span className="truncate max-w-[200px]">{fileName}</span>
                  {file?.file_size && (
                    <span className="text-[10px] text-muted-foreground shrink-0">
                      {file.file_size > 1024 * 1024
                        ? `${(file.file_size / (1024 * 1024)).toFixed(1)} MB`
                        : `${(file.file_size / 1024).toFixed(0)} KB`}
                    </span>
                  )}
                </a>
              )
            })}
          </div>
        </div>
      )}

      {/* Summary file link */}
      {meeting.status === 'completed' && meeting.summary_file_id && (
        <div className="border-b px-4 py-2 bg-blue-50/50 dark:bg-blue-950/20">
          <a
            href={`/api/v1/projects/${projectId}/files/${meeting.summary_file_id}/content`}
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-2 text-xs text-blue-600 hover:underline"
          >
            <FileText className="size-3.5" />
            查看会议纪要文件
          </a>
        </div>
      )}

      {/* Todos after meeting ends */}
      {meeting.status === 'completed' && meeting.todos && meeting.todos.length > 0 && (
        <div className="border-b px-4 py-2">
          <p className="text-xs font-medium text-muted-foreground mb-1">待办事项</p>
          <div className="space-y-1">
            {meeting.todos.map(todo => (
              <div key={todo.id} className="flex items-center gap-2 text-xs">
                <input type="checkbox" className="rounded" disabled={todo.status === 'converted'} />
                <span className="flex-1">{todo.description}</span>
                <span className="text-[10px] text-muted-foreground">→ {todo.responsible_agent_name}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Body */}
      <div className="flex-1 flex overflow-hidden">
        <div className="flex-1 flex flex-col overflow-hidden">
          <div className="flex-1 overflow-y-auto">
            <MeetingMessageList
              messages={messages ?? []}
              loading={msgLoading}
              meetingStatus={meeting.status}
              hostNodeId={meeting?.participants.find(p => p.agent_id === meeting.host_agent_id)?.node_id}
              agentNameMap={agentNameMap}
              onEndMeeting={handleEnd}
              onContinueDiscussion={handleContinue}
            />
          </div>

          {meeting.status !== 'completed' && (
            <div className="border-t bg-card px-4 py-3 shrink-0">
              <div className="rounded-2xl border bg-background p-3 shadow-sm focus-within:shadow-md focus-within:border-primary/30 transition-all">
                <div className="flex gap-2 items-end">
                  <textarea
                    value={input}
                    onChange={e => setInput(e.target.value)}
                    placeholder={meeting.status === 'waiting' ? '输入会议议题...' : '发送消息...'}
                    className="flex-1 bg-transparent outline-none resize-none text-sm min-h-[44px] max-h-[200px] leading-relaxed"
                    style={{ height: '44px' }}
                    onInput={e => {
                      const el = e.currentTarget
                      el.style.height = '44px'
                      el.style.height = Math.min(el.scrollHeight, 200) + 'px'
                    }}
                    onKeyDown={e => {
                      if (e.key === 'Enter' && !e.shiftKey) {
                        e.preventDefault()
                        handleSend()
                      }
                    }}
                  />
                  <Button
                    size="icon"
                    className="size-9 shrink-0 rounded-xl"
                    onClick={handleSend}
                    disabled={sending || !input.trim()}
                  >
                    {sending ? <Loader2 className="size-4 animate-spin" /> : <Send className="size-4" />}
                  </Button>
                </div>
              </div>
            </div>
          )}
        </div>

        <div className="w-56 border-l shrink-0 hidden md:block">
          <MeetingParticipantPanel meeting={meeting} />
        </div>
      </div>
    </div>
  )
}
