import { useRef, useEffect, useCallback, useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import {
  CheckCircle2,
  AlertCircle,
  ArrowRight,
  ArrowDown,
  FileText,
  Download,
  Eye,
  Loader2,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Avatar } from '@/components/ui/avatar'
import { cn, formatRelativeTime, normalizeEscapedText } from '@/lib/utils'
import { TrustMeshLogo } from '@/components/shared/TrustMeshLogo'
import { usePlatformStore } from '@/stores/platformStore'
import { useTaskEvents } from '@/hooks/useTasks'
import { getTaskArtifactContent } from '@/api/tasks'
import { ApiRequestError } from '@/api/client'
import { FileViewer } from '@/components/task/FileViewer'
import { toast } from 'sonner'
import type { Event } from '@/types'

// ─── 配置 ───

const eventLabel: Record<string, string> = {
  task_created: '创建任务',
  task_plan_ready: '规划完成',
  task_status_changed: '更新状态',
  todo_assigned: '分配任务',
  todo_started: '开始执行',
  todo_progress: '进度更新',
  todo_completed: '完成 Todo',
  todo_failed: '执行失败',
  task_comment: '',
  planning_reply: 'PM 回复',
  agent_status_changed: '状态变更',
  artifact_received: '上传文件',
}

const actorRoleBadge: Record<string, { label: string; className: string } | null> = {
  user: null,
  agent: { label: 'Agent', className: 'bg-muted text-muted-foreground' },
  system: { label: '系统', className: 'bg-muted text-muted-foreground' },
}

const taskStatusBadge: Record<string, { label: string; variant: 'secondary' | 'info' | 'success' | 'destructive' }> = {
  planning: { label: '规划中', variant: 'info' },
  pending: { label: '待处理', variant: 'secondary' },
  in_progress: { label: '进行中', variant: 'info' },
  done: { label: '已完成', variant: 'success' },
  failed: { label: '失败', variant: 'destructive' },
  canceled: { label: '已取消', variant: 'secondary' },
}

const agentStatusBadge: Record<string, { label: string; className: string }> = {
  online: { label: '在线', className: 'bg-status-online/15 text-status-online' },
  offline: { label: '离线', className: 'bg-status-offline/15 text-status-offline' },
  busy: { label: '忙碌', className: 'bg-status-busy/15 text-status-busy' },
}

// ─── 工具函数 ───

function shouldGroupWithPrev(current: Event, prev: Event | undefined): boolean {
  if (!prev) return false
  if (current.actor_id !== prev.actor_id) return false
  if (current.event_type !== prev.event_type) return false
  const diffMs = new Date(current.created_at).getTime() - new Date(prev.created_at).getTime()
  return diffMs < 2 * 60 * 1000 // 2 分钟内
}

// ─── 引用块组件 ───

function QuoteBlock({ children, color = 'border-muted-foreground/30' }: { children: React.ReactNode; color?: string }) {
  return (
    <div className={cn('mt-1.5 border-l-2 pl-3 py-1', color)}>
      {children}
    </div>
  )
}

// ─── 状态转换 ───

function StatusTransition({ from, to }: { from: string; to: string }) {
  const fromCfg = taskStatusBadge[from]
  const toCfg = taskStatusBadge[to]
  if (!fromCfg || !toCfg) return null

  return (
    <div className="flex items-center gap-1.5">
      <Badge variant={fromCfg.variant} className="text-[10px] px-1.5 py-0">
        {fromCfg.label}
      </Badge>
      <ArrowRight className="size-3 text-muted-foreground" />
      <Badge variant={toCfg.variant} className="text-[10px] px-1.5 py-0">
        {toCfg.label}
      </Badge>
    </div>
  )
}

function AgentStatusTransition({ from, to }: { from: string; to: string }) {
  const fromCfg = agentStatusBadge[from]
  const toCfg = agentStatusBadge[to]
  if (!fromCfg || !toCfg) return null

  return (
    <div className="flex items-center gap-1.5">
      <Badge className={cn('text-[10px] px-1.5 py-0', fromCfg.className)} variant="outline">
        {fromCfg.label}
      </Badge>
      <ArrowRight className="size-3 text-muted-foreground" />
      <Badge className={cn('text-[10px] px-1.5 py-0', toCfg.className)} variant="outline">
        {toCfg.label}
      </Badge>
    </div>
  )
}

// ─── Markdown prose 样式 ───

const proseSmall = cn(
  'text-xs text-muted-foreground',
  'prose prose-xs dark:prose-invert max-w-none',
  'prose-p:my-0.5 prose-headings:my-1 prose-headings:text-xs prose-headings:text-muted-foreground',
  'prose-pre:my-1 prose-pre:bg-zinc-200/70 prose-pre:text-[11px] prose-pre:text-zinc-800 dark:prose-pre:bg-zinc-800 dark:prose-pre:text-zinc-200',
  'prose-code:text-[11px] prose-code:bg-zinc-200/70 prose-code:text-zinc-800 dark:prose-code:bg-zinc-800 dark:prose-code:text-zinc-200 prose-code:px-1 prose-code:py-0.5 prose-code:rounded',
  '[&>*:first-child]:mt-0 [&>*:last-child]:mb-0',
)

// ─── 事件内容渲染 ───

function EventContent({ event }: { event: Event }) {
  const todoTitle = event.metadata.todo_title as string | undefined
  const taskTitle = event.metadata.task_title as string | undefined
  const markdownContent = normalizeEscapedText(event.content ?? '', { preserveMarkdownCode: true })
  const plainContent = normalizeEscapedText(event.content ?? '')

  if (event.event_type === 'task_comment') {
    return (
      <div className={cn(
        'mt-1 text-sm',
        'prose prose-sm dark:prose-invert max-w-none',
        'prose-p:my-1 prose-ul:my-1 prose-ol:my-1 prose-li:my-0 prose-headings:my-2 prose-headings:text-foreground',
        'prose-pre:my-1.5 prose-pre:bg-zinc-200/70 prose-pre:text-xs prose-pre:text-zinc-800 dark:prose-pre:bg-zinc-800 dark:prose-pre:text-zinc-200',
        'prose-code:text-xs prose-code:bg-zinc-200/70 prose-code:text-zinc-800 prose-code:px-1 prose-code:py-0.5 prose-code:rounded dark:prose-code:bg-zinc-800 dark:prose-code:text-zinc-200',
        'prose-hr:my-2',
        '[&>*:first-child]:mt-0 [&>*:last-child]:mb-0',
      )}>
        <ReactMarkdown remarkPlugins={[remarkGfm]}>
          {markdownContent}
        </ReactMarkdown>
      </div>
    )
  }

  if (event.event_type === 'task_created') {
    return taskTitle
      ? <p className="text-sm text-muted-foreground">{taskTitle}</p>
      : <p className="text-sm text-muted-foreground">创建了新任务</p>
  }

  if (event.event_type === 'task_status_changed') {
    const from = event.metadata.from as string | undefined
    const to = event.metadata.to as string | undefined
    return from && to
      ? <StatusTransition from={from} to={to} />
      : <p className="text-sm text-muted-foreground">任务状态已更新</p>
  }

  if (event.event_type === 'todo_assigned') {
    return (
      <div>
        <p className="text-sm whitespace-pre-wrap">{plainContent || '分配了 Todo'}</p>
        {todoTitle && (
          <QuoteBlock color="border-info/30">
            <p className="text-xs text-muted-foreground">{todoTitle}</p>
          </QuoteBlock>
        )}
      </div>
    )
  }

  if (event.event_type === 'todo_started') {
    return todoTitle
      ? <p className="text-sm text-muted-foreground">{todoTitle}</p>
      : <p className="text-sm text-muted-foreground">开始执行</p>
  }

  if (event.event_type === 'todo_progress') {
    return (
      <div>
        {todoTitle && <p className="text-xs font-medium text-muted-foreground">{todoTitle}</p>}
        {plainContent && (
          <div className={cn(proseSmall, todoTitle ? 'mt-0.5' : '')}>
            <ReactMarkdown remarkPlugins={[remarkGfm]}>
              {markdownContent}
            </ReactMarkdown>
          </div>
        )}
        {!plainContent && !todoTitle && <p className="text-sm text-muted-foreground">进度已更新</p>}
      </div>
    )
  }

  if (event.event_type === 'todo_completed') {
    return (
      <div className="flex items-center gap-1.5">
        <CheckCircle2 className="size-3.5 text-success shrink-0" />
        <span className="text-sm text-muted-foreground line-through decoration-success/40">{todoTitle || '已完成'}</span>
      </div>
    )
  }

  if (event.event_type === 'todo_failed') {
    const error = normalizeEscapedText((event.metadata.error as string | undefined) ?? '')
    return (
      <div className="flex items-start gap-1.5">
        <AlertCircle className="size-3.5 text-destructive shrink-0 mt-0.5" />
        <div className="min-w-0">
          {todoTitle && <p className="text-sm text-muted-foreground">{todoTitle}</p>}
          {error && <p className="text-xs text-destructive mt-0.5 whitespace-pre-wrap">{error}</p>}
        </div>
      </div>
    )
  }

  if (event.event_type === 'planning_reply') {
    return plainContent ? (
      <QuoteBlock color="border-info/30">
        <div className={proseSmall}>
          <ReactMarkdown remarkPlugins={[remarkGfm]}>
            {markdownContent}
          </ReactMarkdown>
        </div>
      </QuoteBlock>
    ) : <p className="text-sm text-muted-foreground">回复了对话</p>
  }

  if (event.event_type === 'agent_status_changed') {
    const from = event.metadata.prev_status as string | undefined
    const to = event.metadata.new_status as string | undefined
    return from && to
      ? <AgentStatusTransition from={from} to={to} />
      : <p className="text-sm text-muted-foreground">状态已变更</p>
  }

  if (event.event_type === 'artifact_received') {
    const fileName = event.metadata.file_name as string | undefined
    const fileSize = event.metadata.file_size as number | undefined
    const mimeType = event.metadata.mime_type as string | undefined
    const transferId = event.metadata.transfer_id as string | undefined
    const sizeLabel = formatFileSize(fileSize)

    return (
      <div className="mt-1 flex items-center gap-2.5 rounded-md border border-border/60 bg-muted/30 px-3 py-2">
        <div className="flex size-7 shrink-0 items-center justify-center rounded bg-muted">
          <FileText className="size-3.5 text-muted-foreground" />
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-xs font-medium truncate">{fileName || '未知文件'}</p>
          <p className="text-[11px] text-muted-foreground">
            {mimeType}{sizeLabel ? ` · ${sizeLabel}` : ''}
          </p>
        </div>
        {transferId && event.task_id && (
          <ArtifactActions taskId={event.task_id} transferId={transferId} fileName={fileName || 'file'} />
        )}
      </div>
    )
  }

  return <p className="text-sm text-muted-foreground">{event.event_type}</p>
}

// ─── 文件操作组件 ───

function ArtifactActions({ taskId, transferId, fileName }: { taskId: string; transferId: string; fileName: string }) {
  const [loading, setLoading] = useState(false)
  const [downloading, setDownloading] = useState(false)
  const [viewerBlob, setViewerBlob] = useState<Blob | null>(null)
  const [viewerOpen, setViewerOpen] = useState(false)

  const handlePreview = async () => {
    setLoading(true)
    try {
      const blob = await getTaskArtifactContent(taskId, transferId)
      setViewerBlob(blob)
      setViewerOpen(true)
    } catch (err) {
      toast.error(err instanceof ApiRequestError ? err.message : '打开文件失败')
    } finally {
      setLoading(false)
    }
  }

  const handleDownload = async () => {
    setDownloading(true)
    try {
      const blob = await getTaskArtifactContent(taskId, transferId)
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = fileName
      document.body.appendChild(a)
      a.click()
      a.remove()
      setTimeout(() => URL.revokeObjectURL(url), 60_000)
    } catch (err) {
      toast.error(err instanceof ApiRequestError ? err.message : '下载文件失败')
    } finally {
      setDownloading(false)
    }
  }

  return (
    <>
      <div className="flex items-center gap-1 shrink-0">
        <Button type="button" variant="ghost" size="icon" className="size-6" disabled={loading} onClick={() => void handlePreview()}>
          {loading ? <Loader2 className="size-3 animate-spin" /> : <Eye className="size-3" />}
        </Button>
        <Button type="button" variant="ghost" size="icon" className="size-6" disabled={downloading} onClick={() => void handleDownload()}>
          {downloading ? <Loader2 className="size-3 animate-spin" /> : <Download className="size-3" />}
        </Button>
      </div>
      <FileViewer
        open={viewerOpen}
        onOpenChange={setViewerOpen}
        blob={viewerBlob}
        fileName={fileName}
        onDownload={() => void handleDownload()}
      />
    </>
  )
}

function formatFileSize(bytes: number | undefined) {
  if (!bytes || bytes <= 0) return ''
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

// ─── 消息组件 ───

function FeedActorAvatar({ event }: { event: Event }) {
  const platformName = usePlatformStore((s) => s.name)
  if (event.actor_type === 'system') {
    return <TrustMeshLogo size={32} platformName={platformName} />
  }

  return (
    <Avatar
      fallback={event.actor_name || (event.actor_type === 'agent' ? 'Agent' : '用户')}
      seed={event.actor_id || event.actor_name}
      kind={event.actor_type === 'agent' ? 'agent' : 'user'}
      role={event.actor_type === 'agent' ? 'custom' : undefined}
      size="md"
    />
  )
}

function FeedMessage({ event, showHeader }: { event: Event; showHeader: boolean }) {
  const roleBadge = actorRoleBadge[event.actor_type]
  const isSystem = event.actor_type === 'system'
  const platformName = usePlatformStore((s) => s.name)
  const label = eventLabel[event.event_type] || ''

  if (!showHeader) {
    return (
      <div className="flex gap-3 pl-11">
        <div className="flex-1 min-w-0">
          <EventContent event={event} />
        </div>
      </div>
    )
  }

  return (
    <div className="flex items-start gap-3">
      <FeedActorAvatar event={event} />
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-1.5">
          <span className="text-sm font-semibold">{isSystem ? platformName : event.actor_name}</span>
          {roleBadge && (
            <span className={cn('text-[10px] px-1.5 py-0 rounded-sm font-medium', roleBadge.className)}>
              {roleBadge.label}
            </span>
          )}
          {label && <span className="text-xs text-muted-foreground">{label}</span>}
          <span className="text-xs text-muted-foreground ml-auto shrink-0">
            {formatRelativeTime(event.created_at)}
          </span>
        </div>
        <EventContent event={event} />
      </div>
    </div>
  )
}

// ─── 主组件 ───

interface TaskFeedProps {
  taskId: string
}

export function TaskFeed({ taskId }: TaskFeedProps) {
  const { data: events, isLoading } = useTaskEvents(taskId)
  const scrollRef = useRef<HTMLDivElement>(null)
  const bottomRef = useRef<HTMLDivElement>(null)
  const [isAtBottom, setIsAtBottom] = useState(true)
  const prevCountRef = useRef(0)
  const initialScrollDone = useRef(false)

  const scrollToBottom = useCallback((smooth = true) => {
    bottomRef.current?.scrollIntoView({ behavior: smooth ? 'smooth' : 'instant' })
  }, [])

  const handleScroll = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 50
    setIsAtBottom(atBottom)
  }, [])

  // 首次加载跳到底部
  useEffect(() => {
    if (events && events.length > 0 && !initialScrollDone.current) {
      initialScrollDone.current = true
      requestAnimationFrame(() => scrollToBottom(false))
    }
  }, [events, scrollToBottom])

  // 新数据到达时自动滚动
  useEffect(() => {
    if (!events) return
    if (events.length > prevCountRef.current && isAtBottom && initialScrollDone.current) {
      requestAnimationFrame(() => scrollToBottom(true))
    }
    prevCountRef.current = events.length
  }, [events, isAtBottom, scrollToBottom])

  if (isLoading) {
    return <div className="flex items-center justify-center h-full text-sm text-muted-foreground">加载中...</div>
  }

  if (!events || events.length === 0) {
    return <div className="flex items-center justify-center h-full text-sm text-muted-foreground">暂无动态</div>
  }

  return (
    <div className="relative h-full">
      <div
        ref={scrollRef}
        className="h-full overflow-y-auto px-5 py-4"
        onScroll={handleScroll}
      >
        <div className="flex flex-col gap-1">
          {events.map((event, index) => {
            const prev = index > 0 ? events[index - 1] : undefined
            const showHeader = !shouldGroupWithPrev(event, prev)

            return (
              <div key={event.id}>
                <div className={cn('py-1.5 rounded-md hover:bg-accent/30 px-2 -mx-2 transition-colors', !showHeader && 'pt-0')}>
                  <FeedMessage event={event} showHeader={showHeader} />
                </div>
              </div>
            )
          })}
        </div>
        <div ref={bottomRef} />
      </div>

      {!isAtBottom && (
        <Button
          size="sm"
          variant="secondary"
          className="absolute bottom-3 left-1/2 -translate-x-1/2 shadow-md gap-1 text-xs"
          onClick={() => scrollToBottom(true)}
        >
          <ArrowDown className="size-3" />
          新消息
        </Button>
      )}
    </div>
  )
}
