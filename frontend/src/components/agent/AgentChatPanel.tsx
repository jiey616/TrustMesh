import { useEffect, useMemo, useRef, useState } from 'react'
import { MessageSquareText, Plus } from 'lucide-react'
import { toast } from 'sonner'
import { Avatar } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { ChatBubbleContent } from '@/components/shared/ChatBubbleContent'
import { ChatAttachmentList } from '@/components/shared/ChatAttachmentList'
import { MessageInput } from '@/components/task-thread/MessageInput'
import { useAgentChat, useAgentChatSession, useAgentChatSessions, useResetAgentChat, useSendAgentChatMessage } from '@/hooks/useAgentChat'
import { cn, formatDateTime, formatRelativeTime } from '@/lib/utils'
import { ApiRequestError } from '@/api/client'
import { usePlatformStore } from '@/stores/platformStore'
import type { Agent, AgentChatSessionSummary, ChatAttachment } from '@/types'

interface AgentChatPanelProps {
  agent: Agent
}

const messageStatusText: Record<string, string> = {
  pending: '发送中',
  sent: '已发送',
  failed: '发送失败',
}

export function AgentChatPanel({ agent }: AgentChatPanelProps) {
  const { data: activeChat, isLoading: activeChatLoading } = useAgentChat(agent.id)
  const { data: sessions = [], isLoading: sessionsLoading } = useAgentChatSessions(agent.id)
  const sendMessage = useSendAgentChatMessage()
  const resetChat = useResetAgentChat()
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null)
  const [isDraftingNewSession, setIsDraftingNewSession] = useState(false)
  const platformName = usePlatformStore((s) => s.name)
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const effectiveSelectedSessionId = useMemo(() => {
    if (isDraftingNewSession) {
      return null
    }
    if (selectedSessionId) {
      if (activeChat?.id === selectedSessionId) {
        return selectedSessionId
      }
      if (sessions.some((session) => session.id === selectedSessionId)) {
        return selectedSessionId
      }
    }
    return sessions[0]?.id ?? null
  }, [activeChat?.id, isDraftingNewSession, selectedSessionId, sessions])

  const selectedSession = useMemo(() => {
    if (isDraftingNewSession) {
      return null
    }
    if (effectiveSelectedSessionId && activeChat?.id === effectiveSelectedSessionId) {
      const existing = sessions.find((session) => session.id === effectiveSelectedSessionId)
      if (existing) {
        return existing
      }
      return {
        id: activeChat.id,
        agent_id: activeChat.agent_id,
        session_key: activeChat.session_key,
        status: activeChat.status,
        message_count: activeChat.messages.length,
        last_message_preview: activeChat.messages[activeChat.messages.length - 1]?.content ?? '',
        last_message_at: activeChat.messages[activeChat.messages.length - 1]?.created_at ?? activeChat.updated_at,
        created_at: activeChat.created_at,
        updated_at: activeChat.updated_at,
      }
    }
    if (sessions.length === 0) {
      return null
    }
    return sessions.find((session) => session.id === effectiveSelectedSessionId) ?? sessions[0]
  }, [activeChat, effectiveSelectedSessionId, isDraftingNewSession, sessions])

  const selectedIsActive = !!selectedSession && selectedSession.status === 'active'
  const { data: historicalChat, isLoading: historicalChatLoading } = useAgentChatSession(
    agent.id,
    selectedSession?.id,
    !!selectedSession && !selectedIsActive
  )
  const chat = selectedIsActive ? activeChat : historicalChat
  const isLoading = sessionsLoading || (selectedIsActive ? activeChatLoading : historicalChatLoading)
  const messages = chat?.messages ?? []
  const messageList = chat?.messages
  const isDraft = isDraftingNewSession && !selectedSession
  const disabled = agent.archived || agent.status !== 'online' || (!selectedIsActive && !isDraft)

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messageList])

  const handleSend = async (content: string, attachments?: ChatAttachment[]) => {
    try {
      const res = await sendMessage.mutateAsync({ agentId: agent.id, content, attachments })
      setIsDraftingNewSession(false)
      setSelectedSessionId(res.data.id)
    } catch (err) {
      toast.error(err instanceof ApiRequestError ? err.message : '消息发送失败')
    }
  }

  const handleReset = async () => {
    try {
      await resetChat.mutateAsync(agent.id)
      setIsDraftingNewSession(true)
      setSelectedSessionId(null)
      toast.success('已开始新对话')
    } catch (err) {
      toast.error(err instanceof ApiRequestError ? err.message : '新建对话失败')
    }
  }

  return (
    <div className="grid h-full min-h-0 grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
      <div className="flex min-h-0 flex-col rounded-2xl bg-card">
        <ScrollArea className="flex-1 p-4">
          {isLoading ? (
            <div className="text-sm text-muted-foreground">加载中...</div>
          ) : isDraft ? (
            <div className="flex h-full min-h-64 flex-col items-center justify-center gap-3 text-center">
              <div className="flex size-12 items-center justify-center rounded-2xl bg-primary/10">
                <MessageSquareText className="size-6 text-primary" />
              </div>
              <div>
                <p className="text-sm font-medium">开始新的对话</p>
                <p className="mt-1 text-sm text-muted-foreground">
                  输入第一条消息后，系统会创建新的 session。
                </p>
              </div>
            </div>
          ) : !selectedSession ? (
            <div className="flex h-full min-h-64 flex-col items-center justify-center gap-3 text-center">
              <div className="flex size-12 items-center justify-center rounded-2xl bg-primary/10">
                <MessageSquareText className="size-6 text-primary" />
              </div>
              <div>
                <p className="text-sm font-medium">开始和 {agent.name} 对话</p>
                <p className="mt-1 text-sm text-muted-foreground">
                  对话会按 session 保存，可以在右侧查看历史记录。
                </p>
              </div>
            </div>
          ) : !chat || messages.length === 0 ? (
            <div className="flex h-full min-h-64 flex-col items-center justify-center gap-3 text-center">
              <div className="flex size-12 items-center justify-center rounded-2xl bg-primary/10">
                <MessageSquareText className="size-6 text-primary" />
              </div>
              <div>
                <p className="text-sm font-medium">{selectedIsActive ? `开始和 ${agent.name} 对话` : '该对话暂无消息'}</p>
                <p className="mt-1 text-sm text-muted-foreground">
                  {selectedIsActive ? `消息会保存在 ${platformName}，远端上下文由 Agent 自身维护。` : '这是历史记录中的空会话。'}
                </p>
              </div>
            </div>
          ) : (
            <div className="space-y-4">
              {messages.map((message) => {
                const isUser = message.sender_type === 'user'
                return (
                  <div key={message.id} className={`flex gap-3 ${isUser ? 'justify-end' : 'justify-start'}`}>
                    {!isUser && (
                      <Avatar
                        fallback={agent.name}
                        seed={agent.node_id}
                        kind="agent"
                        role={agent.role}
                        size="sm"
                      />
                    )}
                    <div className={`max-w-[80%] ${isUser ? 'items-end' : 'items-start'} flex flex-col gap-1`}>
                      <div
                        className={[
                          'rounded-2xl px-4 py-3 text-sm leading-relaxed shadow-xs',
                          isUser ? 'bg-primary text-primary-foreground' : 'border bg-background',
                        ].join(' ')}
                      >
                        <ChatBubbleContent content={message.content} markdown={!isUser} />
                        {message.attachments && message.attachments.length > 0 && (
                          <ChatAttachmentList attachments={message.attachments} compact />
                        )}
                      </div>
                      <div className="text-xs text-muted-foreground" title={formatDateTime(message.created_at)}>
                        {formatRelativeTime(message.created_at)}
                        {isUser ? ` · ${messageStatusText[message.status] ?? message.status}` : ''}
                      </div>
                    </div>
                    {isUser && <Avatar fallback="我" size="sm" />}
                  </div>
                )
              })}
              <div ref={messagesEndRef} />
            </div>
          )}
        </ScrollArea>

        <div className="px-4 py-3">
          {disabled && (
            <div className="mb-3 text-sm text-muted-foreground">
              {agent.archived
                ? '该 Agent 已离职，无法继续发送消息。'
                : agent.status !== 'online'
                  ? 'Agent 离线，暂时无法发送消息。'
                  : '当前查看的是历史对话，请切换到进行中的对话继续发送。'}
            </div>
          )}
          <MessageInput
            onSend={handleSend}
            disabled={disabled || sendMessage.isPending}
            placeholder={disabled ? '当前不可发送消息' : '输入要发给 Agent 的消息...'}
          />
        </div>
      </div>

      <div className="flex min-h-0 flex-col rounded-2xl bg-card">
        <div className="flex items-center justify-between gap-3 px-4 py-3">
          <div className="text-sm font-medium">对话历史</div>
          <Button size="sm" variant="outline" onClick={handleReset} disabled={resetChat.isPending}>
            <Plus className="mr-1.5 size-4" />
            新对话
          </Button>
        </div>
        <ScrollArea className="flex-1 p-2">
          {sessionsLoading ? (
            <div className="px-2 py-3 text-sm text-muted-foreground">加载中...</div>
          ) : sessions.length === 0 ? (
            <div className="px-2 py-3 text-sm text-muted-foreground">暂无历史对话</div>
          ) : (
            <div className="space-y-2">
              {sessions.map((session) => (
                <button
                  key={session.id}
                  type="button"
                  onClick={() => {
                    setIsDraftingNewSession(false)
                    setSelectedSessionId(session.id)
                  }}
                  className={cn(
                    'w-full rounded-xl px-3 py-3 text-left transition-colors hover:bg-accent/40',
                    selectedSession?.id === session.id ? 'bg-primary/5' : ''
                  )}
                >
                  <SessionListItem session={session} selected={selectedSession?.id === session.id} />
                </button>
              ))}
            </div>
          )}
        </ScrollArea>
      </div>
    </div>
  )
}

function SessionListItem({ session, selected }: { session: AgentChatSessionSummary; selected: boolean }) {
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <span className={cn('text-sm font-medium', selected && 'text-foreground')}>
            {session.status === 'active' ? '当前对话' : '历史对话'}
          </span>
          <span className="text-[11px] text-muted-foreground">
            {session.message_count} 条消息
          </span>
          {session.status === 'active' && (
            <Badge variant="secondary" className="text-[10px]">
              进行中
            </Badge>
          )}
        </div>
        <span className="shrink-0 text-[11px] text-muted-foreground">
          {formatRelativeTime(session.last_message_at ?? session.updated_at)}
        </span>
      </div>
      <p className="line-clamp-2 text-sm text-muted-foreground">
        {session.last_message_preview || '暂无消息'}
      </p>
    </div>
  )
}
