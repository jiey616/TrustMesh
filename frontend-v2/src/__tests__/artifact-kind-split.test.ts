import { describe, expect, it } from 'vitest'
import { isDeliverable, splitArtifacts } from '@/lib/artifactKind'
import type { TaskArtifact } from '@/types'

function art(partial: Partial<TaskArtifact>): TaskArtifact {
  return {
    transfer_id: 'tr-1',
    task_id: 't-1',
    file_name: 'a.md',
    file_size: 1,
    mime_type: 'text/markdown',
    from_node_id: 'n',
    from_agent_id: 'ag',
    from_agent_name: 'agent',
    created_at: '2026-09-20T00:00:00Z',
    ...partial,
  }
}

describe('isDeliverable（交付物正向判定，与后端不变式/移动端同口径）', () => {
  it('kind=deliverable ⇒ 交付物', () => {
    expect(isDeliverable(art({ kind: 'deliverable' }))).toBe(true)
  })

  it('kind 为空但 output_name 非空 ⇒ 交付物（历史数据必须放行）', () => {
    expect(isDeliverable(art({ kind: undefined, output_name: '成片文案' }))).toBe(true)
  })

  it('output_name 为纯空白也视作未绑定 ⇒ 过程文件', () => {
    expect(isDeliverable(art({ kind: undefined, output_name: '   ' }))).toBe(false)
  })

  it('output_name 优先：kind=process 但 output_name 非空 ⇒ 按交付物放行（镜像移动端口径；降级时两字段同清，正常不会出现该组合）', () => {
    expect(isDeliverable(art({ kind: 'process', output_name: '残留' }))).toBe(true)
  })

  it('kind 为空且无 output_name ⇒ 过程文件', () => {
    expect(isDeliverable(art({ kind: undefined }))).toBe(false)
  })
})

describe('splitArtifacts', () => {
  it('按口径拆成两组，undefined 输入返回两个空数组', () => {
    const list = [
      art({ transfer_id: 'd1', kind: 'deliverable' }),
      art({ transfer_id: 'p1', kind: 'process' }),
      art({ transfer_id: 'h1', kind: undefined, output_name: '旧交付' }),
      art({ transfer_id: 'p2', kind: undefined }),
    ]
    const { deliverables, processes } = splitArtifacts(list)
    expect(deliverables.map((a) => a.transfer_id)).toEqual(['d1', 'h1'])
    expect(processes.map((a) => a.transfer_id)).toEqual(['p1', 'p2'])
    expect(splitArtifacts(undefined).deliverables).toEqual([])
    expect(splitArtifacts(undefined).processes).toEqual([])
  })
})
