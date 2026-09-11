import { useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, Tabs, Tag, Typography, Space, Skeleton, App, Empty, Timeline, Dropdown, Modal, Progress } from 'antd'
import {
  ArrowLeftOutlined,
  EditOutlined,
  DeleteOutlined,
  InboxOutlined,
  CopyOutlined,
  CheckOutlined,
  ThunderboltOutlined,
  MessageOutlined,
  FileTextOutlined,
  HistoryOutlined,
  ToolOutlined,
  PlusOutlined,
  MoreOutlined,
  LoadingOutlined,
  MinusCircleOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  StopOutlined,
} from '@ant-design/icons'
import dayjs from 'dayjs'
import {
  useAgent,
  useAgentStats,
  useAgentInsights,
  useAgentTasks,
  useAgentEvents,
  useDeleteAgent,
} from '@/hooks/useAgents'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'
import { NeonBadge } from '@/components/NeonBadge'
import { AgentChatPanel } from '@/components/agent/AgentChatPanel'
import { AgentOverview } from '@/components/agent/AgentOverview'
import { AgentAvatar } from '@/components/shared/AgentAvatar'
import { AgentConfigModal } from '@/components/agent/AgentConfigModal'
import { ArchiveAgentModal } from '@/components/agent/ArchiveAgentModal'
import { HermesSkillsTab, HermesModelsTab, HermesJobsTab } from '@/components/agent/HermesCapabilityTab'
import { CreateTaskModal } from '@/components/task/CreateTaskModal'
import { stripReplyPrefix } from '@/lib/text'
import type { Event, EventType, TaskStatus } from '@/types'

const { Title, Text } = Typography

const roleMap: Record<string, string> = {
  pm: 'PM',
  developer: '开发者',
  reviewer: '审核者',
  custom: '自定义',
}

const statusMap: Record<string, { label: string }> = {
  online: { label: '在线' },
  offline: { label: '离线' },
  busy: { label: '忙碌' },
}

const productBadgeColor: Record<string, string> = {
  trustmesh: 'default',
  hermes: 'cyan',
  opc: 'green',
  openclaw: 'orange',
}

// Partial：后端实际事件类型多于此处列举（如 todo_review_* / todo_timeout_* 等），
// 未覆盖的类型走消费处的 fallback，不强制穷尽。
const eventConfig: Partial<Record<EventType, { color: string; label: string }>> = {
  task_created: { color: 'var(--info)', label: '任务创建' },
  task_plan_ready: { color: 'var(--signal)', label: '规划完成' },
  task_status_changed: { color: 'var(--warning)', label: '状态变更' },
  todo_assigned: { color: 'var(--info)', label: '分配 Todo' },
  todo_started: { color: 'var(--cyan)', label: '开始执行' },
  todo_progress: { color: 'var(--text-quaternary)', label: '执行中' },
  todo_completed: { color: 'var(--success)', label: 'Todo 完成' },
  todo_failed: { color: 'var(--error)', label: 'Todo 失败' },
  task_comment: { color: 'var(--text-quaternary)', label: '评论' },
  planning_reply: { color: 'var(--signal)', label: 'PM 规划回复' },
  agent_status_changed: { color: 'var(--warning)', label: '数字员工状态' },
  artifact_received: { color: 'var(--cyan)', label: '上传了文件' },
  todo_ask_received: { color: 'var(--warning)', label: '请求确认' },
}

const eventTypeFilters: { label: string; value: EventType | 'all' }[] = [
  { label: '全部', value: 'all' },
  { label: '任务', value: 'task_created' },
  { label: 'Todo', value: 'todo_completed' },
  { label: '失败', value: 'todo_failed' },
  { label: '进度', value: 'todo_progress' },
]

const taskStatusFilters: { label: string; value: TaskStatus | 'all' }[] = [
  { label: '全部', value: 'all' },
  { label: '规划中', value: 'planning' },
  { label: '进行中', value: 'in_progress' },
  { label: '待处理', value: 'pending' },
  { label: '已完成', value: 'done' },
  { label: '失败', value: 'failed' },
  { label: '已取消', value: 'canceled' },
]

const taskStatusIcon: Record<TaskStatus, { icon: React.ComponentType<{ className?: string }>; className: string }> = {
  planning: { icon: MessageOutlined, className: 'text-[color:var(--signal)]' },
  review: { icon: MessageOutlined, className: 'text-[color:var(--warning)]' },
  pending: { icon: MinusCircleOutlined, className: 'text-[color:var(--text-quaternary)]' },
  in_progress: { icon: LoadingOutlined, className: 'text-[color:var(--info)]' },
  awaiting_review: { icon: MessageOutlined, className: 'text-[color:var(--error)]' },
  waiting_user: { icon: MessageOutlined, className: 'text-[color:var(--warning)]' },
  done: { icon: CheckCircleOutlined, className: 'text-[color:var(--success)]' },
  failed: { icon: CloseCircleOutlined, className: 'text-[color:var(--error)]' },
  canceled: { icon: StopOutlined, className: 'text-[color:var(--text-quaternary)]' },
}

const priorityColor: Record<string, string> = {
  low: 'default',
  medium: 'blue',
  high: 'orange',
  urgent: 'red',
}

const priorityLabel: Record<string, string> = {
  low: '低',
  medium: '中',
  high: '高',
  urgent: '紧急',
}

export function AgentDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { message } = App.useApp()
  const { copied, copy } = useCopyToClipboard()
  const [activeTab, setActiveTab] = useState('overview')
  const [eventFilter, setEventFilter] = useState<EventType | 'all'>('all')
  const [taskFilter, setTaskFilter] = useState<TaskStatus | 'all'>('all')
  const [createTaskOpen, setCreateTaskOpen] = useState(false)
  const [editOpen, setEditOpen] = useState(false)
  const [archiveOpen, setArchiveOpen] = useState(false)

  const { data: agent, isLoading: agentLoading } = useAgent(id)
  const { data: stats } = useAgentStats(id)
  const { data: insights, isLoading: insightsLoading } = useAgentInsights(id)
  const { data: tasks, isLoading: tasksLoading } = useAgentTasks(id, taskFilter === 'all' ? undefined : taskFilter)
  const { data: events, isLoading: eventsLoading } = useAgentEvents(id)
  const deleteAgent = useDeleteAgent()

  const filteredEvents = eventFilter === 'all'
    ? (events ?? [])
    : (events ?? []).filter((e) => e.event_type === eventFilter)

  const handleDelete = async () => {
    if (!agent) return
    if (agent.usage?.in_use) {
      setArchiveOpen(true)
      return
    }
    Modal.confirm({
      title: `确定要删除数字员工 "${agent.name}" 吗？`,
      okText: '删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        try {
          await deleteAgent.mutateAsync(agent.id)
          message.success(`数字员工 "${agent.name}" 已删除`)
          navigate('/agents')
        } catch (err) {
          message.error(err instanceof Error ? err.message : '删除失败')
        }
      },
    })
  }

  if (agentLoading) {
    return <div style={{ padding: 24 }}><Skeleton active /></div>
  }
  if (!agent) {
    return <div style={{ padding: 24, color: 'var(--text-tertiary)' }}>数字员工未找到</div>
  }

  const isHermes = agent.product === 'hermes'

  const tabItems = [
    { key: 'overview', label: '概览', icon: <ThunderboltOutlined />, children: stats ? <AgentOverview stats={stats} insights={insights ?? null} loading={insightsLoading} /> : <Skeleton active /> },
    { key: 'chat', label: '对话', icon: <MessageOutlined />, children: <AgentChatPanel agent={agent} /> },
    {
      key: 'tasks',
      label: '工作记录',
      icon: <FileTextOutlined />,
      children: (
        <div className="flex flex-col gap-4">
          <Space wrap style={{ marginBottom: 4 }}>
            {taskStatusFilters.map((f) => (
              <Button
                key={f.value}
                type={taskFilter === f.value ? 'primary' : 'default'}
                size="small"
                onClick={() => setTaskFilter(f.value)}
              >
                {f.label}
              </Button>
            ))}
          </Space>

          {/* 表头 */}
          <div className="grid items-center gap-3 grid-cols-[minmax(0,1fr)_120px_80px_120px_90px] px-3 py-2 border-b border-[color:var(--line-strong)] text-xs font-medium text-[color:var(--text-quaternary)]">
            <span>任务名称</span>
            <span>项目</span>
            <span className="text-center">优先级</span>
            <span>进度</span>
            <span className="text-right">更新</span>
          </div>

          {tasksLoading ? (
            <Skeleton active paragraph={{ rows: 5 }} />
          ) : !tasks || tasks.length === 0 ? (
            <Empty
              description={taskFilter !== 'all' ? '该状态下暂无任务' : '该数字员工尚未参与任何任务'}
              image={Empty.PRESENTED_IMAGE_SIMPLE}
            />
          ) : (
            <div className="flex flex-col">
              {tasks.map((task) => {
                const StatusIcon = taskStatusIcon[task.status]?.icon ?? MinusCircleOutlined
                const iconClass = taskStatusIcon[task.status]?.className ?? 'text-[color:var(--text-quaternary)]'
                const progress = task.todo_count > 0
                  ? Math.round((task.completed_todo_count / task.todo_count) * 100)
                  : 0
                return (
                  <div
                    key={task.id}
                    onClick={() => navigate(`/projects/${task.project_id}?task=${task.id}`)}
                    className="grid items-center gap-3 grid-cols-[minmax(0,1fr)_120px_80px_120px_90px] px-3 py-2.5 rounded-md cursor-pointer transition-colors hover:bg-[color:var(--surface-sunken)]"
                  >
                    {/* 名称 + 状态图标 + 描述 + PM 徽章 */}
                    <div className="flex items-center gap-2.5 min-w-0">
                      <StatusIcon className={`text-base shrink-0 ${iconClass}`} />
                      <div className="min-w-0 flex-1">
                        <div className="truncate text-sm text-[color:var(--text-primary)] font-medium">
                          {task.title}
                          {task.relation === 'pm' && (
                            <Tag className="!text-[10px] !ml-1.5" color="default">PM</Tag>
                          )}
                        </div>
                        {task.description && (
                          <div className="truncate text-xs text-[color:var(--text-quaternary)] mt-0.5">{task.description}</div>
                        )}
                      </div>
                    </div>

                    {/* 项目 */}
                    <div className="truncate text-xs text-[color:var(--text-tertiary)]">{task.project_name}</div>

                    {/* 优先级 */}
                    <div className="flex justify-center">
                      <Tag className="!text-[11px]" color={priorityColor[task.priority] ?? 'default'}>
                        {priorityLabel[task.priority] ?? task.priority}
                      </Tag>
                    </div>

                    {/* 进度 */}
                    <div className="flex items-center gap-2">
                      {task.todo_count > 0 ? (
                        <>
                          <Progress
                            percent={progress}
                            showInfo={false}
                            size="small"
                            strokeColor={task.failed_todo_count > 0 ? '#ef4444' : '#3b82f6'}
                            className="flex-1"
                          />
                          <span className="text-xs text-[color:var(--text-quaternary)] whitespace-nowrap">
                            {task.completed_todo_count}/{task.todo_count}
                          </span>
                        </>
                      ) : (
                        <span className="text-xs text-[color:var(--text-quaternary)]">-</span>
                      )}
                    </div>

                    {/* 时间 */}
                    <div className="text-xs text-[color:var(--text-quaternary)] text-right whitespace-nowrap">
                      {dayjs(task.updated_at).fromNow()}
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </div>
      ),
    },
    {
      key: 'activity',
      label: '活动日志',
      icon: <HistoryOutlined />,
      children: (
        <div>
          <Space style={{ marginBottom: 16 }} wrap>
            {eventTypeFilters.map((f) => (
              <Button key={f.value} type={eventFilter === f.value ? 'primary' : 'default'} size="small" onClick={() => setEventFilter(f.value)}>
                {f.label}
              </Button>
            ))}
          </Space>
          {eventsLoading ? (
            <Skeleton active paragraph={{ rows: 6 }} />
          ) : filteredEvents.length === 0 ? (
            <Empty description="暂无活动记录" />
          ) : (
            <Timeline
              items={filteredEvents.map((e: Event) => {
                const cfg = eventConfig[e.event_type]
                return {
                  color: cfg?.color,
                  children: (
                    <div style={{ marginBottom: 8 }}>
                      <Space size={8} wrap>
                        <Tag style={{ fontSize: 11, color: cfg?.color, borderColor: cfg?.color }}>{cfg?.label ?? e.event_type}</Tag>
                        <Text style={{ fontSize: 12, color: 'var(--text-secondary)' }}>{stripReplyPrefix(e.content ?? '') || e.actor_name}</Text>
                        <Text type="secondary" style={{ fontSize: 11 }}>{dayjs(e.created_at).format('MM-DD HH:mm')}</Text>
                      </Space>
                      {Boolean(e.metadata?.task_title || e.metadata?.todo_title) && (
                        <div style={{ fontSize: 12, color: 'var(--text-tertiary)', marginTop: 4, paddingLeft: 4 }}>
                          {String(e.metadata?.task_title ?? '')}
                          {e.metadata?.todo_title ? ` › ${String(e.metadata?.todo_title)}` : ''}
                        </div>
                      )}
                    </div>
                  ),
                }
              })}
            />
          )}
        </div>
      ),
    },
    ...(isHermes
      ? [
          {
            key: 'capability-skills',
            label: '技能',
            icon: <ToolOutlined />,
            children: id ? <HermesSkillsTab agentId={id} /> : null,
          },
          {
            key: 'capability-models',
            label: '模型',
            icon: <ToolOutlined />,
            children: id ? <HermesModelsTab agentId={id} /> : null,
          },
          {
            key: 'capability-jobs',
            label: '定时任务',
            icon: <ToolOutlined />,
            children: id ? <HermesJobsTab agentId={id} /> : null,
          },
        ]
      : []),
  ]

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <div style={{ padding: '8px 16px', borderBottom: '1px solid var(--line)', background: 'var(--surface)', backdropFilter: 'var(--glass-blur)', flexShrink: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <Button type="text" icon={<ArrowLeftOutlined />} onClick={() => navigate('/agents')} style={{ flexShrink: 0 }} />
          <AgentAvatar
            name={agent.name}
            role={agent.role}
            seed={agent.id}
            size={32}
          />
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, minWidth: 0 }}>
              <Title level={5} style={{ margin: 0, color: 'var(--text-primary)', fontSize: 16 }} ellipsis={{ tooltip: agent.name }}>
                {agent.name}
              </Title>
              <NeonBadge label={statusMap[agent.status]?.label} variant={agent.status === 'online' ? 'green' : agent.status === 'busy' ? 'amber' : 'rose'} />
              <Tag style={{ margin: 0, fontSize: 10, lineHeight: '16px', height: 18 }} color={productBadgeColor[agent.product] ?? 'default'}>
                {agent.product || 'trustmesh'}
              </Tag>
              {agent.archived && <Tag style={{ margin: 0, fontSize: 10, lineHeight: '16px', height: 18 }} color="default">已离职</Tag>}
              <span style={{ flex: 1 }} />
              <Space size={10} style={{ flexShrink: 0, fontSize: 12 }}>
                <Text type="secondary" style={{ fontSize: 12 }}>{roleMap[agent.role] ?? agent.role}</Text>
                {agent.node_id ? (
                  <Text type="secondary" code style={{ fontSize: 11, fontFamily: "'JetBrains Mono', monospace" }}>{agent.node_id.slice(0, 16)}...</Text>
                ) : (
                  <Text type="secondary" style={{ fontSize: 12 }}>无节点</Text>
                )}
                <Button
                  type="text"
                  size="small"
                  icon={copied ? <CheckOutlined style={{ color: 'var(--success)' }} /> : <CopyOutlined />}
                  onClick={() => copy(agent.node_id)}
                  title="复制节点 ID"
                  style={{ height: 20, padding: '0 4px' }}
                />
                {agent.last_seen_at && (
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    <ClockDot /> {dayjs(agent.last_seen_at).fromNow()}
                  </Text>
                )}
              </Space>
              <Space style={{ flexShrink: 0 }}>
                {agent.role !== 'pm' && !agent.archived && (
                  <Button size="small" icon={<PlusOutlined />} onClick={() => setCreateTaskOpen(true)}>创建任务</Button>
                )}
                {!agent.archived && (
                  <Dropdown
                    menu={{
                      items: [
                        { key: 'edit', icon: <EditOutlined />, label: '编辑', onClick: () => setEditOpen(true) },
                        {
                          key: 'archive',
                          icon: agent.usage?.in_use ? <InboxOutlined /> : <DeleteOutlined />,
                          label: agent.usage?.in_use ? '离职' : '删除',
                          danger: true,
                          onClick: handleDelete,
                        },
                      ],
                    }}
                  >
                    <Button size="small" icon={<MoreOutlined />} />
                  </Dropdown>
                )}
              </Space>
            </div>
            {agent.description && (
              <Text type="secondary" style={{ marginTop: 2, display: 'block', fontSize: 12, maxWidth: 640 }} ellipsis={{ tooltip: agent.description }}>
                {agent.description}
              </Text>
            )}
            {agent.capabilities && agent.capabilities.length > 0 && (
              <div style={{ marginTop: 3, display: 'flex', gap: 4, flexWrap: 'wrap' }}>
                {agent.capabilities.map((cap) => (
                  <Tag key={cap} className="!text-[10px]" style={{ lineHeight: '16px', height: 18, margin: 0 }} color="geekblue">{cap}</Tag>
                ))}
              </div>
            )}
          </div>
        </div>

        <Tabs
          activeKey={activeTab}
          onChange={setActiveTab}
          items={tabItems.map(({ children: _children, ...rest }) => rest)}
          size="small"
          style={{ marginTop: 2, marginBottom: -8 }}
        />
      </div>

      <div style={{ flex: 1, overflow: 'auto', padding: '16px 16px 0' }}>
        {tabItems.find((t) => t.key === activeTab)?.children}
      </div>

      {agent.archived && (
        <div style={{ padding: '0 24px 16px' }}>
          <div className="flex items-center gap-3 rounded-lg border border-amber-500/20 bg-amber-500/5 px-4 py-3 text-sm text-[color:var(--warning)]">
            <InboxOutlined />
            该 Agent 已离职，不再接收新任务
          </div>
        </div>
      )}

      <CreateTaskModal
        open={createTaskOpen}
        onClose={setCreateTaskOpen}
        defaultAgentId={id}
      />
      <AgentConfigModal open={editOpen} onClose={setEditOpen} agent={agent} />
      <ArchiveAgentModal open={archiveOpen} onClose={setArchiveOpen} agent={agent} onArchived={() => navigate('/agents')} />
    </div>
  )
}

function ClockDot() {
  return <span style={{ display: 'inline-block', width: 6, height: 6, borderRadius: 'var(--radius-avatar)', background: 'var(--surface-raised)', marginRight: 4 }} />
}
