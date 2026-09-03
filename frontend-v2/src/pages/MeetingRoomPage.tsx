import { useEffect, useMemo, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, Typography, Space, Spin, App, Empty } from 'antd'
import {
  ArrowLeftOutlined,
  SendOutlined,
  LoadingOutlined,
  PlayCircleOutlined,
  StopOutlined,
  UnorderedListOutlined,
  PaperClipOutlined,
  FileTextOutlined,
  CheckSquareOutlined,
} from '@ant-design/icons'
import { useMeeting, useMeetingMessages } from '@/hooks/useMeetings'
import { sendMeetingMessage, startMeeting, endMeeting } from '@/api/meetings'
import { MeetingMessageList } from '@/components/meeting/MeetingMessageList'
import { MeetingParticipantPanel } from '@/components/meeting/MeetingParticipantPanel'
import { NeonBadge } from '@/components/NeonBadge'

const { Text } = Typography

const statusLabel: Record<string, string> = {
  waiting: '等待开始',
  in_progress: '进行中',
  completed: '已结束',
}

export function MeetingRoomPage() {
  const { id: meetingId } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { message } = App.useApp()
  const { data: meeting, isLoading: meetingLoading } = useMeeting(meetingId)
  const { data: messages, isLoading: msgLoading, refetch } = useMeetingMessages(meetingId)
  const [showAgenda, setShowAgenda] = useState(true)
  const [input, setInput] = useState('')
  const [autoStarted, setAutoStarted] = useState(false)

  const agentNameMap = useMemo(() => {
    const m: Record<string, string> = {}
    for (const p of meeting?.participants ?? []) {
      if (p.agent_id) m[p.agent_id] = p.agent_name
      if (p.node_id) m[p.node_id] = p.agent_name
    }
    return m
  }, [meeting])

  useEffect(() => {
    if (meeting && meeting.status === 'waiting' && !autoStarted && meetingId) {
      setAutoStarted(true)
      startMeeting(meetingId)
        .then(() => {
          refetch()
          message.success('会议已开始')
        })
        .catch(() => {
          message.error('自动开始失败，请手动点击')
          setAutoStarted(false)
        })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [meeting?.status, autoStarted, meetingId])

  const handleSend = async (text: string) => {
    if (!text.trim() || !meetingId) return
    try {
      await sendMeetingMessage(meetingId, text.trim())
      refetch()
    } catch {
      message.error('发送失败')
    }
  }

  const handleStart = async () => {
    if (!meetingId) return
    try {
      await startMeeting(meetingId)
      refetch()
      message.success('会议已开始')
    } catch {
      message.error('启动失败')
    }
  }

  const handleEnd = async () => {
    if (!meetingId) return
    try {
      await endMeeting(meetingId)
      refetch()
      message.success('会议已结束，纪要已保存')
    } catch {
      message.error('结束失败')
    }
  }

  const handleContinue = () => {
    setInput('请对我的发言进行点评，让我更清楚后续如何继续讨论')
    // Focus the composer textarea so the user can immediately edit/send
    setTimeout(() => {
      const textarea = document.querySelector<HTMLTextAreaElement>('textarea')
      textarea?.focus()
    }, 0)
  }

  if (meetingLoading) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
        <Spin indicator={<LoadingOutlined style={{ fontSize: 24 }} spin />} />
      </div>
    )
  }

  if (!meeting) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
        <Empty description="会议不存在" />
      </div>
    )
  }

  const agendaItems = meeting.agenda_items ?? []

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      {/* Header */}
      <div style={{ padding: '12px 16px', borderBottom: '1px solid var(--line)', background: 'var(--surface)', backdropFilter: 'var(--glass-blur)', display: 'flex', alignItems: 'center', gap: 12 }}>
        <Button type="text" icon={<ArrowLeftOutlined />} onClick={() => navigate('/meetings')} />
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Text strong style={{ fontSize: 15, color: 'var(--text-primary)' }}>{meeting.title}</Text>
            <NeonBadge
              label={statusLabel[meeting.status]}
              variant={meeting.status === 'in_progress' ? 'green' : meeting.status === 'waiting' ? 'amber' : 'rose'}
              pulse={meeting.status === 'in_progress'}
            />
          </div>
          {meeting.agenda && <Text type="secondary" style={{ fontSize: 12 }} ellipsis>{meeting.agenda}</Text>}
        </div>
        <Space>
          {meeting.status === 'waiting' && <Button type="primary" ghost size="small" icon={<PlayCircleOutlined />} onClick={handleStart}>开始</Button>}
          {meeting.status === 'in_progress' && <Button danger size="small" icon={<StopOutlined />} onClick={handleEnd}>结束</Button>}
        </Space>
      </div>

      {/* Agenda items */}
      {agendaItems.length > 0 && (
        <div style={{ borderBottom: '1px solid var(--line)' }}>
          <button
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 8,
              padding: '10px 16px',
              width: '100%',
              textAlign: 'left',
              fontSize: 12,
              fontWeight: 500,
              color: 'var(--text-secondary)',
              background: 'transparent',
              border: 'none',
              cursor: 'pointer',
            }}
            onClick={() => setShowAgenda(!showAgenda)}
          >
            <UnorderedListOutlined />
            议程项 ({agendaItems.length})
            <span style={{ marginLeft: 'auto' }}>{showAgenda ? '收起' : '展开'}</span>
          </button>
          {showAgenda && (
            <div style={{ padding: '0 16px 12px', display: 'flex', flexDirection: 'column', gap: 8 }}>
              {agendaItems.map((item, i) => (
                <div key={item.id ?? i} style={{ fontSize: 12 }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '4px 0' }}>
                    <span style={{ flex: 1 }}>{item.description}</span>
                    <span style={{ fontSize: 10, color: 'var(--text-tertiary)', flexShrink: 0 }}>
                      总分{item.assignees?.reduce((s, a) => s + a.weight, 0) ?? 0}
                    </span>
                  </div>
                  {item.assignees && item.assignees.length > 0 && (
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, paddingLeft: 20, paddingBottom: 4 }}>
                      {item.assignees.map((as) => (
                        <span
                          key={as.agent_id}
                          style={{ fontSize: 10, padding: '2px 6px', borderRadius: 'var(--radius-control)', background: 'var(--surface-raised)', color: 'var(--text-secondary)' }}
                        >
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
        <div style={{ padding: '10px 16px', background: 'var(--surface)', borderBottom: '1px solid var(--line)' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12, color: 'var(--text-tertiary)', marginBottom: 6 }}>
            <PaperClipOutlined />
            <span>参考文件 ({meeting.file_ids.length})</span>
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
            {meeting.file_ids.map((fid) => {
              const file = meeting.attached_files?.find((af) => af.id === fid)
              const fileName = file?.file_name ?? `文件 ${fid.slice(0, 8)}`
              return (
                <a
                  key={fid}
                  href={`/api/v1/projects/${meeting.project_id}/files/${fid}/content`}
                  target="_blank"
                  rel="noopener noreferrer"
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 4,
                    fontSize: 12,
                    background: 'var(--surface-raised)',
                    borderRadius: 'var(--radius-control)',
                    padding: '4px 8px',
                    border: '1px solid var(--line-strong)',
                    color: 'var(--text-primary)',
                    textDecoration: 'none',
                  }}
                >
                  <FileTextOutlined style={{ fontSize: 12 }} />
                  <span style={{ maxWidth: 200, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{fileName}</span>
                  {file?.file_size ? (
                    <span style={{ fontSize: 10, color: 'var(--text-tertiary)', flexShrink: 0 }}>
                      {file.file_size > 1024 * 1024
                        ? `${(file.file_size / (1024 * 1024)).toFixed(1)} MB`
                        : `${(file.file_size / 1024).toFixed(0)} KB`}
                    </span>
                  ) : null}
                </a>
              )
            })}
          </div>
        </div>
      )}

      {/* Summary file link */}
      {meeting.status === 'completed' && meeting.summary_file_id && (
        <div style={{ padding: '10px 16px', background: 'rgba(59,130,246,0.08)', borderBottom: '1px solid var(--line)' }}>
          <a
            href={`/api/v1/projects/${meeting.project_id}/files/${meeting.summary_file_id}/content`}
            target="_blank"
            rel="noopener noreferrer"
            style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12, color: 'var(--info)', textDecoration: 'none' }}
          >
            <FileTextOutlined />
            查看会议纪要文件
          </a>
        </div>
      )}

      {/* Todos after meeting ends */}
      {meeting.status === 'completed' && meeting.todos && meeting.todos.length > 0 && (
        <div style={{ padding: '10px 16px', borderBottom: '1px solid var(--line)' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12, color: 'var(--text-tertiary)', marginBottom: 6 }}>
            <CheckSquareOutlined />
            <span>待办事项 ({meeting.todos.length})</span>
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            {meeting.todos.map((todo) => (
              <div key={todo.id} style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12 }}>
                <input type="checkbox" checked={todo.status === 'converted'} readOnly style={{ borderRadius: 'var(--radius-control)' }} />
                <span style={{ flex: 1 }}>{todo.description}</span>
                <span style={{ fontSize: 10, color: 'var(--text-tertiary)' }}>→ {todo.responsible_agent_name}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Body */}
      <div style={{ flex: 1, display: 'flex', overflow: 'hidden' }}>
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
          <div style={{ flex: 1, overflow: 'hidden' }}>
            <MeetingMessageList
              messages={messages ?? []}
              loading={msgLoading}
              meetingStatus={meeting.status}
              hostNodeId={meeting?.participants.find((p) => p.agent_id === meeting.host_agent_id)?.node_id}
              agentNameMap={agentNameMap}
              onEndMeeting={handleEnd}
              onContinueDiscussion={handleContinue}
            />
          </div>

          {meeting.status !== 'completed' && (
            <MeetingComposer value={input} onChange={setInput} onSend={handleSend} />
          )}
        </div>

        <div className="meeting-participant-panel" style={{ width: 220, borderLeft: '1px solid var(--line)', flexShrink: 0, background: 'var(--surface-sunken)', display: 'none' }}>
          <MeetingParticipantPanel meeting={meeting} />
        </div>
      </div>

      <style>{`
        @media (min-width: 768px) {
          .meeting-participant-panel { display: block !important; }
        }
      `}</style>
    </div>
  )
}

interface ComposerProps {
  value: string
  onChange: (v: string) => void
  onSend: (text: string) => Promise<void>
}

function MeetingComposer({ value, onChange, onSend }: ComposerProps) {
  const [sending, setSending] = useState(false)

  const submit = async () => {
    if (!value.trim()) return
    setSending(true)
    try {
      await onSend(value.trim())
      onChange('')
    } finally {
      setSending(false)
    }
  }

  return (
    <div style={{ borderTop: '1px solid var(--line)', background: 'var(--surface)', padding: '12px 16px', flexShrink: 0 }}>
      <div style={{ borderRadius: 'var(--radius-structure)', border: '1px solid var(--line-strong)', background: 'var(--surface-inset)', padding: 12 }}>
        <div style={{ display: 'flex', gap: 8, alignItems: 'flex-end' }}>
          <textarea
            value={value}
            onChange={(e) => onChange(e.target.value)}
            placeholder="发送消息..."
            style={{ flex: 1, background: 'transparent', outline: 'none', resize: 'none', fontSize: 13, minHeight: 44, maxHeight: 200, lineHeight: 1.6, color: 'var(--text-primary)', fontFamily: 'inherit', border: 'none' }}
            onInput={(e) => {
              const el = e.currentTarget
              el.style.height = '44px'
              el.style.height = Math.min(el.scrollHeight, 200) + 'px'
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                submit()
              }
            }}
          />
          <Button type="primary" shape="circle" icon={sending ? <LoadingOutlined spin /> : <SendOutlined />} onClick={submit} disabled={sending || !value.trim()} />
        </div>
      </div>
    </div>
  )
}