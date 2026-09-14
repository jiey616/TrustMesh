import { useEffect, useRef, useState } from 'react'
import { ImagePlus, Paperclip, Send, X } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { uploadChatAttachments } from '@/api/chatAttachments'
import type { ChatAttachment } from '@/types'

interface PendingAttachment {
  key: string
  file: File
  previewUrl?: string
}

interface MessageInputProps {
  onSend: (content: string, attachments: ChatAttachment[]) => void
  disabled?: boolean
  placeholder?: string
}

const isImageFile = (file: File) => file.type.startsWith('image/')

function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  const i = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1024)))
  return `${(bytes / 1024 ** i).toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

export function MessageInput({ onSend, disabled, placeholder = '输入你的需求...' }: MessageInputProps) {
  const [value, setValue] = useState('')
  const [isComposing, setIsComposing] = useState(false)
  const [pending, setPending] = useState<PendingAttachment[]>([])
  const [isUploading, setIsUploading] = useState(false)
  const [isDragging, setIsDragging] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
      textareaRef.current.style.height = Math.min(textareaRef.current.scrollHeight, 200) + 'px'
    }
  }, [value])

  // Release object URLs we created to avoid leaks.
  useEffect(() => {
    const urls = pending.map((p) => p.previewUrl).filter((u): u is string => !!u)
    return () => urls.forEach((u) => URL.revokeObjectURL(u))
  }, [pending])

  const addFiles = (files: File[]) => {
    if (!files.length) return
    const next = files.map((file) => {
      const attachment: PendingAttachment = {
        key: `${file.name}-${file.size}-${file.lastModified}-${Math.random().toString(36).slice(2)}`,
        file,
      }
      if (isImageFile(file)) {
        attachment.previewUrl = URL.createObjectURL(file)
      }
      return attachment
    })
    setPending((prev) => [...prev, ...next])
  }

  const removeAttachment = (key: string) => {
    setPending((prev) => {
      const target = prev.find((p) => p.key === key)
      if (target?.previewUrl) URL.revokeObjectURL(target.previewUrl)
      return prev.filter((p) => p.key !== key)
    })
  }

  const handlePaste = (e: React.ClipboardEvent<HTMLTextAreaElement>) => {
    const files = Array.from(e.clipboardData?.files ?? [])
    if (files.length) {
      addFiles(files)
      e.preventDefault()
    }
  }

  const handleDrop = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    setIsDragging(false)
    addFiles(Array.from(e.dataTransfer?.files ?? []))
  }

  const handleSubmit = async () => {
    const trimmed = value.trim()
    if ((!trimmed && pending.length === 0) || disabled || isUploading) return

    setIsUploading(true)
    let attachments: ChatAttachment[] = []
    if (pending.length) {
      const { ok, failed } = await uploadChatAttachments(pending.map((p) => p.file))
      attachments = ok
      if (failed.length) {
        toast.error(`部分文件上传失败：${failed.join('、')}`)
      }
    }
    setIsUploading(false)

    onSend(trimmed, attachments)
    setValue('')
    setPending([])
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey && !isComposing) {
      e.preventDefault()
      void handleSubmit()
    }
  }

  const canSend = !disabled && !isUploading && (value.trim().length > 0 || pending.length > 0)

  return (
    <div
      className="rounded-2xl border bg-card p-3 shadow-sm transition-shadow focus-within:shadow-md focus-within:border-primary/30"
      onDragOver={(e) => {
        e.preventDefault()
        setIsDragging(true)
      }}
      onDragLeave={() => setIsDragging(false)}
      onDrop={handleDrop}
    >
      {pending.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-2">
          {pending.map((att) => (
            <div
              key={att.key}
              className="relative flex items-center gap-2 rounded-xl border bg-background/80 px-2 py-1 pr-7 text-xs"
            >
              {att.previewUrl ? (
                <img src={att.previewUrl} alt={att.file.name} className="size-8 rounded-md object-cover" />
              ) : (
                <Paperclip className="size-4 shrink-0 text-muted-foreground" />
              )}
              <span className="max-w-40 truncate" title={att.file.name}>
                {att.file.name}
              </span>
              <span className="shrink-0 text-[10px] text-muted-foreground">{formatBytes(att.file.size)}</span>
              <button
                type="button"
                className="absolute right-1 top-1 rounded-md p-0.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                onClick={() => removeAttachment(att.key)}
                aria-label="移除附件"
              >
                <X className="size-3.5" />
              </button>
            </div>
          ))}
        </div>
      )}

      <div className="flex items-end gap-2">
        <Button
          type="button"
          size="icon"
          variant="ghost"
          className="size-9 shrink-0 rounded-xl text-muted-foreground hover:text-foreground"
          onClick={() => fileInputRef.current?.click()}
          disabled={disabled || isUploading}
          title="添加图片或文件（也支持粘贴 / 拖拽）"
        >
          <ImagePlus className="size-4" />
        </Button>
        <input
          ref={fileInputRef}
          type="file"
          multiple
          className="hidden"
          onChange={(e) => {
            if (e.target.files?.length) addFiles(Array.from(e.target.files))
            e.target.value = ''
          }}
        />
        <textarea
          ref={textareaRef}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={handleKeyDown}
          onPaste={handlePaste}
          onCompositionStart={() => setIsComposing(true)}
          onCompositionEnd={() => setIsComposing(false)}
          placeholder={placeholder}
          disabled={disabled || isUploading}
          rows={2}
          className="flex-1 resize-none bg-transparent px-1 py-1 text-sm leading-relaxed outline-none placeholder:text-muted-foreground disabled:opacity-50"
        />
        <Button
          size="icon"
          className="size-9 shrink-0 rounded-xl"
          onClick={() => void handleSubmit()}
          disabled={!canSend}
          title="发送"
        >
          <Send className="size-4" />
        </Button>
      </div>

      {isDragging && (
        <div className="pointer-events-none mt-2 rounded-lg border border-dashed border-primary/40 bg-primary/5 px-2 py-2 text-center text-xs text-muted-foreground">
          松开以上传图片 / 文件
        </div>
      )}
    </div>
  )
}
