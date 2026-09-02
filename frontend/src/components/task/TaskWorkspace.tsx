import { X, MessageSquare, PackageCheck, Paperclip, Download, File, Upload, FolderOpen } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { TaskStatusBadge, PriorityBadge } from '@/components/shared/StatusBadge'
import { TaskFeed } from './TaskFeed'
import { TaskTodoSection } from './TaskTodoSection'
import { TaskThreadSheet } from './TaskThreadSheet'
import { TaskResultView } from './TaskResult'
import { TaskDescription } from './TaskDescription'
import { TaskComposer } from './TaskComposer'
import { TaskCommentComposer, type TaskCommentSubmitInput, type TaskMentionCandidate } from './TaskCommentComposer'
import { CancelTaskDialog } from './CancelTaskDialog'
import { MessageBubble } from '@/components/task-thread/MessageBubble'
import { ThinkingIndicator } from '@/components/task-thread/ThinkingIndicator'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { useTask, useAddTaskComment, useAppendTaskMessage, useCreateTaskFromText, useApprovePlan, useRejectPlan } from '@/hooks/useTasks'
import { useAgents } from '@/hooks/useAgents'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectTrigger,
  SelectContent,
  SelectItem,
  SelectValue,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ApiRequestError } from '@/api/client'
import { reviewTodo } from '@/api/tasks'
import { downloadProjectFile, uploadProjectFile } from '@/api/projectFiles'
import { useProjectFiles } from '@/hooks/useProjectFiles'
import { useProject } from '@/hooks/useProjects'
import { sourceMeta } from '@/lib/fileSource'
import { Checkbox } from '@/components/ui/checkbox'
import { useEffect, useMemo, useRef, useState } from 'react'
import type { TaskMessage, TaskDetail, UIResponse, Todo, TaskAttachedFile, ProjectFile, Workflow } from '@/types'
import { normalizeEscapedText, cn } from '@/lib/utils'

type TaskWorkspaceProps = {
  onClose: () => void
  onTaskCreated?: (taskId: string) => void
} & (
  | { taskId: string; projectId?: never }
  | { projectId: string; taskId?: never }
)

function buildTaskMentionCandidates(task: TaskDetail | undefined): TaskMentionCandidate[] {
  if (!task) {
    return []
  }

  const seen = new Set<string>()
  const candidates: TaskMentionCandidate[] = []

  if (task.pm_agent.id && !seen.has(task.pm_agent.id)) {
    candidates.push({
      id: task.pm_agent.id,
      name: task.pm_agent.name,
      roleLabel: 'PM Agent',
    })
    seen.add(task.pm_agent.id)
  }

  for (const todo of task.todos) {
    if (!todo.assignee.agent_id || seen.has(todo.assignee.agent_id)) {
      continue
    }
    candidates.push({
      id: todo.assignee.agent_id,
      name: todo.assignee.name,
      roleLabel: '执行 Agent',
    })
    seen.add(todo.assignee.agent_id)
  }

  return candidates
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function AttachedFileItem({
  file,
  projectId,
}: {
  file: TaskAttachedFile
  projectId: string
}) {
  const [downloading, setDownloading] = useState(false)

  const handleDownload = async () => {
    setDownloading(true)
    try {
      const blob = await downloadProjectFile(projectId, file.id)
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = file.file_name
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
    } catch {
      toast.error(`下载 ${file.file_name} 失败`)
    } finally {
      setDownloading(false)
    }
  }

  const meta = sourceMeta(file.source)
  return (
    <div className="flex items-center gap-2 px-2.5 py-1.5 rounded-md bg-muted/40 border text-sm">
      <File className="size-3.5 shrink-0 text-muted-foreground" />
      <span className="flex-1 truncate">{file.file_name}</span>
      <span
        className={`shrink-0 text-[10px] leading-none px-1.5 py-0.5 rounded-full border ${meta.cls}`}
      >
        {meta.label}
      </span>
      <span className="shrink-0 text-xs text-muted-foreground">{formatFileSize(file.file_size)}</span>
      <Button
        variant="ghost"
        size="icon"
        className="size-6 shrink-0"
        disabled={downloading}
        onClick={handleDownload}
        title="下载文件"
      >
        <Download className="size-3.5" />
      </Button>
    </div>
  )
}

function AttachedFilesSection({ files, projectId }: { files: TaskAttachedFile[]; projectId: string }) {
  if (!files || files.length === 0) return null

  return (
    <div className="mt-3 flex flex-col gap-1.5">
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <Paperclip className="size-3" />
        <span>附加文件 ({files.length})</span>
      </div>
      {files.map((f) => (
        <AttachedFileItem key={f.id} file={f} projectId={projectId} />
      ))}
    </div>
  )
}

function PlanReviewPanel({
  todos,
  onApprove,
  onReject,
  isApproving,
  isRejecting,
  workflow,
}: {
  todos: Todo[]
  onApprove: () => void
  onReject: (feedback: string) => void
  isApproving: boolean
  isRejecting: boolean
  workflow?: Workflow
}) {
  const [showRejectInput, setShowRejectInput] = useState(false)
  const [feedback, setFeedback] = useState('')

  const handleReject = () => {
    if (!feedback.trim()) return
    onReject(feedback.trim())
  }

  return (
    <div className="border rounded-xl bg-muted/20 p-4 flex flex-col gap-3">
      <div>
        <div className="flex items-center gap-2">
          <p className="text-sm font-medium">PM 已完成规划，请确认后开始执行</p>
          {workflow && workflow.steps.length > 0 && (
            <span className="shrink-0 rounded-md bg-primary/10 px-2 py-0.5 text-[11px] text-primary">
              工作流：{workflow.name || '未命名'}
            </span>
          )}
        </div>
        <p className="text-xs text-muted-foreground mt-1">共 {todos.length} 个子任务</p>
      </div>

      <div className="flex flex-col gap-1.5">
        {todos.map((todo, idx) => (
          <div key={todo.id} className="flex items-start gap-2 rounded-lg bg-background border px-3 py-2 text-sm">
            <span className="shrink-0 text-xs text-muted-foreground w-5 pt-0.5">{idx + 1}.</span>
            <div className="min-w-0 flex-1">
              <p className="font-medium">{todo.title}</p>
              {todo.description && (
                <p className="text-xs text-muted-foreground mt-0.5 whitespace-pre-wrap">{normalizeEscapedText(todo.description)}</p>
              )}
            </div>
            <span className="shrink-0 text-xs text-muted-foreground pt-0.5">{todo.assignee.name}</span>
          </div>
        ))}
      </div>

      {showRejectInput ? (
        <div className="flex flex-col gap-2">
          <textarea
            className="w-full rounded-lg border bg-background px-3 py-2 text-sm resize-none focus:outline-none focus:ring-1 focus:ring-ring"
            rows={3}
            placeholder="说明需要调整的地方..."
            value={feedback}
            onChange={(e) => setFeedback(e.target.value)}
            autoFocus
          />
          <div className="flex gap-2">
            <Button
              size="sm"
              variant="destructive"
              disabled={!feedback.trim() || isRejecting}
              onClick={handleReject}
            >
              {isRejecting ? '提交中...' : '提交修改意见'}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => { setShowRejectInput(false); setFeedback('') }}
            >
              取消
            </Button>
          </div>
        </div>
      ) : (
        <div className="flex gap-2">
          <Button size="sm" disabled={isApproving} onClick={onApprove}>
            {isApproving ? '确认中...' : '确认执行'}
          </Button>
          <Button size="sm" variant="outline" onClick={() => setShowRejectInput(true)}>
            修改规划
          </Button>
        </div>
      )}
    </div>
  )
}

interface SelectedFile {
  id: string
  file_name: string
}

function DraftPlanningState({
  projectId,
  onSubmit,
  disabled,
  candidates,
  workflows,
}: {
  projectId?: string
  onSubmit: (content: string, agentId?: string, fileIds?: string[], workflow?: Workflow) => Promise<void>
  disabled: boolean
  candidates: TaskMentionCandidate[]
  workflows?: Workflow[]
}) {
  const [selectedFiles, setSelectedFiles] = useState<SelectedFile[]>([])
  const [attachOpen, setAttachOpen] = useState(false)
  const [filesPopoverOpen, setFilesPopoverOpen] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [selectedWorkflow, setSelectedWorkflow] = useState<string>('')
  const fileInputRef = useRef<HTMLInputElement>(null)
  const attachRef = useRef<HTMLDivElement>(null)
  const { data: projectFiles } = useProjectFiles(projectId)

  // Close attach dropdown on outside click
  useEffect(() => {
    if (!attachOpen) return
    const handler = (e: MouseEvent) => {
      if (attachRef.current && !attachRef.current.contains(e.target as Node)) {
        setAttachOpen(false)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [attachOpen])

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file || !projectId) return
    setAttachOpen(false)
    setUploading(true)
    try {
      const formData = new FormData()
      formData.append('file', file)
      const res = await uploadProjectFile(projectId, formData)
      setSelectedFiles((prev) => {
        if (prev.some((f) => f.id === res.data.id)) return prev
        return [...prev, { id: res.data.id, file_name: res.data.file_name }]
      })
    } catch {
      toast.error('上传失败')
    } finally {
      setUploading(false)
      if (fileInputRef.current) fileInputRef.current.value = ''
    }
  }

  const toggleProjectFile = (file: ProjectFile) => {
    setSelectedFiles((prev) => {
      const exists = prev.find((f) => f.id === file.id)
      if (exists) return prev.filter((f) => f.id !== file.id)
      return [...prev, { id: file.id, file_name: file.file_name }]
    })
  }

  const removeFile = (fileId: string) => {
    setSelectedFiles((prev) => prev.filter((f) => f.id !== fileId))
  }

  const handleSubmit = async ({ content, mentionAgentIds }: TaskCommentSubmitInput) => {
    const fileIds = selectedFiles.length > 0 ? selectedFiles.map((f) => f.id) : undefined
    const wf = workflows?.find((w) => w.name === selectedWorkflow)
    await onSubmit(content, mentionAgentIds[0], fileIds, wf)
    setSelectedFiles([])
    return true
  }

  const fileList = (projectFiles ?? []) as ProjectFile[]

  return (
    <>
      <div className="px-5 py-4 shrink-0 border-b">
        <h2 className="text-lg font-semibold">新任务</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          描述需求由 PM 规划；输入 @ 直接指派给执行 Agent。
        </p>
        {workflows && workflows.length > 0 && (
          <div className="mt-3 flex items-center gap-2">
            <span className="text-xs text-muted-foreground">工作流</span>
            <Select value={selectedWorkflow} onValueChange={(v: string | null) => setSelectedWorkflow(v ?? '')}>
              <SelectTrigger className="h-8 w-56 text-xs">
                <SelectValue placeholder="不选（PM 自由规划）" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="">不选（PM 自由规划）</SelectItem>
                {workflows.map((w) => (
                  <SelectItem key={w.name} value={w.name}>
                    {w.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}
      </div>

      <div className="flex-1 min-h-0 px-5 py-6">
        <div className="rounded-2xl border border-dashed bg-muted/20 px-5 py-6">
          <p className="text-sm font-medium">从这里开始</p>
          <p className="mt-2 text-sm text-muted-foreground">
            提交后系统会自动判断：有指定执行 Agent 时直接创建并派发任务；否则进入 planning 模式由 PM 澄清需求。
          </p>
        </div>
      </div>

      <div className="border-t px-4 py-3 shrink-0">
        {/* Selected files chips */}
        {selectedFiles.length > 0 && (
          <div className="flex flex-wrap gap-1.5 mb-2">
            {selectedFiles.map((f) => (
              <span
                key={f.id}
                className="inline-flex items-center gap-1 rounded-md bg-muted px-2 py-0.5 text-xs"
              >
                <File className="size-3 text-muted-foreground" />
                <span className="max-w-[140px] truncate">{f.file_name}</span>
                <button
                  type="button"
                  className="ml-0.5 rounded-full hover:bg-muted-foreground/20"
                  onClick={() => removeFile(f.id)}
                >
                  <X className="size-3" />
                </button>
              </span>
            ))}
          </div>
        )}

        <div className="flex items-end gap-2">
          <div className="flex-1">
            <TaskCommentComposer
              candidates={candidates}
              disabled={disabled || uploading}
              placeholder="描述需求，或 @ 执行 Agent 直接指派任务... (Enter 发送，Shift+Enter 换行)"
              onSubmit={handleSubmit}
            />
          </div>

          {/* Attach button */}
          <div ref={attachRef} className="relative shrink-0">
            <Button
              size="icon"
              variant="outline"
              className="size-9"
              onClick={() => setAttachOpen(!attachOpen)}
              disabled={disabled || uploading}
              title="添加文件"
            >
              <Paperclip className="size-4" />
            </Button>

            {attachOpen && (
              <div className="absolute bottom-full right-0 mb-2 w-44 rounded-lg border bg-popover shadow-lg p-1 z-50">
                <button
                  type="button"
                  className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm hover:bg-accent"
                  onClick={() => fileInputRef.current?.click()}
                >
                  <Upload className="size-4" />
                  从本地上传
                </button>
                {projectId && (
                  <button
                    type="button"
                    className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm hover:bg-accent"
                    onClick={() => {
                      setFilesPopoverOpen(true)
                      setAttachOpen(false)
                    }}
                  >
                    <FolderOpen className="size-4" />
                    从项目文件引用
                  </button>
                )}
              </div>
            )}

            <input
              ref={fileInputRef}
              type="file"
              className="hidden"
              onChange={handleUpload}
            />
          </div>
        </div>
      </div>

      {/* Project file reference popover */}
      {filesPopoverOpen && (
        <div className="fixed inset-0 z-50" onClick={() => setFilesPopoverOpen(false)}>
          <div
            className="absolute bottom-24 right-8 w-80 max-h-80 rounded-lg border bg-popover shadow-lg flex flex-col"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between px-3 py-2 border-b shrink-0">
              <span className="text-sm font-medium">项目文件</span>
              <button
                type="button"
                className="rounded-md p-0.5 hover:bg-muted"
                onClick={() => setFilesPopoverOpen(false)}
              >
                <X className="size-4" />
              </button>
            </div>
            <div className="flex-1 overflow-y-auto p-1.5 space-y-0.5">
              {                fileList.length === 0 ? (
                <p className="text-xs text-muted-foreground px-2 py-4 text-center">
                  暂无可用项目文件
                </p>
              ) : (
                fileList.map((f) => {
                  const checked = selectedFiles.some((sf) => sf.id === f.id)
                  const meta = sourceMeta(f.source)
                  return (
                    <label
                      key={f.id}
                      className={cn(
                        'flex items-center gap-2 px-2 py-1.5 rounded-md cursor-pointer text-sm',
                        checked ? 'bg-accent' : 'hover:bg-accent/60',
                      )}
                    >
                      <Checkbox
                        checked={checked}
                        onCheckedChange={() => toggleProjectFile(f)}
                      />
                      <File className="size-3.5 text-muted-foreground shrink-0" />
                      <span className="truncate">{f.file_name}</span>
                      <span
                        className={`shrink-0 text-[10px] leading-none px-1.5 py-0.5 rounded-full border ${meta.cls}`}
                      >
                        {meta.label}
                      </span>
                      <span className="text-xs text-muted-foreground ml-auto shrink-0">
                        {formatFileSize(f.file_size)}
                      </span>
                    </label>
                  )
                })
              )}
            </div>
            <div className="border-t px-3 py-2 shrink-0">
              <Button size="sm" className="w-full" onClick={() => setFilesPopoverOpen(false)}>
                确定
              </Button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}

export function TaskWorkspace(props: TaskWorkspaceProps) {
  const taskId = 'taskId' in props ? props.taskId : undefined
  const projectId = 'projectId' in props ? props.projectId : undefined
  const { data: task } = useTask(taskId)
  const { data: draftProject } = useProject(projectId)
  const [chatOpen, setChatOpen] = useState(false)
  const [resultOpen, setResultOpen] = useState(false)
  const [cancelDialogOpen, setCancelDialogOpen] = useState(false)
  const [rejectTodo, setRejectTodo] = useState<Todo | null>(null)
  const [rejectReason, setRejectReason] = useState('')
  const [reviewing, setReviewing] = useState(false)
  const planningScrollRef = useRef<HTMLDivElement>(null)
  const addComment = useAddTaskComment()
  const appendTaskMessage = useAppendTaskMessage()
  const createTaskFromText = useCreateTaskFromText()
  const approvePlan = useApprovePlan()
  const rejectPlan = useRejectPlan()
  const { data: allAgents } = useAgents()
  const canCancelTask = task?.status === 'planning' || task?.status === 'review' || task?.status === 'pending' || task?.status === 'in_progress'
  const mentionCandidates = buildTaskMentionCandidates(task)
  const isPlanning = task?.status === 'planning'
  const isReview = task?.status === 'review'
  const hasTaskThread = (task?.messages?.length ?? 0) > 0
  const mode: 'planning' | 'building' = task?.status === 'planning' || task?.status === 'review' || !task ? 'planning' : 'building'

  const pendingUIBlocks = useMemo(() => {
    if (!isPlanning || !task?.messages?.length) {
      return null
    }
    const lastMessage = task.messages[task.messages.length - 1]
    if (lastMessage.role === 'pm_agent' && lastMessage.ui_blocks && lastMessage.ui_blocks.length > 0) {
      return lastMessage.ui_blocks
    }
    return null
  }, [isPlanning, task?.messages])

  useEffect(() => {
    if (!isPlanning && !isReview) {
      return
    }
    planningScrollRef.current?.scrollTo({
      top: planningScrollRef.current.scrollHeight,
      behavior: 'smooth',
    })
  }, [isPlanning, isReview, task?.messages, pendingUIBlocks])

  const findNextUserResponse = (messages: TaskMessage[], index: number): TaskMessage | undefined => {
    if (index + 1 < messages.length && messages[index + 1].role === 'user') {
      return messages[index + 1]
    }
    return undefined
  }

  const handleReview = async (todo: Todo, action: 'approve' | 'reject') => {
    if (!taskId) {
      return
    }
    if (action === 'reject') {
      setRejectTodo(todo)
      setRejectReason('')
      return
    }
    setReviewing(true)
    try {
      await reviewTodo(taskId, todo.id, 'approve')
      toast.success(`${todo.title} 已确认通过`)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '操作失败，请稍后重试')
    } finally {
      setReviewing(false)
    }
  }

  const handleRejectConfirm = async () => {
    if (!taskId || !rejectTodo) return
    if (!rejectReason.trim()) {
      toast.error('请填写退回原因')
      return
    }
    setReviewing(true)
    try {
      await reviewTodo(taskId, rejectTodo.id, 'reject', rejectReason.trim())
      setRejectTodo(null)
      toast.success(`${rejectTodo.title} 已退回上一个 Todo 重做`)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '操作失败，请稍后重试')
    } finally {
      setReviewing(false)
    }
  }

  const handleSubmitComment = async ({ content, mentionAgentIds }: TaskCommentSubmitInput) => {
    if (!taskId) {
      return false
    }
    try {
      const response = await addComment.mutateAsync({ taskId, content, mentionAgentIds })
      const failedDeliveries = response.data.mention_deliveries?.filter((item) => item.status !== 'sent') ?? []

      if (failedDeliveries.length === 1) {
        toast.warning(`评论已发布，但 @${failedDeliveries[0].agent_name} 发送失败`)
      } else if (failedDeliveries.length > 1) {
        toast.warning(`评论已发布，但有 ${failedDeliveries.length} 个 Agent 未收到 mention`)
      }

      return true
    } catch (error) {
      const message = error instanceof ApiRequestError ? error.message : '发表评论失败'
      toast.error(message)
      return false
    }
  }

  const handleSendPlanningMessage = async (content: string, uiResponse?: UIResponse) => {
    if (!taskId) {
      return
    }
    try {
      await appendTaskMessage.mutateAsync({
        taskId,
        input: { content, ui_response: uiResponse },
      })
    } catch (error) {
      const message = error instanceof ApiRequestError ? error.message : '发送需求消息失败'
      toast.error(message)
    }
  }

  const handleApprovePlan = async () => {
    if (!taskId) return
    try {
      await approvePlan.mutateAsync({ taskId })
    } catch (error) {
      const message = error instanceof ApiRequestError ? error.message : '确认规划失败'
      toast.error(message)
    }
  }

  const handleRejectPlan = async (feedback: string) => {
    if (!taskId) return
    try {
      await rejectPlan.mutateAsync({ taskId, input: { feedback } })
    } catch (error) {
      const message = error instanceof ApiRequestError ? error.message : '提交修改意见失败'
      toast.error(message)
    }
  }

  const handleCreateTaskFromText = async (content: string, agentId?: string, fileIds?: string[], workflow?: Workflow) => {
    if (!projectId) {
      return
    }
    try {
      const res = await createTaskFromText.mutateAsync({ projectId, content, agentId, fileIds, workflow })
      props.onTaskCreated?.(res.data.id)
    } catch (error) {
      const message = error instanceof ApiRequestError ? error.message : '创建任务失败'
      toast.error(message)
    }
  }

  if (!taskId && projectId) {
    const executorCandidates: TaskMentionCandidate[] = (allAgents ?? [])
      .filter((a) => a.role !== 'pm')
      .map((a) => ({ id: a.id, name: a.name, roleLabel: '执行 Agent' }))

    const draftWorkflows = draftProject?.workflows ?? []

    return (
      <div className="flex flex-col h-full">
        <div className="flex items-center justify-between px-5 py-3 border-b shrink-0">
          <div className="flex items-center gap-2">
            <span className="inline-flex items-center rounded-full bg-info/10 px-2.5 py-1 text-xs font-medium text-info">
              Planning
            </span>
          </div>
          <Button variant="ghost" size="icon" className="size-7" onClick={props.onClose}>
            <X className="size-4" />
          </Button>
        </div>
        <DraftPlanningState
          projectId={projectId}
          onSubmit={handleCreateTaskFromText}
          disabled={createTaskFromText.isPending}
          candidates={executorCandidates}
          workflows={draftWorkflows}
        />
      </div>
    )
  }

  if (!task) {
    return (
      <div className="flex items-center justify-center h-full text-sm text-muted-foreground">
        加载中...
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between px-5 py-3 border-b shrink-0">
        <div className="flex items-center gap-2">
          <TaskStatusBadge status={task.status} />
          <PriorityBadge priority={task.priority} />
        </div>
        <div className="flex items-center gap-1">
          <Button
            variant="outline"
            size="sm"
            disabled={!canCancelTask}
            onClick={() => setCancelDialogOpen(true)}
          >
            终止任务
          </Button>
          <Button variant="ghost" size="icon" className="size-7" onClick={() => setResultOpen(true)} title="查看交付成果">
            <PackageCheck className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            onClick={() => setChatOpen(true)}
            title="查看需求对话"
            disabled={!hasTaskThread}
          >
            <MessageSquare className="size-4" />
          </Button>
          <Button variant="ghost" size="icon" className="size-7" onClick={props.onClose}>
            <X className="size-4" />
          </Button>
        </div>
      </div>

      {/* Task info */}
      <div className="px-5 py-4 shrink-0 border-b">
        <h2 className="text-lg font-semibold">{task.title}</h2>
        {task.description && (
          <TaskDescription description={task.description} />
        )}
        <AttachedFilesSection files={task.attached_files ?? []} projectId={task.project_id} />
        {task.cancel_reason && (
          <p className="mt-2 rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
            终止原因：{task.cancel_reason}
          </p>
        )}
      </div>

      {/* Feed */}
      <div className="flex-1 min-h-0">
        {isPlanning || isReview ? (
          <ScrollArea ref={planningScrollRef} className="h-full px-5 py-4">
            <div className="flex flex-col gap-4">
              {(task.messages ?? []).map((message, index, messages) => {
                const isLastMessage = index === messages.length - 1
                const hasPendingBlocks = isLastMessage && !!pendingUIBlocks
                return (
                  <MessageBubble
                    key={message.id}
                    message={message}
                    nextUserResponse={
                      message.role === 'pm_agent' && message.ui_blocks?.length
                        ? findNextUserResponse(messages, index)
                        : undefined
                    }
                    hideUIBlocks={hasPendingBlocks}
                  />
                )
              })}
              {isPlanning && task.messages && task.messages[task.messages.length - 1]?.role === 'user' && <ThinkingIndicator />}
              {isReview && (
                <PlanReviewPanel
                  todos={task.todos}
                  onApprove={handleApprovePlan}
                  onReject={handleRejectPlan}
                  isApproving={approvePlan.isPending}
                  isRejecting={rejectPlan.isPending}
                  workflow={task.workflow}
                />
              )}
            </div>
          </ScrollArea>
        ) : (
          <ScrollArea className="h-full px-5 py-4">
            <div className="flex flex-col gap-4">
              {task.todos.length > 0 && (
                <TaskTodoSection
                  todos={task.todos}
                  artifacts={task.artifacts ?? []}
                  defaultCollapsed={false}
                  onReviewTodo={handleReview}
                />
              )}
              <TaskFeed taskId={task.id} />
            </div>
          </ScrollArea>
        )}
      </div>

      {/* Comment input */}
      <div className="border-t px-4 py-3 shrink-0">
        {isReview ? null : (
          <TaskComposer
            mode={mode}
            disabled={mode === 'planning' ? appendTaskMessage.isPending : addComment.isPending}
            pendingUIBlocks={pendingUIBlocks}
            buildingCandidates={mentionCandidates}
            onPlanningSubmit={handleSendPlanningMessage}
            onBuildingSubmit={handleSubmitComment}
          />
        )}
      </div>

      <Sheet open={resultOpen} onOpenChange={setResultOpen}>
        <SheetContent className="w-full max-w-2xl! p-0">
          <SheetHeader className="px-4 py-3 border-b">
            <SheetTitle>交付成果</SheetTitle>
          </SheetHeader>
          <ScrollArea className="flex-1 min-h-0">
            <div className="p-4">
              <TaskResultView taskId={task.id} result={task.result} artifacts={task.artifacts} />
            </div>
          </ScrollArea>
        </SheetContent>
      </Sheet>
      {hasTaskThread ? (
        <TaskThreadSheet
          taskId={task.id}
          open={chatOpen}
          onOpenChange={setChatOpen}
        />
      ) : null}
      <CancelTaskDialog
        open={cancelDialogOpen}
        onOpenChange={setCancelDialogOpen}
        task={task}
      />

      <Dialog open={!!rejectTodo} onOpenChange={() => !reviewing && setRejectTodo(null)}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>退回重做</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">
            「{rejectTodo?.title}」的产出不通过，将退回上一个 Todo 重做，并级联重置后续 Todo。退回原因会直接写入前序智能体的重做指令，请给出具体的修改项（如「M-1 第 263 行：广寒宫→广寒弓」），智能体将据此逐条修复。
          </p>
          <Input
            value={rejectReason}
            onChange={(e) => setRejectReason(e.target.value)}
            placeholder="请填写退回原因（必填，将作为智能体的重做依据）"
            className="mt-2"
            disabled={reviewing}
          />
          <DialogFooter>
            <Button variant="outline" onClick={() => setRejectTodo(null)} disabled={reviewing}>
              取消
            </Button>
            <Button variant="destructive" onClick={handleRejectConfirm} disabled={reviewing}>
              {reviewing ? '处理中…' : '确认退回'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
