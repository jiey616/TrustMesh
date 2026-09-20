import { useEffect, useState } from 'react'
import { Toast } from 'antd-mobile'
import { isPdf, isPreviewableImage, formatSize } from '@/lib/artifacts'
import { getArtifactBlob } from '@/api/tasks'
import type { TaskArtifact } from '@/types'

/**
 * 交付物 / 过程产物列表。
 *
 * 🔴 预览**必须**走 `getArtifactBlob`（apiClient 带 Authorization）再转 object URL：
 * 该接口挂在鉴权路由组上，直接 `<img src={...}>` 或 `window.open(接口地址)` 都会恒 401。
 */
export function ArtifactList({
  taskId,
  items,
}: {
  taskId: string
  items: TaskArtifact[]
}) {
  const [preview, setPreview] = useState<{ url: string; name: string } | null>(null)

  // object URL 必须显式回收，否则切换任务时会持续泄漏
  useEffect(() => {
    return () => {
      if (preview) URL.revokeObjectURL(preview.url)
    }
  }, [preview])

  async function open(a: TaskArtifact) {
    try {
      const blob = await getArtifactBlob(taskId, a.transfer_id)
      const url = URL.createObjectURL(blob)
      if (isPreviewableImage(a)) {
        setPreview({ url, name: a.file_name })
        return
      }
      if (isPdf(a)) {
        window.open(url, '_blank')
        return
      }
      const link = document.createElement('a')
      link.href = url
      link.download = a.file_name
      link.click()
    } catch (err) {
      Toast.show({ content: err instanceof Error ? err.message : '打开失败' })
    }
  }

  if (items.length === 0) {
    return <p className="px-1 py-2 text-[13px] text-[var(--tm-text-3)]">暂无文件</p>
  }

  return (
    <>
      <div className="overflow-hidden rounded-[var(--tm-radius-card)] border border-[var(--tm-line)] bg-white">
        {items.map((a) => (
          <button
            key={a.transfer_id}
            type="button"
            onClick={() => void open(a)}
            className="flex w-full items-center gap-3 border-b border-[var(--tm-line)] px-3 py-[10px] text-left last:border-b-0"
          >
            <span className="h-8 w-8 shrink-0 rounded-[6px] bg-[var(--tm-brand-soft)]" />
            <span className="min-w-0 flex-1">
              <span className="block truncate text-[14px]">{a.file_name}</span>
              <span className="block text-[12px] text-[var(--tm-text-3)]">
                {formatSize(a.file_size)} · {a.from_agent_name || '未知来源'}
              </span>
            </span>
            {a.output_name ? (
              <span className="shrink-0 rounded-full bg-[var(--tm-brand-soft)] px-2 py-[2px] text-[11px] text-[var(--tm-brand)]">
                {a.output_name}
              </span>
            ) : null}
          </button>
        ))}
      </div>

      {preview ? (
        <div
          className="fixed inset-0 z-50 flex flex-col bg-black/90"
          onClick={() => setPreview(null)}
        >
          <div className="tm-safe-top flex items-center justify-between px-4 py-3 text-white">
            <span className="max-w-[70%] truncate text-[13px]">{preview.name}</span>
            <button type="button" className="text-[15px]" onClick={() => setPreview(null)}>
              关闭
            </button>
          </div>
          <div className="flex min-h-0 flex-1 items-center justify-center px-2">
            <img src={preview.url} alt={preview.name} className="max-h-full max-w-full object-contain" />
          </div>
        </div>
      ) : null}
    </>
  )
}
