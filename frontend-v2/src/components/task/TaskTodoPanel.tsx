import { useState } from 'react'
import { Button, Input, Select, Tag, App } from 'antd'
import { CheckOutlined, CloseOutlined, EditOutlined, DeleteOutlined, PlusOutlined, DownOutlined, RightOutlined } from '@ant-design/icons'
import { useAddTaskTodo, useUpdateTaskTodo, useRemoveTaskTodo } from '@/hooks/useTasks'
import { useAgents } from '@/hooks/useAgents'
import type { Todo, TaskDetail } from '@/types'

const statusIndicatorColors: Record<string, string> = {
  planning: 'var(--cyan)',
  review: 'var(--warning)',
  pending: 'var(--text-tertiary)',
  in_progress: 'var(--info)',
  awaiting_review: 'var(--warning)',
  waiting_user: 'var(--error)',
  done: 'var(--success)',
  failed: 'var(--error)',
  canceled: 'var(--text-quaternary)',
}

const statusLabels: Record<string, string> = {
  pending: '待处理',
  dispatched: '已派发',
  in_progress: '进行中',
  done: '已完成',
  failed: '失败',
  canceled: '已取消',
  waiting_user: '等待用户',
}

function extractResultText(result: Todo['result']): string | null {
  if (typeof result === 'string') return result || null
  if (result && typeof result === 'object') {
    return (result as { summary?: string }).summary
      ?? (result as { final_output?: string }).final_output
      ?? (result as { output?: string }).output
      ?? null
  }
  return null
}

/** Todo 结果文本：默认 3 行截断，可展开/收起 */
function TodoResultText({ result }: { result: Todo['result'] }) {
  const text = extractResultText(result)
  const [expanded, setExpanded] = useState(false)
  if (!text) return null

  const overflow = text.length > 120 || text.split('\n').length > 3
  const visible = expanded ? text : truncateLines(text, 3)

  return (
    <div style={{ marginTop: 4 }}>
      <div
        style={{
          fontSize: 13,
          color: 'var(--text-secondary)',
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-word',
          lineHeight: 1.6,
          position: 'relative',
          overflow: 'hidden',
          maxHeight: expanded ? undefined : '4.8em',
        }}
      >
        {visible}
        {!expanded && overflow && (
          <span
            style={{
              position: 'absolute',
              right: 0,
              bottom: 0,
              paddingLeft: 24,
              background: 'linear-gradient(90deg, transparent, var(--canvas))',
              color: 'var(--signal)',
              cursor: 'pointer',
              fontWeight: 500,
              fontSize: 12,
            }}
            onClick={(e) => { e.stopPropagation(); setExpanded(true) }}
          >
            展开
          </span>
        )}
      </div>
      {expanded && overflow && (
        <button
          type="button"
          onClick={() => setExpanded(false)}
          style={{ fontSize: 12, color: 'var(--text-tertiary)', background: 'none', border: 'none', cursor: 'pointer', padding: 0, marginTop: 2, fontFamily: 'inherit' }}
        >
          收起
        </button>
      )}
    </div>
  )
}

function truncateLines(text: string, maxLines: number): string {
  const lines = text.split('\n')
  if (lines.length <= maxLines) {
    return text.length > 120 ? text.slice(0, 120) + '…' : text
  }
  return lines.slice(0, maxLines).join('\n')
}

interface TaskTodoPanelProps {
  task: TaskDetail
}

/**
 * 只读的执行进度清单。
 *
 * 这里刻意不做任何审批动作：「通过 / 退回重做」已全部收进右上角「待确认」抽屉。
 * 之前按钮长在 todo 行内，清单又被收进 Drawer 后，用户得点两次抽屉才能审一个
 * todo；拆开之后本组件只负责看进度，职责清楚。
 */
export function TaskTodoPanel({ task }: TaskTodoPanelProps) {
  const { message } = App.useApp()
  const { data: agents } = useAgents()
  const addTodo = useAddTaskTodo()
  const updateTodo = useUpdateTaskTodo()
  const removeTodo = useRemoveTaskTodo()

  const editable = task.status === 'pending' || task.status === 'in_progress'
  const availableAssignees = agents?.filter((a) => a.id !== task.pm_agent?.id) ?? []

  const [addOpen, setAddOpen] = useState(false)
  const [addTitle, setAddTitle] = useState('')
  const [addDesc, setAddDesc] = useState('')
  const [addAssignee, setAddAssignee] = useState('')
  const [addError, setAddError] = useState('')

  const [editingId, setEditingId] = useState<string | null>(null)
  const [editTitle, setEditTitle] = useState('')
  const [editDesc, setEditDesc] = useState('')
  const [editAssignee, setEditAssignee] = useState('')
  const [editError, setEditError] = useState('')

  const [deletingId, setDeletingId] = useState<string | null>(null)
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set())

  const resetAddForm = () => {
    setAddOpen(false)
    setAddTitle('')
    setAddDesc('')
    setAddAssignee('')
    setAddError('')
  }

  const handleAdd = async () => {
    if (!addTitle.trim()) {
      setAddError('请输入 TODO 标题')
      return
    }
    if (!addAssignee) {
      setAddError('请选择执行数字员工')
      return
    }
    setAddError('')
    try {
      await addTodo.mutateAsync({
        taskId: task.id,
        input: { title: addTitle.trim(), description: addDesc.trim() || '', assignee_id: addAssignee },
      })
      message.success('TODO 已添加')
      resetAddForm()
    } catch (err) {
      setAddError(err instanceof Error ? err.message : '添加失败')
    }
  }

  const startEditing = (todo: Todo) => {
    setEditingId(todo.id)
    setEditTitle(todo.title)
    setEditDesc(todo.description ?? '')
    setEditAssignee(todo.assignee?.agent_id ?? '')
    setEditError('')
    setDeletingId(null)
  }

  const handleUpdate = async () => {
    if (!editingId) return
    if (!editTitle.trim()) {
      setEditError('请输入 TODO 标题')
      return
    }
    setEditError('')
    try {
      await updateTodo.mutateAsync({
        taskId: task.id,
        todoId: editingId,
        input: { title: editTitle.trim(), description: editDesc.trim() || '', assignee_id: editAssignee || undefined },
      })
      message.success('TODO 已更新')
      setEditingId(null)
    } catch (err) {
      setEditError(err instanceof Error ? err.message : '更新失败')
    }
  }

  const handleDelete = async (todoId: string) => {
    if (deletingId !== todoId) {
      setDeletingId(todoId)
      return
    }
    try {
      await removeTodo.mutateAsync({ taskId: task.id, todoId })
      message.success('TODO 已删除')
      setDeletingId(null)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败')
    }
  }

  const toggleExpand = (id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const assigneeOptions = availableAssignees.map((a) => ({
    label: `${a.name} (${a.status === 'online' ? '在线' : a.status})`,
    value: a.id,
  }))

  const renderForm = (
    title: string,
    setTitle: (v: string) => void,
    desc: string,
    setDesc: (v: string) => void,
    assignee: string,
    setAssignee: (v: string) => void,
    error: string,
    onSave: () => void,
    onCancel: () => void,
    saving: boolean,
  ) => (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8, borderRadius: 'var(--radius-control)', border: '1px solid rgba(109,95,245,0.25)', background: 'rgba(109,95,245,0.05)', padding: 12 }}>
      <Input
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        placeholder="TODO 标题 *"
        style={{ background: 'var(--surface-inset)', borderColor: 'var(--line-strong)', color: 'var(--text-primary)' }}
      />
      <Input.TextArea
        value={desc}
        onChange={(e) => setDesc(e.target.value)}
        placeholder="描述（可选）"
        rows={2}
        style={{ background: 'var(--surface-inset)', borderColor: 'var(--line-strong)', color: 'var(--text-primary)', resize: 'none', fontSize: 13 }}
      />
      <Select
        value={assignee || undefined}
        onChange={(v) => setAssignee(v ?? '')}
        placeholder="选择执行数字员工 *"
        options={assigneeOptions}
        style={{ width: '100%' }}
      />
      {error && <div style={{ fontSize: 13, color: 'var(--error)' }}>{error}</div>}
      <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
        <Button size="small" onClick={onCancel}>取消</Button>
        <Button size="small" type="primary" loading={saving} onClick={onSave}>保存</Button>
      </div>
    </div>
  )

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      {task.todos.length === 0 && !addOpen && (
        <div style={{ textAlign: 'center', padding: 16, color: 'var(--text-quaternary)', fontSize: 13 }}>暂无 TODO</div>
      )}

      {task.todos.map((todo) => {
        const hasDetails = todo.description || todo.error || (todo.questions?.length ?? 0) > 0
        const isExpanded = expandedIds.has(todo.id)
        const isEditing = editingId === todo.id
        const isDeleting = deletingId === todo.id
        const canModify = todo.status === 'pending' && editable
        const isAwaitingReview = todo.review_status === 'pending_approval'
        const isRejected = todo.review_status === 'rejected'

        if (isEditing) {
          return (
            <div key={todo.id}>
              {renderForm(editTitle, setEditTitle, editDesc, setEditDesc, editAssignee, setEditAssignee, editError, handleUpdate, () => setEditingId(null), updateTodo.isPending)}
            </div>
          )
        }

        return (
          <div
            key={todo.id}
            style={{
              borderRadius: 'var(--radius-control)',
              border: `1px solid ${isAwaitingReview ? 'rgba(245,158,11,0.4)' : 'var(--line)'}`,
              background: isAwaitingReview ? 'rgba(245,158,11,0.05)' : 'var(--surface-sunken)',
              overflow: 'hidden',
              transition: 'all 0.15s',
            }}
          >
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: 10, padding: '8px 10px' }}>
              <span
                style={{
                  width: 8,
                  height: 8,
                  marginTop: 5,
                  borderRadius: 'var(--radius-avatar)',
                  flexShrink: 0,
                  background: statusIndicatorColors[todo.status] ?? 'var(--surface-raised)',
                  boxShadow: todo.status === 'in_progress' ? `0 0 6px ${statusIndicatorColors[todo.status]}` : 'none',
                }}
              />
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <span style={{ fontSize: 13, fontWeight: 500, color: todo.status === 'done' ? 'var(--text-quaternary)' : 'var(--text-primary)', textDecoration: todo.status === 'done' ? 'line-through' : 'none' }}>
                    {todo.title}
                  </span>
                  {isAwaitingReview && <Tag color="warning" style={{ margin: 0, fontSize: 12, lineHeight: '16px', height: 18 }}>⏳ 待人工确认</Tag>}
                  {isRejected && <Tag color="error" style={{ margin: 0, fontSize: 12, lineHeight: '16px', height: 18 }}>🔄 已退回重做</Tag>}
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 3, flexWrap: 'wrap' }}>
                  <span style={{ fontSize: 12, color: 'var(--text-tertiary)' }}>{todo.assignee?.name ?? '未指派'}</span>
                  <span style={{ fontSize: 12, color: statusIndicatorColors[todo.status] ?? 'var(--text-quaternary)' }}>
                    {statusLabels[todo.status] ?? todo.status}
                  </span>
                  {isRejected && todo.review_reason && (
                    <span style={{ fontSize: 12, color: 'var(--text-tertiary)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: 260 }} title={todo.review_reason}>
                      退回原因：{todo.review_reason}
                    </span>
                  )}
                </div>
                {(() => {
                  const text = extractResultText(todo.result)
                  return text ? <TodoResultText result={todo.result} /> : null
                })()}
              </div>

              {/* Actions */}
              <div style={{ display: 'flex', alignItems: 'center', gap: 2, flexShrink: 0 }}>
                {hasDetails && (
                  <Button
                    type="text"
                    size="small"
                    icon={isExpanded ? <DownOutlined style={{ fontSize: 12 }} /> : <RightOutlined style={{ fontSize: 12 }} />}
                    onClick={() => toggleExpand(todo.id)}
                  />
                )}
                {canModify && (
                  <>
                    <Button type="text" size="small" icon={<EditOutlined style={{ fontSize: 13 }} />} title="编辑" onClick={() => startEditing(todo)} />
                    {isDeleting ? (
                      <>
                        <Button type="text" size="small" icon={<CheckOutlined style={{ fontSize: 13, color: 'var(--error)' }} />} title="确认删除" onClick={() => handleDelete(todo.id)} />
                        <Button type="text" size="small" icon={<CloseOutlined style={{ fontSize: 13 }} />} title="取消" onClick={() => setDeletingId(null)} />
                      </>
                    ) : (
                      <Button type="text" size="small" icon={<DeleteOutlined style={{ fontSize: 13 }} />} title="删除" onClick={() => setDeletingId(todo.id)} />
                    )}
                  </>
                )}
              </div>
            </div>

            {isExpanded && hasDetails && (
              <div style={{ padding: '0 10px 10px 28px', display: 'flex', flexDirection: 'column', gap: 6 }}>
                {todo.description && (
                  <div style={{ fontSize: 13, color: 'var(--text-secondary)', whiteSpace: 'pre-wrap' }}>{todo.description}</div>
                )}
                {todo.error && (
                  <div style={{ fontSize: 13, color: 'var(--error)', whiteSpace: 'pre-wrap', background: 'rgba(244,63,94,0.06)', borderRadius: 'var(--radius-control)', padding: 6 }}>
                    {todo.error}
                  </div>
                )}
                {todo.questions && todo.questions.length > 0 && (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                    {todo.questions.map((q) => (
                      <div key={q.id} style={{ fontSize: 13, borderRadius: 'var(--radius-control)', background: 'rgba(244,63,94,0.05)', padding: '4px 8px' }}>
                        <span style={{ color: 'var(--text-secondary)' }}>问：{q.question}</span>
                        {q.answer ? (
                          <div style={{ marginTop: 2, color: 'var(--success)' }}>答：{q.answer === '__timeout__' ? '（超时自动放行）' : q.answer}{q.answered_by ? ` · ${q.answered_by}` : ''}</div>
                        ) : (
                          <div style={{ marginTop: 2, color: 'var(--warning)' }}>等待用户回答…</div>
                        )}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}
          </div>
        )
      })}

      {/* Add form */}
      {addOpen ? (
        renderForm(addTitle, setAddTitle, addDesc, setAddDesc, addAssignee, setAddAssignee, addError, handleAdd, resetAddForm, addTodo.isPending)
      ) : editable && (
        <button
          type="button"
          onClick={() => setAddOpen(true)}
          style={{
            width: '100%',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 6,
            padding: '8px 0',
            borderRadius: 'var(--radius-control)',
            border: '1px dashed var(--line-strong)',
            background: 'transparent',
            color: 'var(--text-tertiary)',
            fontSize: 13,
            cursor: 'pointer',
            fontFamily: 'inherit',
            transition: 'all 0.15s',
          }}
        >
          <PlusOutlined /> 添加 TODO
        </button>
      )}
    </div>
  )
}
