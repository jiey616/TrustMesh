import { useState } from 'react'
import { Archive } from 'lucide-react'
import { toast } from 'sonner'
import { ApiRequestError } from '@/api/client'
import { useArchiveProject } from '@/hooks/useProjects'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import type { Project } from '@/types'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  project: Project | undefined
  onArchived?: () => void
}

export function ArchiveProjectDialog({ open, onOpenChange, project, onArchived }: Props) {
  const archiveProject = useArchiveProject()
  const [error, setError] = useState('')

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      setError('')
    }
    onOpenChange(nextOpen)
  }

  const handleArchive = async () => {
    if (!project || project.status === 'archived') {
      return
    }

    setError('')
    try {
      await archiveProject.mutateAsync(project.id)
      toast.success(`项目“${project.name}”已归档`)
      handleOpenChange(false)
      onArchived?.()
    } catch (err) {
      const message = err instanceof ApiRequestError ? err.message : '归档失败'
      toast.error(message)
      setError(message)
    }
  }

  const alreadyArchived = project?.status === 'archived'

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>归档项目</DialogTitle>
          <DialogDescription>
            {alreadyArchived
              ? '这个项目已经归档，无需重复操作。'
              : '归档后项目会从默认列表中隐藏，且不能继续发起新需求或向现有会话追加消息。'}
          </DialogDescription>
        </DialogHeader>

        <div className="mt-4 rounded-xl border border-destructive/20 bg-destructive/5 p-4">
          <div className="flex items-start gap-3">
            <div className="mt-0.5 rounded-lg bg-destructive/10 p-2 text-destructive">
              <Archive className="size-4" />
            </div>
            <div className="space-y-1">
              <p className="text-sm font-medium">{project?.name ?? '当前项目'}</p>
              <p className="text-sm text-muted-foreground">
                {alreadyArchived
                  ? '项目状态已经是已归档。'
                  : '归档不会删除历史任务和会话，但会冻结继续协作的入口。'}
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
            disabled={!project || alreadyArchived || archiveProject.isPending}
            onClick={handleArchive}
          >
            {archiveProject.isPending ? '归档中...' : '确认归档'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
