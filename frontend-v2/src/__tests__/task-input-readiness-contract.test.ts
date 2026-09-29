/// <reference types="node" />
// 契约测试要读源码做断言，因此显式引入 node 类型（原因同 task-pending-inline-contract.test.ts）。
import fs from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'
import {
  describeTodoInput,
  normalizeTodoInputState,
  sortTodoInputsForDisplay,
  todoInputActionHint,
  todoInputReadiness,
  todoInputStateLabel,
  todoInputStateTone,
  todoInputSummaryText,
  todoInputSummaryTone,
} from '@/lib/todoInputs'
import type { TodoInput } from '@/types'

// 「输入位就绪度」跨层契约守卫。
//
// 🔴 为什么需要这个测试：
//  1. 这个模块存在的**唯一理由**是三态不塌成两态。一旦有人把 `pending`（等就行）
//     和 `missing`（必须人工介入）合并成同一个标签/颜色，界面就又回到「已解析/未解析」，
//     用户重新变成只能去问人 —— 而那正是 2026-09-29 事故的原始形态。
//     所以「三种未就绪状态的标签必须两两不同」是本文件的第一条断言。
//  2. 文案与色调只准有一份（在 lib 里）。组件里再写一份映射必然漂移，且漂移时
//     tsc / lint / 组件自身都不报错 —— 只能靠源码断言挡。
//  3. 认不出来的状态**不能当成就绪**：后端将来加第四态时，最危险的失败是「静默显示已就绪」，
//     让用户以为数字员工拿到文件了。
//  4. 老数据零影响：`inputs` 缺省时整块不渲染，任何调用都不许抛。

const ROOT = path.resolve(__dirname, '../../..')
const FE = path.join(ROOT, 'frontend-v2/src')

// 🔴 必须归一化换行符：本仓库混行尾（`types/index.ts` 是 CRLF），
//    凡「匹配到空行/到文件尾」的跨块正则在 CRLF 文件上会恒不命中。
function read(p: string) {
  return fs.readFileSync(p, 'utf8').replace(/\r\n/g, '\n')
}

// 注释里提到组件名/文案是正常且必要的，所以「不许出现 X」这类断言必须先剥注释。
function stripComments(src: string): string {
  return src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '')
}

function input(state: string, extra: Partial<TodoInput> = {}): TodoInput {
  return { name: '分组视频生成提示词', state, ...extra }
}

describe('输入位就绪度 · 三态不可塌陷（lib 真实调用）', () => {
  it('🔴 三种未就绪状态的标签两两不同（塌陷即回归）', () => {
    const labels = ['pending', 'missing', 'unknown'].map((s) => todoInputStateLabel(s))
    expect(new Set(labels).size).toBe(labels.length)
    expect(labels.every((l) => l.trim().length > 0)).toBe(true)
  })

  it('🔴 pending 与 missing 的色调必须不同：一个是「等」，一个是「动手」', () => {
    expect(todoInputStateTone('pending')).toBe('warning')
    expect(todoInputStateTone('missing')).toBe('error')
    expect(todoInputStateTone('resolved')).toBe('success')
  })

  it('未知状态按「不是就绪」处理，且归入需人工介入', () => {
    expect(normalizeTodoInputState('whatever')).toBe('unknown')
    expect(normalizeTodoInputState(null)).toBe('unknown')
    expect(normalizeTodoInputState(undefined)).toBe('unknown')
    const r = todoInputReadiness([input('whatever')])
    expect(r.ready).toBe(0)
    expect(r.allReady).toBe(false)
    expect(r.needsHuman).toBe(true)
    expect(todoInputSummaryTone(r)).toBe('error')
  })

  it('unknown 的标签既不是「已就绪」也不是「上游缺失」（不能误导成不用管）', () => {
    const label = todoInputStateLabel('unknown')
    expect(label).not.toBe(todoInputStateLabel('resolved'))
    expect(label).not.toBe(todoInputStateLabel('missing'))
  })
})

describe('输入位就绪度 · 统计与摘要', () => {
  it('计数按状态分流', () => {
    const r = todoInputReadiness([input('resolved'), input('resolved'), input('pending'), input('missing')])
    expect(r.total).toBe(4)
    expect(r.ready).toBe(2)
    expect(r.pending).toBe(1)
    expect(r.missing).toBe(1)
    expect(r.unknown).toBe(0)
    expect(r.allReady).toBe(false)
    expect(r.hasUnresolved).toBe(true)
    expect(r.needsHuman).toBe(true)
  })

  it('全部就绪时 allReady=true 且无需介入', () => {
    const r = todoInputReadiness([input('resolved')])
    expect(r.allReady).toBe(true)
    expect(r.hasUnresolved).toBe(false)
    expect(r.needsHuman).toBe(false)
    expect(todoInputSummaryTone(r)).toBe('success')
    expect(todoInputActionHint(r)).toBe('')
  })

  it('摘要文案就是方案里的「输入位 3/4 已就绪」；无输入位时为空串', () => {
    expect(todoInputSummaryText(todoInputReadiness([input('resolved'), input('resolved'), input('resolved'), input('missing')])))
      .toBe('输入位 3/4 已就绪')
    expect(todoInputSummaryText(todoInputReadiness([]))).toBe('')
    expect(todoInputSummaryText(todoInputReadiness(undefined))).toBe('')
    expect(todoInputSummaryText(todoInputReadiness(null))).toBe('')
  })

  it('🔴 只有 pending 时提示「等待上游产出」，不得催人去动手', () => {
    const r = todoInputReadiness([input('resolved'), input('pending')])
    expect(todoInputActionHint(r)).toBe('等待上游产出')
    expect(todoInputSummaryTone(r)).toBe('warning')
  })

  it('🔴 有 missing 时升级为「需人工介入」（这是整块可见性的目的）', () => {
    const r = todoInputReadiness([input('pending'), input('missing')])
    expect(todoInputActionHint(r)).toBe('需人工介入')
    expect(todoInputSummaryTone(r)).toBe('error')
  })

  it('空/缺省输入位不抛（老数据零影响）', () => {
    expect(() => todoInputReadiness(undefined)).not.toThrow()
    expect(() => todoInputReadiness(null)).not.toThrow()
    expect(todoInputReadiness(undefined).total).toBe(0)
  })
})

describe('输入位就绪度 · 明细文案', () => {
  it('只拼已知字段，且绝不渲染 undefined', () => {
    expect(describeTodoInput(input('missing'))).toBe('')
    const full = describeTodoInput({
      name: '分组视频生成提示词',
      source_step: '分镜分组',
      output_name: '分组视频生成提示词',
      file_name: 'prompt.md',
      file_size: 51382,
      state: 'resolved',
    })
    expect(full).toContain('分镜分组')
    expect(full).toContain('分组视频生成提示词')
    expect(full).toContain('prompt.md')
    expect(full).toContain('50.2 KB')
    expect(full).not.toMatch(/undefined/)
    expect(describeTodoInput(null)).toBe('')
  })

  it('未就绪排在前面：missing → unknown → pending → resolved，且保持组内原序', () => {
    const list: TodoInput[] = [
      { name: 'r1', state: 'resolved' },
      { name: 'p1', state: 'pending' },
      { name: 'm1', state: 'missing' },
      { name: 'u1', state: 'weird' },
      { name: 'm2', state: 'missing' },
    ]
    expect(sortTodoInputsForDisplay(list).map((i) => i.name)).toEqual(['m1', 'm2', 'u1', 'p1', 'r1'])
    // 不该改动入参顺序（纯函数）
    expect(list[0].name).toBe('r1')
    expect(sortTodoInputsForDisplay(undefined)).toEqual([])
  })
})

describe('输入位就绪度 · 前端类型也不承载取件凭证', () => {
  it('🔴 TodoInput 类型里不出现 download_url / file_ref / artifact_id', () => {
    const src = read(path.join(FE, 'types/index.ts'))
    const iface = src.match(/export interface TodoInput \{[\s\S]*?\n\}/)
    expect(iface, 'types/index.ts 里找不到 TodoInput').toBeTruthy()
    const body = iface![0]
    for (const forbidden of ['download_url', 'file_ref', 'artifact_id', 'source_todo_id']) {
      expect(body, `TodoInput 不该有 ${forbidden}`).not.toContain(forbidden)
    }
  })

  it('Todo 上挂了 inputs（输入方向的 Outputs 对称物）', () => {
    const src = read(path.join(FE, 'types/index.ts'))
    expect(src).toMatch(/inputs\?: TodoInput\[\]/)
  })
})

describe('输入位就绪度 · 组件契约（TaskTodoPanel 源码）', () => {
  const PANEL = path.join(FE, 'components/task/TaskTodoPanel.tsx')

  it('渲染逻辑来自 lib，组件不自己造第二份文案/色调映射', () => {
    const code = stripComments(read(PANEL))
    expect(code).toMatch(/from '@\/lib\/todoInputs'/)
    expect(code).toMatch(/todoInputSummaryText\(/)
    expect(code).toMatch(/todoInputStateLabel\(/)
    // 🔴 四种状态文案只准出现在 lib 里；组件里出现就意味着开始分叉。
    for (const label of ['已就绪', '等待上游产出', '上游缺失', '状态未知']) {
      expect(code, `组件里硬编码了状态文案「${label}」`).not.toContain(label)
    }
  })

  it('输入位进入 hasDetails：否则展开箭头不出现、明细永远看不到', () => {
    const code = stripComments(read(PANEL))
    expect(code).toMatch(/const hasDetails = [^\n]*inputSummary/)
  })

  it('折叠态就能看出「等还是动手」：徽标带 actionHint，且可点开明细', () => {
    const code = stripComments(read(PANEL))
    expect(code).toMatch(/todoInputActionHint\(inputReadiness\)/)
    const badge = code.match(/inputSummary && \([\s\S]*?\n\s*\)\}/)
    expect(badge, '找不到就绪度徽标').toBeTruthy()
    expect(badge![0]).toMatch(/toggleExpand\(todo\.id\)/)
  })

  it('展开区把输入位逐条列出（未就绪的可被看到原因）', () => {
    const code = stripComments(read(PANEL))
    expect(code).toMatch(/<TodoInputList inputs=\{todo\.inputs\} \/>/)
    expect(code).toMatch(/describeTodoInput\(/)
    expect(code).toMatch(/sortTodoInputsForDisplay\(/)
  })
})
