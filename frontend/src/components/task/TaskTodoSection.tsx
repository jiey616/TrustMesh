import { ChevronDown, ChevronRight, Plus } from 'lucide-react'
import { useState } from 'react'
import { TodoList } from './TodoList'
import { Button } from '@/components/ui/button'
import type { TaskArtifact, Todo } from '@/types'

interface TaskTodoSectionProps {
  todos: Todo[]
  artifacts: TaskArtifact[]
  defaultCollapsed?: boolean
  editable?: boolean
  onAddTodo?: () => void
  onEditTodo?: (todo: Todo) => void
  onDeleteTodo?: (todo: Todo) => void
  onReviewTodo?: (todo: Todo, action: 'approve' | 'reject') => void
}

function summarizeTodos(todos: Todo[]) {
  let done = 0
  let inProgress = 0
  let failed = 0
  let pending = 0
  let canceled = 0

  for (const todo of todos) {
    switch (todo.status) {
      case 'done':
        done += 1
        break
      case 'in_progress':
        inProgress += 1
        break
      case 'failed':
        failed += 1
        break
      case 'canceled':
        canceled += 1
        break
      default:
        pending += 1
        break
    }
  }

  const parts = [`${todos.length} 项`]
  if (done > 0) parts.push(`${done} 完成`)
  if (inProgress > 0) parts.push(`${inProgress} 进行中`)
  if (pending > 0) parts.push(`${pending} 待处理`)
  if (failed > 0) parts.push(`${failed} 失败`)
  if (canceled > 0) parts.push(`${canceled} 已取消`)

  return parts.join(' · ')
}

export function TaskTodoSection({
  todos,
  artifacts,
  defaultCollapsed = true,
  editable = false,
  onAddTodo,
  onEditTodo,
  onDeleteTodo,
  onReviewTodo,
}: TaskTodoSectionProps) {
  const [collapsed, setCollapsed] = useState(defaultCollapsed)
  const summary = summarizeTodos(todos)

  return (
    <section className="rounded-lg border bg-card">
      <button
        type="button"
        className="flex w-full items-center justify-between gap-3 px-4 py-3 text-left"
        onClick={() => setCollapsed((current) => !current)}
      >
        <div className="min-w-0">
          <div className="text-sm font-medium">执行清单</div>
          <div className="mt-1 text-xs text-muted-foreground">{summary}</div>
        </div>
        {collapsed ? (
          <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
        ) : (
          <ChevronDown className="size-4 shrink-0 text-muted-foreground" />
        )}
      </button>

      {!collapsed && (
        <div className="border-t px-3 py-3">
          <TodoList
            todos={todos}
            artifacts={artifacts}
            onEditTodo={editable ? onEditTodo : undefined}
            onDeleteTodo={editable ? onDeleteTodo : undefined}
            onReviewTodo={onReviewTodo}
          />
          {editable && onAddTodo && (
            <Button
              variant="ghost"
              size="sm"
              className="mt-2 w-full gap-1 text-muted-foreground hover:text-foreground"
              onClick={onAddTodo}
            >
              <Plus className="size-3.5" />
              添加 TODO
            </Button>
          )}
        </div>
      )}
    </section>
  )
}