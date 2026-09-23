import { Button, Typography } from 'antd'
import { ExclamationCircleOutlined } from '@ant-design/icons'
import { pendingItemSummary, type PendingItem } from '@/lib/pendingItems'
import { usePendingStore } from '@/stores/pendingStore'

const { Text } = Typography

/**
 * 「待人工确认」在 composer 位的入口条 —— 只给入口，不给内容。
 *
 * 为什么存在（与「失败任务重试/继续」同一条不变式）：
 * `awaiting_review` / `waiting_user` 都**不是**终态，所以默认分支会渲染普通评论框。
 * 但用户在评论框里打字走的是 `task.mention` → chat 通道，**既不会回答数字员工的问题、
 * 也不会批准审核** —— 和「终态留一个假装能用的输入框」是同一类误导。这里把真正能推进
 * 任务的那个入口放回用户手边（评论框位置）。
 *
 * 🔴 为什么是「一条 + 抽屉」而不是把卡片直接塞进来（2026-09-22 改）：
 * 输入区是固定高度区（外层 `flexShrink: 0`），卡片高度却随成果摘要长度变化。早先的实现
 * 把整张 `TodoReviewCard` 内联进来，于是长摘要被卡片内的 `maxHeight:160 + overflowY:auto`
 * 裁掉、在输入位里长出一条内部滚动条，卡片一高还把消息流往上顶 —— 用户看到的就是
 * 「内容显示不全，很怪」。现在条只有一行固定高度，完整内容回到抽屉里展示。
 *
 * 只覆盖「一眼就能决策」的两类：
 *   - `todo_ask`    一个问题 + 选项，本质就是「回复」；
 *   - `todo_review` 通过 / 退回，决策依据是卡片里已经给出的结果摘要。
 * `plan_review`（要通读整份方案才能点头）与 `plan_clarify`（多字段表单）**刻意不内联**
 * —— 塞进一条固定高度的输入条会诱导用户没看内容就点通过，比多一次点击危险得多。
 * 它们的数量由 `otherCount` 带进来，好在条上如实报出总数：否则用户在「1 项待确认」的
 * 提示下打开抽屉却看到 3 张卡，会怀疑自己看漏了什么。
 *
 * 🔴 卡片本体只在 `PendingApprovalsDrawer` 里渲染**唯一那一份**：本组件不渲染任何表单、
 * 也不调用任何 mutation，所以「两处实现悄悄漂移」在这条链路上已被结构性消除（不靠纪律）。
 *
 * 「哪些类型可内联」的判定放在 `lib/pendingItems.ts` 的 `isInlineConfirmable`、
 * 摘要文案放在同文件的 `pendingItemSummary`：本文件只导出组件，把非组件导出放这里会
 * 触发 `react-refresh/only-export-components`。
 */

interface Props {
  /** 可内联的待确认项（至少 1 项，否则 `TaskWorkspace` 根本不渲染本组件）。 */
  items: PendingItem[]
  /** 抽屉里其余**不可内联**的待确认项数量（`plan_review` / `plan_clarify`）。 */
  otherCount: number
}

export function TaskPendingConfirmEntry({ items, otherCount }: Props) {
  const setPendingOpen = usePendingStore((s) => s.setOpen)

  // 条上报的是**抽屉里的总项数**：按钮打开的就是抽屉，报两个数只会让人对不上账。
  const total = items.length + otherCount
  const first = items[0]
  const summary = total === 1 && first ? pendingItemSummary(first) : null

  return (
    <div
      style={{
        maxWidth: 560,
        margin: '0 auto',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: 12,
        background: 'var(--surface-sunken)',
        border: '1px solid var(--line)',
        borderRadius: 'var(--radius-control)',
        padding: '10px 12px',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 0 }}>
        <ExclamationCircleOutlined style={{ color: 'var(--warning)', fontSize: 14, flexShrink: 0 }} />
        {/* 摘要长度在运行时不可预知（步骤名可长可短），交给 CSS 省略号截断而不是硬截字符串 —— 硬截会切出半个字。 */}
        <Text
          style={{
            fontSize: 13,
            color: 'var(--text-secondary)',
            whiteSpace: 'nowrap',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            minWidth: 0,
          }}
        >
          {summary ? `「${summary}」待人工确认` : `有 ${total} 项待人工确认`}
        </Text>
      </div>
      <Button type="primary" onClick={() => setPendingOpen(true)} style={{ flexShrink: 0 }}>
        去确认
      </Button>
    </div>
  )
}
