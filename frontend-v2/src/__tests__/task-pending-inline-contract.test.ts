/// <reference types="node" />
// 契约测试要读源码做断言，因此显式引入 node 类型（原因同 manual-block-contract.test.ts）。
import fs from 'node:fs'
import path from 'node:path'
import { beforeEach, describe, expect, it } from 'vitest'
import { isInlineConfirmable, type PendingItem } from '@/lib/pendingItems'
import { usePendingStore } from '@/stores/pendingStore'

// 「待确认」内联到 composer 条的跨层契约守卫。
//
// 🔴 为什么需要这个测试：
//  1. 同一条待确认事项会**同时**被两处渲染（composer 条内联 + 待确认抽屉）。
//     两处各写一份表单/各调一次 mutation，就会重演「两份实现悄悄漂移」那类事故 ——
//     而两侧编译器都不报错。所以「必须复用同一份卡片」要由测试钉死。
//  2. 内联的适用范围是个安全边界：`plan_review` 要通读整份方案才能决策，
//     把它塞进固定高度的输入条会诱导「没看就点通过」。这条边界只能靠测试守。
//  3. 两处都能提交 ⇒ 必须防重复提交（同一问答/审核发两次请求）。

const ROOT = path.resolve(__dirname, '../../..')
const FE = path.join(ROOT, 'frontend-v2/src')

// 🔴 必须归一化换行符：本仓库混行尾（`types/index.ts`、`skills/*/SKILL.md` 是 CRLF），
//    凡「匹配到空行/到文件尾」的跨块正则在 CRLF 文件上会恒不命中。
function read(p: string) {
  return fs.readFileSync(p, 'utf8').replace(/\r\n/g, '\n')
}

/** 造一个最小可用条目，只关心 kind。 */
function pending(kind: PendingItem['kind']): PendingItem {
  return { kind, id: `id-${kind}`, taskId: 't1', createdAt: '' } as unknown as PendingItem
}

describe('待确认内联 · 适用范围（真实调用谓词）', () => {
  it('todo_ask / todo_review 可以内联', () => {
    expect(isInlineConfirmable(pending('todo_ask'))).toBe(true)
    expect(isInlineConfirmable(pending('todo_review'))).toBe(true)
  })

  it('🔴 plan_review / plan_clarify 不可内联（要塞进输入条会诱导「没看就点通过」）', () => {
    expect(isInlineConfirmable(pending('plan_review'))).toBe(false)
    expect(isInlineConfirmable(pending('plan_clarify'))).toBe(false)
  })

  it('空值安全', () => {
    expect(isInlineConfirmable(null)).toBe(false)
    expect(isInlineConfirmable(undefined)).toBe(false)
  })
})

describe('待确认内联 · 重复提交防护（真实调用 store）', () => {
  beforeEach(() => {
    usePendingStore.setState({ submitting: {} })
  })

  it('beginSubmit 是原子的「测试并置位」：第二次必须失败', () => {
    const s = usePendingStore.getState()
    expect(s.beginSubmit('x')).toBe(true)
    // 内联控件与抽屉同时渲染同一 id 时，第二处从这里被挡下。
    expect(usePendingStore.getState().beginSubmit('x')).toBe(false)
    // 不同 id 互不影响
    expect(usePendingStore.getState().beginSubmit('y')).toBe(true)
  })

  it('endSubmit 释放后可以再次提交', () => {
    const s = usePendingStore.getState()
    s.beginSubmit('x')
    s.endSubmit('x')
    expect(usePendingStore.getState().beginSubmit('x')).toBe(true)
  })

  it('endSubmit 对未置位的 id 是幂等的', () => {
    expect(() => usePendingStore.getState().endSubmit('nope')).not.toThrow()
    expect(usePendingStore.getState().submitting.nope).toBeUndefined()
  })
})

describe('待确认内联 · 复用同一份卡片（不许各写一套）', () => {
  const ENTRY = path.join(FE, 'components/task/TaskPendingConfirmEntry.tsx')
  const DRAWER = path.join(FE, 'components/task/PendingApprovalsDrawer.tsx')

  it('内联控件从抽屉模块导入两张卡片，而不是自己实现', () => {
    const src = read(ENTRY)
    expect(src).toMatch(/import\s*\{\s*TodoAskCard,\s*TodoReviewCard\s*\}\s*from\s*'@\/components\/task\/PendingApprovalsDrawer'/)
    expect(src).toMatch(/<TodoAskCard\s+item=\{item\}/)
    expect(src).toMatch(/<TodoReviewCard\s+item=\{item\}/)
  })

  it('🔴 内联控件不得自己调用 mutation（否则就是第二份实现）', () => {
    const src = read(ENTRY)
    expect(src).not.toMatch(/useAnswerTodo|useReviewTodo/)
    expect(src).not.toMatch(/useAppendTaskMessage|useApprovePlan|useRejectPlan/)
  })

  it('抽屉侧确实导出了这两张卡片（导出被去掉则内联直接编译不过）', () => {
    const src = read(DRAWER)
    expect(src).toMatch(/export function TodoAskCard\(/)
    expect(src).toMatch(/export function TodoReviewCard\(/)
  })

  it('两张卡片都接了共享在途标记（busy = 自身 isPending || store.submitting）', () => {
    const src = read(DRAWER)
    const ask = src.match(/export function TodoAskCard\([\s\S]*?\n\}/)
    const review = src.match(/export function TodoReviewCard\([\s\S]*?\n\}/)
    if (!ask || !review) throw new Error('cards not found')
    for (const [name, body] of [['TodoAskCard', ask[0]], ['TodoReviewCard', review[0]]] as const) {
      expect(body, `${name} 缺 beginSubmit`).toMatch(/beginSubmit\(item\.id\)/)
      expect(body, `${name} 缺 endSubmit 释放`).toMatch(/endSubmit\(item\.id\)/)
      expect(body, `${name} 缺 busy 合成`).toMatch(/const busy = [^\n]*isSubmitting/)
    }
  })
})

describe('待确认内联 · composer 分支装配', () => {
  const WS = path.join(FE, 'components/task/TaskWorkspace.tsx')

  it('inlinePending 只在「恰好一项 且 可内联」时成立', () => {
    const src = read(WS)
    const m = src.match(/const inlinePending = useMemo\([\s\S]*?\)\n/)
    if (!m) throw new Error('inlinePending not found')
    expect(m[0]).toMatch(/pendingItems\.length === 1/)
    expect(m[0]).toMatch(/isInlineConfirmable\(pendingItems\[0\]\)/)
  })

  it('与抽屉共用同一份 collectPendingItems（避免两处漂移）', () => {
    const src = read(WS)
    expect(src).toMatch(/const pendingItems = useMemo\(\(\) => collectPendingItems\(task, events\)/)
    expect(src).toMatch(/items=\{pendingItems\}/)
  })

  it('🔴 内联分支排在 TaskCommentComposer 之前（否则评论框照旧渲染）', () => {
    const src = read(WS)
    const iInline = src.indexOf(') : inlinePending ? (')
    const iComposer = src.lastIndexOf('<TaskCommentComposer')
    expect(iInline).toBeGreaterThan(-1)
    expect(iComposer).toBeGreaterThan(-1)
    expect(iInline).toBeLessThan(iComposer)
  })

  it('内联分支排在终态分支之后（终态优先，失败任务仍走「重试/继续」）', () => {
    const src = read(WS)
    const iTerminal = src.indexOf(') : isTerminal ? (')
    const iInline = src.indexOf(') : inlinePending ? (')
    expect(iTerminal).toBeGreaterThan(-1)
    expect(iInline).toBeGreaterThan(iTerminal)
  })

  it('评论框仍在（≠ 把所有情况都内联掉）', () => {
    const src = read(WS)
    expect(src).toMatch(/<TaskCommentComposer/)
  })
})
