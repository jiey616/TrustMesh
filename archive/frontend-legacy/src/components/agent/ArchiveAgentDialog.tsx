import { useState } from 'react'
import { Archive } from 'lucide-react'
import { toast } from 'sonner'
import { ApiRequestError } from '@/api/client'
import { useDeleteAgent } from '@/hooks/useAgents'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import type { Agent } from '@/types'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  agent: Agent | undefined
  onArchived?: () => void
}

function formatUsage(agent: Agent) {
  const parts: string[] = []
  if (agent.usage.project_count > 0) parts.push(`${agent.usage.project_count} 个项目`)
  if (agent.usage.task_count > 0) parts.push(`${agent.usage.task_count} 个任务`)
  if (agent.usage.todo_count > 0) parts.push(`${agent.usage.todo_count} 个 Todo`)
  return parts.join('、')
}

export function ArchiveAgentDialog({ open, onOpenChange, agent, onArchived }: Props) {
  const deleteAgent = useDeleteAgent()
  const [error, setError] = useState('')

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      setError('')
    }
    onOpenChange(nextOpen)
  }

  const handleArchive = async () => {
    if (!agent || agent.archived) {
      return
    }

    setError('')
    try {
      await deleteAgent.mutateAsync(agent.id)
      toast.success(`Agent "${agent.name}" 已离职`)
      handleOpenChange(false)
      onArchived?.()
    } catch (err) {
      const message = err instanceof ApiRequestError ? err.message : '离职处理失败'
      toast.error(message)
      setError(message)
    }
  }

  const alreadyArchived = agent?.archived === true

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Agent 离职</DialogTitle>
          <DialogDescription>
            {alreadyArchived
              ? '这个 Agent 已经离职，无需重复操作。'
              : '离职后 Agent 会从列表中隐藏，且不能被分配到新的项目或任务。'}
          </DialogDescription>
        </DialogHeader>

        <div className="mt-4 rounded-xl border border-destructive/20 bg-destructive/5 p-4">
          <div className="flex items-start gap-3">
            <div className="mt-0.5 rounded-lg bg-destructive/10 p-2 text-destructive">
              <Archive className="size-4" />
            </div>
            <div className="space-y-1">
              <p className="text-sm font-medium">{agent?.name ?? '当前 Agent'}</p>
              <p className="text-sm text-muted-foreground">
                {alreadyArchived
                  ? 'Agent 已经处于离职状态。'
                  : agent?.usage.in_use
                    ? `当前被 ${formatUsage(agent)} 引用。离职不会影响已关联的历史任务和数据，但该 Agent 的节点 ID 将被释放。`
                    : '离职不会影响已关联的历史任务和数据，但该 Agent 的节点 ID 将被释放。'}
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
            disabled={!agent || alreadyArchived || deleteAgent.isPending}
            onClick={handleArchive}
          >
            {deleteAgent.isPending ? '处理中...' : '确认离职'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
