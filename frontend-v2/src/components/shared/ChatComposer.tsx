import { useEffect, useRef, useState } from 'react'
import { App, Button } from 'antd'
import { SendOutlined, PaperClipOutlined } from '@ant-design/icons'
import { useChatAttachments } from '@/hooks/useChatAttachments'
import { PendingAttachmentList } from '@/components/shared/PendingAttachmentList'
import type { ChatAttachment } from '@/types'

interface ChatComposerProps {
  disabled?: boolean
  /** 发送请求进行中（禁用发送） */
  pending?: boolean
  placeholder?: string
  /** 返回 true 表示发送成功，输入与附件清空；返回 false 保留内容以便重试 */
  onSubmit: (content: string, attachments: ChatAttachment[]) => Promise<boolean> | boolean
}

/**
 * 通用附件 Composer：支持粘贴、拖拽、文件选择器三种方式采集本地文件字节，
 * 图片即时缩略预览。发送时先上传成 ChatAttachment 再连同正文提交给 onSubmit。
 * （浏览器只能读到文件字节，拿不到本地绝对路径；真实路径读取需桌面壳那一步。）
 */
export function ChatComposer({ disabled, pending, placeholder, onSubmit }: ChatComposerProps) {
  const { message } = App.useApp()
  const [content, setContent] = useState('')
  const { pendingFiles, uploading, addFiles, removeFile, clearPending, uploadPending } = useChatAttachments()
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const isComposingRef = useRef(false)

  useEffect(() => {
    const ta = textareaRef.current
    if (!ta) return
    ta.style.height = 'auto'
    ta.style.height = `${Math.min(ta.scrollHeight, 120)}px`
  }, [content])

  const handleSubmit = async () => {
    const trimmed = content.trim()
    if (disabled || pending || uploading) return
    if (!trimmed && pendingFiles.length === 0) return

    const { attachments, failedCount } = await uploadPending()

    if (failedCount > 0) {
      message.warning(`${failedCount} 个附件上传失败：${failedCount === pendingFiles.length ? '请重试' : '已忽略失败项，其余正常发送'}`)
    }
    if (!trimmed && attachments.length === 0) {
      // 只有附件且全部上传失败 → 保留内容与待发附件
      return
    }

    const okResult = await onSubmit(trimmed, attachments)
    if (okResult) {
      setContent('')
      clearPending()
      const ta = textareaRef.current
      if (ta) ta.style.height = 'auto'
    }
  }

  const canSend = !disabled && !pending && !uploading && (content.trim().length > 0 || pendingFiles.length > 0)

  return (
    <div>
      <PendingAttachmentList files={pendingFiles} onRemove={removeFile} />

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
          value={content}
          disabled={disabled}
          placeholder={placeholder ?? '输入消息，Enter 发送，Shift+Enter 换行…'}
          rows={1}
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
          onChange={(e) => setContent(e.target.value)}
          onPaste={(e) => {
            if (e.clipboardData?.files && e.clipboardData.files.length > 0) {
              addFiles(e.clipboardData.files)
            }
          }}
          onCompositionStart={() => { isComposingRef.current = true }}
          onCompositionEnd={() => { isComposingRef.current = false }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey && !isComposingRef.current) {
              e.preventDefault()
              void handleSubmit()
            }
          }}
        />
        <Button
          type="primary"
          shape="circle"
          icon={<SendOutlined />}
          disabled={!canSend}
          loading={pending || uploading}
          onClick={() => void handleSubmit()}
        />
      </div>
    </div>
  )
}