import { useEffect, useRef } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Avatar } from '@/components/ui/avatar'
import { Check, Square, MessageSquare } from 'lucide-react'
import { cn, normalizeEscapedText } from '@/lib/utils'
import type { MeetingMessage } from '@/types/meeting'

interface Props {
  messages: MeetingMessage[]
  loading: boolean
  meetingStatus?: string
  /** 主持人（PM）的 node_id，用于消息头像显示 PM 徽章并与其他视图配色一致 */
  hostNodeId?: string
  /** agent_id / node_id → 名称 映射，用于把主持人点名的目标 ID 显示为智能体名称 */
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

function formatFullTime(dateStr: string): string {
  return new Date(dateStr).toLocaleString('zh-CN', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

// 解析主持人点名的目标 ID → 智能体名称（不存在则返回 undefined）
function resolveTargetName(target?: string, map?: Record<string, string>): string | undefined {
  if (!target) return undefined
  if (target === 'all' || target === 'host') return undefined
  return map?.[target]
}

// 纯“加载XXX技能”系统提示（无实质内容），不渲染到会议页面
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
      <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
        加载中...
      </div>
    )
  }

  if (messages.length === 0) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center space-y-2">
          <div className="rounded-2xl bg-primary/10 p-4 inline-flex">
            <MessageSquare className="size-8 text-primary/60" />
          </div>
          <p className="text-sm font-medium">暂无消息</p>
          <p className="text-xs text-muted-foreground">开始你的第一个议题，智能体会自动参与讨论</p>
        </div>
      </div>
    )
  }

  const isCompleted = meetingStatus === 'completed'

  // Filter out internal protocol ACK messages
  const displayMessages = messages.filter(msg => {
    const c = msg.content.trim()
    if (c === 'WAITING') return false
    if (c.startsWith('ACK ') && /WAITING/i.test(c)) return false
    if (c.startsWith('↻ Resumed session')) return false
    if (isSkillLoadNotice(c)) return false
    return true
  })

  return (
    <ScrollArea className="h-full">
      <div className="flex flex-col gap-4 p-4">
        {displayMessages.map(msg => {
          const isUser = msg.sender_type === 'user'
          const isSystem = msg.sender_type === 'system'

          // 把主持人点名消息里的 @目标ID 替换为 @智能体名称，便于阅读
          const targetName = resolveTargetName(msg.target, agentNameMap)
          const displayContent =
            targetName && msg.target && msg.content.startsWith('@' + msg.target)
              ? msg.content.replace('@' + msg.target, '@' + targetName)
              : msg.content

          if (isSystem) {
            return (
              <div key={msg.id} className="flex justify-center">
                <div className="bg-muted/50 text-muted-foreground text-xs italic rounded-lg px-3 py-1.5 max-w-[90%] text-center">
                  {msg.content}
                </div>
              </div>
            )
          }

          return (
            <div
              key={msg.id}
              className={cn('flex gap-2.5', isUser ? 'flex-row-reverse' : 'flex-row')}
            >
              {/* Avatar - only for agent messages */}
              {!isUser && (
                <div className="shrink-0 mt-0.5">
                  <Avatar
                    seed={msg.sender_id}
                    kind="agent"
                    size="sm"
                    role={hostNodeId && msg.sender_id === hostNodeId ? 'pm' : undefined}
                    fallback={msg.sender_name ?? 'A'}
                  />
                </div>
              )}

              <div className={cn('flex flex-col max-w-[80%]', isUser ? 'items-end' : 'items-start')}>
                {/* Sender name + phase badge for agent messages */}
                {!isUser && (
                  <span className="flex items-center gap-1.5 mb-1 px-1">
                    <span className="text-xs font-medium text-muted-foreground">{msg.sender_name}</span>
                    {msg.phase && PHASE_LABELS[msg.phase] && (
                      <span className="text-[10px] leading-none px-1.5 py-0.5 rounded-full bg-primary/10 text-primary border border-primary/20">
                        {PHASE_LABELS[msg.phase]}
                      </span>
                    )}
                    {msg.target && msg.target !== 'all' && msg.target !== 'host' && (
                      <span className="text-[10px] leading-none px-1.5 py-0.5 rounded-full bg-muted text-muted-foreground border border-border/60">
                        → {targetName ? `@${targetName}` : '定向'}
                      </span>
                    )}
                  </span>
                )}

                {/* Bubble */}
                <div
                  className={cn(
                    'rounded-2xl px-4 py-3 shadow-xs',
                    isUser
                      ? 'bg-primary text-primary-foreground rounded-br-md'
                      : 'bg-background border rounded-bl-md',
                  )}
                >
                  {/* Context brief (distilled prior-discussion summary from host) */}
                  {!isUser && !isSystem && msg.context_brief && msg.context_brief.trim() !== '' && (
                    <details className="mb-2 rounded-lg bg-muted/60 border border-border/50 px-2.5 py-1.5 group">
                      <summary className="text-[11px] font-medium text-muted-foreground cursor-pointer select-none list-none flex items-center gap-1">
                        <span className="inline-block transition-transform group-open:rotate-90">▸</span>
                        上下文摘要（主持人提炼）
                      </summary>
                      <p className="text-[11px] leading-relaxed text-muted-foreground/90 mt-1 whitespace-pre-wrap">
                        {msg.context_brief}
                      </p>
                    </details>
                  )}

                  {/* Message content */}
                  {isUser ? (
                    <p className="text-sm whitespace-pre-wrap">{normalizeEscapedText(displayContent)}</p>
                  ) : (
                    <div className={cn(
                      'prose prose-sm max-w-none dark:prose-invert',
                      'prose-p:my-1 prose-headings:my-2',
                      'prose-ul:my-1 prose-ol:my-1',
                      'prose-pre:overflow-x-auto prose-pre:text-xs',
                      'prose-code:text-xs',
                    )}>
                      <ReactMarkdown remarkPlugins={[remarkGfm]}>
                        {normalizeEscapedText(displayContent, { preserveMarkdownCode: true, softBreak: true })}
                      </ReactMarkdown>
                    </div>
                  )}

                  {/* UI Blocks - confirm button to end meeting */}
                  {!isCompleted && msg.ui_blocks && msg.ui_blocks.length > 0 && onEndMeeting && (
                    <div className="flex flex-col gap-2 mt-3 pt-3 border-t border-border/50">
                      {msg.ui_blocks.map(block => {
                        if (block.type === 'confirm') {
                          return (
                            <div key={block.id}>
                              {block.label && (
                                <p className="text-xs font-medium text-muted-foreground mb-1.5">{block.label}</p>
                              )}
                              <div className="flex gap-2">
                                <Button
                                  size="sm"
                                  variant="default"
                                  className="h-8 text-xs"
                                  onClick={onEndMeeting}
                                >
                                  <Check className="size-3 mr-1" />
                                  {block.confirm_label ?? '确认结束'}
                                </Button>
                                {block.cancel_label && (
                                  <Button size="sm" variant="outline" className="h-8 text-xs" onClick={onContinueDiscussion}>
                                    <Square className="size-3 mr-1" />
                                    {block.cancel_label}
                                  </Button>
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

                {/* Timestamp outside bubble */}
                <span
                  className="text-[10px] text-muted-foreground/60 mt-0.5 px-1"
                  title={formatFullTime(msg.created_at)}
                >
                  {formatRelativeTime(msg.created_at)}
                </span>
              </div>

              {/* Spacer for user avatar (empty to keep layout) */}
              {isUser && <div className="shrink-0 w-7" />}
            </div>
          )
        })}
        <div ref={bottomRef} />
      </div>
    </ScrollArea>
  )
}
