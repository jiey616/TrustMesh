import { TodoAskCard, TodoReviewCard } from '@/components/task/PendingApprovalsDrawer'
import type { PendingItem } from '@/lib/pendingItems'

/**
 * 待确认事项的 composer 条内联控件。
 *
 * 为什么存在（与「失败任务重试/继续」同一条不变式）：
 * `awaiting_review` / `waiting_user` 都**不是**终态，所以默认分支会渲染普通评论框。
 * 但用户在评论框里打字走的是 `task.mention` → chat 通道，**既不会回答数字员工的问题、
 * 也不会批准审核** —— 和「终态留一个假装能用的输入框」是同一类误导。这里把真正能推进
 * 任务的那个控件放回用户手边（评论框位置）。
 *
 * 只覆盖「一眼就能决策」的两类：
 *   - `todo_ask`    一个问题 + 选项，本质就是「回复」；
 *   - `todo_review` 通过 / 退回，决策依据是卡片里已经给出的结果摘要。
 * `plan_review`（要通读整份方案才能点头）与 `plan_clarify`（多字段表单）**刻意不内联**
 * —— 塞进一条固定高度的输入条会诱导用户没看内容就点通过，比多一次点击危险得多。
 * 它们仍然走待确认抽屉（见 `TaskWorkspace` 的 `inlinePending` 判定）。
 *
 * 不额外加提示文案：卡片自带标题与操作按钮，语义已自明；多一行说明只会挤占输入区。
 *
 * 🔴 表单本体直接复用抽屉里的 `TodoAskCard` / `TodoReviewCard`，**不另写一份**：
 * 两处渲染同一条目是常态，各自实现必然漂移（重复提交的防护在 `pendingStore.submitting`）。
 *
 * 「哪些类型可内联」的判定放在 `lib/pendingItems.ts` 的 `isInlineConfirmable`：
 * 本文件只导出组件，把谓词放这里会触发 `react-refresh/only-export-components`。
 */

/** 纯布局容器：与其它 composer 分支保持同样的居中宽度。 */
function InlineSlot({ children }: { children: React.ReactNode }) {
  return <div style={{ maxWidth: 720, margin: '0 auto' }}>{children}</div>
}

export function TaskPendingConfirmEntry({ item }: { item: PendingItem }) {
  // 直接按 kind 收窄（不借助辅助谓词，否则 TS 无法把联合类型窄化到具体卡片所需的类型）。
  if (item.kind === 'todo_ask') {
    return (
      <InlineSlot>
        <TodoAskCard item={item} />
      </InlineSlot>
    )
  }
  if (item.kind === 'todo_review') {
    return (
      <InlineSlot>
        <TodoReviewCard item={item} />
      </InlineSlot>
    )
  }
  // plan_review / plan_clarify 不内联。
  return null
}
