import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Input } from 'antd'
import {
  ArrowRightOutlined,
  CloseOutlined,
  DeleteOutlined,
  LoadingOutlined,
  SendOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'
import { useAssistant } from '@/hooks/useAssistant'
import { useAssistantStore } from '@/stores/assistantStore'
import type { AssistantMessage } from '@/types/assistant'

/**
 * AI 助手悬浮入口（Ctrl+K 唤起）。V2 风格：紫 CTA + 内联样式 + CSS 变量。
 * 移植自旧前端 AssistantFab/Panel，SSE 流式 + 工具调用时间线。
 */
export function AssistantFab() {
  const isOpen = useAssistantStore((s) => s.isOpen)
  const toggle = useAssistantStore((s) => s.toggle)

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        toggle()
      }
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [toggle])

  return (
    <div
      style={{
        position: 'fixed',
        right: 24,
        bottom: 24,
        zIndex: 1000,
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'flex-end',
        gap: 12,
      }}
    >
      {isOpen && <AssistantPanel />}
      <Button
        type="primary"
        shape="circle"
        onClick={toggle}
        title="AI 助手（Ctrl+K）"
        icon={<ThunderboltOutlined style={{ fontSize: 20 }} />}
        style={{
          width: 48,
          height: 48,
          background: 'linear-gradient(135deg, #6d5ff5 0%, #8b5cf6 100%)',
          boxShadow: '0 8px 24px rgba(109, 95, 245, 0.4)',
        }}
        aria-label="AI 助手"
      />
    </div>
  )
}

function AssistantPanel() {
  const navigate = useNavigate()
  const { messages, isProcessing, close, sendMessage, cancel, clearMessages, getToolLabel } =
    useAssistant()
  const listRef = useRef<HTMLDivElement>(null)
  const [draft, setDraft] = useState('')

  // 新消息自动滚底
  useEffect(() => {
    listRef.current?.scrollTo({ top: listRef.current.scrollHeight, behavior: 'smooth' })
  }, [messages])

  const send = () => {
    const content = draft.trim()
    if (!content || isProcessing) return
    setDraft('')
    sendMessage(content)
  }

  const last = messages[messages.length - 1]
  const thinking = isProcessing && last?.role === 'assistant' && !last?.content

  return (
    <div
      style={{
        width: 'min(460px, calc(100vw - 48px))',
        height: 'min(640px, calc(100vh - 120px))',
        display: 'flex',
        flexDirection: 'column',
        borderRadius: 16,
        border: '1px solid var(--line)',
        background: 'var(--surface)',
        boxShadow: '0 24px 64px rgba(0, 0, 0, 0.5)',
        overflow: 'hidden',
      }}
    >
      {/* Header */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '10px 14px',
          borderBottom: '1px solid var(--line)',
          background: 'rgba(255, 255, 255, 0.03)',
        }}
      >
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8, fontSize: 13, fontWeight: 600 }}>
          <ThunderboltOutlined style={{ color: '#6d5ff5' }} />
          AI 助手
          <span style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>Ctrl+K</span>
        </span>
        <span style={{ display: 'inline-flex', gap: 4 }}>
          <Button
            type="text"
            size="small"
            icon={<DeleteOutlined style={{ fontSize: 13 }} />}
            onClick={clearMessages}
            title="清空对话"
            style={{ color: 'var(--text-tertiary)' }}
          />
          <Button
            type="text"
            size="small"
            icon={<CloseOutlined style={{ fontSize: 13 }} />}
            onClick={close}
            style={{ color: 'var(--text-tertiary)' }}
          />
        </span>
      </div>

      {/* Messages */}
      <div
        ref={listRef}
        style={{
          flex: 1,
          overflowY: 'auto',
          padding: 14,
          display: 'flex',
          flexDirection: 'column',
          gap: 12,
        }}
      >
        {messages.length === 0 && (
          <div
            style={{
              margin: 'auto 0',
              textAlign: 'center',
              color: 'var(--text-tertiary)',
              fontSize: 12,
            }}
          >
            <ThunderboltOutlined style={{ fontSize: 28, color: 'var(--text-quaternary)' }} />
            <div style={{ marginTop: 8 }}>问我任何关于项目、任务、知识库的问题</div>
            <div style={{ marginTop: 4, color: 'var(--text-quaternary)' }}>
              例如：「帮我看看最近的卡住任务」「统计一下当前进度」
            </div>
          </div>
        )}
        {messages.map((m) => (
          <MessageBubble key={m.id} msg={m} getToolLabel={getToolLabel} onNavigate={navigate} />
        ))}
        {thinking && (
          <div
            style={{
              color: 'var(--text-tertiary)',
              fontSize: 12,
              display: 'flex',
              alignItems: 'center',
              gap: 6,
            }}
          >
            <LoadingOutlined spin /> 思考中…
          </div>
        )}
      </div>

      {/* Input */}
      <div style={{ padding: 10, borderTop: '1px solid var(--line)' }}>
        <Input.TextArea
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onPressEnter={(e) => {
            if (!e.shiftKey) {
              e.preventDefault()
              send()
            }
          }}
          placeholder={isProcessing ? '助手回复中…' : '输入消息，Enter 发送（Shift+Enter 换行）'}
          autoSize={{ minRows: 1, maxRows: 4 }}
          disabled={isProcessing}
          style={{ background: 'rgba(255, 255, 255, 0.03)', borderColor: 'var(--line)', fontSize: 13 }}
        />
        <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 8, gap: 8 }}>
          {isProcessing && (
            <Button size="small" onClick={cancel} style={{ fontSize: 12 }}>
              停止
            </Button>
          )}
          <Button
            size="small"
            type="primary"
            icon={<SendOutlined />}
            disabled={isProcessing || !draft.trim()}
            onClick={send}
            style={{ fontSize: 12, background: '#6d5ff5' }}
          >
            发送
          </Button>
        </div>
      </div>
    </div>
  )
}

function MessageBubble({
  msg,
  getToolLabel,
  onNavigate,
}: {
  msg: AssistantMessage
  getToolLabel: (tool: string) => string
  onNavigate: (path: string) => void
}) {
  const isUser = msg.role === 'user'
  return (
    <div style={{ display: 'flex', justifyContent: isUser ? 'flex-end' : 'flex-start' }}>
      <div style={{ maxWidth: '86%', display: 'flex', flexDirection: 'column', gap: 6 }}>
        {/* 工具调用时间线（助手消息） */}
        {!isUser && (msg.toolCalls?.length ?? 0) > 0 && (
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
            {msg.toolCalls!.map((tc, i) => (
              <span
                key={`${tc.tool}_${i}`}
                style={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 4,
                  fontSize: 10,
                  padding: '2px 8px',
                  borderRadius: 999,
                  border: '1px solid var(--line)',
                  color: tc.status === 'running' ? 'var(--signal)' : 'var(--text-tertiary)',
                  background: 'rgba(255, 255, 255, 0.03)',
                }}
              >
                {tc.status === 'running' ? <LoadingOutlined spin /> : '✓'}
                {getToolLabel(tc.tool)}
              </span>
            ))}
          </div>
        )}

        <div
          style={{
            padding: '8px 12px',
            borderRadius: isUser ? '12px 12px 4px 12px' : '12px 12px 12px 4px',
            background: isUser
              ? 'linear-gradient(135deg, #6d5ff5 0%, #7c5cf0 100%)'
              : 'rgba(255, 255, 255, 0.05)',
            border: isUser ? 'none' : '1px solid var(--line)',
            color: isUser ? '#fff' : 'var(--text-primary)',
            fontSize: 13,
            lineHeight: 1.6,
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
          }}
        >
          {msg.content || (msg.isStreaming ? '…' : '')}
        </div>

        {/* 导航动作按钮 */}
        {msg.navigateAction && (
          <Button
            size="small"
            icon={<ArrowRightOutlined style={{ fontSize: 11 }} />}
            onClick={() => onNavigate(msg.navigateAction!.path)}
            style={{
              alignSelf: 'flex-start',
              fontSize: 11,
              color: 'var(--signal)',
              borderColor: 'var(--line)',
            }}
          >
            前往：{msg.navigateAction.label || msg.navigateAction.path}
          </Button>
        )}
      </div>
    </div>
  )
}
