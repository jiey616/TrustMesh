import { FileText } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { ChatAttachment } from '@/types'

const isImageMime = (mime: string | undefined) => !!mime && mime.startsWith('image/')

// Renders chat message attachments: images as tappable thumbnails, other files
// as chips. Clicking opens the signed download URL in a new tab.
export function ChatAttachmentList({
  attachments,
  className,
  compact,
}: {
  attachments?: ChatAttachment[]
  className?: string
  compact?: boolean
}) {
  if (!attachments || attachments.length === 0) return null
  const images = attachments.filter((a) => isImageMime(a.mime_type))
  const files = attachments.filter((a) => !isImageMime(a.mime_type))

  return (
    <div className={cn('flex flex-col gap-2 pt-2', className)}>
      {images.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {images.map((att) => (
            <a
              key={att.id}
              href={att.url}
              target="_blank"
              rel="noreferrer"
              className={cn(
                'block overflow-hidden rounded-lg border bg-background/40',
                compact ? 'size-16' : 'size-24',
              )}
              title={att.file_name}
            >
              <img
                src={att.url}
                alt={att.file_name}
                className="size-full object-cover"
                loading="lazy"
              />
            </a>
          ))}
        </div>
      )}
      {files.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {files.map((att) => (
            <a
              key={att.id}
              href={att.url}
              target="_blank"
              rel="noreferrer"
              className="flex items-center gap-1.5 rounded-lg border bg-background/40 px-2 py-1 text-xs hover:bg-accent/40"
              title={`${att.file_name}（点击下载）`}
            >
              <FileText className="size-3.5 shrink-0" />
              <span className="max-w-40 truncate">{att.file_name}</span>
            </a>
          ))}
        </div>
      )}
    </div>
  )
}
