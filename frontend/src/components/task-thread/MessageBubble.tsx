import { cn } from '@/lib/utils'
import type { TaskMessage } from '@/types'
import { formatRelativeTime } from '@/lib/utils'
import { ChatBubbleContent } from '@/components/shared/ChatBubbleContent'
import { ChatAttachmentList } from '@/components/shared/ChatAttachmentList'
import { UIBlockRenderer } from './UIBlockRenderer'

interface MessageBubbleProps {
  message: TaskMessage
  /** 下一条用户消息的 ui_response（用于回显选择结果） */
  nextUserResponse?: TaskMessage
  /** ui_blocks 正在底部交互面板中展示，气泡内隐藏 */
  hideUIBlocks?: boolean
}

export function MessageBubble({ message, nextUserResponse, hideUIBlocks }: MessageBubbleProps) {
  const isUser = message.role === 'user'
  const hasUIBlocks = !isUser && !hideUIBlocks && message.ui_blocks && message.ui_blocks.length > 0

  return (
    <div className={cn('flex', isUser && 'justify-end')}>
      <div className={cn('flex flex-col gap-1 max-w-[85%]', isUser && 'items-end')}>
        <div
          className={cn(
            'rounded-3xl px-5 py-3.5 text-sm leading-relaxed overflow-hidden',
            isUser
              ? 'bg-primary text-primary-foreground'
              : 'bg-muted'
          )}
        >
          <ChatBubbleContent content={message.content} markdown={!isUser} />
          {message.attachments && message.attachments.length > 0 && (
            <ChatAttachmentList attachments={message.attachments} compact />
          )}
          {hasUIBlocks && (
            <UIBlockRenderer
              blocks={message.ui_blocks!}
              responses={nextUserResponse?.ui_response?.blocks}
            />
          )}
        </div>
        <p className={cn('text-[10px] text-muted-foreground px-1', isUser && 'text-right')}>
          {formatRelativeTime(message.created_at)}
        </p>
      </div>
    </div>
  )
}
