import type { Event, TaskDetail, Todo, UIBlock, Workflow } from '@/types'

/**
 * 任务里所有「等人类拍板」的事项，统一成一种形状，供待确认抽屉消费。
 *
 * 四类来源原本散落在四处（Feed 内联 / 事件流内联 / 底部 Composer / 任务清单行内），
 * 这里只做汇总与归一化，不含任何 UI 与副作用。
 *
 * id 必须稳定——抽屉收起再打开要靠它把草稿对回原条目，因此每类都锚定到
 * 服务端已有的稳定标识（taskId / messageId / eventId / todoId），不用数组下标。
 */

export type PendingKind = 'plan_review' | 'plan_clarify' | 'todo_ask' | 'todo_review'

interface PendingBase {
  id: string
  taskId: string
  /** 排序用，空串表示不参与时间排序 */
  createdAt: string
}

export interface PlanReviewPending extends PendingBase {
  kind: 'plan_review'
  todos: Todo[]
  workflow?: Workflow
}

export interface PlanClarifyPending extends PendingBase {
  kind: 'plan_clarify'
  messageId: string
  blocks: UIBlock[]
}

export interface TodoAskPending extends PendingBase {
  kind: 'todo_ask'
  todoId: string
  questionId: string
  question: string
  options: string[]
}

export interface TodoReviewPending extends PendingBase {
  kind: 'todo_review'
  todoId: string
  todoTitle: string
  todo: Todo
}

export type PendingItem =
  | PlanReviewPending
  | PlanClarifyPending
  | TodoAskPending
  | TodoReviewPending

export interface PendingUIBlocks {
  messageId: string
  blocks: UIBlock[]
}

/** 抽屉里的分组标题，按处理优先级排：先把卡住流程的放前面 */
export const pendingKindLabel: Record<PendingKind, string> = {
  plan_clarify: '规划澄清',
  plan_review: '方案确认',
  todo_ask: '执行提问',
  todo_review: '成果审核',
}

/**
 * 从任务消息里找「最近一条未被用户回复的」pm_agent ui_blocks 消息。
 * 一旦往回碰到用户消息就说明上一轮已应答，直接返回 null。
 */
export function findPendingUIBlocks(task: TaskDetail | null | undefined): PendingUIBlocks | null {
  if (!task || task.status !== 'planning' || !task.messages?.length) return null
  for (let i = task.messages.length - 1; i >= 0; i--) {
    const msg = task.messages[i]
    if (!msg) continue
    if (msg.role === 'user') return null
    if (msg.role === 'pm_agent' && msg.ui_blocks && msg.ui_blocks.length > 0) {
      return { messageId: msg.id, blocks: msg.ui_blocks }
    }
  }
  return null
}

export function collectPendingItems(
  task: TaskDetail | null | undefined,
  events: Event[] | null | undefined,
): PendingItem[] {
  if (!task) return []
  const items: PendingItem[] = []
  const taskId = task.id

  // 1. 方案确认：PM 规划完毕等人点头
  if (task.status === 'review') {
    items.push({
      kind: 'plan_review',
      id: `plan_review:${taskId}`,
      taskId,
      createdAt: task.updated_at ?? '',
      todos: task.todos ?? [],
      workflow: task.workflow,
    })
  }

  // 2. 规划澄清：PM 抛了 ui_blocks 还没等到回答
  const pendingUI = findPendingUIBlocks(task)
  if (pendingUI) {
    items.push({
      kind: 'plan_clarify',
      id: `plan_clarify:${pendingUI.messageId}`,
      taskId,
      createdAt: '',
      messageId: pendingUI.messageId,
      blocks: pendingUI.blocks,
    })
  }

  // 3. 执行提问：agent 中途 ask，尚未回答（已答的事件带 metadata.answer）
  for (const ev of events ?? []) {
    if (ev.event_type !== 'todo_ask_received') continue
    const questionId = ev.metadata?.question_id as string | undefined
    if (!questionId) continue
    if (ev.metadata?.answer != null) continue
    items.push({
      kind: 'todo_ask',
      id: `todo_ask:${ev.id}`,
      taskId,
      createdAt: ev.created_at ?? '',
      todoId: (ev.metadata?.todo_id as string | undefined) || ev.todo_id || '',
      questionId,
      question: (ev.metadata?.question as string | undefined) || ev.content || '数字员工请求你的确认',
      options: (ev.metadata?.options as string[] | undefined) ?? [],
    })
  }

  // 4. 成果审核：todo 完成但还没通过/退回
  for (const todo of task.todos ?? []) {
    if (todo.review_status !== 'pending_approval') continue
    items.push({
      kind: 'todo_review',
      id: `todo_review:${todo.id}`,
      taskId,
      createdAt: todo.updated_at ?? '',
      todoId: todo.id,
      todoTitle: todo.title,
      todo,
    })
  }

  // 同一类内部保持稳定顺序：plan_clarify 永远最前（它卡着整个规划流程）
  const order: PendingKind[] = ['plan_clarify', 'plan_review', 'todo_ask', 'todo_review']
  return items.sort((a, b) => {
    const d = order.indexOf(a.kind) - order.indexOf(b.kind)
    if (d !== 0) return d
    return a.createdAt.localeCompare(b.createdAt)
  })
}
