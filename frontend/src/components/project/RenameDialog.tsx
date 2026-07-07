import { useState, useEffect, useRef } from 'react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

interface RenameDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentName: string
  isFolder: boolean
  onConfirm: (newName: string) => void
}

export function RenameDialog({
  open,
  onOpenChange,
  currentName,
  isFolder,
  onConfirm,
}: RenameDialogProps) {
  const [name, setName] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (open) {
      setName(currentName)
      // Focus input after dialog animation
      setTimeout(() => inputRef.current?.focus(), 100)
    }
  }, [open, currentName])

  const handleConfirm = () => {
    const trimmed = name.trim()
    if (!trimmed || trimmed === currentName) {
      onOpenChange(false)
      return
    }
    onConfirm(trimmed)
    onOpenChange(false)
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      handleConfirm()
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            重命名{isFolder ? '文件夹' : '文件'}
          </DialogTitle>
        </DialogHeader>
        <div className="py-4">
          <Input
            ref={inputRef}
            value={name}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="输入新名称"
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={handleConfirm} disabled={!name.trim() || name.trim() === currentName}>
            确定
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
