import { create } from 'zustand'
import { persist, createJSONStorage } from 'zustand/middleware'

interface WorkspaceState {
  personalOrgId: string | null
  /** null = 个人空间 */
  activeOrgId: string | null
  /** 门闩：为 false 时不允许渲染业务内容，避免用上一个空间的数据闪一下 */
  calibrated: boolean

  setPersonalOrgId: (id: string | null) => void
  setActiveOrgId: (id: string | null) => void
  setCalibrated: (v: boolean) => void
  reset: () => void
}

export const useWorkspaceStore = create<WorkspaceState>()(
  persist(
    (set) => ({
      personalOrgId: null,
      activeOrgId: null,
      calibrated: false,

      setPersonalOrgId: (personalOrgId) => set({ personalOrgId }),
      setActiveOrgId: (activeOrgId) => set({ activeOrgId }),
      setCalibrated: (calibrated) => set({ calibrated }),
      reset: () => set({ personalOrgId: null, activeOrgId: null, calibrated: false }),
    }),
    {
      name: 'tm-mobile-workspace',
      storage: createJSONStorage(() => localStorage),
      partialize: (s) => ({
        personalOrgId: s.personalOrgId,
        activeOrgId: s.activeOrgId,
      }),
    },
  ),
)

// ============================================================================
// 工作区校准顺序（🔴 顺序写反会「静默复发」，静态检查与常规测试都不报）
// ============================================================================
//
// 必须是：setPersonalOrgId → setActiveOrgId → 清空查询缓存 → setCalibrated(true)
//   · 先 personal 后 active：active 的合法性依赖 personal 已就绪；
//   · 清空缓存必须在切 org **之后**、放开渲染**之前**，否则旧空间的响应会被新空间复用；
//   · calibrated 最后置位，作为"数据已对齐"的唯一可信信号。

export interface CalibrationIO {
  setPersonalOrgId: (id: string | null) => void
  setActiveOrgId: (id: string | null) => void
  clearQueries: () => void
  setCalibrated: (v: boolean) => void
}

export interface CalibrationPlan {
  personalOrgId: string | null
  activeOrgId: string | null
}

export function runWorkspaceCalibration(io: CalibrationIO, plan: CalibrationPlan): void {
  io.setCalibrated(false)
  io.setPersonalOrgId(plan.personalOrgId)
  io.setActiveOrgId(plan.activeOrgId)
  io.clearQueries()
  io.setCalibrated(true)
}
