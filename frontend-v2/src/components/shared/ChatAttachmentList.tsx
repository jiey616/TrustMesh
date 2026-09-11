import { FileTextOutlined } from '@ant-design/icons'
import type { ChatAttachment } from '@/types'

function formatFileSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return ''
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function isImage(att: ChatAttachment): boolean {
  return (att.mime_type || '').startsWith('image/') && !!att.url
}

interface ChatAttachmentListProps {
  attachments?: ChatAttachment[]
  compact?: boolean
  /** 用户消息靠右时让附件列表右对齐 */
  align?: 'start' | 'end'
}

/**
 * 已发送消息的附件渲染：图片出缩略图、其它文件出芯片。
 * att.url 是后端签名过的短时下载链接，点击新窗口打开。
 */
export function ChatAttachmentList({ attachments, compact, align = 'start' }: ChatAttachmentListProps) {
  if (!attachments || attachments.length === 0) return null
  return (
    <div
      style={{
        display: 'flex',
        flexWrap: 'wrap',
        gap: compact ? 6 : 8,
        justifyContent: align === 'end' ? 'flex-end' : 'flex-start',
        marginTop: 6,
      }}
    >
      {attachments.map((att) =>
        isImage(att) ? (
          <a
            key={att.id}
            href={att.url}
            target="_blank"
            rel="noreferrer"
            title={att.file_name}
            style={{
              display: 'block',
              overflow: 'hidden',
              borderRadius: 8,
              border: '1px solid var(--line)',
              background: 'var(--surface-raised)',
              lineHeight: 0,
            }}
          >
            <img
              src={att.url}
              alt={att.file_name}
              style={{
                width: compact ? 96 : 120,
                height: compact ? 72 : 90,
                objectFit: 'cover',
                display: 'block',
              }}
            />
          </a>
        ) : (
          <a
            key={att.id}
            href={att.url ?? undefined}
            target={att.url ? '_blank' : undefined}
            rel="noreferrer"
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              padding: compact ? '3px 8px' : '5px 10px',
              borderRadius: 'var(--radius-control)',
              border: '1px solid var(--line-strong)',
              background: 'var(--surface-inset)',
              color: 'var(--text-secondary)',
              fontSize: compact ? 11 : 12,
              textDecoration: 'none',
              maxWidth: '100%',
            }}
          >
            <FileTextOutlined style={{ color: 'var(--text-tertiary)', flexShrink: 0 }} />
            <span style={{ maxWidth: 180, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {att.file_name}
            </span>
            <span style={{ color: 'var(--text-quaternary)', flexShrink: 0 }}>
              {formatFileSize(att.file_size)}
            </span>
          </a>
        ),
      )}
    </div>
  )
}
