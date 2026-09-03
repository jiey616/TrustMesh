import { useEffect, useRef } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Button } from 'antd'
import { AgentAvatar } from '@/components/shared/AgentAvatar'
import { CheckOutlined, MessageOutlined } from '@ant-design/icons'
import { stripReplyPrefix } from '@/lib/text'
import type { MeetingMessage } from '@/types'

interface Props {
  messages: MeetingMessage[]
  loading: boolean
  meetingStatus?: string
  hostNodeId?: string
  agentNameMap?: Record<string, string>
  onEndMeeting?: () => void
  onContinueDiscussion?: () => void
}

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
  return `${days}天前`
}

function resolveTargetName(target?: string, map?: Record<string, string>): string | undefined {
  if (!target) return undefined
  if (target === 'all' || target === 'host') return undefined
  return map?.[target]
}

function isSkillLoadNotice(content: string): boolean {
  const c = content.trim()
  if (!c) return false
  const re = /^(请\s*)?(加载|装载|载入).*(技能|skill)|^(已\s*)?(加载|装载|载入).*tm-meeting/is
  if (!re.test(c)) return false
  const stripped = c
    .replace(/^(请\s*)?(已\s*)?(加载|装载|载入).*(技能|skill|tm-meeting).*$/is, '')
    .trim()
  return stripped === ''
}

const PHASE_LABELS: Record<string, string> = {
  init: '初始化',
  prep: '准备',
  speak: '发言',
  review: '交叉评审',
  'cross-review': '交叉评审',
  clarify: '追问',
  'follow-up': '追问',
  summary: '阶段小结',
  confirm: '纪要确认',
  end: '结束',
}

export function MeetingMessageList({ messages, loading, meetingStatus, hostNodeId, agentNameMap, onEndMeeting, onContinueDiscussion }: Props) {
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  if (loading && messages.length === 0) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: 'var(--text-tertiary)' }}>
        加载中...
      </div>
    )
  }

  if (messages.length === 0) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
        <div style={{ textAlign: 'center' }}>
          <div style={{ borderRadius: 'var(--radius-structure)', background: 'rgba(109,95,245,0.15)', padding: 16, display: 'inline-flex', marginBottom: 8 }}>
            <MessageOutlined style={{ fontSize: 32, color: 'var(--signal)' }} />
          </div>
          <p style={{ fontSize: 14, fontWeight: 500 }}>暂无消息</p>
          <p style={{ fontSize: 12, color: 'var(--text-tertiary)' }}>开始你的第一个议题，数字员工会自动参与讨论</p>
        </div>
      </div>
    )
  }

  const isCompleted = meetingStatus === 'completed'

  const displayMessages = messages.filter((msg) => {
    const c = msg.content.trim()
    if (c === 'WAITING') return false
    if (c.startsWith('ACK ') && /WAITING/i.test(c)) return false
    if (c.startsWith('? Resumed session')) return false
    if (isSkillLoadNotice(c)) return false
    return true
  })

  return (
    <div style={{ overflowY: 'auto', height: '100%', padding: 16 }}>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
        {displayMessages.map((msg) => {
          const isUser = msg.sender_type === 'user'
          const isSystem = msg.sender_type === 'system'
          const targetName = resolveTargetName(msg.target, agentNameMap)
          const displayContent = stripReplyPrefix(
            targetName && msg.target && msg.content.startsWith('@' + msg.target)
              ? msg.content.replace('@' + msg.target, '@' + targetName)
              : msg.content,
          )

          if (isSystem) {
            return (
              <div key={msg.id} style={{ display: 'flex', justifyContent: 'center' }}>
                <div style={{ background: 'var(--surface-raised)', color: 'var(--text-secondary)', fontSize: 12, fontStyle: 'italic', borderRadius: 'var(--radius-control)', padding: '6px 12px', maxWidth: '90%', textAlign: 'center' }}>
                  {stripReplyPrefix(msg.content)}
                </div>
              </div>
            )
          }

          return (
            <div key={msg.id} style={{ display: 'flex', gap: 10, flexDirection: isUser ? 'row-reverse' : 'row' }}>
              {!isUser && (
                <div style={{ flexShrink: 0, marginTop: 2 }}>
                  <AgentAvatar
                    name={msg.sender_name ?? '数字员工'}
                    role={hostNodeId && msg.sender_id === hostNodeId ? 'pm' : 'custom'}
                    seed={String(msg.sender_id || msg.sender_name || 'agent')}
                    size={24}
                  />
                </div>
              )}

              <div style={{ display: 'flex', flexDirection: 'column', maxWidth: '80%', alignItems: isUser ? 'flex-end' : 'flex-start' }}>
                {!isUser && (
                  <span style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 4, padding: '0 4px' }}>
                    <span style={{ fontSize: 12, fontWeight: 500, color: 'var(--text-secondary)' }}>{msg.sender_name}</span>
                    {msg.phase && PHASE_LABELS[msg.phase] && (
                      <span style={{ fontSize: 10, padding: '2px 6px', borderRadius: 'var(--radius-control)', background: 'rgba(109,95,245,0.2)', color: 'var(--signal-hover)', border: '1px solid rgba(109,95,245,0.3)' }}>
                        {PHASE_LABELS[msg.phase]}
                      </span>
                    )}
                    {msg.target && msg.target !== 'all' && msg.target !== 'host' && (
                      <span style={{ fontSize: 10, padding: '2px 6px', borderRadius: 'var(--radius-control)', background: 'var(--surface-raised)', color: 'var(--text-secondary)', border: '1px solid var(--line-strong)' }}>
                        → {targetName ? `@${targetName}` : '定向'}
                      </span>
                    )}
                  </span>
                )}

                <div
                  style={{
                    borderRadius: 'var(--radius-structure)',
                    padding: '12px 16px',
                    background: isUser ? 'linear-gradient(135deg, var(--info), var(--signal))' : 'var(--surface-raised)',
                    border: isUser ? 'none' : '1px solid rgba(255,255,255,0.1)',
                    borderBottomRightRadius: isUser ? 6 : 16,
                    borderBottomLeftRadius: isUser ? 16 : 6,
                  }}
                >
                  {!isUser && msg.context_brief && msg.context_brief.trim() !== '' && (
                    <details style={{ marginBottom: 8, borderRadius: 'var(--radius-control)', background: 'var(--surface-inset)', padding: '6px 10px' }}>
                      <summary style={{ fontSize: 11, fontWeight: 500, color: 'var(--text-secondary)', cursor: 'pointer', listStyle: 'none' }}>
                        ? 上下文摘要（主持人提炼）
                      </summary>
                      <p style={{ fontSize: 11, lineHeight: 1.5, color: 'var(--text-secondary)', marginTop: 6, whiteSpace: 'pre-wrap' }}>{msg.context_brief}</p>
                    </details>
                  )}

                  {isUser ? (
                    <p style={{ margin: 0, fontSize: 13, whiteSpace: 'pre-wrap', color: 'var(--text-primary)' }}>{displayContent}</p>
                  ) : (
                    <div style={{ fontSize: 13, color: 'var(--text-primary)' }}>
                      <ReactMarkdown remarkPlugins={[remarkGfm]}>{displayContent}</ReactMarkdown>
                    </div>
                  )}

                  {!isCompleted && msg.ui_blocks && msg.ui_blocks.length > 0 && onEndMeeting && (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 8, marginTop: 12, paddingTop: 12, borderTop: '1px solid var(--line-strong)' }}>
                      {msg.ui_blocks.map((block) => {
                        if (block.type === 'confirm') {
                          return (
                            <div key={block.id}>
                              {block.label && <p style={{ fontSize: 12, fontWeight: 500, color: 'var(--text-secondary)', marginBottom: 6 }}>{block.label}</p>}
                              <div style={{ display: 'flex', gap: 8 }}>
                                <Button type="primary" size="small" onClick={onEndMeeting}><CheckOutlined />{block.confirm_label ?? '确认结束'}</Button>
                                {block.cancel_label && (
                                  <Button size="small" onClick={onContinueDiscussion}>{block.cancel_label}</Button>
                                )}
                              </div>
                            </div>
                          )
                        }
                        return null
                      })}
                    </div>
                  )}
                </div>

                <span style={{ fontSize: 10, color: 'var(--text-quaternary)', marginTop: 4, padding: '0 4px' }}>
                  {formatRelativeTime(msg.created_at)}
                </span>
              </div>

              {isUser && <div style={{ flexShrink: 0, width: 28 }} />}
            </div>
          )
        })}
        <div ref={bottomRef} />
      </div>
    </div>
  )
}