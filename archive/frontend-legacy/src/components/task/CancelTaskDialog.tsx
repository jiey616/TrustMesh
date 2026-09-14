import { useState } from 'react'
import { CircleSlash2 } from 'lucide-react'
import { toast } from 'sonner'
import { ApiRequestError } from '@/api/client'
import { useCancelTask } from '@/hooks/useTasks'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import type { TaskDetail } from '@/types'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  task: TaskDetail | undefined
  onCanceled?: () => void
}

export function CancelTaskDialog({ open, onOpenChange, task, onCanceled }: Props) {
  const cancelTask = useCancelTask()
  const [error, setError] = useState('')

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      setError('')
    }
    onOpenChange(nextOpen)
  }

  const cancelable = task?.status === 'planning' || task?.status === 'pending' || task?.status === 'in_progress'

  const handleCancelTask = async () => {
    if (!task || !cancelable) {
      return
    }

    setError('')
    try {
      await cancelTask.mutateAsync({
        taskId: task.id,
        reason: '用户手动终止',
      })
      toast.success('任务已终止')
      handleOpenChange(false)
      onCanceled?.()
    } catch (err) {
      const message = err instanceof ApiRequestError ? err.message : '终止任务失败'
      toast.error(message)
      setError(message)
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>终止任务</DialogTitle>
          <DialogDescription>
            {cancelable
              ? '终止后，任务会停止接收后续进度和结果回写，未完成的 Todo 会一并标记为已取消。'
              : '当前任务已经结束或已取消，无需重复终止。'}
          </DialogDescription>
        </DialogHeader>

        <div className="mt-4 rounded-xl border border-destructive/20 bg-destructive/5 p-4">
          <div className="flex items-start gap-3">
            <div className="mt-0.5 rounded-lg bg-destructive/10 p-2 text-destructive">
              <CircleSlash2 className="size-4" />
            </div>
            <div className="space-y-1">
              <p className="text-sm font-medium">{task?.title ?? '当前任务'}</p>
              <p className="text-sm text-muted-foreground">
                {cancelable
                  ? '终止不会删除已产生的结果与评论，但会冻结这次执行。'
                  : '任务当前不处于可终止状态。'}
              </p>
            </div>
          </div>
        </div>

        {error && <p className="text-sm text-destructive">{error}</p>}

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => handleOpenChange(false)}>
            取消
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={!task || !cancelable || cancelTask.isPending}
            onClick={handleCancelTask}
          >
            {cancelTask.isPending ? '终止中...' : '确认终止'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
