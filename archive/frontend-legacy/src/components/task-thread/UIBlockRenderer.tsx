import { Check, ChevronRight, Info } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { UIBlock, UIBlockResponse } from '@/types'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { normalizeEscapedText } from '@/lib/utils'

interface UIBlockRendererProps {
  blocks: UIBlock[]
  /** 只读模式：已回复的历史消息 */
  responses?: Record<string, UIBlockResponse>
}

/**
 * 只读渲染 PM 消息中的 ui_blocks（在 MessageBubble 内使用）。
 * 如果有 responses，展示用户的选择结果；否则展示空组件预览。
 */
export function UIBlockRenderer({ blocks, responses }: UIBlockRendererProps) {
  return (
    <div className="flex flex-col gap-3 mt-3 pt-3 border-t border-border/50">
      {blocks.map((block) => {
        const response = responses?.[block.id]
        switch (block.type) {
          case 'single_select':
            return <SelectBlockReadonly key={block.id} block={block} response={response} />
          case 'text_input':
            return <TextInputBlockReadonly key={block.id} block={block} response={response} />
          case 'confirm':
            return <ConfirmBlockReadonly key={block.id} block={block} response={response} />
          case 'info':
            return <InfoBlockReadonly key={block.id} block={block} />
          default:
            return null
        }
      })}
    </div>
  )
}

// ─── 只读子组件 ───

function BlockLabel({ label }: { label: string }) {
  return <p className="text-xs font-medium text-muted-foreground mb-1.5">{label}</p>
}

function SelectBlockReadonly({ block, response }: { block: UIBlock; response?: UIBlockResponse }) {
  const selected = response?.selected ?? []
  return (
    <div>
      <BlockLabel label={block.label} />
      <div className="flex flex-wrap gap-1.5">
        {block.options?.map((opt) => {
          const isSelected = selected.includes(opt.value)
          return (
            <span
              key={opt.value}
              className={cn(
                'inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs transition-colors',
                isSelected
                  ? 'bg-primary/15 text-primary font-medium'
                  : 'bg-muted/50 text-muted-foreground'
              )}
            >
              {isSelected && <Check className="size-3" />}
              {opt.label}
            </span>
          )
        })}
      </div>
    </div>
  )
}

function TextInputBlockReadonly({ block, response }: { block: UIBlock; response?: UIBlockResponse }) {
  const text = response?.text
  return (
    <div>
      <BlockLabel label={block.label} />
      {text ? (
        <p className="text-xs text-foreground bg-muted/30 rounded-lg px-3 py-2">{text}</p>
      ) : (
        <p className="text-xs text-muted-foreground italic">{block.placeholder ?? '未填写'}</p>
      )}
    </div>
  )
}

function ConfirmBlockReadonly({ block, response }: { block: UIBlock; response?: UIBlockResponse }) {
  const confirmed = response?.confirmed
  return (
    <div>
      <BlockLabel label={block.label} />
      {confirmed != null ? (
        <span className={cn(
          'inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs font-medium',
          confirmed ? 'bg-green-500/15 text-green-600' : 'bg-orange-500/15 text-orange-600'
        )}>
          {confirmed ? (block.confirm_label ?? '已确认') : (block.cancel_label ?? '已取消')}
        </span>
      ) : (
        <div className="flex gap-2">
          <span className="inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs bg-muted/50 text-muted-foreground">
            <ChevronRight className="size-3" />
            {block.confirm_label ?? '确认'}
          </span>
          <span className="inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs bg-muted/50 text-muted-foreground">
            {block.cancel_label ?? '取消'}
          </span>
        </div>
      )}
    </div>
  )
}

function InfoBlockReadonly({ block }: { block: UIBlock }) {
  return (
    <div>
      <div className="flex items-center gap-1.5 mb-1.5">
        <Info className="size-3 text-blue-500" />
        <p className="text-xs font-medium text-muted-foreground">{block.label}</p>
      </div>
      {block.content && (
        <div className="text-xs bg-blue-500/5 rounded-lg px-3 py-2 prose prose-sm dark:prose-invert max-w-none prose-p:my-0.5">
          <ReactMarkdown remarkPlugins={[remarkGfm]}>
            {normalizeEscapedText(block.content, { preserveMarkdownCode: true })}
          </ReactMarkdown>
        </div>
      )}
    </div>
  )
}
