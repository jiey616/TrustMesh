import { useState, useMemo } from 'react'
import { cn } from '@/lib/utils'
import type { AssistantMessage } from '@/types'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Loader2, ExternalLink, ChevronRight, Brain } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { normalizeEscapedText } from '@/lib/utils'
import { KnowledgeResultCard } from './results/KnowledgeResultCard'
import { TaskResultCard } from './results/TaskResultCard'
import { StatsResultCard } from './results/StatsResultCard'
import { TaskDetailResultCard } from './results/TaskDetailResultCard'

interface Props {
  message: AssistantMessage
  getToolLabel: (tool: string) => string
}

/**
 * Parse <think>...</think> blocks from content.
 * Handles both complete and streaming (unclosed) think tags.
 */
function parseThinkBlocks(content: string): { thinking: string; reply: string; isThinking: boolean } {
  // Match complete <think>...</think> blocks
  const thinkRegex = /<think>([\s\S]*?)<\/think>/g
  let thinking = ''
  let isThinking = false

  // Collect all complete think blocks
  const cleaned = content.replace(thinkRegex, (_, thinkContent: string) => {
    thinking += (thinking ? '\n' : '') + thinkContent.trim()
    return ''
  })

  // Check for an unclosed <think> tag (streaming in progress)
  const unclosedMatch = cleaned.match(/<think>([\s\S]*)$/)
  if (unclosedMatch) {
    thinking += (thinking ? '\n' : '') + unclosedMatch[1].trim()
    isThinking = true
    const reply = cleaned.slice(0, unclosedMatch.index).trim()
    return { thinking, reply, isThinking }
  }

  return { thinking, reply: cleaned.trim(), isThinking }
}

export function AssistantMessageBubble({ message, getToolLabel }: Props) {
  const navigate = useNavigate()
  const isUser = message.role === 'user'
  const [thinkExpanded, setThinkExpanded] = useState(false)
  const normalizedContent = useMemo(
    () => normalizeEscapedText(message.content, { preserveMarkdownCode: true }),
    [message.content]
  )

  const { thinking, reply, isThinking } = useMemo(
    () => (!isUser && normalizedContent ? parseThinkBlocks(normalizedContent) : { thinking: '', reply: normalizedContent, isThinking: false }),
    [isUser, normalizedContent]
  )

  return (
    <div className={cn('flex', isUser && 'justify-end')}>
      <div className={cn('flex flex-col gap-1.5 max-w-[88%]', isUser && 'items-end')}>
        {/* Tool calls in progress */}
        {!isUser && message.toolCalls && message.toolCalls.length > 0 && (
          <div className="flex flex-col gap-1 px-1">
            {message.toolCalls.map((tc, i) => (
              <div key={i} className="flex items-center gap-1.5 text-xs text-muted-foreground">
                {tc.status === 'running' ? (
                  <Loader2 className="size-3 animate-spin" />
                ) : (
                  <span className="size-3 text-center">✓</span>
                )}
                <span>{getToolLabel(tc.tool)}</span>
              </div>
            ))}
          </div>
        )}

        {/* Structured result cards */}
        {!isUser && message.results && message.results.length > 0 && (
          <div className="flex flex-col gap-2 w-full">
            {message.results.map((result, i) => {
              switch (result.type) {
                case 'knowledge':
                  return <KnowledgeResultCard key={i} items={result.items} />
                case 'tasks':
                  return <TaskResultCard key={i} items={result.items} />
                case 'task_detail':
                  return <TaskDetailResultCard key={i} task={result.task} />
                case 'stats':
                  return <StatsResultCard key={i} stats={result.stats} />
                default:
                  return null
              }
            })}
          </div>
        )}

        {/* Thinking block (collapsible) */}
        {!isUser && thinking && (
          <div className="rounded-xl border border-dashed border-muted-foreground/20 overflow-hidden">
            <button
              className="flex items-center gap-1.5 w-full px-3 py-1.5 text-xs text-muted-foreground hover:bg-muted/50 transition-colors"
              onClick={() => setThinkExpanded(!thinkExpanded)}
            >
              {isThinking ? (
                <Loader2 className="size-3 animate-spin shrink-0" />
              ) : (
                <Brain className="size-3 shrink-0" />
              )}
              <span>{isThinking ? '思考中...' : '思考过程'}</span>
              <ChevronRight className={cn('size-3 ml-auto transition-transform', thinkExpanded && 'rotate-90')} />
            </button>
            {thinkExpanded && (
              <div className="px-3 pb-2 text-xs text-muted-foreground leading-relaxed border-t border-dashed border-muted-foreground/20">
                <div className="pt-2 prose prose-sm dark:prose-invert max-w-none prose-p:my-0.5 prose-p:text-xs prose-p:text-muted-foreground">
                  <ReactMarkdown remarkPlugins={[remarkGfm]}>
                    {thinking}
                  </ReactMarkdown>
                </div>
              </div>
            )}
          </div>
        )}

        {/* Text content (reply without think blocks) */}
        {(isUser ? message.content : reply) && (
          <div
            className={cn(
              'rounded-2xl px-4 py-2.5 text-sm leading-relaxed overflow-hidden',
              isUser
                ? 'bg-primary text-primary-foreground'
                : 'bg-muted'
            )}
          >
            {isUser ? (
              <p className="whitespace-pre-wrap wrap-break-word">{normalizedContent}</p>
            ) : (
              <div className="prose prose-sm dark:prose-invert max-w-none prose-p:my-1 prose-ul:my-1 prose-ol:my-1 prose-li:my-0.5 prose-headings:my-2">
                <ReactMarkdown remarkPlugins={[remarkGfm]}>
                  {reply}
                </ReactMarkdown>
              </div>
            )}
          </div>
        )}

        {/* Streaming indicator (only when no content and no thinking yet) */}
        {!isUser && message.isStreaming && !message.content && (!message.toolCalls || message.toolCalls.length === 0) && (
          <div className="flex items-center gap-1.5 px-4 py-2 text-xs text-muted-foreground">
            <Loader2 className="size-3 animate-spin" />
            <span>思考中...</span>
          </div>
        )}

        {/* Navigate action */}
        {!isUser && message.navigateAction && (
          <Button
            variant="outline"
            size="sm"
            className="gap-1.5 self-start"
            onClick={() => navigate(message.navigateAction!.path)}
          >
            <ExternalLink className="size-3.5" />
            {message.navigateAction.label}
          </Button>
        )}
      </div>
    </div>
  )
}
