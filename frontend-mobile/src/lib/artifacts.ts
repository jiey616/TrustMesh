import type { TaskArtifact } from '@/types'

/**
 * 交付物判定（与后端不变式对齐）：**`output_name != ""` ⇒ deliverable**。
 *
 * 🔴 不能写成 `kind !== 'deliverable' ⇒ 过程产物`：`kind` 为空的历史数据在消费路径上
 * 必须放行，否则会把既有交付物误判成过程文件。这里用「是交付物」的正向判定，
 * 其余一律归入过程产物（含 kind 为空的历史数据）。
 */
export function isDeliverable(a: TaskArtifact): boolean {
  if (a.kind === 'deliverable') return true
  return Boolean(a.output_name && a.output_name.trim() !== '')
}

export function splitArtifacts(list: TaskArtifact[] | undefined) {
  const all = list ?? []
  return {
    deliverables: all.filter(isDeliverable),
    processes: all.filter((a) => !isDeliverable(a)),
  }
}

export function isPreviewableImage(a: TaskArtifact): boolean {
  return a.mime_type.startsWith('image/')
}

export function isPdf(a: TaskArtifact): boolean {
  return a.mime_type === 'application/pdf' || a.file_name.toLowerCase().endsWith('.pdf')
}

export function formatSize(bytes: number): string {
  if (!bytes) return '—'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}
