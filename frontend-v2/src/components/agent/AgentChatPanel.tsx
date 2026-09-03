import { useEffect, useMemo, useRef, useState } from 'react'
import { Button, Tag, App, Avatar } from 'antd'
import {
  SendOutlined,
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
import type { Agent, AgentChatSessionSummary } from '@/types'

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
  const [draft, setDraft] = useState('')
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  // 输入框自动高度（与任务 Composer 一致）
  useEffect(() => {
    const ta = textareaRef.current
    if (!ta) return
    ta.style.height = 'auto'
    ta.style.height = `${Math.min(ta.scrollHeight, 120)}px`
  }, [draft])

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

  const handleSend = async () => {
    const content = draft.trim()
    if (!content) return
    setDraft('')
    try {
      const res = await sendMessage.mutateAsync({ agentId: agent.id, content })
      setIsDraftingNewSession(false)
      setSelectedSessionId(res.data.id)
    } catch {
      message.error('消息发送失败')
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
      <div className="flex size-12 items-center justify-center rounded-xl bg-white/5">
        <MessageOutlined className="text-2xl text-[color:var(--signal)]" />
      </div>
      <div>
        <p className="text-sm font-medium text-white/90">{title}</p>
        <p className="mt-1 text-sm text-white/50">{desc}</p>
      </div>
    </div>
  )

  return (
    <div className="grid h-full min-h-0 grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
      {/* 对话主区域 */}
      <div className="flex min-h-0 flex-col rounded-xl border border-white/[0.08] bg-white/[0.03]">
        <div className="flex-1 overflow-y-auto p-4">
          {isLoading ? (
            <div className="text-sm text-white/50">加载中...</div>
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
                            ? 'border border-[#6d5ff5]/30 bg-[linear-gradient(135deg,rgba(109,95,245,0.25),rgba(99,102,241,0.2))] text-white/90'
                            : 'border border-white/[0.08] bg-white/[0.04] text-white/90',
                        ].join(' ')}
                      >
                        {stripReplyPrefix(m.content)}
                      </div>
                      <div className="text-xs text-white/40" title={dayjs(m.created_at).format('YYYY-MM-DD HH:mm')}>
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

        <div className="border-t border-white/[0.08] p-3">
          {disabled && (
            <div className="mb-2 text-sm text-white/50">
              {agent.archived
                ? '该数字员工已离职，无法继续发送消息。'
                : agent.status !== 'online'
                  ? '数字员工离线，暂时无法发送消息。'
                  : '当前查看的是历史对话，请切换到进行中的对话继续发送。'}
            </div>
          )}
          {/* 与任务 Composer 一致的输入框 */}
          <div
            style={{
              display: 'flex',
              alignItems: 'flex-end',
              gap: 8,
              borderRadius: 'var(--radius-control)',
              border: '1px solid var(--line-strong)',
              background: 'var(--surface-inset)',
              padding: '8px 8px 8px 12px',
              transition: 'border-color 0.2s',
            }}
          >
            <textarea
              ref={textareaRef}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              disabled={disabled || sendMessage.isPending}
              placeholder={disabled ? '当前不可发送消息' : '输入要发给数字员工的消息… (Enter 发送，Shift+Enter 换行)'}
              rows={1}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault()
                  void handleSend()
                }
              }}
              style={{
                flex: 1,
                minHeight: 36,
                maxHeight: 120,
                resize: 'none',
                background: 'transparent',
                border: 'none',
                outline: 'none',
                color: 'var(--text-primary)',
                fontSize: 13,
                lineHeight: 1.6,
                padding: '4px 0',
                fontFamily: 'inherit',
              }}
            />
            <Button
              type="primary"
              shape="circle"
              icon={<SendOutlined />}
              disabled={disabled || !draft.trim()}
              loading={sendMessage.isPending}
              onClick={handleSend}
            />
          </div>
        </div>
      </div>

      {/* 历史会话 */}
      <div className="flex min-h-0 flex-col rounded-xl border border-white/[0.08] bg-white/[0.03]">
        <div className="flex items-center justify-between gap-3 border-b border-white/[0.08] px-4 py-3">
          <div className="text-sm font-medium text-white/90">对话历史</div>
          <Button size="small" icon={<PlusOutlined />} onClick={handleReset} loading={resetChat.isPending}>
            新对话
          </Button>
        </div>
        <div className="flex-1 overflow-y-auto p-2">
          {sessionsLoading ? (
            <div className="px-2 py-3 text-sm text-white/50">加载中...</div>
          ) : sessions.length === 0 ? (
            <div className="px-2 py-3 text-sm text-white/50">暂无历史对话</div>
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
                    className={`w-full rounded-xl px-3 py-3 text-left transition-colors hover:bg-white/5 ${
                      selected ? 'bg-white/[0.06]' : ''
                    }`}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <div className="flex items-center gap-2">
                        <span className={`text-sm font-medium text-white/90 ${selected ? '' : 'opacity-80'}`}>
                          {s.status === 'active' ? '当前对话' : '历史对话'}
                        </span>
                        <span className="text-[11px] text-white/40">{s.message_count} 条消息</span>
                        {s.status === 'active' && <Tag color="green" className="!text-[10px]">进行中</Tag>}
                      </div>
                      <span className="shrink-0 text-[11px] text-white/40">
                        {dayjs(s.last_message_at ?? s.updated_at).fromNow()}
                      </span>
                    </div>
                    <p className="mt-1 line-clamp-2 text-sm text-white/50">{s.last_message_preview || '暂无消息'}</p>
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
