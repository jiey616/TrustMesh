import { ChevronDown, ChevronRight, MoreHorizontal, Pencil, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { cn } from '@/lib/utils'
import { normalizeEscapedText } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import type { Todo, TaskArtifact } from '@/types'

const statusIndicatorColors = {
  planning: 'bg-cyan-500',
  review: 'bg-amber-500',
  pending: 'bg-slate-400',
  in_progress: 'bg-sky-500 animate-pulse',
  done: 'bg-emerald-500',
  failed: 'bg-rose-500',
  canceled: 'bg-slate-500/60',
}

interface TodoListProps {
  todos: Todo[]
  artifacts: TaskArtifact[]
  variant?: 'card' | 'nested'
  onEditTodo?: (todo: Todo) => void
  onDeleteTodo?: (todo: Todo) => void
}

export function TodoList({ todos, artifacts, variant = 'card', onEditTodo, onDeleteTodo }: TodoListProps) {
  const safeArtifacts = artifacts ?? []

  if (todos.length === 0) {
    return <p className="py-8 text-center text-sm text-muted-foreground">暂无 Todo</p>
  }

  return (
    <div className={cn('flex flex-col', variant === 'card' ? 'gap-1' : 'gap-0')}>
      {todos.map((todo) => (
        <TodoItem
          key={todo.id}
          todo={todo}
          artifacts={safeArtifacts}
          variant={variant}
          onEdit={onEditTodo}
          onDelete={onDeleteTodo}
        />
      ))}
    </div>
  )
}

function TodoItem({
  todo,
  artifacts,
  variant,
  onEdit,
  onDelete,
}: {
  todo: Todo
  artifacts: TaskArtifact[]
  variant: 'card' | 'nested'
  onEdit?: (todo: Todo) => void
  onDelete?: (todo: Todo) => void
}) {
  const [expanded, setExpanded] = useState(false)
  const relatedArtifacts = (artifacts ?? []).filter((a) => a.todo_id === todo.id)
  const hasDetails = todo.description || todo.error || relatedArtifacts.length > 0
  const isCard = variant === 'card'
  const canModify = todo.status === 'pending'

  return (
    <div
      className={cn(
        isCard
          ? 'rounded-lg border bg-card'
          : 'border-t border-border/60 first:border-t-0'
      )}
    >
      <div className={cn('flex items-start gap-3', isCard ? 'p-3' : 'py-3 pr-1')}>
        <button
          type="button"
          onClick={() => hasDetails && setExpanded(!expanded)}
          className={cn(
            'flex min-w-0 flex-1 items-start gap-3 text-left',
            hasDetails && 'cursor-pointer rounded-md transition-colors hover:bg-muted/40',
          )}
        >
          <span
            aria-hidden="true"
            className={cn(
              'mt-1.5 size-2 shrink-0 rounded-full',
              statusIndicatorColors[todo.status],
            )}
          />
          <div className="flex min-w-0 flex-1 items-start gap-3">
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <span className={cn('truncate font-medium', isCard ? 'text-sm' : 'text-[13px]')}>
                  {todo.title}
                </span>
              </div>
            </div>
            <span className="mt-0.5 shrink-0 text-xs text-muted-foreground">
              {todo.assignee.name}
            </span>
          </div>
          {hasDetails && (
            expanded
              ? <ChevronDown className="size-4 shrink-0 text-muted-foreground mt-0.5" />
              : <ChevronRight className="size-4 shrink-0 text-muted-foreground mt-0.5" />
          )}
        </button>

        {/* Edit/Delete menu for pending todos */}
        {canModify && (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button variant="ghost" size="icon" className="size-7 shrink-0 -mr-1">
                  <MoreHorizontal className="size-3.5" />
                </Button>
              }
            />
            <DropdownMenuContent align="end" className="w-32">
              {onEdit && (
                <DropdownMenuItem onClick={() => onEdit(todo)}>
                  <Pencil className="mr-2 size-3.5" />
                  编辑
                </DropdownMenuItem>
              )}
              {onDelete && (
                <DropdownMenuItem onClick={() => onDelete(todo)} className="text-destructive focus:text-destructive">
                  <Trash2 className="mr-2 size-3.5" />
                  删除
                </DropdownMenuItem>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>

      {expanded && hasDetails && (
        <div
          className={cn(
            'ml-5 flex flex-col gap-2 text-sm',
            isCard ? 'px-3 pb-3 pt-0' : 'mb-3 border-l border-border/70 pl-3',
          )}
        >
          {todo.description && (
            <p className="text-muted-foreground whitespace-pre-wrap">{normalizeEscapedText(todo.description)}</p>
          )}
          {todo.error && (
            <p className="rounded-md bg-destructive/10 p-2 text-xs text-destructive whitespace-pre-wrap">
              {normalizeEscapedText(todo.error)}
            </p>
          )}
          {relatedArtifacts.length > 0 && (
            <div className="flex flex-wrap gap-1">
              {relatedArtifacts.map((a) => (
                <Badge key={a.transfer_id} variant={isCard ? 'secondary' : 'outline'} className="text-xs">
                  {a.file_name}
                </Badge>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}