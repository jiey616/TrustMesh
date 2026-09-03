import { useEffect, useMemo, useRef, useState } from 'react'
import { Progress, Spin, Input } from 'antd'
import { DownOutlined, RightOutlined, SearchOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import { useTask } from '@/hooks/useTasks'
import type { TaskListItem, TaskStatus, TaskPriority } from '@/types'

interface Props {
  tasks: TaskListItem[]
  selectedTaskId: string | null
  onTaskClick: (id: string) => void
}

const statusConfig: Record<TaskStatus, { color: string; label: string; dot: string; active: boolean }> = {
  planning: { color: 'var(--signal)', label: '规划中', dot: 'var(--signal)', active: true },
  review: { color: 'var(--warning)', label: '待确认', dot: 'var(--warning)', active: true },
  pending: { color: 'var(--info)', label: '待处理', dot: 'var(--info)', active: true },
  in_progress: { color: 'var(--cyan)', label: '进行中', dot: 'var(--cyan)', active: true },
  awaiting_review: { color: 'var(--error)', label: '待人工确认', dot: 'var(--error)', active: true },
  waiting_user: { color: 'var(--error)', label: '等待用户', dot: 'var(--error)', active: true },
  done: { color: 'var(--success)', label: '已完成', dot: 'var(--success)', active: false },
  failed: { color: 'var(--error)', label: '失败', dot: 'var(--error)', active: false },
  canceled: { color: 'var(--text-quaternary)', label: '已取消', dot: 'var(--text-quaternary)', active: false },
}

const statusGroups: TaskStatus[] = [
  'planning', 'review', 'in_progress', 'awaiting_review', 'waiting_user', 'pending', 'done', 'failed', 'canceled',
]

const priorityColor: Record<TaskPriority, string> = {
  low: 'green',
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

/** 大分组展开截断阈值 */
const GROUP_TRUNCATE = 15

/** 行内执行清单（点击任务行首展开） */
function InlineTodos({ taskId, status }: { taskId: string; status: string }) {
  const { data: taskDetail, isLoading } = useTask(taskId)
  const doneCount = (taskDetail?.todos ?? []).filter((t) => t.status === 'done').length

  if (isLoading) {
    return (
      <div style={{ padding: '8px 4px', textAlign: 'center' }}>
        <Spin size="small" />
      </div>
    )
  }

  const todos = taskDetail?.todos ?? []
  if (todos.length === 0) {
    return (
      <div style={{ padding: '8px 4px', fontSize: 13, color: 'var(--text-quaternary)' }}>
        {status === 'planning' ? '等待 PM 规划' : '暂无执行清单'}
      </div>
    )
  }

  return (
    <div style={{ padding: '6px 2px', display: 'flex', flexDirection: 'column', gap: 3 }}>
      <div style={{ fontSize: 12, color: 'var(--text-quaternary)', marginBottom: 2 }}>
        {doneCount}/{todos.length} 完成
      </div>
      {todos.map((todo, i) => (
        <div key={todo.id} style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13 }}>
          <span
            style={{
              width: 6,
              height: 6,
              borderRadius: 'var(--radius-avatar)',
              flexShrink: 0,
              background:
                todo.status === 'done' ? '#10b981'
                : todo.status === 'in_progress' ? '#22d3ee'
                : todo.status === 'failed' ? '#ef4444'
                : todo.status === 'waiting_user' ? '#f43f5e'
                : 'rgba(255,255,255,0.3)',
              boxShadow: todo.status === 'in_progress' ? '0 0 5px #22d3ee' : 'none',
            }}
          />
          <span style={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', color: todo.status === 'done' ? 'rgba(255,255,255,0.4)' : 'rgba(255,255,255,0.7)', textDecoration: todo.status === 'done' ? 'line-through' : 'none' }}>
            {i + 1}. {todo.title}
          </span>
          <span style={{ flexShrink: 0, fontSize: 12, color: 'var(--text-quaternary)' }}>{todo.assignee?.name ?? ''}</span>
        </div>
      ))}
    </div>
  )
}

function TaskCard({ task, selected, onTaskClick }: { task: TaskListItem; selected: boolean; onTaskClick: (id: string) => void }) {
  const cfg = statusConfig[task.status]
  const pct = task.todo_count > 0 ? Math.round((task.completed_todo_count / task.todo_count) * 100) : 0
  const [todosExpanded, setTodosExpanded] = useState(false)
  const [prevStatus, setPrevStatus] = useState(task.status)

  // 任务变为进行中时自动展开执行清单（默认已展开，此处兜底）
  if (prevStatus !== task.status) {
    setPrevStatus(task.status)
    if (task.status === 'in_progress') setTodosExpanded(true)
  }

  const canExpandTodos = task.todo_count > 0 || task.status === 'pending' || task.status === 'in_progress' || task.status === 'planning'

  return (
    <div>
      <div
        onClick={() => onTaskClick(task.id)}
        style={{
          cursor: 'pointer',
          borderRadius: 'var(--radius-control)',
          padding: '9px 12px',
          background: selected ? 'rgba(109,95,245,0.10)' : 'rgba(255,255,255,0.02)',
          border: `1px solid ${selected ? 'rgba(109,95,245,0.35)' : 'rgba(255,255,255,0.05)'}`,
          transition: 'all 0.15s',
        }}
      >
        {/* Title row */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, minWidth: 0 }}>
          {canExpandTodos && (
            <span
              onClick={(e) => {
                e.stopPropagation()
                setTodosExpanded((v) => !v)
              }}
              style={{ flexShrink: 0, cursor: 'pointer', color: 'var(--text-quaternary)', display: 'flex', alignItems: 'center' }}
              title={todosExpanded ? '收起执行清单' : '展开执行清单'}
            >
              {todosExpanded ? <DownOutlined style={{ fontSize: 11 }} /> : <RightOutlined style={{ fontSize: 11 }} />}
            </span>
          )}
          <span
            style={{
              width: 7,
              height: 7,
              borderRadius: 'var(--radius-avatar)',
              background: cfg.dot,
              flexShrink: 0,
              boxShadow: task.status === 'in_progress' ? `0 0 6px ${cfg.dot}` : 'none',
            }}
            title={cfg.label}
          />
          <span
            style={{
              flex: 1,
              minWidth: 0,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              fontSize: 14,
              fontWeight: selected ? 600 : 500,
              color: selected ? '#fff' : 'rgba(255,255,255,0.88)',
            }}
          >
            {task.title}
          </span>
          <span style={{ fontSize: 12, color: 'var(--text-quaternary)', flexShrink: 0 }} title={dayjs(task.updated_at).format('YYYY-MM-DD HH:mm:ss')}>
            {dayjs(task.updated_at).fromNow()}
          </span>
        </div>

        {/* Requirement preview */}
        <div
          style={{
            marginTop: 4,
            fontSize: 13,
            color: 'var(--text-tertiary)',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            paddingLeft: canExpandTodos ? 15 : 13,
          }}
          title={task.description}
        >
          {task.description || '暂无描述'}
        </div>

        {/* Meta row: priority + progress */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 6, paddingLeft: canExpandTodos ? 15 : 13 }}>
          <span
            style={{
              fontSize: 12,
              color: priorityColor[task.priority] === 'red' ? '#f87171' : priorityColor[task.priority] === 'orange' ? '#fbbf24' : priorityColor[task.priority] === 'blue' ? '#60a5fa' : '#34d399',
              background: 'var(--surface)',
              border: '1px solid var(--line)',
              borderRadius: 'var(--radius-control)',
              padding: '0 5px',
              lineHeight: '15px',
            }}
          >
            {priorityLabel[task.priority] ?? task.priority}
          </span>
          <span style={{ flex: 1 }} />
          {task.todo_count > 0 ? (
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
              <Progress percent={pct} size="small" showInfo={false} strokeColor={cfg.color} style={{ width: 48, margin: 0 }} />
              <span style={{ fontSize: 12, color: 'var(--text-tertiary)' }}>
                {task.completed_todo_count}/{task.todo_count}
              </span>
              {task.failed_todo_count > 0 && (
                <span style={{ fontSize: 12, color: 'var(--error)' }}>{task.failed_todo_count}!</span>
              )}
            </span>
          ) : (
            <span style={{ fontSize: 13, color: 'var(--text-quaternary)' }}>
              {task.status === 'planning' ? '等待规划' : '—'}
            </span>
          )}
        </div>
      </div>

      {/* Inline todos */}
      {todosExpanded && canExpandTodos && (
        <div style={{ marginLeft: 22, marginTop: 4, paddingLeft: 10, borderLeft: '1px solid var(--line)' }}>
          <InlineTodos taskId={task.id} status={task.status} />
        </div>
      )}
    </div>
  )
}

const filterChips: { key: TaskStatus | 'all' | 'active'; label: string; color?: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'active', label: '活跃', color: 'var(--cyan)' },
  { key: 'in_progress', label: '进行中', color: 'var(--cyan)' },
  { key: 'awaiting_review', label: '待人工确认', color: 'var(--error)' },
  { key: 'waiting_user', label: '等待用户', color: 'var(--error)' },
  { key: 'pending', label: '待处理', color: 'var(--info)' },
  { key: 'done', label: '已完成', color: 'var(--success)' },
  { key: 'failed', label: '失败', color: 'var(--error)' },
  { key: 'canceled', label: '已取消', color: 'var(--text-quaternary)' },
]

export function TaskListView({ tasks, selectedTaskId, onTaskClick }: Props) {
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>(() => {
    // 只折叠失败/已取消分组；进行中、未完成、已完成等分组默认展开
    const init: Record<string, boolean> = {}
    for (const status of statusGroups) {
      if (status === 'failed' || status === 'canceled') init[status] = true
    }
    return init
  })
  const [search, setSearch] = useState('')
  const [filter, setFilter] = useState<TaskStatus | 'all' | 'active'>('all')
  const [expandedAll, setExpandedAll] = useState<Record<string, boolean>>({})
  const scrollRef = useRef<HTMLDivElement>(null)
  const prevSelectedRef = useRef<string | null>(null)

  const selectedTask = selectedTaskId ? tasks.find((t) => t.id === selectedTaskId) : undefined
  // 选中任务所在分组强制展开
  const effectiveCollapsed = selectedTask && collapsed[selectedTask.status]
    ? { ...collapsed, [selectedTask.status]: false }
    : collapsed

  useEffect(() => {
    if (!selectedTaskId || selectedTaskId === prevSelectedRef.current) return
    prevSelectedRef.current = selectedTaskId
    requestAnimationFrame(() => {
      const row = scrollRef.current?.querySelector(`[data-task-id="${selectedTaskId}"]`)
      row?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
    })
  }, [selectedTaskId])

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    return tasks.filter((t) => {
      if (filter === 'active') {
        if (!statusConfig[t.status]?.active) return false
      } else if (filter !== 'all' && t.status !== filter) {
        return false
      }
      if (q && !t.title.toLowerCase().includes(q)) return false
      return true
    })
  }, [tasks, search, filter])

  const groups = statusGroups
    .map((status) => ({ status, ...statusConfig[status], tasks: filtered.filter((t) => t.status === status) }))
    .filter((g) => g.tasks.length > 0)

  const hasAnyActiveFilter = filter !== 'all' || search.trim() !== ''

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* 搜索 + 筛选 */}
      <div style={{ flexShrink: 0, paddingBottom: 8, display: 'flex', flexDirection: 'column', gap: 6 }}>
        <Input
          allowClear
          size="small"
          prefix={<SearchOutlined style={{ color: 'var(--text-quaternary)', fontSize: 13 }} />}
          placeholder="搜索任务标题…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          style={{ background: 'var(--surface)', borderColor: 'var(--line-strong)', color: 'var(--text-primary)' }}
        />
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
          {filterChips.map((chip) => {
            const active = filter === chip.key
            return (
              <button
                key={chip.key}
                type="button"
                onClick={() => setFilter(chip.key)}
                style={{
                  fontSize: 12,
                  lineHeight: '16px',
                  padding: '0 8px',
                  borderRadius: 'var(--radius-pill)',
                  border: `1px solid ${active ? (chip.color ?? 'rgba(109,95,245,0.6)') : 'rgba(255,255,255,0.08)'}`,
                  background: active ? (chip.color ? `${chip.color}22` : 'rgba(109,95,245,0.15)') : 'transparent',
                  color: active ? (chip.color ?? '#8b7ff8') : 'rgba(255,255,255,0.5)',
                  cursor: 'pointer',
                  fontFamily: 'inherit',
                  transition: 'all 0.15s',
                }}
              >
                {chip.label}
              </button>
            )
          })}
        </div>
      </div>

      <div ref={scrollRef} style={{ flex: 1, overflowY: 'auto', paddingRight: 2, display: 'flex', flexDirection: 'column', gap: 4 }}>
        {groups.length === 0 && (
          <div style={{ padding: '40px 0', textAlign: 'center', color: 'var(--text-quaternary)', fontSize: 13 }}>
            {hasAnyActiveFilter ? '没有匹配的任务' : '暂无任务'}
          </div>
        )}

        {groups.map((group) => {
          const isCollapsed = effectiveCollapsed[group.status]
          const showAll = expandedAll[group.status]
          const visibleTasks = showAll ? group.tasks : group.tasks.slice(0, GROUP_TRUNCATE)
          const truncated = group.tasks.length > GROUP_TRUNCATE && !showAll
          return (
            <div key={group.status}>
              <div
                onClick={() => setCollapsed((prev) => ({ ...prev, [group.status]: !prev[group.status] }))}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 6,
                  padding: '6px 8px',
                  cursor: 'pointer',
                  userSelect: 'none',
                  borderRadius: 'var(--radius-control)',
                }}
              >
                {isCollapsed ? <RightOutlined style={{ fontSize: 11, color: 'var(--text-quaternary)' }} /> : <DownOutlined style={{ fontSize: 11, color: 'var(--text-quaternary)' }} />}
                <span style={{ width: 6, height: 6, borderRadius: 'var(--radius-avatar)', background: group.dot, flexShrink: 0 }} />
                <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-secondary)' }}>{group.label}</span>
                <span
                  style={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    minWidth: 16,
                    height: 16,
                    borderRadius: 'var(--radius-pill)',
                    padding: '0 5px',
                    fontSize: 12,
                    fontWeight: 500,
                    background: 'var(--surface-raised)',
                    color: 'var(--text-tertiary)',
                  }}
                >
                  {group.tasks.length}
                </span>
              </div>
              {!isCollapsed && (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  {visibleTasks.map((task) => (
                    <div key={task.id} data-task-id={task.id}>
                      <TaskCard task={task} selected={task.id === selectedTaskId} onTaskClick={onTaskClick} />
                    </div>
                  ))}
                  {truncated && (
                    <button
                      type="button"
                      onClick={() => setExpandedAll((prev) => ({ ...prev, [group.status]: true }))}
                      style={{
                        fontSize: 13,
                        color: 'var(--text-tertiary)',
                        background: 'none',
                        border: 'none',
                        cursor: 'pointer',
                        padding: '4px 0',
                        fontFamily: 'inherit',
                      }}
                    >
                      展开全部 {group.tasks.length} 条
                    </button>
                  )}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
