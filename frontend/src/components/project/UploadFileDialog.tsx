import { useRef, useState } from 'react'
import { Upload, Loader2, FileText } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogClose,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'

interface Props {
  projectId: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onUpload: (formData: FormData) => Promise<void>
}

export function UploadFileDialog({ projectId: _projectId, open, onOpenChange, onUpload }: Props) {
  const [selectedFile, setSelectedFile] = useState<File | null>(null)
  const [uploading, setUploading] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0] ?? null
    setSelectedFile(file)
  }

  const handleSubmit = async () => {
    if (!selectedFile) return
    setUploading(true)
    try {
      const formData = new FormData()
      formData.append('file', selectedFile)
      await onUpload(formData)
      setSelectedFile(null)
      onOpenChange(false)
    } catch {
      // error handled by parent via mutation state
    } finally {
      setUploading(false)
    }
  }

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      setSelectedFile(null)
    }
    onOpenChange(nextOpen)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>上传文件</DialogTitle>
          <DialogDescription>
            选择文件上传到当前项目
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          {/* File picker area */}
          <div
            className="border-2 border-dashed rounded-lg p-8 text-center cursor-pointer hover:border-primary/50 transition-colors"
            onClick={() => fileInputRef.current?.click()}
          >
            {selectedFile ? (
              <div className="flex flex-col items-center gap-2">
                <FileText className="size-8 text-primary" />
                <span className="text-sm font-medium">{selectedFile.name}</span>
                <span className="text-xs text-muted-foreground">
                  {(selectedFile.size / 1024).toFixed(1)} KB
                </span>
              </div>
            ) : (
              <div className="flex flex-col items-center gap-2 text-muted-foreground">
                <Upload className="size-8" />
                <span className="text-sm">点击选择文件</span>
                <span className="text-xs">支持任意文件类型</span>
              </div>
            )}
            <input
              ref={fileInputRef}
              type="file"
              className="hidden"
              onChange={handleFileChange}
            />
          </div>
        </div>

        <div className="flex justify-end gap-2">
          <DialogClose>
            <Button variant="outline" disabled={uploading}>取消</Button>
          </DialogClose>
          <Button onClick={handleSubmit} disabled={!selectedFile || uploading}>
            {uploading ? (
              <>
                <Loader2 className="size-4 mr-1.5 animate-spin" />
                上传中...
              </>
            ) : (
              '上传'
            )}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
