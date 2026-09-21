import fs from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

// 失败任务「重试/继续」的跨层契约守卫。
//
// 🔴 为什么需要这个测试：
//  1. 它是**跨仓库契约** —— 平台发带 `resume: true` 的 todo.remind，执行侧的
//     tm-task-exec skill 靠这个标记决定「这是执行指令」而不是「超时心跳」。
//     标记名漂移的后果是静默的：agent 收到提醒只会回一句进度就停，用户以为
//     点了重试但什么都没发生，且**两边编译器都不报错**。
//  2. 链路顺序是正确性的一部分 —— 必须先 ReopenTodo（置 in_progress）再发 remind。
//     顺序反了/并发做，agent 做完的 todo.complete 会被 TODO_ALREADY_FAILED 拒收，
//     产出静默丢失。
//  3. 「只有 failed 可重试」这条规则在 store / handler / 前端三处各有一份判断。
//     任何一处放宽都会让 canceled/done 走进重试路径。

const ROOT = path.resolve(__dirname, '../../..')
const FE = path.join(ROOT, 'frontend-v2/src')

// 🔴 必须归一化换行符再断言。
//   本仓库的文件是混行尾的：`types/index.ts` 和 `skills/*/SKILL.md` 是 CRLF，
//   而 backend 的 .go 与大部分 .tsx 是 LF。凡是跨块的正则（`[\s\S]*?\n\n`
//   这类"匹配到空行为止"的写法）在 CRLF 文件上恒不命中 —— 这是纯粹的环境
//   噪声，不是产品缺陷，一旦踩上就会浪费大量时间去怀疑被测代码。
function read(p: string) {
  return fs.readFileSync(p, 'utf8').replace(/\r\n/g, '\n')
}

describe('重试/继续 · 前端 API 契约', () => {
  const API = path.join(FE, 'api/tasks.ts')

  it('resumeTodo 打的是 resume 路由，且 reason 是必填参数（非可选）', () => {
    const src = read(API)
    const fn = src.match(/export async function resumeTodo\([\s\S]*?\n\}/)
    if (!fn) throw new Error('resumeTodo not found in api/tasks.ts')
    expect(fn[0]).toMatch(/todos\/\$\{todoId\}\/resume/)
    // 与 reopenTodo 的关键差异：reason 不带 `?`、也不做 `?? ''` 兜底 ——
    // 空理由必须让后端 422，而不是前端悄悄拼一个空串上去。
    expect(fn[0]).toMatch(/reason:\s*string/)
    expect(fn[0]).not.toMatch(/reason\?/)
    expect(fn[0]).not.toMatch(/\?\?\s*''/)
  })

  it('reopen 与 resume 是两个不同的路由（不能互相复用）', () => {
    const src = read(API)
    const re = src.match(/export async function reopenTodo\([\s\S]*?\n\}/)
    if (!re) throw new Error('reopenTodo not found')
    expect(re[0]).toMatch(/todos\/\$\{todoId\}\/reopen/)
    expect(re[0]).not.toMatch(/\/resume/)
  })
})

describe('重试/继续 · 后端链路顺序', () => {
  const HANDLER = path.join(ROOT, 'backend/internal/handler/task.go')

  it('handler 先 ResumeFailedTodo（重开）再发 remind —— 顺序不可颠倒', () => {
    const src = read(HANDLER)
    const fn = src.match(/func \(h \*TaskHandler\) ResumeTodo\(c \*gin\.Context\) \{[\s\S]*?\n\}/)
    if (!fn) throw new Error('TaskHandler.ResumeTodo not found')
    const body = fn[0]
    const iReopen = body.indexOf('ResumeFailedTodo')
    const iRemind = body.indexOf('webhookHandler.ResumeTodo')
    expect(iReopen).toBeGreaterThan(-1)
    expect(iRemind).toBeGreaterThan(-1)
    // 🔴 一旦 remind 跑到 reopen 前面，agent 的产出就会被 TODO_ALREADY_FAILED 丢掉。
    expect(iReopen).toBeLessThan(iRemind)
    // 中间必须有"重开失败就 return"的短路，否则失败时照样会发提醒。
    const between = body.slice(iReopen, iRemind)
    expect(between).toMatch(/if appErr != nil \{[\s\S]*?return[\s\S]*?\}/)
  })

  it('store 层只接受 failed（canceled/done 必须被拒）', () => {
    const src = read(path.join(ROOT, 'backend/internal/store/workflow.go'))
    const fn = src.match(/func \(s \*Store\) ResumeFailedTodo\([\s\S]*?\n\}/)
    if (!fn) throw new Error('ResumeFailedTodo not found')
    expect(fn[0]).toMatch(/todo\.Status != "failed"/)
    expect(fn[0]).toMatch(/TODO_NOT_FAILED/)
  })

  it('resume 与 reopen 的终态白名单不同（reopen 允许三态，resume 只允许 failed）', () => {
    const src = read(path.join(ROOT, 'backend/internal/store/workflow.go'))
    const reopen = src.match(/func \(s \*Store\) ReopenTodo\([\s\S]*?\n\}/)
    if (!reopen) throw new Error('ReopenTodo not found')
    expect(reopen[0]).toMatch(/case "failed", "canceled", "done":/)
  })
})

describe('重试/继续 · remind payload 的 resume 标记', () => {
  const WH = path.join(ROOT, 'backend/internal/clawsynapse/webhook.go')

  it('平台在 payload 里写 resume: true + source: user_resume + reason', () => {
    const src = read(WH)
    const fn = src.match(/func \(h \*WebhookHandler\) remindTodo\([\s\S]*?\n\}/)
    if (!fn) throw new Error('remindTodo not found')
    const body = fn[0]
    expect(body).toMatch(/payload\["resume"\]\s*=\s*true/)
    expect(body).toMatch(/"user_resume"/)
    expect(body).toMatch(/payload\["reason"\]\s*=\s*reason/)
    // 超时默认分支必须仍在，否则 timeout_monitor 的催办语义会被改掉。
    expect(body).toMatch(/timeout_remind/)
  })

  it('🔴 执行侧 skill 的判别位与平台写出的字段名一致', () => {
    // 这是整个特性最容易静默失效的地方：平台写 `resume`，skill 认 `resume: true`。
    // 任一侧改名都不会编译报错，只会让 agent 收不到"这是执行指令"的信号。
    const skill = read(path.join(ROOT, 'skills/tm-task-exec/SKILL.md'))
    expect(skill).toMatch(/resume:\s*true/)
    // 平台侧写出的字段名（webhook.go）必须与 skill 认的完全一致 —— 做一次真正的
    // 字面量对接，而不是各查各的。
    expect(read(path.join(ROOT, 'backend/internal/clawsynapse/webhook.go'))).toMatch(/payload\["resume"\]\s*=\s*true/)
    // 判别**只能**认 resume：老 node 的 payload 里没有 source，也不能因此走错分支。
    // skill 必须把「无 resume」明确判给超时心跳分支。
    expect(skill).toMatch(/无 `resume`[^\n]*超时/)
    // 两分支都要在文档里存在，且分支 B（用户重试）必须被明确标为执行指令。
    expect(skill).toMatch(/分支 A/)
    expect(skill).toMatch(/分支 B/)
    expect(skill).toMatch(/执行指令/)
  })
})

describe('重试/继续 · 终态不再接受评论', () => {
  const WS = path.join(FE, 'components/task/TaskWorkspace.tsx')

  it('终态判断覆盖 failed/canceled/done，且先于评论框渲染', () => {
    const src = read(WS)
    expect(src).toMatch(/isTerminal\s*=\s*task \? \['failed', 'canceled', 'done'\]\.includes\(task\.status\)/)
    // 终态分支必须排在 TaskCommentComposer 之前，否则输入框仍会渲染出来。
    const iTerminal = src.indexOf(') : isTerminal ? (')
    const iComposer = src.lastIndexOf('<TaskCommentComposer')
    expect(iTerminal).toBeGreaterThan(-1)
    expect(iComposer).toBeGreaterThan(-1)
    expect(iTerminal).toBeLessThan(iComposer)
  })

  it('失败任务渲染「重试/继续」入口，而非只给一句说明', () => {
    const src = read(WS)
    expect(src).toMatch(/<TaskResumeEntry\s+taskId=\{taskId\}\s+failedTodos=\{failedTodos\}/)
    expect(src).toMatch(/failedTodos\s*=\s*useMemo\(\(\) => \(task\?\.todos \?\? \[\]\)\.filter\(\(t\) => t\.status === 'failed'\)/)
  })

  it('入口组件要求理由必填、并对重试上限做前置拦截', () => {
    const src = read(path.join(FE, 'components/task/TaskResumeEntry.tsx'))
    expect(src).toMatch(/reason\.trim\(\)\.length < 2/)
    expect(src).toMatch(/MAX_REOPENS = 3/)
    expect(src).toMatch(/reopen_count/)
  })

  it('「重开 todo」不再承诺「去评论区 @ 执行员工」', () => {
    // mention 走 chat 通道，在已失败 todo 上没有在途 run，根本不会触发执行。
    const panel = read(path.join(FE, 'components/task/TaskTodoPanel.tsx'))
    expect(panel).not.toMatch(/请到任务评论 @ 执行员工唤醒其继续执行/)
  })

  it('Todo 类型声明了 reopen_count（否则上限提示恒显示 0/3）', () => {
    const types = read(path.join(FE, 'types/index.ts'))
    const decl = types.match(/export interface Todo \{[\s\S]*?\n\}/)
    if (!decl) throw new Error('Todo interface not found')
    expect(decl[0]).toMatch(/reopen_count\?: number/)
  })
})

describe('重试/继续 · 事件类型', () => {
  it('EventType 包含后端会写的 todo_resumed / todo_reopened', () => {
    const types = read(path.join(FE, 'types/index.ts'))
    const decl = types.match(/export type EventType =[\s\S]*?\n\n/)
    if (!decl) throw new Error('EventType not found')
    expect(decl[0]).toMatch(/'todo_resumed'/)
    expect(decl[0]).toMatch(/'todo_reopened'/)
  })

  it('后端为 todo_resumed 配了通知文案（否则事件不进通知流）', () => {
    const src = read(path.join(ROOT, 'backend/internal/store/store_notification_internal.go'))
    expect(src).toMatch(/case "todo_resumed":/)
  })
})
