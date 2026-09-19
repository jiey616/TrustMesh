import type { TaskArtifact } from '@/types'

/**
 * 交付物判定（与后端不变式对齐）：**`output_name != ""` ⇒ deliverable**。
 *
 * 🔴 不能写成 `kind !== 'deliverable' ⇒ 过程文件`：`kind` 为空的历史数据在消费路径上
 * 必须放行，否则会把既有交付物误判成过程文件。这里用「是交付物」的正向判定，
 * 其余一律归入过程文件（含 kind 为空的历史数据）。
 *
 * 与移动端 `frontend-mobile/src/lib/artifacts.ts` 的 isDeliverable 保持同一口径；
 * 两边是独立子项目，刻意不复用代码，改口径必须两边同步。
 */
export function isDeliverable(a: TaskArtifact): boolean {
  if (a.kind === 'deliverable') return true
  return Boolean(a.output_name && a.output_name.trim() !== '')
}

/** 拆分任务产物：交付物在前（始终展示），过程文件单独归组（UI 可折叠）。 */
export function splitArtifacts(list: TaskArtifact[] | undefined) {
  const all = list ?? []
  return {
    deliverables: all.filter(isDeliverable),
    processes: all.filter((a) => !isDeliverable(a)),
  }
}
