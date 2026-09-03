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
  setOpen: (open: boolean) => void
  setDraft: (id: string, patch: Partial<PendingDraft>) => void
  clearDraft: (id: string) => void
  getDraft: (id: string) => PendingDraft
}

export const usePendingStore = create<PendingState>()((set, get) => ({
  open: false,
  drafts: {},
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
}))
