import { CloseOutlined, FileTextOutlined } from '@ant-design/icons'
import { formatFileSize, type PendingAttachmentFile } from '@/hooks/useChatAttachments'

interface PendingAttachmentListProps {
  files: PendingAttachmentFile[]
  onRemove: (index: number) => void
}

/**
 * 待发送附件预览条：图片缩略、非图片文件芯片，右上角可移除。
 * 由 ChatComposer 与 TaskCommentComposer 共用，保证两支输入框附件观感一致。
 */
export function PendingAttachmentList({ files, onRemove }: PendingAttachmentListProps) {
  if (files.length === 0) return null
  return (
    <div
      style={{
        display: 'flex',
        flexWrap: 'wrap',
        gap: 8,
        marginBottom: 8,
        alignItems: 'flex-end',
      }}
    >
      {files.map((p, index) => {
        const img = p.previewUrl
        return (
          <div
            key={`${p.file.name}-${index}`}
            style={{ position: 'relative', borderRadius: 8, border: '1px solid var(--line-strong)', background: 'var(--surface-raised)', padding: 4 }}
          >
            {img ? (
              <img src={img} alt={p.file.name} style={{ width: 88, height: 66, objectFit: 'cover', display: 'block', borderRadius: 6 }} />
            ) : (
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, height: 66, padding: '0 10px' }}>
                <FileTextOutlined style={{ color: 'var(--text-tertiary)' }} />
                <div style={{ minWidth: 0 }}>
                  <div style={{ fontSize: 12, color: 'var(--text-primary)', maxWidth: 120, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{p.file.name}</div>
                  <div style={{ fontSize: 11, color: 'var(--text-quaternary)' }}>{formatFileSize(p.file.size)}</div>
                </div>
              </div>
            )}
            <button
              type="button"
              onClick={() => onRemove(index)}
              aria-label="移除附件"
              style={{
                position: 'absolute',
                top: -6,
                right: -6,
                width: 18,
                height: 18,
                borderRadius: '50%',
                border: 'none',
                background: 'var(--text-secondary)',
                color: 'var(--surface)',
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                padding: 0,
                fontSize: 10,
                lineHeight: 1,
              }}
            >
              <CloseOutlined />
            </button>
          </div>
        )
      })}
    </div>
  )
}