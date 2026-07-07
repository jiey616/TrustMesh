import { useState } from 'react'
import { toast } from 'sonner'
import { Plus, Pencil, Trash2, Check, X, ChevronDown, ChevronRight } from 'lucide-react'
import { cn } from '@/lib/utils'
import { normalizeEscapedText } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useAgents } from '@/hooks/useAgents'
import { useAddTaskTodo, useUpdateTaskTodo, useRemoveTaskTodo } from '@/hooks/useTasks'
import { ApiRequestError } from '@/api/client'
import type { Todo, TaskDetail, Agent } from '@/types'

interface TaskTodoPanelProps {
  task: TaskDetail
  /** "panel" = 独立面板（有头部、固定高度、内部滚动）；"embedded" = 嵌入式（无头部、自适应高度） */
  variant?: 'panel' | 'embedded'
}

const statusIndicatorColors: Record<string, string> = {
  planning: 'bg-cyan-500',
  review: 'bg-amber-500',
  pending: 'bg-slate-400',
  in_progress: 'bg-sky-500 animate-pulse',
  done: 'bg-emerald-500',
  failed: 'bg-rose-500',
  canceled: 'bg-slate-500/60',
}

const statusLabels: Record<string, string> = {
  pending: '待处理',
  in_progress: '进行中',
  done: '已完成',
  failed: '失败',
  canceled: '已取消',
}

export function TaskTodoPanel({ task, variant = 'panel' }: TaskTodoPanelProps) {
  const { data: agents } = useAgents()
  const addTodo = useAddTaskTodo()
  const updateTodo = useUpdateTaskTodo()
  const removeTodo = useRemoveTaskTodo()

  const editable = task.status === 'pending' || task.status === 'in_progress'

  const doneCount = task.todos.filter((t) => t.status === 'done').length

  // Add form state
  const [addOpen, setAddOpen] = useState(false)
  const [addTitle, setAddTitle] = useState('')
  const [addDesc, setAddDesc] = useState('')
  const [addAssignee, setAddAssignee] = useState('')
  const [addError, setAddError] = useState('')

  // Edit state
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editTitle, setEditTitle] = useState('')
  const [editDesc, setEditDesc] = useState('')
  const [editAssignee, setEditAssignee] = useState('')
  const [editError, setEditError] = useState('')

  // Delete confirm state
  const [deletingId, setDeletingId] = useState<string | null>(null)

  // Expanded todo IDs for details
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set())

  // Filter out PM agent from assignee list
  const availableAssignees = agents?.filter((a: Agent) => a.id !== task.pm_agent?.id) ?? []

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
      setAddError('请选择执行 Agent')
      return
    }
    setAddError('')
    try {
      await addTodo.mutateAsync({
        taskId: task.id,
        input: {
          title: addTitle.trim(),
          description: addDesc.trim() || '',
          assignee_id: addAssignee,
        },
      })
      toast.success('TODO 已添加')
      resetAddForm()
    } catch (err) {
      setAddError(err instanceof ApiRequestError ? err.message : '添加失败')
    }
  }

  const startEditing = (todo: Todo) => {
    setEditingId(todo.id)
    setEditTitle(todo.title)
    setEditDesc(todo.description)
    setEditAssignee(todo.assignee.agent_id)
    setEditError('')
    setDeletingId(null)
  }

  const cancelEditing = () => {
    setEditingId(null)
    setEditError('')
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
        input: {
          title: editTitle.trim(),
          description: editDesc.trim() || '',
          assignee_id: editAssignee || undefined,
        },
      })
      toast.success('TODO 已更新')
      cancelEditing()
    } catch (err) {
      setEditError(err instanceof ApiRequestError ? err.message : '更新失败')
    }
  }

  const handleDelete = async (todoId: string) => {
    if (deletingId !== todoId) {
      setDeletingId(todoId)
      return
    }
    setEditError('')
    try {
      await removeTodo.mutateAsync({
        taskId: task.id,
        todoId,
      })
      toast.success('TODO 已删除')
      setDeletingId(null)
    } catch (err) {
      setEditError(err instanceof ApiRequestError ? err.message : '删除失败')
    }
  }

  const cancelDelete = () => {
    setDeletingId(null)
  }

  const toggleExpand = (id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  return (
    <div className={variant === 'panel' ? 'flex flex-col h-full' : 'flex flex-col'}>
      {/* Panel header (only in panel mode) */}
      {variant === 'panel' && (
        <div className="shrink-0 px-4 py-3 border-b">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">执行清单</span>
            <span className="text-xs text-muted-foreground">
              {task.todos.length} 项{doneCount > 0 ? ` · ${doneCount} 完成` : ''}
            </span>
          </div>
        </div>
      )}

      {/* Todo list */}
      <div className={variant === 'panel' ? 'flex-1 overflow-y-auto px-3 py-3 space-y-2' : 'px-1 py-2 space-y-2'}>
        {task.todos.length === 0 && !addOpen && (
          <p className="text-xs text-muted-foreground text-center py-8">暂无 TODO</p>
        )}

        {task.todos.map((todo) => {
          const hasDetails = todo.description || todo.error
          const isExpanded = expandedIds.has(todo.id)
          const isEditing = editingId === todo.id
          const isDeleting = deletingId === todo.id
          const canModify = todo.status === 'pending' && editable

          if (isEditing) {
            return (
              <div
                key={todo.id}
                className="rounded-lg border bg-blue-50/40 dark:bg-blue-950/10 p-3 space-y-2"
              >
                <Input
                  value={editTitle}
                  onChange={(e) => setEditTitle(e.target.value)}
                  placeholder="TODO 标题"
                  className="h-8 text-xs"
                />
                <Textarea
                  value={editDesc}
                  onChange={(e) => setEditDesc(e.target.value)}
                  placeholder="描述（可选）"
                  rows={2}
                  className="text-xs resize-none"
                />
                <Select value={editAssignee} onValueChange={(val) => setEditAssignee(val ?? '')}>
                  <SelectTrigger className="h-7 text-xs">
                    <SelectValue placeholder="选择执行 Agent" />
                  </SelectTrigger>
                  <SelectContent>
                    {availableAssignees.map((a: Agent) => (
                      <SelectItem key={a.id} value={a.id} className="text-xs">
                        {a.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {editError && <p className="text-xs text-destructive">{editError}</p>}
                <div className="flex justify-end gap-2 pt-1">
                  <Button type="button" variant="ghost" size="sm" className="h-7 text-xs" onClick={cancelEditing}>
                    <X className="size-3 mr-1" /> 取消
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    className="h-7 text-xs"
                    disabled={updateTodo.isPending}
                    onClick={() => void handleUpdate()}
                  >
                    <Check className="size-3 mr-1" /> 保存
                  </Button>
                </div>
              </div>
            )
          }

          return (
            <div
              key={todo.id}
              className={cn(
                'rounded-lg border bg-card overflow-hidden transition-colors',
                todo.status === 'in_progress' && 'border-blue-200 dark:border-blue-800',
                todo.status === 'done' && 'opacity-75',
              )}
            >
              <div className="flex items-start gap-2.5 px-3 py-2.5">
                {/* Status dot */}
                <span
                  className={cn(
                    'mt-1 size-2 shrink-0 rounded-full',
                    statusIndicatorColors[todo.status],
                  )}
                />

                {/* Content */}
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span
                      className={cn(
                        'text-sm truncate',
                        todo.status === 'done' && 'line-through text-muted-foreground',
                      )}
                    >
                      {todo.title}
                    </span>
                  </div>
                  <div className="flex items-center gap-2 mt-0.5">
                    <span className="text-xs text-muted-foreground">{todo.assignee.name}</span>
                    <span className="text-[10px] text-muted-foreground/60">·</span>
                    <span className={cn(
                      'text-[10px]',
                      todo.status === 'done' && 'text-emerald-600',
                      todo.status === 'in_progress' && 'text-sky-600',
                      todo.status === 'failed' && 'text-rose-600',
                      todo.status === 'pending' && 'text-slate-500',
                    )}>
                      {statusLabels[todo.status] || todo.status}
                    </span>
                  </div>
                </div>

                {/* Actions */}
                <div className="flex items-center gap-0.5 shrink-0">
                  {hasDetails && (
                    <button
                      type="button"
                      onClick={() => toggleExpand(todo.id)}
                      className="size-6 flex items-center justify-center rounded hover:bg-muted"
                    >
                      {isExpanded
                        ? <ChevronDown className="size-3.5 text-muted-foreground" />
                        : <ChevronRight className="size-3.5 text-muted-foreground" />
                      }
                    </button>
                  )}
                  {canModify && (
                    <>
                      <button
                        type="button"
                        onClick={() => startEditing(todo)}
                        className="size-6 flex items-center justify-center rounded hover:bg-muted"
                        title="编辑"
                      >
                        <Pencil className="size-3 text-muted-foreground" />
                      </button>
                      {isDeleting ? (
                        <div className="flex items-center gap-0.5">
                          <button
                            type="button"
                            onClick={() => void handleDelete(todo.id)}
                            className="size-6 flex items-center justify-center rounded bg-destructive/10 hover:bg-destructive/20"
                            title="确认删除"
                          >
                            <Check className="size-3 text-destructive" />
                          </button>
                          <button
                            type="button"
                            onClick={cancelDelete}
                            className="size-6 flex items-center justify-center rounded hover:bg-muted"
                            title="取消"
                          >
                            <X className="size-3 text-muted-foreground" />
                          </button>
                        </div>
                      ) : (
                        <button
                          type="button"
                          onClick={() => setDeletingId(todo.id)}
                          className="size-6 flex items-center justify-center rounded hover:bg-muted"
                          title="删除"
                        >
                          <Trash2 className="size-3 text-muted-foreground hover:text-destructive" />
                        </button>
                      )}
                    </>
                  )}
                </div>
              </div>

              {/* Expanded details */}
              {isExpanded && hasDetails && (
                <div className="px-3 pb-3 pl-[26px] space-y-1.5">
                  {todo.description && (
                    <p className="text-xs text-muted-foreground whitespace-pre-wrap">
                      {normalizeEscapedText(todo.description)}
                    </p>
                  )}
                  {todo.error && (
                    <p className="text-xs text-destructive whitespace-pre-wrap bg-destructive/5 rounded p-1.5">
                      {normalizeEscapedText(todo.error)}
                    </p>
                  )}
                </div>
              )}
            </div>
          )
        })}

        {/* Inline add form */}
        {addOpen ? (
          <div className="rounded-lg border bg-amber-50/40 dark:bg-amber-950/10 p-3 space-y-2">
            <Input
              value={addTitle}
              onChange={(e) => setAddTitle(e.target.value)}
              placeholder="TODO 标题 *"
              className="h-8 text-xs"
              autoFocus
            />
            <Textarea
              value={addDesc}
              onChange={(e) => setAddDesc(e.target.value)}
              placeholder="描述（可选）"
              rows={2}
              className="text-xs resize-none"
            />
            <Select value={addAssignee} onValueChange={(val) => setAddAssignee(val ?? '')}>
              <SelectTrigger className="h-7 text-xs">
                <SelectValue placeholder="选择执行 Agent *" />
              </SelectTrigger>
              <SelectContent>
                {availableAssignees.map((a: Agent) => (
                  <SelectItem key={a.id} value={a.id} className="text-xs">
                    {a.name} ({a.status === 'online' ? '在线' : a.status})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {addError && <p className="text-xs text-destructive">{addError}</p>}
            <div className="flex justify-end gap-2 pt-1">
              <Button type="button" variant="ghost" size="sm" className="h-7 text-xs" onClick={resetAddForm}>
                <X className="size-3 mr-1" /> 取消
              </Button>
              <Button
                type="button"
                size="sm"
                className="h-7 text-xs"
                disabled={addTodo.isPending}
                onClick={() => void handleAdd()}
              >
                <Plus className="size-3 mr-1" /> 添加
              </Button>
            </div>
          </div>
        ) : editable && (
          <button
            type="button"
            onClick={() => setAddOpen(true)}
            className="w-full flex items-center justify-center gap-1.5 py-2 rounded-lg border border-dashed border-muted-foreground/30 text-xs text-muted-foreground hover:border-muted-foreground/60 hover:text-foreground transition-colors"
          >
            <Plus className="size-3.5" />
            添加 TODO
          </button>
        )}
      </div>
    </div>
  )
}