/// <reference types="node" />
// 契约测试要读源码做断言，因此显式引入 node 类型（原因同 manual-block-contract.test.ts）。
import fs from 'node:fs'
import path from 'node:path'
import { beforeEach, describe, expect, it } from 'vitest'
import { isInlineConfirmable, pendingItemSummary, type PendingItem } from '@/lib/pendingItems'
import { usePendingStore } from '@/stores/pendingStore'

// 「待确认」在 composer 位的入口形态 · 跨层契约守卫。
//
// 🔴 为什么需要这个测试：
//  1. 入口形态在 2026-09-22 变过一次（整张卡片内联 → 一行条 + 抽屉），最容易发生的退化
//     就是「有人又觉得把卡片塞回输入条更省一次点击」。输入区是固定高度区，塞进来的卡片
//     一旦变高，长内容就会被卡片内部的 maxHeight 裁掉、还在输入位里长出内部滚动条 ——
//     这个用户可见的故障只能靠测试钉死。
//  2. 卡片本体必须**只有抽屉那一份**：composer 条自己不渲染表单、不调 mutation，
//     否则又会退回「两处实现悄悄漂移」（两侧编译器都不报错那类事故）。
//  3. 内联的适用范围是个安全边界：`plan_review` 要通读整份方案才能决策，把它塞进固定
//     高度的输入条会诱导「没看就点通过」。
//  4. 条上要报**抽屉里的总项数**（含不可内联项），否则「条上说 1 项、抽屉里 3 张卡」
//     会对不上账。
//  5. 提交必须防重复：局部 `mutation.isPending` 拦不住双击与卡片重挂载。

const ROOT = path.resolve(__dirname, '../../..')
const FE = path.join(ROOT, 'frontend-v2/src')

// 🔴 必须归一化换行符：本仓库混行尾（`types/index.ts`、`skills/*/SKILL.md` 是 CRLF），
//    凡「匹配到空行/到文件尾」的跨块正则在 CRLF 文件上会恒不命中。
function read(p: string) {
  return fs.readFileSync(p, 'utf8').replace(/\r\n/g, '\n')
}

// 注释里提到组件名/函数名是正常且必要的（写「为什么这么改」就必然会提到），
// 所以「不许出现 X」这类断言必须先剥掉注释，否则改一次注释就误报。
function stripComments(src: string): string {
  return src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '')
}

/** 造一个最小可用条目，只关心 kind。 */
function pending(kind: PendingItem['kind']): PendingItem {
  return { kind, id: `id-${kind}`, taskId: 't1', createdAt: '' } as unknown as PendingItem
}

/** 同上，但带上摘要要用到的字段。 */
function item(kind: PendingItem['kind'], extra: Record<string, unknown> = {}): PendingItem {
  return { ...pending(kind), ...extra } as PendingItem
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

describe('待确认内联 · 入口条摘要（真实调用函数）', () => {
  it('成果审核取步骤名、执行提问取问题原文', () => {
    expect(pendingItemSummary(item('todo_review', { todoTitle: '剪辑成片' }))).toBe('剪辑成片')
    expect(pendingItemSummary(item('todo_ask', { question: '要 4K 还是 1080p？' }))).toBe('要 4K 还是 1080p？')
  })

  it('🔴 四类都必须给出非空摘要（否则条上会渲染出「「undefined」待人工确认」）', () => {
    for (const kind of ['plan_review', 'plan_clarify', 'todo_ask', 'todo_review'] as const) {
      const s = pendingItemSummary(item(kind, { todoTitle: '步骤', question: '问题' }))
      expect(s, `${kind} 的摘要为空`).toBeTruthy()
      expect(s, `${kind} 的摘要渲染出了 undefined`).not.toMatch(/undefined/)
    }
  })
})

describe('待确认内联 · 条只给入口，卡片本体唯一', () => {
  const ENTRY = path.join(FE, 'components/task/TaskPendingConfirmEntry.tsx')
  const DRAWER = path.join(FE, 'components/task/PendingApprovalsDrawer.tsx')

  it('🔴 composer 条不自己渲染表单：既不导入卡片、也不调 mutation', () => {
    const code = stripComments(read(ENTRY))
    expect(code).not.toMatch(/TodoAskCard|TodoReviewCard/)
    expect(code).not.toMatch(/useAnswerTodo|useReviewTodo/)
    expect(code).not.toMatch(/useAppendTaskMessage|useApprovePlan|useRejectPlan/)
  })

  it('点「去确认」打开待确认抽屉（唯一那份卡片的宿主）', () => {
    const code = stripComments(read(ENTRY))
    expect(code).toMatch(/usePendingStore\(\(s\) => s\.setOpen\)/)
    expect(code).toMatch(/setPendingOpen\(true\)/)
  })

  it('条上只报抽屉里的总项数（含不可内联项）', () => {
    const code = stripComments(read(ENTRY))
    expect(code).toMatch(/items\.length \+ otherCount/)
    expect(code).toMatch(/有 \$\{total\} 项待人工确认/)
  })

  it('抽屉侧导出并在抽屉里渲染这两张卡片', () => {
    const src = read(DRAWER)
    expect(src).toMatch(/export function TodoAskCard\(/)
    expect(src).toMatch(/export function TodoReviewCard\(/)
    expect(src).toMatch(/<TodoAskCard item=\{item\} \/>/)
    expect(src).toMatch(/<TodoReviewCard item=\{item\} \/>/)
  })

  it('两张卡片都接了跨实例在途标记（busy = 自身 isPending || store.submitting）', () => {
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

describe('待确认内联 · 重复提交防护（真实调用 store）', () => {
  beforeEach(() => {
    usePendingStore.setState({ submitting: {} })
  })

  it('beginSubmit 是原子的「测试并置位」：第二次必须失败', () => {
    const s = usePendingStore.getState()
    expect(s.beginSubmit('x')).toBe(true)
    // 双击 / 卡片重挂载后的第二次提交从这里被挡下。
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

describe('待确认内联 · composer 分支装配', () => {
  const WS = path.join(FE, 'components/task/TaskWorkspace.tsx')

  it('入口条在「至少一项可内联」时出现（不再限定恰好一项）', () => {
    const src = read(WS)
    const m = src.match(/const inlinePendingItems = useMemo\([\s\S]*?\)\n/)
    if (!m) throw new Error('inlinePendingItems not found')
    expect(m[0]).toMatch(/pendingItems\.filter\(isInlineConfirmable\)/)
    // 🔴 旧实现写死 pendingItems.length === 1 ⇒ ≥2 项时入口整个消失，只剩右上角 Badge。
    //    注释里会提到这个旧条件，所以断言要剥注释。
    expect(stripComments(src)).not.toMatch(/pendingItems\.length === 1/)
    expect(src).toMatch(/\) : inlinePendingItems\.length > 0 \? \(/)
  })

  it('把不可内联项的计数一并传给入口条（条上总数要对得上抽屉）', () => {
    const src = read(WS)
    expect(src).toMatch(/const otherPendingCount = pendingItems\.length - inlinePendingItems\.length/)
    expect(src).toMatch(
      /<TaskPendingConfirmEntry items=\{inlinePendingItems\} otherCount=\{otherPendingCount\} \/>/,
    )
  })

  it('与抽屉共用同一份 collectPendingItems（避免两处漂移）', () => {
    const src = read(WS)
    expect(src).toMatch(/const pendingItems = useMemo\(\(\) => collectPendingItems\(task, events\)/)
    expect(src).toMatch(/items=\{pendingItems\}/)
  })

  it('🔴 内联分支排在 TaskCommentComposer 之前（否则评论框照旧渲染）', () => {
    const src = read(WS)
    const iInline = src.indexOf(') : inlinePendingItems.length > 0 ? (')
    const iComposer = src.lastIndexOf('<TaskCommentComposer')
    expect(iInline).toBeGreaterThan(-1)
    expect(iComposer).toBeGreaterThan(-1)
    expect(iInline).toBeLessThan(iComposer)
  })

  it('内联分支排在终态分支之后（终态优先，失败任务仍走「重试/继续」）', () => {
    const src = read(WS)
    const iTerminal = src.indexOf(') : isTerminal ? (')
    const iInline = src.indexOf(') : inlinePendingItems.length > 0 ? (')
    expect(iTerminal).toBeGreaterThan(-1)
    expect(iInline).toBeGreaterThan(iTerminal)
  })

  it('评论框仍在（≠ 把所有情况都内联掉）', () => {
    const src = read(WS)
    expect(src).toMatch(/<TaskCommentComposer/)
  })
})
