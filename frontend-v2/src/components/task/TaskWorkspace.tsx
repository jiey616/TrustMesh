import { useEffect, useMemo, useRef, useState } from 'react'
import { Badge, Button, Tag, Typography, Space, App, Empty, Drawer, Select, Skeleton, Popover } from 'antd'
import {
  CloseOutlined,
  PlusOutlined,
  SendOutlined,
  StopOutlined,
  LoadingOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  PlayCircleOutlined,
  PaperClipOutlined,
  MessageOutlined,
  QuestionCircleOutlined,
  SwapOutlined,
  FileTextOutlined,
  RobotOutlined,
  UserOutlined,
  FileDoneOutlined,
  DownloadOutlined,
  EyeOutlined,
  UnorderedListOutlined,
  ExclamationCircleOutlined,
} from '@ant-design/icons'
import dayjs from 'dayjs'
import {
  useTask,
  useCancelTask,
  useCreateTaskFromText,
  useTaskEvents,
  useTasks,
  useAppendTaskMessage,
  useAddTaskComment,
} from '@/hooks/useTasks'
import { useProject, useWorkflowProgress } from '@/hooks/useProjects'
import { useAgents } from '@/hooks/useAgents'
import { FileSelector } from '@/components/shared/FileSelector'
import { downloadProjectFile } from '@/api/projectFiles'
import { getTaskArtifactContent } from '@/api/tasks'
import { Markdown } from '@/components/task/Markdown'
import { stripReplyPrefix } from '@/lib/text'
import { TaskDescription } from '@/components/task/TaskDescription'
import { TaskResultView } from '@/components/task/TaskResultView'
import { TaskTodoPanel } from '@/components/task/TaskTodoPanel'
import { UIBlockRenderer } from '@/components/task/UIBlockRenderer'
import { PendingApprovalsDrawer } from '@/components/task/PendingApprovalsDrawer'
import { collectPendingItems, findPendingUIBlocks } from '@/lib/pendingItems'
import { usePendingStore } from '@/stores/pendingStore'
import { ThinkingIndicator } from '@/components/task/ThinkingIndicator'
import { TaskCommentComposer, type TaskMentionCandidate, type TaskCommentSubmitInput } from '@/components/task/TaskCommentComposer'
import { FileViewer } from '@/components/task/FileViewer'
import { AgentAvatar } from '@/components/shared/AgentAvatar'
import type { TaskMessage, Workflow, WorkflowStepProgress, Event, EventType, TaskDetail, UIResponse } from '@/types'

const { Title, Text } = Typography

const statusMap: Record<string, { color: string; label: string }> = {
  planning: { color: 'var(--signal)', label: '规划中' },
  review: { color: 'var(--warning)', label: '待确认' },
  pending: { color: 'var(--info)', label: '待处理' },
  in_progress: { color: 'var(--cyan)', label: '进行中' },
  awaiting_review: { color: 'var(--error)', label: '待人工确认' },
  waiting_user: { color: 'var(--error)', label: '等待用户' },
  done: { color: 'var(--success)', label: '已完成' },
  failed: { color: 'var(--error)', label: '失败' },
  canceled: { color: 'var(--text-quaternary)', label: '已取消' },
}

const priorityLabel: Record<string, string> = {
  low: '低',
  medium: '中',
  high: '高',
  urgent: '紧急',
}

// Partial：后端实际事件类型多于此处列举（如 todo_review_* / todo_timeout_* 等），
// 未覆盖的类型走消费处的 fallback，不强制穷尽。
const eventConfig: Partial<Record<EventType, { icon: React.ReactNode; color: string; label: string }>> = {
  task_created: { icon: <PlusOutlined />, color: 'var(--info)', label: '任务创建' },
  task_plan_ready: { icon: <RobotOutlined />, color: 'var(--info)', label: '规划完成' },
  task_status_changed: { icon: <SwapOutlined />, color: 'var(--warning)', label: '状态变更' },
  todo_assigned: { icon: <UserOutlined />, color: 'var(--info)', label: '分配 Todo' },
  todo_started: { icon: <PlayCircleOutlined />, color: 'var(--cyan)', label: '开始执行' },
  todo_progress: { icon: <LoadingOutlined />, color: 'var(--cyan)', label: '执行中' },
  todo_completed: { icon: <CheckCircleOutlined />, color: 'var(--success)', label: 'Todo 完成' },
  todo_failed: { icon: <CloseCircleOutlined />, color: 'var(--error)', label: 'Todo 失败' },
  task_comment: { icon: <MessageOutlined />, color: 'var(--text-tertiary)', label: '评论' },
  planning_reply: { icon: <MessageOutlined />, color: 'var(--signal)', label: 'PM 规划回复' },
  agent_status_changed: { icon: <RobotOutlined />, color: 'var(--warning)', label: '数字员工状态' },
  artifact_received: { icon: <PaperClipOutlined />, color: 'var(--success)', label: '交付文件' },
  todo_ask_received: { icon: <QuestionCircleOutlined />, color: 'var(--warning)', label: '请求确认' },
}

function formatFileSize(bytes: number | undefined): string {
  if (!bytes || bytes <= 0) return ''
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function downloadBlob(blob: Blob, fileName: string) {
  const objectURL = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = objectURL
  anchor.download = fileName
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  window.setTimeout(() => URL.revokeObjectURL(objectURL), 60_000)
}

function buildTaskMentionCandidates(task: TaskDetail | undefined): TaskMentionCandidate[] {
  if (!task) return []
  const seen = new Set<string>()
  const candidates: TaskMentionCandidate[] = []
  if (task.pm_agent.id && !seen.has(task.pm_agent.id)) {
    candidates.push({ id: task.pm_agent.id, name: task.pm_agent.name, roleLabel: 'PM 数字员工' })
    seen.add(task.pm_agent.id)
  }
  for (const todo of task.todos) {
    if (!todo.assignee?.agent_id || seen.has(todo.assignee.agent_id)) continue
    candidates.push({ id: todo.assignee.agent_id, name: todo.assignee.name, roleLabel: '执行数字员工' })
    seen.add(todo.assignee.agent_id)
  }
  return candidates
}

/* ============================================================
 *  Draft (new task) workspace
 * ============================================================ */

interface Props {
  taskId?: string
  projectId?: string
  onClose: () => void
  onTaskCreated?: (taskId: string) => void
  /** 默认 draft(新任务对话框)作为主界面时隐藏右上角关闭按钮 */
  closable?: boolean
  /** 空态「最近任务」点击后打开指定任务 */
  onOpenTask?: (taskId: string) => void
}

function deriveTitlePreview(content: string): string {
  const trimmed = content.trim()
  if (!trimmed) return ''
  const firstLine = trimmed.split('\n')[0].trim()
  return firstLine.length > 30 ? firstLine.slice(0, 30) + '…' : firstLine
}

const EXAMPLE_TEMPLATES = [
  '实现用户登录功能，支持邮箱注册、密码找回；登录后展示项目列表。',
  '审查项目现有代码，修复发现的缺陷并输出修改说明。',
  '分析需求文档，输出技术方案与分步实施计划。',
]

function DraftTaskWorkspace({
  projectId,
  onClose,
  onTaskCreated,
  closable = true,
  onOpenTask,
}: {
  projectId: string
  onClose: () => void
  onTaskCreated?: (taskId: string) => void
  closable?: boolean
  onOpenTask?: (taskId: string) => void
}) {
  const { message } = App.useApp()
  const createFromText = useCreateTaskFromText()
  const { data: project } = useProject(projectId)
  const { data: agents } = useAgents()
  const { data: recentTasks } = useTasks(projectId)
  const [content, setContent] = useState('')
  const [fileIds, setFileIds] = useState<string[]>([])
  const [workflowName, setWorkflowName] = useState<string | undefined>(undefined)
  const [toolsOpen, setToolsOpen] = useState(false)
  const [stepRange, setStepRange] = useState<{ from: number; to: number } | null>(null)

  const workflows = project?.workflows ?? []
  const selectedWorkflow = workflows.find((w) => w.name === workflowName)
  // 选中的工作流是否为项目总流程（决定是否展示"负责步骤段"选择）
  const isPrimaryWorkflow =
    project && project.primary_workflow_index >= 0 && Array.isArray(project.workflows)
      ? project.workflows[project.primary_workflow_index]?.name === workflowName
      : false
  const primarySteps = isPrimaryWorkflow ? (selectedWorkflow?.steps ?? []) : []
  const rangeValue = stepRange ?? { from: 0, to: Math.max(0, primarySteps.length - 1) }
  // @ 指派执行数字员工候选（排除 PM）
  const executorCandidates: TaskMentionCandidate[] = (agents ?? [])
    .filter((a) => a.role !== 'pm')
    .map((a) => ({ id: a.id, name: a.name, roleLabel: '执行数字员工' }))
  // 空态最近任务（取最新 5 个非已取消任务）
  const recentTaskItems = (recentTasks ?? [])
    .filter((t) => t.status !== 'canceled')
    .slice(0, 5)

  const handleSubmit = async ({ content: text, mentionAgentIds }: TaskCommentSubmitInput) => {
    if (!text.trim()) {
      message.error('请描述任务需求')
      return false
    }
    try {
      const res = await createFromText.mutateAsync({
        projectId,
        content: text.trim(),
        agent_id: mentionAgentIds[0],
        file_ids: fileIds.length > 0 ? fileIds : undefined,
        workflow: selectedWorkflow,
        workflow_index: isPrimaryWorkflow && project ? project.primary_workflow_index : undefined,
        step_from: isPrimaryWorkflow ? rangeValue.from : undefined,
        step_to: isPrimaryWorkflow ? rangeValue.to : undefined,
      })
      message.success('任务已创建')
      onTaskCreated?.((res.data as { id: string }).id)
      setFileIds([])
      setWorkflowName(undefined)
      setStepRange(null)
      return true
    } catch (err) {
      message.error(err instanceof Error ? err.message : '创建任务失败')
      return false
    }
  }

  const toolsContent = (
    <div style={{ width: 260 }}>
      {projectId && (
        <>
          <Text style={{ fontSize: 13, color: 'var(--text-tertiary)' }}>参考文件</Text>
          <div style={{ marginTop: 8 }}>
            <FileSelector
              projectId={projectId}
              selectedIds={fileIds}
              onToggle={(id) => setFileIds((prev) => (prev.includes(id) ? prev.filter((f) => f !== id) : [...prev, id]))}
              label="选择文件"
            />
          </div>
        </>
      )}
      {workflows.length > 0 && (
        <>
          {projectId && (
            <div style={{ height: 1, background: 'var(--surface-raised)', margin: '12px 0' }} />
          )}
          <Text style={{ fontSize: 13, color: 'var(--text-tertiary)' }}>工作流</Text>
          <Select
            style={{ width: '100%', marginTop: 8 }}
            value={workflowName}
            onChange={(v) => {
              setWorkflowName(v)
              setStepRange(null)
            }}
            placeholder="不选（PM 自由规划）"
            allowClear
            options={workflows.map((w: Workflow) => ({ value: w.name, label: w.name }))}
          />
          {isPrimaryWorkflow && primarySteps.length > 0 && (
            <>
              <Text style={{ fontSize: 12, color: 'var(--text-tertiary)', marginTop: 10, display: 'block' }}>
                负责总流程步骤
              </Text>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 6 }}>
                <Select
                  style={{ flex: 1, minWidth: 0 }}
                  size="small"
                  value={rangeValue.from}
                  onChange={(v: number) => setStepRange({ from: v, to: Math.max(v, rangeValue.to) })}
                  options={primarySteps.map((s, i) => ({ value: i, label: `步骤 ${i + 1} · ${s.name}` }))}
                />
                <Text type="secondary" style={{ fontSize: 12 }}>至</Text>
                <Select
                  style={{ flex: 1, minWidth: 0 }}
                  size="small"
                  value={rangeValue.to}
                  onChange={(v: number) => setStepRange({ from: Math.min(rangeValue.from, v), to: v })}
                  options={primarySteps.map((s, i) => ({ value: i, label: `步骤 ${i + 1} · ${s.name}` }))}
                />
              </div>
            </>
          )}
        </>
      )}
      {!projectId && workflows.length === 0 && (
        <Empty description="暂无可用选项" image={Empty.PRESENTED_IMAGE_SIMPLE} />
      )}
    </div>
  )

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '12px 16px', borderBottom: '1px solid var(--line)', flexShrink: 0 }}>
        <Space>
          <Title level={5} style={{ margin: 0, color: 'var(--text-primary)' }}>新任务</Title>
          <Tag color="cyan">Planning</Tag>
        </Space>
        {closable && <Button type="text" icon={<CloseOutlined />} onClick={onClose} />}
      </div>

      {/* 显示区:需求实时预览 */}
      <div style={{ flex: 1, overflowY: 'auto', padding: 16, display: 'flex', flexDirection: 'column', gap: 14 }}>
        {content.trim() ? (
          <>
            <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
              <div style={{ maxWidth: '80%', display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 8 }}>
                <div
                  style={{
                    padding: '10px 14px',
                    borderRadius: '14px 14px 4px 14px',
                    background: 'var(--signal-soft)',
                    border: '1px solid rgba(109,95,245,0.3)',
                    color: 'var(--text-primary)',
                    fontSize: 14,
                    lineHeight: 1.6,
                    whiteSpace: 'pre-wrap',
                    wordBreak: 'break-word',
                  }}
                >
                  {content}
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <Tag color="purple" style={{ margin: 0 }}>标题</Tag>
                  <Text style={{ color: 'var(--text-primary)', fontWeight: 600, fontSize: 15 }}>{deriveTitlePreview(content)}</Text>
                </div>
              </div>
            </div>
            <div style={{ textAlign: 'center', color: 'var(--text-quaternary)', fontSize: 13, marginTop: 4 }}>
              PM 数字员工将基于以上需求规划执行方案
            </div>
          </>
        ) : (
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 18, padding: '0 24px' }}>
            <div style={{ textAlign: 'center' }}>
              <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--text-secondary)' }}>描述你的需求，开始创建任务</div>
              <div style={{ fontSize: 13, color: 'var(--text-quaternary)', marginTop: 4 }}>由 PM 数字员工规划执行方案；输入 @ 可直接指派执行数字员工</div>
            </div>

            {/* 示例需求模板 */}
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6, width: '100%', maxWidth: 480 }}>
              <div style={{ fontSize: 12, color: 'var(--text-quaternary)', textAlign: 'center' }}>示例需求</div>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, justifyContent: 'center' }}>
                {EXAMPLE_TEMPLATES.map((tpl) => (
                  <button
                    key={tpl}
                    type="button"
                    onClick={() => setContent(tpl)}
                    style={{
                      fontSize: 13,
                      lineHeight: '20px',
                      padding: '3px 12px',
                      borderRadius: 'var(--radius-pill)',
                      border: '1px solid rgba(109,95,245,0.3)',
                      background: 'rgba(109,95,245,0.08)',
                      color: 'var(--signal-hover)',
                      cursor: 'pointer',
                      fontFamily: 'inherit',
                      transition: 'all 0.15s',
                      maxWidth: 320,
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {tpl}
                  </button>
                ))}
              </div>
            </div>

            {/* 最近任务快捷入口 */}
            {recentTaskItems.length > 0 && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 6, width: '100%', maxWidth: 480 }}>
                <div style={{ fontSize: 12, color: 'var(--text-quaternary)', textAlign: 'center' }}>最近任务</div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, justifyContent: 'center' }}>
                  {recentTaskItems.map((t) => {
                    const cfg = statusMap[t.status]
                    return (
                      <button
                        key={t.id}
                        type="button"
                        onClick={() => onOpenTask?.(t.id)}
                        style={{
                          display: 'inline-flex',
                          alignItems: 'center',
                          gap: 6,
                          fontSize: 13,
                          lineHeight: '22px',
                          padding: '2px 10px',
                          borderRadius: 'var(--radius-control)',
                          border: '1px solid var(--line-strong)',
                          background: 'var(--surface)',
                          color: 'var(--text-secondary)',
                          cursor: 'pointer',
                          fontFamily: 'inherit',
                          maxWidth: 240,
                        }}
                        title={t.title}
                      >
                        <span style={{ width: 6, height: 6, borderRadius: 'var(--radius-avatar)', background: cfg?.color ?? 'var(--surface-raised)', flexShrink: 0 }} />
                        <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{t.title}</span>
                      </button>
                    )
                  })}
                </div>
              </div>
            )}
          </div>
        )}
      </div>

      {/* 输入区:Codex 风格 Composer */}
      <div style={{ borderTop: '1px solid var(--line)', background: 'var(--surface)', padding: '12px 16px', flexShrink: 0 }}>
        <TaskCommentComposer
          candidates={executorCandidates}
          disabled={createFromText.isPending}
          placeholder="描述需求，或 @ 执行数字员工直接指派任务… (Enter 发送，Shift+Enter 换行)"
          onSubmit={handleSubmit}
          value={content}
          onValueChange={setContent}
          leadingAccessory={
            <Popover
              content={toolsContent}
              title="附加选项"
              trigger="click"
              open={toolsOpen}
              onOpenChange={setToolsOpen}
              placement="topLeft"
              overlayStyle={{ maxWidth: 300 }}
            >
              <Button shape="circle" icon={<PlusOutlined />} size="small" title="附加文件 / 选择工作流" />
            </Popover>
          }
        />

        {(fileIds.length > 0 || workflowName) && (
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 10, flexWrap: 'wrap' }}>
            {fileIds.length > 0 && (
              <Tag color="purple" style={{ margin: 0 }}>{fileIds.length} 个参考文件</Tag>
            )}
            {workflowName && (
              <Tag color="cyan" style={{ margin: 0 }}>{workflowName}</Tag>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

/* ============================================================
 *  Message bubble (planning thread)
 * ============================================================ */

function findNextUserResponse(messages: TaskMessage[], index: number): TaskMessage | undefined {
  if (index + 1 < messages.length && messages[index + 1].role === 'user') {
    return messages[index + 1]
  }
  return undefined
}

function MessageBubble({ message, pmName, pmSeed, nextUserResponse, hideUIBlocks }: {
  message: TaskMessage
  pmName?: string
  pmSeed?: string
  nextUserResponse?: TaskMessage
  hideUIBlocks?: boolean
}) {
  const role = message.role
  const isUser = role === 'user'
  const isPM = role === 'pm_agent'
  const hasUIBlocks = !isUser && !hideUIBlocks && message.ui_blocks && message.ui_blocks.length > 0

  if (role === 'system') {
    return (
      <div style={{ textAlign: 'center', margin: '8px 0' }}>
        <Text type="secondary" style={{ fontSize: 13 }}>{message.content}</Text>
      </div>
    )
  }

  return (
    <div style={{ display: 'flex', gap: 8, justifyContent: isUser ? 'flex-end' : 'flex-start' }}>
      {!isUser && (
        <AgentAvatar
          name={pmName || (isPM ? 'PM 数字员工' : '数字员工')}
          role={isPM ? 'pm' : 'custom'}
          seed={pmSeed || pmName || message.id}
          size={28}
        />
      )}
      <div style={{ maxWidth: '80%', display: 'flex', flexDirection: 'column', alignItems: isUser ? 'flex-end' : 'flex-start' }}>
        {!isUser && (
          <Text type="secondary" style={{ fontSize: 12, marginBottom: 2 }}>
            {isPM ? (pmName || 'PM 数字员工') : role === 'agent' ? '数字员工' : role}
          </Text>
        )}
        <div
          style={{
            padding: '8px 12px',
            borderRadius: isUser ? '14px 14px 4px 14px' : '14px 14px 14px 4px',
            background: isUser ? 'var(--signal-soft)' : 'var(--surface-raised)',
            border: `1px solid ${isUser ? 'var(--signal-border)' : 'var(--line)'}`,
            color: 'var(--text-primary)',
            fontSize: 14,
            lineHeight: 1.6,
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
          }}
        >
          {typeof message.content === 'string' && message.content ? (
            isUser ? message.content : <Markdown content={stripReplyPrefix(message.content)} size="small" />
          ) : (
            '…'
          )}
          {hasUIBlocks && message.ui_blocks && (
            <UIBlockRenderer blocks={message.ui_blocks} responses={nextUserResponse?.ui_response?.blocks} />
          )}
        </div>
        <Text type="secondary" style={{ fontSize: 12, marginTop: 2 }}>
          {dayjs(message.created_at).fromNow()}
        </Text>
      </div>
    </div>
  )
}

/* ============================================================
 *  待确认事项入口提示
 * ============================================================ */

/**
 * 确认类交互已统一收到右上角「待确认」抽屉，原位置只留一个入口。
 * 留提示是为了避免用户面对一个「看着像卡住了、却不知道该点哪」的界面。
 */
function PendingHint({ text, onOpen }: { text: string; onOpen: () => void }) {
  return (
    <div
      style={{
        borderRadius: 'var(--radius-control)',
        border: '1px solid rgba(245,158,11,0.25)',
        background: 'rgba(245,158,11,0.05)',
        padding: '10px 12px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: 10,
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 0 }}>
        <ExclamationCircleOutlined style={{ color: 'var(--warning)', fontSize: 14, flexShrink: 0 }} />
        <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>{text}</span>
      </div>
      <Button size="small" type="primary" onClick={onOpen} style={{ flexShrink: 0 }}>去处理</Button>
    </div>
  )
}

/* ============================================================
 *  Execution event feed
 * ============================================================ */

function ArtifactActions({ taskId, transferId, fileName }: { taskId: string; transferId: string; fileName: string }) {
  const { message } = App.useApp()
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
      message.error(err instanceof Error ? err.message : '打开文件失败')
    } finally {
      setLoading(false)
    }
  }

  const handleDownload = async () => {
    setDownloading(true)
    try {
      const blob = await getTaskArtifactContent(taskId, transferId)
      downloadBlob(blob, fileName)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '下载文件失败')
    } finally {
      setDownloading(false)
    }
  }

  return (
    <>
      <div style={{ display: 'flex', gap: 2, flexShrink: 0 }}>
        <Button type="text" size="small" icon={loading ? <LoadingOutlined /> : <EyeOutlined />} onClick={handlePreview} />
        <Button type="text" size="small" icon={downloading ? <LoadingOutlined /> : <DownloadOutlined />} onClick={handleDownload} />
      </div>
      <FileViewer open={viewerOpen} onOpenChange={setViewerOpen} blob={viewerBlob} fileName={fileName} onDownload={handleDownload} />
    </>
  )
}

function shouldGroupWithPrev(current: Event, prev: Event | undefined): boolean {
  if (!prev) return false
  if (current.actor_id !== prev.actor_id) return false
  if (current.event_type !== prev.event_type) return false
  const diffMs = new Date(current.created_at).getTime() - new Date(prev.created_at).getTime()
  return diffMs < 2 * 60 * 1000 // 2 分钟内
}

/** todo_ask_received 事件：已回答 → 展示答案；未回答 → 提示去右上角待确认抽屉 */
function TodoAskBlock({ event }: { event: Event }) {
  const setPendingOpen = usePendingStore((s) => s.setOpen)
  const questionId = event.metadata?.question_id as string | undefined
  const question = (event.metadata?.question as string | undefined) || event.content || '数字员工请求你的确认'
  const answer = event.metadata?.answer as string | undefined
  const timedOut = event.metadata?.timed_out as boolean | undefined

  return (
    <div style={{ marginTop: 4 }}>
      <div style={{ fontSize: 14, color: 'var(--text-primary)', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
        {question}
      </div>
      {answer != null ? (
        <div
          style={{
            marginTop: 6,
            borderRadius: 'var(--radius-control)',
            border: '1px solid rgba(39,166,68,0.35)',
            background: 'rgba(39,166,68,0.08)',
            padding: '6px 10px',
            fontSize: 13,
            color: 'var(--success)',
            display: 'inline-flex',
            alignItems: 'center',
            gap: 4,
          }}
        >
          <CheckCircleOutlined /> 已确认：{answer}
          {timedOut && <span style={{ color: 'var(--text-tertiary)', marginLeft: 4 }}>（超时未答，数字员工自行决断）</span>}
        </div>
      ) : questionId ? (
        <PendingHint text="数字员工正在等待你的回复" onOpen={() => setPendingOpen(true)} />
      ) : null}
    </div>
  )
}

/** 事件流左侧头像：agent 用 AgentAvatar，user/system 用通用图标 */
function ActorAvatar({ event, size = 28 }: { event: Event; size?: number }) {
  if (event.actor_type === 'agent') {
    return (
      <AgentAvatar
        name={event.actor_name || '数字员工'}
        role="custom"
        seed={event.actor_id || event.actor_name || 'agent'}
        size={size}
      />
    )
  }
  if (event.actor_type === 'user') {
    return (
      <div
        style={{
          width: size,
          height: size,
          borderRadius: 'var(--radius-avatar)',
          flexShrink: 0,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          background: 'rgba(109,95,245,0.2)',
          border: '1px solid rgba(109,95,245,0.35)',
          color: 'var(--signal-hover)',
          fontSize: size * 0.5,
        }}
      >
        <UserOutlined />
      </div>
    )
  }
  return (
    <div
      style={{
        width: size,
        height: size,
        borderRadius: 'var(--radius-avatar)',
        flexShrink: 0,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: 'var(--surface-raised)',
        border: '1px solid var(--line-strong)',
        color: 'var(--text-tertiary)',
        fontSize: size * 0.5,
      }}
    >
      <RobotOutlined />
    </div>
  )
}

function ExecutionEventItem({ event, showHeader = true }: { event: Event; showHeader?: boolean }) {
  const cfg = eventConfig[event.event_type] ?? { icon: <MessageOutlined />, color: 'var(--text-tertiary)', label: event.event_type }
  const fromStatus = event.metadata?.from as string | undefined
  const toStatus = event.metadata?.to as string | undefined
  const fileName = event.metadata?.file_name as string | undefined
  const fileSize = event.metadata?.file_size as number | undefined
  const transferId = event.metadata?.transfer_id as string | undefined
  const todoTitle = event.metadata?.todo_title as string | undefined
  const error = event.metadata?.error as string | undefined

  const isTaskComment = event.event_type === 'task_comment'
  const isPlanningReply = event.event_type === 'planning_reply'
  const isTodoAsk = event.event_type === 'todo_ask_received'

  return (
    <div style={{ display: 'flex', gap: 10 }}>
      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
        {showHeader && <ActorAvatar event={event} />}
        <span style={{ width: 1, flex: 1, background: 'var(--surface-raised)', marginTop: showHeader ? 4 : 0 }} />
      </div>
      <div style={{ flex: 1, minWidth: 0, paddingBottom: showHeader ? 14 : 8 }}>
        {showHeader && (
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <span style={{ color: cfg.color, fontSize: 13 }}>{cfg.icon}</span>
            <Text style={{ fontSize: 13, fontWeight: 500, color: 'var(--text-secondary)' }}>{cfg.label}</Text>
            {event.actor_name && <Text type="secondary" style={{ fontSize: 12 }}>· {event.actor_name}</Text>}
            <Text type="secondary" style={{ fontSize: 12 }} title={dayjs(event.created_at).format('YYYY-MM-DD HH:mm:ss')}>
              · {dayjs(event.created_at).fromNow()}
            </Text>
          </div>
        )}

        {(isTaskComment || isPlanningReply) && typeof event.content === 'string' && event.content && (
          <div style={{ marginTop: 4 }}>
            <Markdown content={stripReplyPrefix(event.content)} size="small" />
          </div>
        )}

        {!isTaskComment && !isPlanningReply && !isTodoAsk && typeof event.content === 'string' && event.content && (
          <div style={{ fontSize: 13, color: 'var(--text-secondary)', marginTop: 3, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
            {stripReplyPrefix(event.content)}
          </div>
        )}

        {isTodoAsk && <TodoAskBlock event={event} />}

        {todoTitle && event.event_type !== 'task_comment' && !isTodoAsk && (
          <div style={{ fontSize: 13, color: 'var(--text-secondary)', marginTop: 3 }}>
            {event.event_type === 'todo_completed' ? <s>{todoTitle}</s> : todoTitle}
          </div>
        )}

        {error && (
          <div style={{ fontSize: 13, color: 'var(--error)', marginTop: 3, whiteSpace: 'pre-wrap' }}>{error}</div>
        )}

        {fromStatus && toStatus && (
          <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 4, fontSize: 12 }}>
            <Tag style={{ margin: 0 }}>{statusMap[fromStatus]?.label ?? fromStatus}</Tag>
            <SwapOutlined style={{ color: 'var(--text-quaternary)', fontSize: 12 }} />
            <Tag color="cyan" style={{ margin: 0 }}>{statusMap[toStatus]?.label ?? toStatus}</Tag>
          </div>
        )}

        {fileName && (
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 4, fontSize: 13, color: 'var(--cyan)', background: 'var(--surface)', border: '1px solid var(--line)', borderRadius: 'var(--radius-control)', padding: '6px 8px' }}>
            <FileTextOutlined />
            <span style={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{fileName}</span>
            <span style={{ color: 'var(--text-quaternary)', fontSize: 12 }}>{formatFileSize(fileSize)}</span>
            {transferId && event.task_id && (
              <ArtifactActions taskId={event.task_id} transferId={transferId} fileName={fileName} />
            )}
          </div>
        )}
      </div>
    </div>
  )
}

/* ============================================================
 *  Attached files section
 * ============================================================ */

function AttachedFileItem({ file, projectId }: { file: { id: string; file_name: string; file_size: number; mime_type: string; source: string }; projectId: string }) {
  const { message } = App.useApp()
  const [downloading, setDownloading] = useState(false)

  const handleDownload = async () => {
    setDownloading(true)
    try {
      const blob = await downloadProjectFile(projectId, file.id)
      downloadBlob(blob, file.file_name)
    } catch {
      message.error(`下载 ${file.file_name} 失败`)
    } finally {
      setDownloading(false)
    }
  }

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '4px 10px', borderRadius: 'var(--radius-control)', background: 'var(--surface)', border: '1px solid var(--line)', fontSize: 14 }}>
      <FileTextOutlined style={{ color: 'var(--text-tertiary)', flexShrink: 0 }} />
      <span style={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', color: 'var(--text-primary)' }}>{file.file_name}</span>
      <span style={{ flexShrink: 0, fontSize: 12, color: 'var(--text-quaternary)' }}>{file.source === 'agent_artifact' ? '数字员工产物' : file.source === 'meeting_minutes' ? '会议纪要' : '上传文件'}</span>
      <span style={{ flexShrink: 0, fontSize: 12, color: 'var(--text-quaternary)' }}>{formatFileSize(file.file_size)}</span>
      <Button type="text" size="small" icon={downloading ? <LoadingOutlined /> : <DownloadOutlined />} onClick={handleDownload} title="下载文件" />
    </div>
  )
}

function AttachedFilesSection({ files, projectId }: { files: Array<{ id: string; file_name: string; file_size: number; mime_type: string; source: string }>; projectId: string }) {
  if (!files || files.length === 0) return null
  return (
    <div style={{ marginTop: 10, display: 'flex', flexDirection: 'column', gap: 4 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13, color: 'var(--text-tertiary)' }}>
        <PaperClipOutlined style={{ fontSize: 12 }} />
        <span>附加文件 ({files.length})</span>
      </div>
      {files.map((f) => (
        <AttachedFileItem key={f.id} file={f} projectId={projectId} />
      ))}
    </div>
  )
}

/* ============================================================
 *  Main workspace
 * ============================================================ */

/**
 * 任务完成后的「进入下一流程」常驻横幅（可收起）：
 * 仅当任务是总流程切片任务、状态为 done（failed/canceled 不弹）、
 * 且总流程中该切片之后还有未编排（unassigned）的连续步骤时显示。
 * 支持选择下一任务覆盖的步骤范围（默认连续未编排段全部），确认后
 * 按切片创建新任务 → PM 自动开始规划 → 跳转到新任务。
 */
function NextFlowBanner({ task, onTaskCreated }: { task: TaskDetail; onTaskCreated?: (taskId: string) => void }) {
  const { message } = App.useApp()
  const projectId = task.project_id
  const { data: project } = useProject(projectId)
  const { data: progress } = useWorkflowProgress(projectId)
  const { data: projectTasks } = useTasks(projectId)
  const createFromText = useCreateTaskFromText()
  const [collapsed, setCollapsed] = useState(false)
  const [endIdx, setEndIdx] = useState<number | null>(null)

  const primaryWf =
    project && project.primary_workflow_index >= 0 && Array.isArray(project.workflows)
      ? project.workflows[project.primary_workflow_index]
      : undefined
  const ref = task.workflow_ref
  const sameWf = !!ref && !!primaryWf && primaryWf.name === ref.workflow_name

  // 总流程中该任务实际执行到的位置之后、尚未开始的连续步骤段。
  // 不能只看 workflow_ref.step_to：PM 可能用 deliver_scope 截断了计划
  //（实测 2026-09-04：切片声明 0-5，实际只执行到 3），且未开始的步骤
  // 状态是 pending（被主任务绑住）而非 unassigned。
  // 归属判断：绑在已 done 的任务上的 pending 步骤（声明了但永远不会再
  // 执行，实测分镜视频生成绑在旧 done 任务上）也视为可认领。
  const boundStatusById = useMemo(
    () => new Map((projectTasks ?? []).map((t) => [t.id, t.status])),
    [projectTasks],
  )
  const run: WorkflowStepProgress[] = useMemo(() => {
    if (!sameWf || !progress?.steps?.length || !ref) return []
    const byIndex = new Map(progress.steps.map((s) => [s.index, s]))
    // 从切片起点逐格确认本任务连续 done 到哪一步
    let execEnd = ref.step_from - 1
    for (let i = Math.max(ref.step_from, 0); ; i++) {
      const s = byIndex.get(i)
      if (!s || s.task_id !== task.id || s.status !== 'done') break
      execEnd = i
    }
    // 其后连续的 pending/unassigned 且归属可认领的步骤 = 下一流程候选
    const out: WorkflowStepProgress[] = []
    for (let i = execEnd + 1; ; i++) {
      const s = byIndex.get(i)
      if (!s) break
      const notStarted = s.status === 'pending' || s.status === 'unassigned'
      const bound = s.task_id ? boundStatusById.get(s.task_id) : undefined
      const freeOrOurs =
        !s.task_id || s.task_id === task.id || bound === 'done'
      if (!notStarted || !freeOrOurs) break
      out.push(s)
    }
    return out
  }, [sameWf, progress, ref, task.id, boundStatusById])

  if (task.status !== 'done' || run.length === 0) return null

  if (collapsed) {
    return (
      <div style={{ padding: '6px 16px 0', flexShrink: 0 }}>
        <Button size="small" type="text" icon={<UnorderedListOutlined />} onClick={() => setCollapsed(false)}>
          后续流程（{run.length} 步未开始）
        </Button>
      </div>
    )
  }

  const slice = run.filter((s) => s.index <= (endIdx ?? run[run.length - 1].index))
  const rangeOptions = run.map((s, k) => ({
    value: s.index,
    label: run.slice(0, k + 1).map((x) => x.name).join(' → '),
  }))

  const handleStart = async () => {
    if (!project || !primaryWf) return
    try {
      const res = await createFromText.mutateAsync({
        projectId,
        content: `继续项目总流程：执行「${slice.map((s) => s.name).join('、')}」步骤`,
        workflow: primaryWf,
        workflow_index: project.primary_workflow_index,
        step_from: slice[0].index,
        step_to: slice[slice.length - 1].index,
      })
      message.success('下一流程任务已创建，PM 开始规划')
      const newId = (res.data as { id: string }).id
      if (newId) onTaskCreated?.(newId)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '创建任务失败')
    }
  }

  return (
    <div
      style={{
        margin: '8px 16px 0',
        padding: '8px 12px',
        borderRadius: 'var(--radius-control)',
        border: '1px solid rgba(16,185,129,0.4)',
        background: 'rgba(16,185,129,0.14)',
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        flexWrap: 'wrap',
        flexShrink: 0,
      }}
    >
      <CheckCircleOutlined style={{ color: 'var(--success)', flexShrink: 0 }} />
      <Text style={{ fontSize: 13, color: 'var(--text-primary)' }}>
        任务完成，总流程后续还有 {run.length} 步未开始
      </Text>
      <Select size="small" style={{ minWidth: 180 }} value={slice[slice.length - 1].index} onChange={setEndIdx} options={rangeOptions} />
      <Button size="small" type="primary" loading={createFromText.isPending} onClick={handleStart}>
        开始下一流程
      </Button>
      <Button size="small" type="text" icon={<CloseOutlined />} aria-label="收起" onClick={() => setCollapsed(true)} />
    </div>
  )
}

export function TaskWorkspace({ taskId, projectId, onClose, onTaskCreated, closable = true, onOpenTask }: Props) {
  // Draft (new task) mode
  if (!taskId && projectId) {
    return (
      <DraftTaskWorkspace projectId={projectId} onClose={onClose} onTaskCreated={onTaskCreated} closable={closable} onOpenTask={onOpenTask} />
    )
  }

  const { message } = App.useApp()
  const { data: task, isLoading } = useTask(taskId)
  const { data: events } = useTaskEvents(taskId)
  const cancelTask = useCancelTask()
  const appendMessage = useAppendTaskMessage()
  const addComment = useAddTaskComment()
  const [input, setInput] = useState('')
  const [showCancel, setShowCancel] = useState(false)
  const [resultOpen, setResultOpen] = useState(false)
  const [todoOpen, setTodoOpen] = useState(false)
  const feedRef = useRef<HTMLDivElement>(null)

  const isPlanningMode = task ? ['planning', 'review'].includes(task.status) : false
  const activeStatus = task && !['done', 'failed', 'canceled'].includes(task.status)
  const mentionCandidates = useMemo(() => buildTaskMentionCandidates(task), [task])
  const todoStats = useMemo(() => {
    const todos = task?.todos ?? []
    return { done: todos.filter((t) => t.status === 'done').length, total: todos.length }
  }, [task])

  // 从最新消息向前找「最近一条未被用户回复的」pm_agent ui_blocks 消息。
  // 判定逻辑与待确认抽屉共用 lib/pendingItems 的同一份实现，避免两处漂移。
  const pendingUIBlocks = useMemo(() => findPendingUIBlocks(task)?.blocks ?? null, [task])
  const pendingItems = useMemo(() => collectPendingItems(task, events), [task, events])

  const pendingOpen = usePendingStore((s) => s.open)
  const setPendingOpen = usePendingStore((s) => s.setOpen)

  useEffect(() => {
    feedRef.current?.scrollTo({ top: feedRef.current.scrollHeight, behavior: 'smooth' })
  }, [task?.messages?.length, task?.todos?.length, events?.length, pendingUIBlocks])

  const handleSendPlanning = async (content: string, uiResponse?: UIResponse) => {
    if (!taskId || !content.trim()) return
    try {
      await appendMessage.mutateAsync({ taskId, content: content.trim(), uiResponse })
    } catch {
      message.error('发送失败，请稍后重试')
    }
  }

  const handleSubmitComment = async ({ content, mentionAgentIds }: TaskCommentSubmitInput): Promise<boolean> => {
    if (!taskId) return false
    try {
      const res = await addComment.mutateAsync({
        taskId,
        content,
        mentions: mentionAgentIds.length > 0 ? mentionAgentIds.map((agent_id) => ({ agent_id })) : undefined,
      })
      const data = res.data as { mention_deliveries?: Array<{ agent_name: string; status: string }> }
      const failedDeliveries = data.mention_deliveries?.filter((item) => item.status !== 'sent') ?? []
      if (failedDeliveries.length === 1) {
        message.warning(`评论已发布，但 @${failedDeliveries[0].agent_name} 发送失败`)
      } else if (failedDeliveries.length > 1) {
        message.warning(`评论已发布，但有 ${failedDeliveries.length} 个数字员工未收到 mention`)
      }
      return true
    } catch (err) {
      message.error(err instanceof Error ? err.message : '发表评论失败')
      return false
    }
  }

  const handleCancel = async () => {
    if (!taskId) return
    try {
      await cancelTask.mutateAsync({ taskId, reason: '用户手动终止' })
      message.success('任务已终止')
      setShowCancel(false)
    } catch {
      message.error('取消失败')
    }
  }

  if (isLoading) {
    return <Skeleton active paragraph={{ rows: 8 }} style={{ padding: 16 }} />
  }

  if (!task) {
    return (
      <div style={{ padding: 16, color: 'var(--text-tertiary)' }}>
        任务加载失败
        <Button type="link" onClick={onClose}>关闭</Button>
      </div>
    )
  }

  const messages = task.messages ?? []
  const isReview = task.status === 'review'

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      {/* Header */}
      <div style={{ padding: '10px 16px', borderBottom: '1px solid var(--line)', display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8, flexShrink: 0 }}>
        <div style={{ minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span
              style={{
                width: 8,
                height: 8,
                borderRadius: 'var(--radius-avatar)',
                background: statusMap[task.status]?.color,
                boxShadow: task.status === 'in_progress' ? `0 0 6px ${statusMap[task.status]?.color}` : 'none',
                flexShrink: 0,
              }}
            />
            <Title level={5} style={{ margin: 0, color: 'var(--text-primary)', fontSize: 17 }} ellipsis={{ tooltip: task.title }}>{task.title}</Title>
          </div>
          <Space size={6} style={{ marginTop: 6 }} wrap>
            <Tag color={statusMap[task.status]?.color} style={{ margin: 0, fontSize: 12 }}>{statusMap[task.status]?.label}</Tag>
            <Tag color="blue" style={{ margin: 0, fontSize: 12 }}>{priorityLabel[task.priority] ?? task.priority}</Tag>
            {task.workflow && (
              <Tag color="gold" style={{ margin: 0, fontSize: 12 }}>工作流：{task.workflow.name}</Tag>
            )}
            <Text type="secondary" style={{ fontSize: 12 }}>PM: {task.pm_agent?.name || '—'}</Text>
            {task.cancel_reason && (
              <Text type="secondary" style={{ fontSize: 12 }}>终止: {task.cancel_reason}</Text>
            )}
          </Space>

          {/* Task info */}
          {task.description && <TaskDescription description={task.description} />}
          <AttachedFilesSection files={task.attached_files ?? []} projectId={task.project_id} />
        </div>
        <Space style={{ flexShrink: 0 }}>
          {activeStatus && !showCancel && (
            <Button danger size="small" icon={<StopOutlined />} onClick={() => setShowCancel(true)}>终止任务</Button>
          )}
          {!isPlanningMode && todoStats.total > 0 && (
            <Button
              size="small"
              icon={<UnorderedListOutlined />}
              title="查看执行清单"
              onClick={() => setTodoOpen(true)}
            >
              执行清单 {todoStats.done}/{todoStats.total}
            </Button>
          )}
          {pendingItems.length > 0 && (
            <Badge count={pendingItems.length} size="small">
              <Button
                size="small"
                icon={<ExclamationCircleOutlined />}
                style={{ borderColor: 'var(--warning)', color: 'var(--warning)' }}
                onClick={() => setPendingOpen(true)}
              >
                待确认
              </Button>
            </Badge>
          )}
          <Button size="small" icon={<FileDoneOutlined />} title="查看交付成果" onClick={() => setResultOpen(true)} />
          <Button type="text" icon={<CloseOutlined />} onClick={onClose} />
        </Space>
      </div>

      {/* 任务完成且总流程还有后续未编排步骤时，提示进入下一流程（可收起） */}
      <NextFlowBanner task={task} onTaskCreated={onTaskCreated} />

      {/* Cancel confirm */}
      {showCancel && (
        <div style={{ padding: '10px 16px', borderBottom: '1px solid var(--line)', background: 'rgba(244,63,94,0.06)', display: 'flex', flexDirection: 'column', gap: 8, flexShrink: 0 }}>
          <Text type="secondary" style={{ fontSize: 13 }}>
            终止后任务将停止接收后续进度和结果回写，未完成的 Todo 会一并标记为已取消。
          </Text>
          <Space>
            <Button size="small" onClick={() => setShowCancel(false)}>取消</Button>
            <Button size="small" type="primary" danger onClick={handleCancel} loading={cancelTask.isPending}>确认终止</Button>
          </Space>
        </div>
      )}

      {/* Feed */}
      <div ref={feedRef} style={{ flex: 1, overflowY: 'auto', padding: 16, display: 'flex', flexDirection: 'column', gap: 14 }}>
        {/* Planning/review: dialogue */}
        {(isPlanningMode) && (
          <>
            {messages.length > 0 && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 2 }}>
                  <MessageOutlined style={{ color: 'var(--signal)', fontSize: 13 }} />
                  <Text style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-secondary)' }}>需求对话</Text>
                </div>
                {messages.map((m, i, arr) => (
                  <MessageBubble
                    key={m.id}
                    message={m}
                    pmName={task.pm_agent?.name}
                    pmSeed={task.pm_agent?.node_id}
                    nextUserResponse={m.role === 'pm_agent' && m.ui_blocks?.length ? findNextUserResponse(arr, i) : undefined}
                    hideUIBlocks={i === arr.length - 1 && !!pendingUIBlocks}
                  />
                ))}
              </div>
            )}
            {task.status === 'planning' && messages[messages.length - 1]?.role === 'user' && <ThinkingIndicator />}
            {isReview && <PendingHint text="PM 已完成规划，等待你确认方案" onOpen={() => setPendingOpen(true)} />}
            {messages.length === 0 && (
              <Empty description="等待 PM 规划需求…" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            )}
          </>
        )}

        {/* Building: todos + events */}
        {!isPlanningMode && (
          <>
            {events && events.length > 0 && (
              <div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 10 }}>
                  <PaperClipOutlined style={{ color: 'var(--warning)', fontSize: 13 }} />
                  <Text style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-secondary)' }}>执行过程</Text>
                </div>
                <div>
                  {events.map((ev, i) => {
                    const prev = i > 0 ? events[i - 1] : undefined
                    const showHeader = !shouldGroupWithPrev(ev, prev)
                    return <ExecutionEventItem key={ev.id} event={ev} showHeader={showHeader} />
                  })}
                </div>
              </div>
            )}

            {messages.length === 0 && (!events || events.length === 0) && (
              <Empty
                description={todoStats.total > 0 ? '执行过程暂无内容，可点右上角「执行清单」查看步骤进度' : '暂无执行过程'}
                image={Empty.PRESENTED_IMAGE_SIMPLE}
              />
            )}
          </>
        )}
      </div>

      {/* Composer */}
      <div style={{ borderTop: '1px solid var(--line)', background: 'var(--surface)', padding: '10px 16px', flexShrink: 0 }}>
        {isPlanningMode ? (
          pendingUIBlocks ? (
            <div style={{ maxWidth: 560, margin: '0 auto' }}>
              <PendingHint text="PM 有澄清问题等你回答" onOpen={() => setPendingOpen(true)} />
            </div>
          ) : (
            <div style={{ display: 'flex', gap: 8, alignItems: 'flex-end' }}>
              <textarea
                value={input}
                onChange={(e) => setInput(e.target.value)}
                placeholder="继续补充需求或回答 PM 的问题… (Enter 发送)"
                rows={1}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault()
                    if (input.trim()) {
                      handleSendPlanning(input)
                      setInput('')
                    }
                  }
                }}
                style={{
                  flex: 1,
                  background: 'var(--surface-inset)',
                  border: '1px solid var(--line-strong)',
                  borderRadius: 'var(--radius-control)',
                  padding: '8px 12px',
                  color: 'var(--text-primary)',
                  fontSize: 14,
                  lineHeight: 1.6,
                  resize: 'none',
                  outline: 'none',
                  maxHeight: 120,
                  fontFamily: 'inherit',
                }}
              />
              <Button
                type="primary"
                shape="circle"
                icon={<SendOutlined />}
                disabled={!input.trim()}
                loading={appendMessage.isPending}
                onClick={() => {
                  handleSendPlanning(input)
                  setInput('')
                }}
              />
            </div>
          )
        ) : (
          <TaskCommentComposer
            candidates={mentionCandidates}
            disabled={addComment.isPending}
            onSubmit={handleSubmitComment}
            placeholder="评论任务 / @ 提醒数字员工… (Enter 发送)"
          />
        )}
      </div>

      {/* 执行清单 Drawer */}
      <Drawer
        title={`执行清单 (${todoStats.done}/${todoStats.total})`}
        open={todoOpen}
        onClose={() => setTodoOpen(false)}
        width={520}
        styles={{ body: { background: 'var(--canvas)', paddingTop: 8 } }}
        style={{ background: 'var(--canvas)' }}
      >
        {task && <TaskTodoPanel task={task} />}
      </Drawer>

      {/* 待确认抽屉：方案确认 / 规划澄清 / 执行提问 / 成果审核 */}
      <PendingApprovalsDrawer
        open={pendingOpen}
        onClose={() => setPendingOpen(false)}
        items={pendingItems}
      />

      {/* 交付成果 Drawer */}
      <Drawer
        title="交付成果"
        open={resultOpen}
        onClose={() => setResultOpen(false)}
        width={560}
        styles={{ body: { background: 'var(--canvas)', paddingTop: 8 } }}
        style={{ background: 'var(--canvas)' }}
      >
        <TaskResultView
          taskId={task.id}
          result={task.result}
          artifacts={task.artifacts ?? []}
          workflow={task.workflow ?? null}
          todos={task.todos ?? []}
        />
      </Drawer>

    </div>
  )
}
