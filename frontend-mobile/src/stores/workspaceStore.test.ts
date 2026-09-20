import { describe, expect, it } from 'vitest'
import { runWorkspaceCalibration, type CalibrationIO } from './workspaceStore'

function recorder() {
  const calls: string[] = []
  const io: CalibrationIO = {
    setPersonalOrgId: () => calls.push('setPersonalOrgId'),
    setActiveOrgId: () => calls.push('setActiveOrgId'),
    clearQueries: () => calls.push('clearQueries'),
    setCalibrated: (v) => calls.push(`setCalibrated:${v}`),
  }
  return { calls, io }
}

describe('runWorkspaceCalibration', () => {
  it('严格按 personal → active → 清缓存 → 放开渲染 的顺序执行', () => {
    const { calls, io } = recorder()
    runWorkspaceCalibration(io, { personalOrgId: 'org_p', activeOrgId: 'org_a' })
    expect(calls).toEqual([
      'setCalibrated:false',
      'setPersonalOrgId',
      'setActiveOrgId',
      'clearQueries',
      'setCalibrated:true',
    ])
  })

  it('切换时先关闸，杜绝旧空间数据闪现', () => {
    const { calls, io } = recorder()
    runWorkspaceCalibration(io, { personalOrgId: 'org_p', activeOrgId: null })
    expect(calls[0]).toBe('setCalibrated:false')
    expect(calls[calls.length - 1]).toBe('setCalibrated:true')
  })
})
