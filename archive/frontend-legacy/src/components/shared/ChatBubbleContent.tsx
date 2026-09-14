import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { cn, normalizeEscapedText } from '@/lib/utils'

interface ChatBubbleContentProps {
  content: string
  markdown?: boolean
  className?: string
}

export function ChatBubbleContent({ content, markdown = false, className }: ChatBubbleContentProps) {
  const displayContent = normalizeEscapedText(content, { preserveMarkdownCode: markdown })

  if (!markdown) {
    return (
      <p className={cn('whitespace-pre-wrap wrap-break-word', className)}>
        {displayContent}
      </p>
    )
  }

  return (
    <div
      className={cn(
        'prose prose-sm dark:prose-invert max-w-none prose-p:my-1 prose-ul:my-1 prose-ol:my-1 prose-li:my-0.5 prose-headings:my-2',
        className
      )}
    >
      <ReactMarkdown remarkPlugins={[remarkGfm]}>
        {displayContent}
      </ReactMarkdown>
    </div>
  )
}
