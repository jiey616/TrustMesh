import { describe, expect, it } from 'vitest'
import { isDeliverable, splitArtifacts } from './artifacts'
import type { TaskArtifact } from '@/types'

function art(over: Partial<TaskArtifact>): TaskArtifact {
  return {
    transfer_id: 'tr_1',
    task_id: 't_1',
    file_name: 'a.md',
    file_size: 10,
    mime_type: 'text/markdown',
    from_agent_name: 'agent',
    created_at: '',
    ...over,
  }
}

describe('交付物判定', () => {
  it('output_name 非空即交付物（后端不变式：output_name != "" ⇒ deliverable）', () => {
    expect(isDeliverable(art({ output_name: '剧本' }))).toBe(true)
    expect(isDeliverable(art({ kind: 'deliverable' }))).toBe(true)
  })

  it('kind=process 且无 output_name 归为过程产物', () => {
    expect(isDeliverable(art({ kind: 'process' }))).toBe(false)
  })

  it('🔴 kind 为空的历史数据：有 output_name 仍算交付物，没有则归过程产物', () => {
    // 反向写法 `kind !== 'deliverable'` 会把这类历史数据误判成过程文件，故用正向判定。
    expect(isDeliverable(art({ kind: undefined, output_name: '报告' }))).toBe(true)
    expect(isDeliverable(art({ kind: undefined }))).toBe(false)
  })

  it('splitArtifacts 把两类分开且不丢项', () => {
    const list = [
      art({ transfer_id: 'a', output_name: 'x' }),
      art({ transfer_id: 'b', kind: 'process' }),
      art({ transfer_id: 'c', kind: undefined }),
    ]
    const { deliverables, processes } = splitArtifacts(list)
    expect(deliverables.map((x) => x.transfer_id)).toEqual(['a'])
    expect(processes.map((x) => x.transfer_id)).toEqual(['b', 'c'])
    expect(deliverables.length + processes.length).toBe(list.length)
  })
})
