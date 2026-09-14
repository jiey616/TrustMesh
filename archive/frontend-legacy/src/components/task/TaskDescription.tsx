import { useState, useRef, useEffect } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { cn, normalizeEscapedText } from '@/lib/utils'
import { ChevronDown } from 'lucide-react'

const COLLAPSED_HEIGHT_PX = 48 // ~2 lines of text-sm (14px * 1.43 line-height * 2 ≈ 40, +8 for margin)

interface TaskDescriptionProps {
  description: string
}

export function TaskDescription({ description }: TaskDescriptionProps) {
  const [expanded, setExpanded] = useState(false)
  const [overflows, setOverflows] = useState(false)
  const contentRef = useRef<HTMLDivElement>(null)
  const displayDescription = normalizeEscapedText(description, { preserveMarkdownCode: true })

  useEffect(() => {
    const el = contentRef.current
    if (!el) return
    setOverflows(el.scrollHeight > COLLAPSED_HEIGHT_PX + 4)
  }, [displayDescription])

  return (
    <div className="mt-1.5">
      <div
        ref={contentRef}
        className={cn(
          'text-sm text-muted-foreground overflow-hidden transition-[max-height] duration-200',
          'prose prose-sm dark:prose-invert max-w-none',
          'prose-p:my-0 prose-ul:my-0.5 prose-ol:my-0.5 prose-li:my-0 prose-headings:my-1 prose-headings:text-muted-foreground',
          '[&>*:first-child]:mt-0 [&>*:last-child]:mb-0',
        )}
        style={!expanded && overflows ? { maxHeight: `${COLLAPSED_HEIGHT_PX}px` } : undefined}
      >
        <ReactMarkdown remarkPlugins={[remarkGfm]}>
          {displayDescription}
        </ReactMarkdown>
      </div>
      {overflows && (
        <button
          type="button"
          className="inline-flex items-center gap-0.5 text-xs text-muted-foreground/70 hover:text-muted-foreground mt-0.5 cursor-pointer"
          onClick={() => setExpanded(v => !v)}
        >
          {expanded ? '收起' : '展开'}
          <ChevronDown className={cn('size-3 transition-transform', expanded && 'rotate-180')} />
        </button>
      )}
    </div>
  )
}
