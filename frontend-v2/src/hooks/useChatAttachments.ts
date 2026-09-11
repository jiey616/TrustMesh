import { useEffect, useRef, useState } from 'react'
import { uploadChatAttachments } from '@/api/chatAttachments'
import type { ChatAttachment } from '@/types'

export interface PendingAttachmentFile {
  file: File
  previewUrl?: string
}

export interface UploadResult {
  attachments: ChatAttachment[]
  /** 上传失败的文件个数（容忍部分失败：成功项仍会交给 onSubmit） */
  failedCount: number
}

export function isImageFile(file: File): boolean {
  return (file.type || '').startsWith('image/')
}

export function formatFileSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return ''
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

/**
 * 附件采集共享 hook：粘贴 / 拖拽 / 文件选择器三种方式拿到本地文件字节，
 * 图片即时 objectURL 缩略预览，发送时统一上传成 ChatAttachment。
 *
 * 浏览器只能读到文件字节、拿不到本地绝对路径；真实路径读取需桌面壳那一步。
 * pending 文件在卸载时统一 revoke 预览 objectURL。
 */
export function useChatAttachments() {
  const [pendingFiles, setPendingFiles] = useState<PendingAttachmentFile[]>([])
  const [uploading, setUploading] = useState(false)
  const pendingRef = useRef<PendingAttachmentFile[]>([])

  const setPendingBoth = (next: PendingAttachmentFile[]) => {
    pendingRef.current = next
    setPendingFiles(next)
  }

  // 卸载时释放所有预览 objectURL
  useEffect(() => {
    return () => {
      pendingRef.current.forEach((p) => {
        if (p.previewUrl) URL.revokeObjectURL(p.previewUrl)
      })
    }
  }, [])

  const addFiles = (files: FileList | File[] | null) => {
    if (!files || files.length === 0) return
    const additions: PendingAttachmentFile[] = Array.from(files).map((file) => ({
      file,
      previewUrl: isImageFile(file) ? URL.createObjectURL(file) : undefined,
    }))
    setPendingBoth([...pendingRef.current, ...additions])
  }

  const removeFile = (index: number) => {
    const target = pendingRef.current[index]
    if (target?.previewUrl) URL.revokeObjectURL(target.previewUrl)
    setPendingBoth(pendingRef.current.filter((_, i) => i !== index))
  }

  const clearPending = () => {
    pendingRef.current.forEach((p) => {
      if (p.previewUrl) URL.revokeObjectURL(p.previewUrl)
    })
    setPendingBoth([])
  }

  const uploadPending = async (): Promise<UploadResult> => {
    const files = pendingRef.current
    if (files.length === 0) return { attachments: [], failedCount: 0 }
    setUploading(true)
    try {
      const { ok, failed } = await uploadChatAttachments(files.map((p) => p.file))
      return { attachments: ok, failedCount: failed.length }
    } finally {
      setUploading(false)
    }
  }

  return { pendingFiles, uploading, addFiles, removeFile, clearPending, uploadPending }
}