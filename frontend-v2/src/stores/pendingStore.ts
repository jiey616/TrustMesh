import { create } from 'zustand'
import type { UIBlockResponse } from '@/types'

/**
 * 待确认抽屉的草稿缓存。
 *
 * 动机：四类确认原本把填写中的内容存在各自组件的 useState 里，抽屉一关组件就卸载，
 * 草稿随之蒸发。用户收起抽屉去别处看一眼再回来，输入全没了。这里按条目 id 把草稿
 * 提到全局，抽屉卸载不影响用户已经填了一半的内容。
 *
 * 刻意不持久化到 localStorage：草稿是临时输入，服务端内容才是事实来源，
 * 刷新页面重填一次比留下一堆过期半成品更干净。
 */
export interface PendingDraft {
  /** 自由文本：执行提问的回答 / 方案确认的修改意见 / 成果审核的退回原因 */
  text?: string
  /** 规划澄清的逐步回答，key 为 ui_block id */
  responses?: Record<string, UIBlockResponse>
  /** 规划澄清当前停在第几步 */
  step?: number
  /** 成果审核：是否已展开退回原因输入 */
  rejecting?: boolean
}

interface PendingState {
  open: boolean
  drafts: Record<string, PendingDraft>
  /**
   * 正在提交的条目 id。
   *
   * 🔴 动机：同一条待确认事项**可能同时被两处渲染** —— composer 条的内联确认控件
   * （`TaskPendingConfirmEntry`）和待确认抽屉。两者是各自独立的组件实例，各自的
   * `mutation.isPending` 互不可见，用户在内联处提交后若立刻打开抽屉再点一次，就会
   * 发出两次请求（同一问答/同一审核）。这里用一个共享的在途标记把两边串起来。
   *
   * 用 `beginSubmit` 做原子的「测试并置位」，返回 false 表示别处已在提交。
   */
  submitting: Record<string, true>
  setOpen: (open: boolean) => void
  setDraft: (id: string, patch: Partial<PendingDraft>) => void
  clearDraft: (id: string) => void
  getDraft: (id: string) => PendingDraft
  beginSubmit: (id: string) => boolean
  endSubmit: (id: string) => void
}

export const usePendingStore = create<PendingState>()((set, get) => ({
  open: false,
  drafts: {},
  submitting: {},
  setOpen: (open) => set({ open }),
  setDraft: (id, patch) =>
    set((s) => ({ drafts: { ...s.drafts, [id]: { ...s.drafts[id], ...patch } } })),
  clearDraft: (id) =>
    set((s) => {
      if (!s.drafts[id]) return s
      const next = { ...s.drafts }
      delete next[id]
      return { drafts: next }
    }),
  getDraft: (id) => get().drafts[id] ?? {},
  beginSubmit: (id) => {
    if (get().submitting[id]) return false
    set((s) => ({ submitting: { ...s.submitting, [id]: true } }))
    return true
  },
  endSubmit: (id) =>
    set((s) => {
      if (!s.submitting[id]) return s
      const next = { ...s.submitting }
      delete next[id]
      return { submitting: next }
    }),
}))
