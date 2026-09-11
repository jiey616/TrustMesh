import { useEffect, useMemo, useRef, useState } from 'react'
import { Button, Tag, App, Avatar } from 'antd'
import {
  PlusOutlined,
  UserOutlined,
  MessageOutlined,
} from '@ant-design/icons'
import dayjs from 'dayjs'
import { stripReplyPrefix } from '@/lib/text'
import { useAgentChat,
  useAgentChatSession,
  useAgentChatSessions,
  useResetAgentChat,
  useSendAgentChatMessage,
} from '@/hooks/useAgentChat'
import { AgentAvatar } from '@/components/shared/AgentAvatar'
import { ChatComposer } from '@/components/shared/ChatComposer'
import { ChatAttachmentList } from '@/components/shared/ChatAttachmentList'
import type { Agent, AgentChatSessionSummary, ChatAttachment } from '@/types'

interface Props {
  agent: Agent
}

const messageStatusText: Record<string, string> = {
  pending: '发送中',
  sent: '已发送',
  failed: '发送失败',
}

export function AgentChatPanel({ agent }: Props) {
  const { message } = App.useApp()
  const { data: activeChat, isLoading: activeChatLoading } = useAgentChat(agent.id)
  const { data: sessions = [], isLoading: sessionsLoading } = useAgentChatSessions(agent.id)
  const sendMessage = useSendAgentChatMessage()
  const resetChat = useResetAgentChat()
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null)
  const [isDraftingNewSession, setIsDraftingNewSession] = useState(false)
  const messagesEndRef = useRef<HTMLDivElement>(null)

  const effectiveSelectedSessionId = useMemo(() => {
    if (isDraftingNewSession) return null
    if (selectedSessionId) {
      if (activeChat?.id === selectedSessionId) return selectedSessionId
      if (sessions.some((s) => s.id === selectedSessionId)) return selectedSessionId
    }
    return sessions[0]?.id ?? null
  }, [activeChat?.id, isDraftingNewSession, selectedSessionId, sessions])

  const selectedSession = useMemo<AgentChatSessionSummary | null>(() => {
    if (isDraftingNewSession) return null
    if (effectiveSelectedSessionId && activeChat?.id === effectiveSelectedSessionId) {
      const existing = sessions.find((s) => s.id === effectiveSelectedSessionId)
      if (existing) return existing
      return {
        id: activeChat.id,
        agent_id: activeChat.agent_id,
        session_key: activeChat.session_key,
        status: activeChat.status,
        message_count: activeChat.messages.length,
        last_message_preview: activeChat.messages[activeChat.messages.length - 1]?.content ?? '',
        last_message_at:
          activeChat.messages[activeChat.messages.length - 1]?.created_at ?? activeChat.updated_at,
        created_at: activeChat.created_at,
        updated_at: activeChat.updated_at,
      }
    }
    if (sessions.length === 0) return null
    return sessions.find((s) => s.id === effectiveSelectedSessionId) ?? sessions[0]
  }, [activeChat, effectiveSelectedSessionId, isDraftingNewSession, sessions])

  const selectedIsActive = !!selectedSession && selectedSession.status === 'active'
  const { data: historicalChat, isLoading: historicalChatLoading } = useAgentChatSession(
    agent.id,
    selectedSession?.id,
    !!selectedSession && !selectedIsActive,
  )
  const chat = selectedIsActive ? activeChat : historicalChat
  const isLoading = sessionsLoading || (selectedIsActive ? activeChatLoading : historicalChatLoading)
  const messages = chat?.messages ?? []
  const isDraft = isDraftingNewSession && !selectedSession
  const disabled = agent.archived || agent.status !== 'online' || (!selectedIsActive && !isDraft)

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages.length])

  const handleSend = async (content: string, attachments: ChatAttachment[]): Promise<boolean> => {
    try {
      const res = await sendMessage.mutateAsync({ agentId: agent.id, content, attachments })
      setIsDraftingNewSession(false)
      setSelectedSessionId(res.data.id)
      return true
    } catch {
      message.error('消息发送失败')
      return false
    }
  }

  const handleReset = async () => {
    try {
      await resetChat.mutateAsync(agent.id)
      setIsDraftingNewSession(true)
      setSelectedSessionId(null)
      message.success('已开始新对话')
    } catch {
      message.error('新建对话失败')
    }
  }

  const renderEmpty = (title: string, desc: string) => (
    <div className="flex h-full min-h-[16rem] flex-col items-center justify-center gap-3 text-center">
      <div className="flex size-12 items-center justify-center rounded-xl bg-[color:var(--surface)]">
        <MessageOutlined className="text-2xl text-[color:var(--signal)]" />
      </div>
      <div>
        <p className="text-sm font-medium text-[color:var(--text-primary)]">{title}</p>
        <p className="mt-1 text-sm text-[color:var(--text-tertiary)]">{desc}</p>
      </div>
    </div>
  )

  return (
    <div className="grid h-full min-h-0 grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
      {/* 对话主区域 */}
      <div className="flex min-h-0 flex-col rounded-xl border border-[color:var(--line)] bg-[color:var(--surface)]">
        <div className="flex-1 overflow-y-auto p-4">
          {isLoading ? (
            <div className="text-sm text-[color:var(--text-tertiary)]">加载中...</div>
          ) : isDraft ? (
            renderEmpty('开始新的对话', '输入第一条消息后，系统会创建新的 session。')
          ) : !selectedSession ? (
            renderEmpty(`开始和 ${agent.name} 对话`, '对话会按 session 保存，可以在右侧查看历史记录。')
          ) : !chat || messages.length === 0 ? (
            renderEmpty(
              selectedIsActive ? `开始和 ${agent.name} 对话` : '该对话暂无消息',
              selectedIsActive
                ? '消息会保存在平台，远端上下文由数字员工自身维护。'
                : '这是历史记录中的空会话。',
            )
          ) : (
            <div className="space-y-4">
              {messages.map((m) => {
                const isUser = m.sender_type === 'user'
                return (
                  <div key={m.id} className={`flex gap-3 ${isUser ? 'justify-end' : 'justify-start'}`}>
                    {!isUser && (
                      <AgentAvatar
                        name={agent.name}
                        role={agent.role}
                        seed={agent.id}
                        size={32}
                        status={agent.status}
                      />
                    )}
                    <div className={`flex max-w-[80%] flex-col gap-1 ${isUser ? 'items-end' : 'items-start'}`}>
                      <div
                        className={[
                          'whitespace-pre-wrap break-words rounded-xl px-4 py-3 text-sm leading-relaxed shadow-sm',
                          isUser
                            ? 'border border-[#6d5ff5]/30 bg-[linear-gradient(135deg,rgba(109,95,245,0.25),rgba(99,102,241,0.2))] text-[color:var(--text-primary)]'
                            : 'border border-[color:var(--line)] bg-[color:var(--surface)] text-[color:var(--text-primary)]',
                        ].join(' ')}
                      >
                        {stripReplyPrefix(m.content)}
                      </div>
                      {!!m.attachments?.length && (
                        <ChatAttachmentList attachments={m.attachments} compact align={isUser ? 'end' : 'start'} />
                      )}
                      <div className="text-xs text-[color:var(--text-quaternary)]" title={dayjs(m.created_at).format('YYYY-MM-DD HH:mm')}>
                        {dayjs(m.created_at).fromNow()}
                        {isUser ? ` · ${messageStatusText[m.status] ?? m.status}` : ''}
                      </div>
                    </div>
                    {isUser && <Avatar size={32} icon={<UserOutlined />} style={{ background: 'var(--surface-raised)', flexShrink: 0 }} />}
                  </div>
                )
              })}
              <div ref={messagesEndRef} />
            </div>
          )}
        </div>

        <div className="border-t border-[color:var(--line)] p-3">
          {disabled && (
            <div className="mb-2 text-sm text-[color:var(--text-tertiary)]">
              {agent.archived
                ? '该数字员工已离职，无法继续发送消息。'
                : agent.status !== 'online'
                  ? '数字员工离线，暂时无法发送消息。'
                  : '当前查看的是历史对话，请切换到进行中的对话继续发送。'}
            </div>
          )}
          {/* 附件 Composer：粘贴 / 拖拽 / 选择器采集图片或文件 */}
          <ChatComposer
            disabled={disabled}
            pending={sendMessage.isPending}
            placeholder={disabled ? '当前不可发送消息' : '输入要发给数字员工的消息，可直接粘贴或选择图片/文件… (Enter 发送)'}
            onSubmit={handleSend}
          />
        </div>
      </div>

      {/* 历史会话 */}
      <div className="flex min-h-0 flex-col rounded-xl border border-[color:var(--line)] bg-[color:var(--surface)]">
        <div className="flex items-center justify-between gap-3 border-b border-[color:var(--line)] px-4 py-3">
          <div className="text-sm font-medium text-[color:var(--text-primary)]">对话历史</div>
          <Button size="small" icon={<PlusOutlined />} onClick={handleReset} loading={resetChat.isPending}>
            新对话
          </Button>
        </div>
        <div className="flex-1 overflow-y-auto p-2">
          {sessionsLoading ? (
            <div className="px-2 py-3 text-sm text-[color:var(--text-tertiary)]">加载中...</div>
          ) : sessions.length === 0 ? (
            <div className="px-2 py-3 text-sm text-[color:var(--text-tertiary)]">暂无历史对话</div>
          ) : (
            <div className="space-y-2">
              {sessions.map((s) => {
                const selected = selectedSession?.id === s.id
                return (
                  <button
                    key={s.id}
                    type="button"
                    onClick={() => {
                      setIsDraftingNewSession(false)
                      setSelectedSessionId(s.id)
                    }}
                    className={`w-full rounded-xl px-3 py-3 text-left transition-colors hover:bg-[color:var(--surface-sunken)] ${
                      selected ? 'bg-[color:var(--surface-raised)]' : ''
                    }`}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <div className="flex items-center gap-2">
                        <span className={`text-sm font-medium text-[color:var(--text-primary)] ${selected ? '' : 'opacity-80'}`}>
                          {s.status === 'active' ? '当前对话' : '历史对话'}
                        </span>
                        <span className="text-[11px] text-[color:var(--text-quaternary)]">{s.message_count} 条消息</span>
                        {s.status === 'active' && <Tag color="green" className="!text-[10px]">进行中</Tag>}
                      </div>
                      <span className="shrink-0 text-[11px] text-[color:var(--text-quaternary)]">
                        {dayjs(s.last_message_at ?? s.updated_at).fromNow()}
                      </span>
                    </div>
                    <p className="mt-1 line-clamp-2 text-sm text-[color:var(--text-tertiary)]">{s.last_message_preview || '暂无消息'}</p>
                  </button>
                )
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
