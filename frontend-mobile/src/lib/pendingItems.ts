import type { TaskDetail, TaskEvent, Todo, UIBlock } from '@/types'

/**
 * 任务里所有「等人类拍板」的事项，统一成一种形状。
 *
 * 口径**照搬桌面端 `frontend-v2/src/lib/pendingItems.ts`**（移动端不另造一套）：
 *   · plan_review  —— 任务 status === 'review'（PM 规划完等人点头）
 *   · plan_clarify —— planning 中最近一条**未被用户回复的** pm_agent ui_blocks 消息
 *   · todo_ask     —— 事件流里 `todo_ask_received` 且 metadata.answer 为空
 *   · todo_review  —— todo.review_status === 'pending_approval'
 *
 * id 必须稳定（锚定服务端标识，不用数组下标），否则草稿/忽略态会对错条目。
 */

export type PendingKind = 'plan_review' | 'plan_clarify' | 'todo_ask' | 'todo_review'

interface PendingBase {
  id: string
  taskId: string
  taskTitle: string
  createdAt: string
}

export interface PlanReviewPending extends PendingBase {
  kind: 'plan_review'
  todos: Todo[]
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

export const pendingKindLabel: Record<PendingKind, string> = {
  plan_clarify: '规划澄清',
  plan_review: '方案确认',
  todo_ask: '执行提问',
  todo_review: '成果审核',
}

/** 处理优先级：卡住整个流程的排前面 */
export const pendingKindOrder: PendingKind[] = [
  'plan_clarify',
  'plan_review',
  'todo_ask',
  'todo_review',
]

function findPendingUIBlocks(task: TaskDetail): { messageId: string; blocks: UIBlock[] } | null {
  if (task.status !== 'planning' || !task.messages?.length) return null
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
  events: TaskEvent[] | null | undefined,
): PendingItem[] {
  if (!task) return []
  const items: PendingItem[] = []
  const taskId = task.id
  const taskTitle = task.title

  if (task.status === 'review') {
    items.push({
      kind: 'plan_review',
      id: `plan_review:${taskId}`,
      taskId,
      taskTitle,
      createdAt: task.updated_at ?? '',
      todos: task.todos ?? [],
    })
  }

  const pendingUI = findPendingUIBlocks(task)
  if (pendingUI) {
    items.push({
      kind: 'plan_clarify',
      id: `plan_clarify:${pendingUI.messageId}`,
      taskId,
      taskTitle,
      createdAt: '',
      messageId: pendingUI.messageId,
      blocks: pendingUI.blocks,
    })
  }

  for (const ev of events ?? []) {
    if (ev.event_type !== 'todo_ask_received') continue
    const questionId = ev.metadata?.question_id as string | undefined
    if (!questionId) continue
    if (ev.metadata?.answer != null) continue
    items.push({
      kind: 'todo_ask',
      id: `todo_ask:${ev.id}`,
      taskId,
      taskTitle,
      createdAt: ev.created_at ?? '',
      todoId: (ev.metadata?.todo_id as string | undefined) || ev.todo_id || '',
      questionId,
      question:
        (ev.metadata?.question as string | undefined) || ev.content || '数字员工请求你的确认',
      options: (ev.metadata?.options as string[] | undefined) ?? [],
    })
  }

  for (const todo of task.todos ?? []) {
    if (todo.review_status !== 'pending_approval') continue
    items.push({
      kind: 'todo_review',
      id: `todo_review:${todo.id}`,
      taskId,
      taskTitle,
      createdAt: todo.updated_at ?? '',
      todoId: todo.id,
      todoTitle: todo.title,
      todo,
    })
  }

  return items.sort((a, b) => {
    const d = pendingKindOrder.indexOf(a.kind) - pendingKindOrder.indexOf(b.kind)
    if (d !== 0) return d
    return a.createdAt.localeCompare(b.createdAt)
  })
}
