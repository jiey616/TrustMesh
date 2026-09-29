import type { TodoInput } from '@/types'

/**
 * 「输入位就绪度」—— 把后端 `model.TodoInput` 的快照翻译成用户能**据以行动**的一句话。
 *
 * 🔴 为什么要三态，而不是「已解析 / 未解析」两态：
 *   `pending`（上游还活着，再等等就行）与 `missing`（上游已终态、或压根没有任何任务
 *   持有该步骤 —— 等到天荒地老也不会出现）要求用户做**相反**的事。折成一态之后，
 *   界面只能要么一律催人、要么一律让人等。2026-09-29 的事故里两种错都真实发生过：
 *   前序任务被取消、它仍持有该步骤且已产出文件，下游却报「未解析」；用户以为再等等
 *   就好，数字员工则白花一次 `todo.ask` 去问一个**永远不会自动产出**的文件。
 *
 * 🔴 文案与色调**只在本文件定义**，组件不许再写第二份映射：
 *   两份映射必然漂移，而漂移时两侧的检查都不会报错（这类事故已经发生过多次）。
 */

/** 与后端 `model.InputState*` 一一对应。🔴 不允许出现第四种业务取值。 */
export type TodoInputState = 'resolved' | 'pending' | 'missing'

/**
 * 把后端可能给出的任意字符串收敛到已知状态。
 *
 * 认不出来的取值（老数据、后端将来新增的第四态）**一律不算就绪** —— 宁可提示
 * 「状态未知」，也不能因为认不出来就当成就绪：那会让用户误以为数字员工已经
 * 拿到文件了，而这正是本模块要消灭的误解。
 */
export function normalizeTodoInputState(state: string | null | undefined): TodoInputState | 'unknown' {
  switch (state) {
    case 'resolved':
    case 'pending':
    case 'missing':
      return state
    default:
      return 'unknown'
  }
}

/** 该输入位是否已就绪（拿到了文件）。 */
export function isTodoInputResolved(input: TodoInput | null | undefined): boolean {
  return normalizeTodoInputState(input?.state) === 'resolved'
}

export interface TodoInputReadiness {
  total: number
  ready: number
  /** 上游仍活跃，等就行 */
  pending: number
  /** 上游已终态 / 无 holder ⇒ 必须人工介入 */
  missing: number
  /** 状态认不出来（老数据） */
  unknown: number
  allReady: boolean
  /** 只要有一项不是 resolved 就为 true */
  hasUnresolved: boolean
  /** 存在「等到天荒地老也不会好」的项 ⇒ 界面要用错误色并说清「要人动手」 */
  needsHuman: boolean
}

export function todoInputReadiness(inputs: TodoInput[] | null | undefined): TodoInputReadiness {
  const list = inputs ?? []
  let ready = 0
  let pending = 0
  let missing = 0
  let unknown = 0
  for (const input of list) {
    switch (normalizeTodoInputState(input?.state)) {
      case 'resolved':
        ready += 1
        break
      case 'pending':
        pending += 1
        break
      case 'missing':
        missing += 1
        break
      default:
        unknown += 1
    }
  }
  const total = list.length
  return {
    total,
    ready,
    pending,
    missing,
    unknown,
    allReady: total > 0 && ready === total,
    hasUnresolved: ready < total,
    // unknown 也归入「要人看一眼」：它同样不会自己变好。
    needsHuman: missing > 0 || unknown > 0,
  }
}

/** 卡片上的就绪度摘要，如 `输入位 3/4 已就绪`。无输入位时返回空串（调用方不渲染）。 */
export function todoInputSummaryText(readiness: TodoInputReadiness): string {
  if (readiness.total === 0) return ''
  return `输入位 ${readiness.ready}/${readiness.total} 已就绪`
}

/**
 * 不展开卡片也能看出「该等还是该动手」的一句话。
 *
 * 🔴 这是三态在**折叠态**下的唯一出口：只报计数的话，「1 项缺失」与「1 项还没产出」
 *    长得一模一样，用户仍得展开才知道要不要叫人 —— 那三态就白设计了。
 */
export function todoInputActionHint(readiness: TodoInputReadiness): string {
  if (readiness.total === 0 || readiness.allReady) return ''
  if (readiness.needsHuman) return '需人工介入'
  return '等待上游产出'
}

/** 单项状态的中文标签。三种未就绪状态必须**文案互不相同**（否则无法据以行动）。 */
export function todoInputStateLabel(state: string | null | undefined): string {
  switch (normalizeTodoInputState(state)) {
    case 'resolved':
      return '已就绪'
    case 'pending':
      return '等待上游产出'
    case 'missing':
      return '上游缺失'
    default:
      return '状态未知'
  }
}

export type TodoInputTone = 'success' | 'warning' | 'error' | 'neutral'

/** 单项状态的色调。`pending`=黄（等）、`missing`=红（动手）—— 与标签同源，不可各写一份。 */
export function todoInputStateTone(state: string | null | undefined): TodoInputTone {
  switch (normalizeTodoInputState(state)) {
    case 'resolved':
      return 'success'
    case 'pending':
      return 'warning'
    case 'missing':
      return 'error'
    default:
      return 'neutral'
  }
}

/** 摘要徽标整体的色调：只要有一项要人工介入就升级为错误色。 */
export function todoInputSummaryTone(readiness: TodoInputReadiness): TodoInputTone {
  if (readiness.total === 0) return 'neutral'
  if (readiness.allReady) return 'success'
  return readiness.needsHuman ? 'error' : 'warning'
}

function formatFileSize(bytes: number | null | undefined): string {
  if (!bytes || bytes <= 0) return ''
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

/**
 * 展开后的单行副文案：`来源步骤 · 输出位 · 文件名 1.2 KB`。
 * 只拼已知字段 —— 缺失的字段整段省略，避免渲染出 `undefined`。
 */
export function describeTodoInput(input: TodoInput | null | undefined): string {
  if (!input) return ''
  const parts: string[] = []
  if (input.source_step) parts.push(`来自「${input.source_step}」`)
  if (input.output_name) parts.push(`输出位 ${input.output_name}`)
  if (input.file_name) {
    const size = formatFileSize(input.file_size)
    parts.push(size ? `${input.file_name}（${size}）` : input.file_name)
  }
  return parts.join(' · ')
}

/** 未就绪项（要展示的原因行）—— 已就绪项在展开区里也列，但排在后面。 */
export function sortTodoInputsForDisplay(inputs: TodoInput[] | null | undefined): TodoInput[] {
  const list = inputs ?? []
  // 未就绪优先、且 missing（要人动手）排最前：用户展开就是为了找「卡在哪」。
  const rank = (input: TodoInput) =>
    normalizeTodoInputState(input?.state) === 'missing'
      ? 0
      : normalizeTodoInputState(input?.state) === 'unknown'
        ? 1
        : normalizeTodoInputState(input?.state) === 'pending'
          ? 2
          : 3
  return list
    .map((input, index) => ({ input, index }))
    .sort((a, b) => rank(a.input) - rank(b.input) || a.index - b.index)
    .map((entry) => entry.input)
}
