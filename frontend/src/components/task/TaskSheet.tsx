import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { TaskStatusBadge, PriorityBadge } from '@/components/shared/StatusBadge'
import { TaskFeed } from './TaskFeed'
import { TaskResultView } from './TaskResult'
import { TaskDescription } from './TaskDescription'
import { TaskTodoSection } from './TaskTodoSection'
import { useTask } from '@/hooks/useTasks'
import { reviewTodo } from '@/api/tasks'
import { Separator } from '@/components/ui/separator'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from 'sonner'
import { useState } from 'react'
import type { TaskDetail, Todo } from '@/types'

interface TaskSheetProps {
  taskId: string | null
  onClose: () => void
}

export function TaskSheet({ taskId, onClose }: TaskSheetProps) {
  const { data: task } = useTask(taskId ?? undefined)

  return (
    <Sheet open={!!taskId} onOpenChange={() => onClose()}>
      <SheetContent className="max-w-2xl">
        {task && (
          <TaskSheetBody key={task.id} task={task} />
        )}
      </SheetContent>
    </Sheet>
  )
}

function TaskSheetBody({ task }: { task: TaskDetail }) {
  const [tab, setTab] = useState('feed')
  const [rejectTodo, setRejectTodo] = useState<Todo | null>(null)
  const [rejectReason, setRejectReason] = useState('')
  const [reviewing, setReviewing] = useState(false)

  const handleReview = async (todo: Todo, action: 'approve' | 'reject') => {
    if (action === 'reject') {
      setRejectTodo(todo)
      setRejectReason('')
      return
    }
    setReviewing(true)
    try {
      await reviewTodo(task.id, todo.id, 'approve')
      toast.success(`${todo.title} 已确认通过`)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '操作失败，请稍后重试')
    } finally {
      setReviewing(false)
    }
  }

  const handleRejectConfirm = async () => {
    if (!rejectTodo) return
    if (!rejectReason.trim()) {
      toast.error('请填写退回原因')
      return
    }
    setReviewing(true)
    try {
      await reviewTodo(task.id, rejectTodo.id, 'reject', rejectReason.trim())
      setRejectTodo(null)
      toast.success(`${rejectTodo.title} 已退回上一个 Todo 重做`)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '操作失败，请稍后重试')
    } finally {
      setReviewing(false)
    }
  }

  return (
    <>
      <SheetHeader>
        <div className="flex items-center gap-2 flex-wrap pr-8">
          <TaskStatusBadge status={task.status} />
          <PriorityBadge priority={task.priority} />
        </div>
        <SheetTitle className="text-lg mt-1">{task.title}</SheetTitle>
        {task.description && (
          <TaskDescription description={task.description} />
        )}
      </SheetHeader>

      <Separator className="my-4" />

      <div className="flex flex-1 flex-col gap-4 px-6 pb-6">
        <TaskTodoSection
          todos={task.todos}
          artifacts={task.artifacts}
          onReviewTodo={handleReview}
        />

        <Tabs value={tab} onValueChange={setTab}>
          <TabsList>
            <TabsTrigger value="feed">动态</TabsTrigger>
            <TabsTrigger value="result">结果</TabsTrigger>
          </TabsList>

          <TabsContent value="feed">
            <div className="h-[calc(100vh-280px)]">
              <TaskFeed taskId={task.id} />
            </div>
          </TabsContent>

          <TabsContent value="result">
            <ScrollArea className="max-h-[calc(100vh-280px)]">
              <TaskResultView taskId={task.id} result={task.result} artifacts={task.artifacts} />
            </ScrollArea>
          </TabsContent>
        </Tabs>
      </div>

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
    </>
  )
}
