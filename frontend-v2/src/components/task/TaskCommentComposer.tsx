import { useEffect, useRef, useState } from 'react'
import { App, Button } from 'antd'
import { SendOutlined, PaperClipOutlined } from '@ant-design/icons'
import { useChatAttachments } from '@/hooks/useChatAttachments'
import { PendingAttachmentList } from '@/components/shared/PendingAttachmentList'
import type { ChatAttachment } from '@/types'

export interface TaskMentionCandidate {
  id: string
  name: string
  roleLabel: string
}

export interface TaskCommentSubmitInput {
  content: string
  mentionAgentIds: string[]
  attachments?: ChatAttachment[]
}

interface MentionSearch {
  start: number
  end: number
  query: string
}

interface TaskCommentComposerProps {
  candidates: TaskMentionCandidate[]
  disabled?: boolean
  onSubmit: (input: TaskCommentSubmitInput) => Promise<boolean> | boolean
  placeholder?: string
  /** 受控值（可选）：用于外部实时预览输入内容 */
  value?: string
  onValueChange?: (value: string) => void
  /** 输入框左侧的附加控件（如「+」附件按钮），与右侧发送按钮同处一个圆角输入框内 */
  leadingAccessory?: React.ReactNode
}

function getMentionSearch(value: string, cursor: number): MentionSearch | null {
  const prefix = value.slice(0, cursor)
  const match = /(?:^|\s)@([^\s@]*)$/.exec(prefix)
  if (!match) return null
  return {
    start: cursor - match[1].length - 1,
    end: cursor,
    query: match[1],
  }
}

/** 评论输入框，支持 @ 提及任务参与数字员工与本地附件（粘贴/拖拽/选择器） */
export function TaskCommentComposer({
  candidates,
  disabled,
  onSubmit,
  placeholder = '输入评论... (输入 @ 提及数字员工，Enter 发送，Shift+Enter 换行)',
  value: controlledValue,
  onValueChange,
  leadingAccessory,
}: TaskCommentComposerProps) {
  const { message } = App.useApp()
  const [value, setValue] = useState('')
  const effectiveValue = controlledValue ?? value
  const isComposingRef = useRef(false)
  const [mentionSearch, setMentionSearch] = useState<MentionSearch | null>(null)
  const [activeIndex, setActiveIndex] = useState(0)
  const [selectedMentions, setSelectedMentions] = useState<Record<string, string>>({})
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const { pendingFiles, uploading, addFiles, removeFile, clearPending, uploadPending } = useChatAttachments()

  const setBoth = (nextValue: string) => {
    setValue(nextValue)
    onValueChange?.(nextValue)
  }

  const filteredCandidates = mentionSearch
    ? candidates
      .filter((candidate) => {
        const query = mentionSearch.query.trim().toLowerCase()
        if (!query) return true
        return candidate.name.toLowerCase().includes(query) || candidate.roleLabel.toLowerCase().includes(query)
      })
      .slice(0, 6)
    : []

  useEffect(() => {
    const textarea = textareaRef.current
    if (!textarea) return
    textarea.style.height = 'auto'
    textarea.style.height = `${Math.min(textarea.scrollHeight, 120)}px`
  }, [effectiveValue])

  const updateMentionSearch = (nextValue: string, cursor: number | null | undefined) => {
    if (cursor == null) {
      setMentionSearch(null)
      return
    }
    setMentionSearch(getMentionSearch(nextValue, cursor))
  }

  const handleSelectCandidate = (candidate: TaskMentionCandidate) => {
    if (!mentionSearch) return
    const nextValue = `${effectiveValue.slice(0, mentionSearch.start)}@${candidate.name} ${effectiveValue.slice(mentionSearch.end)}`
    const nextCursor = mentionSearch.start + candidate.name.length + 2
    setBoth(nextValue)
    setSelectedMentions((current) => ({ ...current, [candidate.id]: candidate.name }))
    setMentionSearch(null)
    setActiveIndex(0)
    requestAnimationFrame(() => {
      const textarea = textareaRef.current
      if (!textarea) return
      textarea.focus()
      textarea.setSelectionRange(nextCursor, nextCursor)
    })
  }

  const handleSubmit = async () => {
    const trimmed = effectiveValue.trim()
    if ((!trimmed && pendingFiles.length === 0) || disabled || uploading) return

    const { attachments, failedCount } = await uploadPending()
    if (failedCount > 0) {
      message.warning(`${failedCount} 个附件上传失败：${failedCount === pendingFiles.length ? '请重试' : '已忽略失败项，其余正常发送'}`)
    }
    if (!trimmed && attachments.length === 0) {
      // 只有附件且全部上传失败 → 保留内容与待发附件
      return
    }

    const mentionAgentIds = Object.entries(selectedMentions)
      .filter(([, name]) => trimmed.includes(`@${name}`))
      .map(([agentId]) => agentId)
    const ok = await onSubmit({ content: trimmed, mentionAgentIds, attachments })
    if (!ok) return
    setBoth('')
    setSelectedMentions({})
    setMentionSearch(null)
    setActiveIndex(0)
    clearPending()
    if (textareaRef.current) textareaRef.current.style.height = 'auto'
  }

  const handleKeyDown = async (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    const safeActiveIndex = filteredCandidates.length === 0 ? 0 : Math.min(activeIndex, filteredCandidates.length - 1)

    if (mentionSearch && filteredCandidates.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setActiveIndex((current) => (current + 1) % filteredCandidates.length)
        return
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        setActiveIndex((current) => (current - 1 + filteredCandidates.length) % filteredCandidates.length)
        return
      }
      if ((e.key === 'Enter' || e.key === 'Tab') && !e.shiftKey && !isComposingRef.current) {
        e.preventDefault()
        handleSelectCandidate(filteredCandidates[safeActiveIndex] ?? filteredCandidates[0])
        return
      }
    }

    if (e.key === 'Escape' && mentionSearch) {
      e.preventDefault()
      setMentionSearch(null)
      return
    }

    if (e.key === 'Enter' && !e.shiftKey && !isComposingRef.current) {
      e.preventDefault()
      await handleSubmit()
    }
  }

  return (
    <div style={{ position: 'relative' }}>
      {mentionSearch && (
        <div
          style={{
            position: 'absolute',
            inset: 'auto 0 auto 0',
            bottom: '100%',
            marginBottom: 8,
            overflow: 'hidden',
            borderRadius: 'var(--radius-control)',
            border: '1px solid var(--line-strong)',
            background: 'var(--canvas-elevated)',
            boxShadow: '0 8px 24px rgba(0,0,0,0.4)',
            zIndex: 10,
          }}
        >
          <div style={{ borderBottom: '1px solid var(--line)', padding: '6px 12px', fontSize: 12, color: 'var(--text-tertiary)' }}>
            选择要提及的任务参与 Agent
          </div>
          {filteredCandidates.length > 0 ? (
            <div style={{ maxHeight: 224, overflowY: 'auto', padding: 6 }}>
              {filteredCandidates.map((candidate, index) => (
                <button
                  key={candidate.id}
                  type="button"
                  onMouseDown={(event) => {
                    event.preventDefault()
                    handleSelectCandidate(candidate)
                  }}
                  style={{
                    display: 'flex',
                    width: '100%',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    borderRadius: 'var(--radius-control)',
                    padding: '8px 10px',
                    textAlign: 'left',
                    background: index === Math.min(activeIndex, filteredCandidates.length - 1) ? 'rgba(109,95,245,0.15)' : 'transparent',
                    color: 'var(--text-primary)',
                    border: 'none',
                    cursor: 'pointer',
                    fontFamily: 'inherit',
                  }}
                >
                  <span>
                    <span style={{ display: 'block', fontSize: 13, fontWeight: 500 }}>@{candidate.name}</span>
                    <span style={{ display: 'block', fontSize: 11, color: 'var(--text-quaternary)' }}>{candidate.roleLabel}</span>
                  </span>
                  <span style={{ color: 'var(--text-quaternary)', fontSize: 12 }}>@</span>
                </button>
              ))}
            </div>
          ) : (
            <div style={{ padding: '12px', fontSize: 13, color: 'var(--text-quaternary)' }}>没有匹配的数字员工</div>
          )}
        </div>
      )}

      <PendingAttachmentList files={pendingFiles} onRemove={removeFile} />

      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          borderRadius: 'var(--radius-control)',
          border: '1px solid var(--line-strong)',
          background: 'var(--surface-inset)',
          padding: '8px 8px 8px 12px',
          transition: 'border-color 0.2s',
        }}
        onDragOver={(e) => { e.preventDefault() }}
        onDrop={(e) => {
          e.preventDefault()
          if (e.dataTransfer?.files) addFiles(e.dataTransfer.files)
        }}
      >
        <input
          ref={fileInputRef}
          type="file"
          multiple
          hidden
          onChange={(e) => {
            addFiles(e.target.files)
            e.target.value = ''
          }}
        />
        {leadingAccessory}
        <Button
          shape="circle"
          size="small"
          icon={<PaperClipOutlined />}
          title="添加本地文件 / 图片（也可直接粘贴或拖拽）"
          disabled={disabled}
          onClick={() => fileInputRef.current?.click()}
        />
        <textarea
          ref={textareaRef}
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
          placeholder={placeholder}
          rows={1}
          value={effectiveValue}
          disabled={disabled}
          onChange={(event) => {
            setBoth(event.target.value)
            updateMentionSearch(event.target.value, event.target.selectionStart)
          }}
          onClick={(event) => updateMentionSearch(event.currentTarget.value, event.currentTarget.selectionStart)}
          onKeyUp={(event) => updateMentionSearch(event.currentTarget.value, event.currentTarget.selectionStart)}
          onKeyDown={handleKeyDown}
          onCompositionStart={() => { isComposingRef.current = true }}
          onCompositionEnd={(event) => {
            isComposingRef.current = false
            updateMentionSearch(event.currentTarget.value, event.currentTarget.selectionStart)
          }}
          onPaste={(e) => {
            if (e.clipboardData?.files && e.clipboardData.files.length > 0) {
              addFiles(e.clipboardData.files)
            }
          }}
        />
        <Button
          type="primary"
          shape="circle"
          icon={<SendOutlined />}
          disabled={disabled || uploading || (!effectiveValue.trim() && pendingFiles.length === 0)}
          loading={uploading}
          onClick={() => void handleSubmit()}
        />
      </div>
    </div>
  )
}
