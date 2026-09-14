import { useState, type ReactNode } from 'react'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: string
  submitLabel: string
  pendingLabel?: string
  pending?: boolean
  error?: string
  submitDisabled?: boolean
  initialName?: string
  initialDescription?: string
  children?: ReactNode
  onSubmit: (values: { name: string; description: string }) => Promise<void>
}

export function ProjectDialog({
  open,
  onOpenChange,
  title,
  description,
  submitLabel,
  pendingLabel = '保存中...',
  pending = false,
  error = '',
  submitDisabled = false,
  initialName = '',
  initialDescription = '',
  children,
  onSubmit,
}: Props) {
  const [name, setName] = useState(initialName)
  const [projectDescription, setProjectDescription] = useState(initialDescription)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    await onSubmit({
      name: name.trim(),
      description: projectDescription.trim(),
    })
  }

  const isSubmitDisabled = pending || submitDisabled || !name.trim() || !projectDescription.trim()

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="mt-4 flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <label className="text-sm font-medium">项目名称</label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="例如：TrustMesh MVP"
              required
            />
          </div>
          <div className="flex flex-col gap-2">
            <label className="text-sm font-medium">项目描述</label>
            <Textarea
              value={projectDescription}
              onChange={(e) => setProjectDescription(e.target.value)}
              placeholder="描述项目的目标和范围"
              rows={3}
              required
            />
          </div>
          {children}
          {error && <p className="text-sm text-destructive">{error}</p>}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" disabled={isSubmitDisabled}>
              {pending ? pendingLabel : submitLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
