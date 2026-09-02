import { useEffect, useMemo, useRef, useState } from 'react'
import { Button, Tag, Typography, Space, App, Empty, Drawer, Modal, Input, Select, Skeleton, Popover } from 'antd'
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
  useApprovePlan,
  useRejectPlan,
  useReviewTodo,
  useAnswerTodo,
} from '@/hooks/useTasks'
import { useProject } from '@/hooks/useProjects'
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
import { UIResponsePanel } from '@/components/task/UIResponsePanel'
import { ThinkingIndicator } from '@/components/task/ThinkingIndicator'
import { TaskCommentComposer, type TaskMentionCandidate, type TaskCommentSubmitInput } from '@/components/task/TaskCommentComposer'
import { FileViewer } from '@/components/task/FileViewer'
import { AgentAvatar } from '@/components/shared/AgentAvatar'
import type { Todo, TaskMessage, Workflow, Event, EventType, TaskDetail, UIResponse } from '@/types'

const { Title, Text } = Typography

const statusMap: Record<string, { color: string; label: string }> = {
  planning: { color: '#8b5cf6', label: '规划中' },
  review: { color: '#f59e0b', label: '待确认' },
  pending: { color: '#3b82f6', label: '待处理' },
  in_progress: { color: '#22d3ee', label: '进行中' },
  awaiting_review: { color: '#f43f5e', label: '待人工确认' },
  waiting_user: { color: '#f43f5e', label: '等待用户' },
  done: { color: '#10b981', label: '已完成' },
  failed: { color: '#ef4444', label: '失败' },
  canceled: { color: '#6b7280', label: '已取消' },
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
  task_created: { icon: <PlusOutlined />, color: '#3b82f6', label: '任务创建' },
  task_plan_ready: { icon: <RobotOutlined />, color: '#3b82f6', label: '规划完成' },
  task_status_changed: { icon: <SwapOutlined />, color: '#f59e0b', label: '状态变更' },
  todo_assigned: { icon: <UserOutlined />, color: '#3b82f6', label: '分配 Todo' },
  todo_started: { icon: <PlayCircleOutlined />, color: '#22d3ee', label: '开始执行' },
  todo_progress: { icon: <LoadingOutlined />, color: '#22d3ee', label: '执行中' },
  todo_completed: { icon: <CheckCircleOutlined />, color: '#10b981', label: 'Todo 完成' },
  todo_failed: { icon: <CloseCircleOutlined />, color: '#ef4444', label: 'Todo 失败' },
  task_comment: { icon: <MessageOutlined />, color: '#8b8f9e', label: '评论' },
  planning_reply: { icon: <MessageOutlined />, color: '#6d5ff5', label: 'PM 规划回复' },
  agent_status_changed: { icon: <RobotOutlined />, color: '#f59e0b', label: '数字员工状态' },
  artifact_received: { icon: <PaperClipOutlined />, color: '#10b981', label: '交付文件' },
  todo_ask_received: { icon: <QuestionCircleOutlined />, color: '#f59e0b', label: '请求确认' },
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
          <Text style={{ fontSize: 13, color: 'rgba(255,255,255,0.5)' }}>参考文件</Text>
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
            <div style={{ height: 1, background: 'rgba(255,255,255,0.08)', margin: '12px 0' }} />
          )}
          <Text style={{ fontSize: 13, color: 'rgba(255,255,255,0.5)' }}>工作流</Text>
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
              <Text style={{ fontSize: 12, color: 'rgba(255,255,255,0.5)', marginTop: 10, display: 'block' }}>
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
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '12px 16px', borderBottom: '1px solid rgba(255,255,255,0.08)', flexShrink: 0 }}>
        <Space>
          <Title level={5} style={{ margin: 0, color: '#fff' }}>新任务</Title>
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
                    background: 'linear-gradient(135deg, rgba(109,95,245,0.25), rgba(99,102,241,0.2))',
                    border: '1px solid rgba(109,95,245,0.3)',
                    color: 'rgba(255,255,255,0.9)',
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
                  <Text style={{ color: '#f4f4f8', fontWeight: 600, fontSize: 15 }}>{deriveTitlePreview(content)}</Text>
                </div>
              </div>
            </div>
            <div style={{ textAlign: 'center', color: 'rgba(255,255,255,0.3)', fontSize: 13, marginTop: 4 }}>
              PM 数字员工将基于以上需求规划执行方案
            </div>
          </>
        ) : (
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 18, padding: '0 24px' }}>
            <div style={{ textAlign: 'center' }}>
              <div style={{ fontSize: 15, fontWeight: 600, color: 'rgba(255,255,255,0.75)' }}>描述你的需求，开始创建任务</div>
              <div style={{ fontSize: 13, color: 'rgba(255,255,255,0.4)', marginTop: 4 }}>由 PM 数字员工规划执行方案；输入 @ 可直接指派执行数字员工</div>
            </div>

            {/* 示例需求模板 */}
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6, width: '100%', maxWidth: 480 }}>
              <div style={{ fontSize: 12, color: 'rgba(255,255,255,0.4)', textAlign: 'center' }}>示例需求</div>
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
                      borderRadius: 999,
                      border: '1px solid rgba(109,95,245,0.3)',
                      background: 'rgba(109,95,245,0.08)',
                      color: '#8b7ff8',
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
                <div style={{ fontSize: 12, color: 'rgba(255,255,255,0.4)', textAlign: 'center' }}>最近任务</div>
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
                          borderRadius: 8,
                          border: '1px solid rgba(255,255,255,0.1)',
                          background: 'rgba(255,255,255,0.03)',
                          color: 'rgba(255,255,255,0.7)',
                          cursor: 'pointer',
                          fontFamily: 'inherit',
                          maxWidth: 240,
                        }}
                        title={t.title}
                      >
                        <span style={{ width: 6, height: 6, borderRadius: '50%', background: cfg?.color ?? '#94a3b8', flexShrink: 0 }} />
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
      <div style={{ borderTop: '1px solid rgba(255,255,255,0.08)', background: 'rgba(255,255,255,0.03)', padding: '12px 16px', flexShrink: 0 }}>
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
            background: isUser ? 'linear-gradient(135deg, rgba(109,95,245,0.25), rgba(99,102,241,0.2))' : 'rgba(255,255,255,0.06)',
            border: `1px solid ${isUser ? 'rgba(109,95,245,0.3)' : 'rgba(255,255,255,0.08)'}`,
            color: 'rgba(255,255,255,0.9)',
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
 *  Plan review panel
 * ============================================================ */

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

  return (
    <div style={{ borderRadius: 14, border: '1px solid rgba(245,158,11,0.25)', background: 'rgba(245,158,11,0.04)', padding: 16, display: 'flex', flexDirection: 'column', gap: 12 }}>
      <div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
          <span style={{ fontSize: 15, fontWeight: 600, color: '#f4f4f8' }}>PM 已完成规划，请确认后开始执行</span>
          {workflow && workflow.steps.length > 0 && (
            <Tag color="gold" style={{ margin: 0, fontSize: 12 }}>工作流：{workflow.name || '未命名'}</Tag>
          )}
        </div>
        <div style={{ fontSize: 13, color: 'rgba(255,255,255,0.5)', marginTop: 4 }}>共 {todos.length} 个子任务</div>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        {todos.map((todo, idx) => (
          <div key={todo.id} style={{ display: 'flex', alignItems: 'flex-start', gap: 8, borderRadius: 8, background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.05)', padding: '8px 10px', fontSize: 14 }}>
            <span style={{ flexShrink: 0, fontSize: 13, color: 'rgba(255,255,255,0.45)', paddingTop: 1 }}>{idx + 1}.</span>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ fontWeight: 500, color: '#f4f4f8' }}>{todo.title}</div>
              {todo.description && (
                <div style={{ fontSize: 13, color: 'rgba(255,255,255,0.55)', marginTop: 2, whiteSpace: 'pre-wrap' }}>{todo.description}</div>
              )}
            </div>
            <span style={{ flexShrink: 0, fontSize: 13, color: 'rgba(255,255,255,0.5)', paddingTop: 1 }}>{todo.assignee?.name ?? ''}</span>
          </div>
        ))}
      </div>

      {showRejectInput ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <Input.TextArea
            value={feedback}
            onChange={(e) => setFeedback(e.target.value)}
            placeholder="说明需要调整的地方..."
            rows={3}
            style={{ background: 'rgba(0,0,0,0.25)', borderColor: 'rgba(255,255,255,0.12)', color: '#fff', resize: 'none', fontSize: 14 }}
            autoFocus
          />
          <div style={{ display: 'flex', gap: 8 }}>
            <Button
              size="small"
              danger
              type="primary"
              disabled={!feedback.trim() || isRejecting}
              loading={isRejecting}
              onClick={() => onReject(feedback.trim())}
            >
              提交修改意见
            </Button>
            <Button size="small" onClick={() => { setShowRejectInput(false); setFeedback('') }}>取消</Button>
          </div>
        </div>
      ) : (
        <div style={{ display: 'flex', gap: 8 }}>
          <Button size="small" type="primary" loading={isApproving} onClick={onApprove}>确认执行</Button>
          <Button size="small" onClick={() => setShowRejectInput(true)}>修改规划</Button>
        </div>
      )}
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

/* ---------- todo.ask 用户确认交互（执行页面） ---------- */

/** 可交互确认块：选项按钮 + 自由文本输入，提交后展示已提交状态 */
function InteractiveQuestionBlock({ taskId, todoId, questionId, options }: {
  taskId: string
  todoId: string
  questionId: string
  options: string[]
}) {
  const { message } = App.useApp()
  const answerTodo = useAnswerTodo()
  const [value, setValue] = useState('')
  const [submitted, setSubmitted] = useState(false)

  const submit = async (answer: string) => {
    if (!answer.trim() || answerTodo.isPending) return
    try {
      await answerTodo.mutateAsync({ taskId, todoId, questionId, answer: answer.trim() })
      setSubmitted(true)
      setValue('')
      message.success('已提交，数字员工继续执行中')
    } catch (err) {
      message.error(err instanceof Error ? err.message : '提交失败，请稍后重试')
    }
  }

  if (submitted) {
    return (
      <div
        style={{
          marginTop: 6,
          borderRadius: 10,
          border: '1px solid rgba(39,166,68,0.35)',
          background: 'rgba(39,166,68,0.08)',
          padding: '6px 10px',
          fontSize: 13,
          color: '#6dc67f',
        }}
      >
        <CheckCircleOutlined /> 已提交，数字员工继续执行中
      </div>
    )
  }

  return (
    <div
      style={{
        marginTop: 6,
        borderRadius: 12,
        border: '1px solid rgba(109,95,245,0.35)',
        background: 'rgba(109,95,245,0.06)',
        padding: 12,
        display: 'flex',
        flexDirection: 'column',
        gap: 8,
      }}
    >
      <div style={{ fontSize: 12, fontWeight: 600, color: '#a5b4fc', letterSpacing: 0.5 }}>需要你确认</div>
      {options.length > 0 && (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
          {options.map((opt) => (
            <button
              key={opt}
              type="button"
              disabled={answerTodo.isPending}
              onClick={() => void submit(opt)}
              style={{
                height: 28,
                padding: '0 12px',
                borderRadius: 8,
                border: '1px solid rgba(109,95,245,0.5)',
                background: 'rgba(109,95,245,0.1)',
                color: '#a5b4fc',
                fontSize: 13,
                cursor: 'pointer',
                transition: 'background 0.12s ease',
              }}
              onMouseEnter={(e) => { e.currentTarget.style.background = 'rgba(109,95,245,0.2)' }}
              onMouseLeave={(e) => { e.currentTarget.style.background = 'rgba(109,95,245,0.1)' }}
            >
              {opt}
            </button>
          ))}
        </div>
      )}
      <div style={{ display: 'flex', gap: 8 }}>
        <Input
          size="small"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onPressEnter={() => void submit(value.trim())}
          disabled={answerTodo.isPending}
          placeholder={options.length > 0 ? '或输入自定义回答…' : '输入你的回答…'}
          style={{ flex: 1 }}
        />
        <Button
          size="small"
          type="primary"
          icon={<SendOutlined />}
          disabled={!value.trim() || answerTodo.isPending}
          loading={answerTodo.isPending}
          onClick={() => void submit(value.trim())}
        >
          提交
        </Button>
      </div>
    </div>
  )
}

/** todo_ask_received 事件：已回答 → 展示答案；未回答 → 可交互确认块 */
function TodoAskBlock({ event }: { event: Event }) {
  const questionId = event.metadata?.question_id as string | undefined
  const question = (event.metadata?.question as string | undefined) || event.content || '数字员工请求你的确认'
  const options = (event.metadata?.options as string[] | undefined) ?? []
  const todoId = (event.metadata?.todo_id as string | undefined) || event.todo_id || ''
  const answer = event.metadata?.answer as string | undefined
  const timedOut = event.metadata?.timed_out as boolean | undefined

  return (
    <div style={{ marginTop: 4 }}>
      <div style={{ fontSize: 14, color: 'rgba(255,255,255,0.85)', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
        {question}
      </div>
      {answer != null ? (
        <div
          style={{
            marginTop: 6,
            borderRadius: 10,
            border: '1px solid rgba(39,166,68,0.35)',
            background: 'rgba(39,166,68,0.08)',
            padding: '6px 10px',
            fontSize: 13,
            color: '#6dc67f',
            display: 'inline-flex',
            alignItems: 'center',
            gap: 4,
          }}
        >
          <CheckCircleOutlined /> 已确认：{answer}
          {timedOut && <span style={{ color: 'rgba(255,255,255,0.45)', marginLeft: 4 }}>（超时未答，数字员工自行决断）</span>}
        </div>
      ) : (
        questionId &&
        event.task_id && (
          <InteractiveQuestionBlock
            taskId={event.task_id}
            todoId={todoId}
            questionId={questionId}
            options={options}
          />
        )
      )}
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
          borderRadius: '50%',
          flexShrink: 0,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          background: 'rgba(109,95,245,0.2)',
          border: '1px solid rgba(109,95,245,0.35)',
          color: '#8b7ff8',
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
        borderRadius: '50%',
        flexShrink: 0,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: 'rgba(255,255,255,0.06)',
        border: '1px solid rgba(255,255,255,0.1)',
        color: 'rgba(255,255,255,0.5)',
        fontSize: size * 0.5,
      }}
    >
      <RobotOutlined />
    </div>
  )
}

function ExecutionEventItem({ event, showHeader = true }: { event: Event; showHeader?: boolean }) {
  const cfg = eventConfig[event.event_type] ?? { icon: <MessageOutlined />, color: '#8b8f9e', label: event.event_type }
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
        <span style={{ width: 1, flex: 1, background: 'rgba(255,255,255,0.08)', marginTop: showHeader ? 4 : 0 }} />
      </div>
      <div style={{ flex: 1, minWidth: 0, paddingBottom: showHeader ? 14 : 8 }}>
        {showHeader && (
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <span style={{ color: cfg.color, fontSize: 13 }}>{cfg.icon}</span>
            <Text style={{ fontSize: 13, fontWeight: 500, color: 'rgba(255,255,255,0.75)' }}>{cfg.label}</Text>
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
          <div style={{ fontSize: 13, color: 'rgba(255,255,255,0.6)', marginTop: 3, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
            {stripReplyPrefix(event.content)}
          </div>
        )}

        {isTodoAsk && <TodoAskBlock event={event} />}

        {todoTitle && event.event_type !== 'task_comment' && !isTodoAsk && (
          <div style={{ fontSize: 13, color: 'rgba(255,255,255,0.65)', marginTop: 3 }}>
            {event.event_type === 'todo_completed' ? <s>{todoTitle}</s> : todoTitle}
          </div>
        )}

        {error && (
          <div style={{ fontSize: 13, color: '#f87171', marginTop: 3, whiteSpace: 'pre-wrap' }}>{error}</div>
        )}

        {fromStatus && toStatus && (
          <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 4, fontSize: 12 }}>
            <Tag style={{ margin: 0 }}>{statusMap[fromStatus]?.label ?? fromStatus}</Tag>
            <SwapOutlined style={{ color: 'rgba(255,255,255,0.4)', fontSize: 12 }} />
            <Tag color="cyan" style={{ margin: 0 }}>{statusMap[toStatus]?.label ?? toStatus}</Tag>
          </div>
        )}

        {fileName && (
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 4, fontSize: 13, color: '#22d3ee', background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.06)', borderRadius: 8, padding: '6px 8px' }}>
            <FileTextOutlined />
            <span style={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{fileName}</span>
            <span style={{ color: 'rgba(255,255,255,0.4)', fontSize: 12 }}>{formatFileSize(fileSize)}</span>
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
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '4px 10px', borderRadius: 8, background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.06)', fontSize: 14 }}>
      <FileTextOutlined style={{ color: 'rgba(255,255,255,0.45)', flexShrink: 0 }} />
      <span style={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', color: 'rgba(255,255,255,0.85)' }}>{file.file_name}</span>
      <span style={{ flexShrink: 0, fontSize: 12, color: 'rgba(255,255,255,0.4)' }}>{file.source === 'agent_artifact' ? '数字员工产物' : file.source === 'meeting_minutes' ? '会议纪要' : '上传文件'}</span>
      <span style={{ flexShrink: 0, fontSize: 12, color: 'rgba(255,255,255,0.4)' }}>{formatFileSize(file.file_size)}</span>
      <Button type="text" size="small" icon={downloading ? <LoadingOutlined /> : <DownloadOutlined />} onClick={handleDownload} title="下载文件" />
    </div>
  )
}

function AttachedFilesSection({ files, projectId }: { files: Array<{ id: string; file_name: string; file_size: number; mime_type: string; source: string }>; projectId: string }) {
  if (!files || files.length === 0) return null
  return (
    <div style={{ marginTop: 10, display: 'flex', flexDirection: 'column', gap: 4 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13, color: 'rgba(255,255,255,0.45)' }}>
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
  const approvePlan = useApprovePlan()
  const rejectPlan = useRejectPlan()
  const reviewTodo = useReviewTodo()
  const [input, setInput] = useState('')
  const [showCancel, setShowCancel] = useState(false)
  const [resultOpen, setResultOpen] = useState(false)
  const [rejectTodo, setRejectTodo] = useState<Todo | null>(null)
  const [rejectReason, setRejectReason] = useState('')
  const feedRef = useRef<HTMLDivElement>(null)

  const isPlanningMode = task ? ['planning', 'review'].includes(task.status) : false
  const activeStatus = task && !['done', 'failed', 'canceled'].includes(task.status)
  const mentionCandidates = useMemo(() => buildTaskMentionCandidates(task), [task])

  const pendingUIBlocks = useMemo(() => {
    if (!task || task.status !== 'planning' || !task.messages?.length) return null
    // 从最新消息向前找「最近一条未被用户回复的」pm_agent ui_blocks 消息。
    // 不能只看最后一条：PM 发出结构化提问后，常会再补一条无 blocks 的状态
    // 消息（如「已发送澄清请求…WAITING」），旧逻辑会让回复面板被顶掉，
    // 用户从此无法通过 UI 回复（实测卡死规划澄清阶段）。
    for (let i = task.messages.length - 1; i >= 0; i--) {
      const msg = task.messages[i]
      if (!msg) continue
      if (msg.role === 'user') return null // 用户已回复 → 该轮提问已解决
      if (msg.role === 'pm_agent' && msg.ui_blocks && msg.ui_blocks.length > 0) {
        return msg.ui_blocks
      }
      // 其他消息（pm 的状态跟进 / system 等）：继续向前找
    }
    return null
  }, [task])

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

  const handleApprovePlan = async () => {
    if (!taskId) return
    try {
      await approvePlan.mutateAsync({ taskId })
    } catch {
      message.error('确认规划失败')
    }
  }

  const handleRejectPlan = async (feedback: string) => {
    if (!taskId) return
    try {
      await rejectPlan.mutateAsync({ taskId, feedback })
    } catch {
      message.error('提交修改意见失败')
    }
  }

  const handleReviewTodo = async (todo: Todo, action: 'approve' | 'reject') => {
    if (!taskId) return
    if (action === 'reject') {
      setRejectTodo(todo)
      setRejectReason('')
      return
    }
    try {
      await reviewTodo.mutateAsync({ taskId, todoId: todo.id, action: 'approve' })
      message.success(`${todo.title} 已确认通过`)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败，请稍后重试')
    }
  }

  const handleRejectConfirm = async () => {
    if (!taskId || !rejectTodo) return
    if (!rejectReason.trim()) {
      message.error('请填写退回原因')
      return
    }
    try {
      await reviewTodo.mutateAsync({ taskId, todoId: rejectTodo.id, action: 'reject', reason: rejectReason.trim() })
      setRejectTodo(null)
      message.success(`${rejectTodo.title} 已退回上一个 Todo 重做`)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败，请稍后重试')
    }
  }

  if (isLoading) {
    return <Skeleton active paragraph={{ rows: 8 }} style={{ padding: 16 }} />
  }

  if (!task) {
    return (
      <div style={{ padding: 16, color: 'rgba(255,255,255,0.5)' }}>
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
      <div style={{ padding: '10px 16px', borderBottom: '1px solid rgba(255,255,255,0.08)', display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8, flexShrink: 0 }}>
        <div style={{ minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span
              style={{
                width: 8,
                height: 8,
                borderRadius: '50%',
                background: statusMap[task.status]?.color,
                boxShadow: task.status === 'in_progress' ? `0 0 6px ${statusMap[task.status]?.color}` : 'none',
                flexShrink: 0,
              }}
            />
            <Title level={5} style={{ margin: 0, color: '#fff', fontSize: 17 }} ellipsis={{ tooltip: task.title }}>{task.title}</Title>
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
          <Button size="small" icon={<FileDoneOutlined />} title="查看交付成果" onClick={() => setResultOpen(true)} />
          <Button type="text" icon={<CloseOutlined />} onClick={onClose} />
        </Space>
      </div>

      {/* Cancel confirm */}
      {showCancel && (
        <div style={{ padding: '10px 16px', borderBottom: '1px solid rgba(255,255,255,0.08)', background: 'rgba(244,63,94,0.06)', display: 'flex', flexDirection: 'column', gap: 8, flexShrink: 0 }}>
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
                  <MessageOutlined style={{ color: '#6d5ff5', fontSize: 13 }} />
                  <Text style={{ fontSize: 13, fontWeight: 600, color: 'rgba(255,255,255,0.6)' }}>需求对话</Text>
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
            {messages.length === 0 && (
              <Empty description="等待 PM 规划需求…" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            )}
          </>
        )}

        {/* Building: todos + events */}
        {!isPlanningMode && (
          <>
            {task.todos.length > 0 && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <CheckCircleOutlined style={{ color: '#22d3ee', fontSize: 13 }} />
                  <Text style={{ fontSize: 13, fontWeight: 600, color: 'rgba(255,255,255,0.6)' }}>
                    执行清单 ({task.todos.filter((t) => t.status === 'done').length}/{task.todos.length})
                  </Text>
                </div>
                <TaskTodoPanel task={task} onReviewTodo={handleReviewTodo} />
              </div>
            )}

            {events && events.length > 0 && (
              <div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 10 }}>
                  <PaperClipOutlined style={{ color: '#f59e0b', fontSize: 13 }} />
                  <Text style={{ fontSize: 13, fontWeight: 600, color: 'rgba(255,255,255,0.6)' }}>执行过程</Text>
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

            {messages.length === 0 && task.todos.length === 0 && (!events || events.length === 0) && (
              <Empty description="暂无执行过程" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            )}
          </>
        )}
      </div>

      {/* Composer */}
      <div style={{ borderTop: '1px solid rgba(255,255,255,0.08)', background: 'rgba(255,255,255,0.03)', padding: '10px 16px', flexShrink: 0 }}>
        {isPlanningMode ? (
          pendingUIBlocks ? (
            <div style={{ maxWidth: 560, margin: '0 auto' }}>
              <UIResponsePanel
                blocks={pendingUIBlocks}
                onSubmit={(content, uiResponse) => handleSendPlanning(content, uiResponse)}
                disabled={appendMessage.isPending}
              />
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
                  background: 'rgba(0,0,0,0.25)',
                  border: '1px solid rgba(255,255,255,0.1)',
                  borderRadius: 12,
                  padding: '8px 12px',
                  color: '#fff',
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

      {/* 交付成果 Drawer */}
      <Drawer
        title="交付成果"
        open={resultOpen}
        onClose={() => setResultOpen(false)}
        width={560}
        styles={{ body: { background: '#0a0a14', paddingTop: 8 } }}
        style={{ background: '#0a0a14' }}
      >
        <TaskResultView taskId={task.id} result={task.result} artifacts={task.artifacts ?? []} />
      </Drawer>

      {/* 退回重做 Dialog */}
      <Modal
        title="退回重做"
        open={!!rejectTodo}
        onCancel={() => setRejectTodo(null)}
        footer={[
          <Button key="cancel" onClick={() => setRejectTodo(null)}>取消</Button>,
          <Button key="ok" danger type="primary" loading={reviewTodo.isPending} onClick={handleRejectConfirm}>确认退回</Button>,
        ]}
      >
        <Text type="secondary" style={{ fontSize: 14 }}>
          「{rejectTodo?.title}」的产出不通过，将退回上一个 Todo 重做，并级联重置后续 Todo。退回原因将作为数字员工的重做依据。
        </Text>
        <Input.TextArea
          value={rejectReason}
          onChange={(e) => setRejectReason(e.target.value)}
          placeholder="请填写退回原因（必填，将作为数字员工的重做依据）"
          rows={3}
          style={{ marginTop: 12, background: 'rgba(0,0,0,0.25)', borderColor: 'rgba(255,255,255,0.12)', color: '#fff', resize: 'none', fontSize: 14 }}
        />
      </Modal>
    </div>
  )
}
