/// <reference types="node" />
// 契约测试要读源码做断言，因此显式引入 node 类型（原因同 desktop-notification-contract.test.ts）。
import fs from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

// 任务查询失效精确化 + 待确认操作乐观更新的契约守卫。
//
// 🔴 为什么需要它（2026-09-24 生产实测）：
//  1) `invalidateTask` 里那句 `queryKey: ['tasks']` 是**前缀匹配**，会命中缓存里
//     每一个任务的 detail / events / comments —— 而任务事件流单次就有 542 KB。
//     服务端日志实测：单次人工确认后 53 ms 内并发 5 个请求，350 ms 后又重复同样 5 个。
//  2) 确认类 mutation 只有 onSuccess 失效、没有乐观更新 ⇒ 用户点完「通过」后要盯着
//     已点过的按钮等一个公网往返（实测 1.4–2.3 s），期间「待确认」条目还在、
//     弹框也没消失 —— 用户描述的「确认完了框还显示没确认」就是这么来的。
//
// 这两条都是**性能/体验型**缺陷：不报错、不崩、类型与 lint 全绿，只是慢。
// 没人会写测试去测「慢」，所以只能靠源码契约把形状钉死。

const ROOT = path.resolve(__dirname, '../../..')
const FE = path.join(ROOT, 'frontend-v2')

function read(p: string) {
  return fs.readFileSync(p, 'utf8').replace(/\r\n/g, '\n')
}

function stripComments(src: string): string {
  return src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '')
}

const TASKS = path.join(FE, 'src/hooks/useTasks.ts')
const PENDING = path.join(FE, 'src/lib/pendingItems.ts')
const API = path.join(FE, 'src/api/tasks.ts')

/** 截取一个顶层 `export function name(...) {...}` / `function name(...) {...}` 的源码块。 */
function fnBlock(source: string, name: string): string {
  const m = source.match(new RegExp(`(?:export )?function ${name}\\([\\s\\S]*?\\n\\}`))
  if (!m) throw new Error(`function ${name} not found`)
  return m[0]
}

describe('任务查询失效 · 精确化', () => {
  it("🔴 invalidateTask 不再用 ['tasks'] 宽前缀（前缀匹配会波及其它任务的 542KB 事件流）", () => {
    const blk = fnBlock(stripComments(read(TASKS)), 'invalidateTask')
    expect(blk).not.toMatch(/queryKey: \['tasks'\]/)
    expect(blk).toMatch(/predicate:/)
  })

  it('🔴 predicate 必须把「其它任务」的 detail / events / comments 排除掉', () => {
    const blk = fnBlock(stripComments(read(TASKS)), 'invalidateTask')
    expect(blk).toMatch(/k\[0\] !== 'tasks'/)
    expect(blk).toMatch(/k\[1\] === 'detail'/)
    expect(blk).toMatch(/k\[2\] !== taskId/)
  })

  it('任务列表查询仍然失效（第二段不是 detail）', () => {
    const blk = fnBlock(stripComments(read(TASKS)), 'invalidateTask')
    expect(blk).toMatch(/\n\s*return true\n/)
  })

  it('已用权威响应写过详情时可跳过重复拉取（skipDetail）', () => {
    const blk = fnBlock(stripComments(read(TASKS)), 'invalidateTask')
    expect(blk).toMatch(/skipDetail/)
  })
})

describe('待确认操作 · 乐观更新', () => {
  it('🔴 三个确认类 mutation 都必须 onMutate + onError 回滚', () => {
    const code = stripComments(read(TASKS))
    for (const fn of ['useApprovePlan', 'useReviewTodo', 'useAnswerTodo']) {
      const blk = fnBlock(code, fn)
      expect(blk, `${fn} 缺 onMutate`).toMatch(/onMutate:/)
      expect(blk, `${fn} 缺 onError`).toMatch(/onError:/)
      expect(blk, `${fn} onError 未恢复快照`).toMatch(/restore(TaskDetail|TaskEvents)\(/)
    }
  })

  it('🔴 乐观写入的字段就是 pendingItems 真正读的那些', () => {
    const code = stripComments(read(TASKS))
    // plan_review ← task.status === 'review'
    expect(fnBlock(code, 'useApprovePlan')).toMatch(/status: 'in_progress'/)
    // todo_review ← todo.review_status === 'pending_approval'
    expect(fnBlock(code, 'useReviewTodo')).toMatch(/review_status:/)
    // todo_ask ← events 里 todo_ask_received 且 metadata.answer == null
    expect(fnBlock(code, 'useAnswerTodo')).toMatch(/metadata: \{ \.\.\.ev\.metadata, answer \}/)
  })

  it('🔴 这三条判据确实是 pendingItems.ts 在用的（跨文件配对，防单边漂移）', () => {
    const p = stripComments(read(PENDING))
    expect(p).toMatch(/task\.status === 'review'/)
    expect(p).toMatch(/todo\.review_status !== 'pending_approval'/)
    expect(p).toMatch(/todo_ask_received/)
    expect(p).toMatch(/metadata\?\.answer != null/)
  })

  it('approve / review 用后端返回的权威 task 直接写缓存（省一次往返）', () => {
    const code = stripComments(read(TASKS))
    for (const fn of ['useApprovePlan', 'useReviewTodo']) {
      expect(fnBlock(code, fn), `${fn} 未用权威响应写缓存`).toMatch(
        /qc\.setQueryData\(\['tasks', 'detail', taskId\], res\.data\)/,
      )
    }
  })

  it('🔴 answerTodo 不得把返回值写进详情缓存（后端返回的不是 TaskDetail）', () => {
    // handler/task.go 的 AnswerTodo 传的是 gin.H{"status","question_id"}，
    // 而 api/tasks.ts 曾把它声明成 ApiResponse<TaskDetail>。照该类型写缓存会把
    // {status:'ok'} 当任务对象塞进去。这里同时钉住「不写缓存」与「类型声明诚实」。
    expect(fnBlock(stripComments(read(TASKS)), 'useAnswerTodo')).not.toMatch(
      /setQueryData\(\['tasks', 'detail'/,
    )
    const apiBlk = fnBlock(stripComments(read(API)), 'answerTodo')
    expect(apiBlk).not.toMatch(/ApiResponse<TaskDetail>/)
    expect(apiBlk).toMatch(/question_id: string/)
  })

  it('claim 过的 mutations 都走精确失效（不再出现 invalidateTask 的旧单参调用）', () => {
    // 旧签名 invalidateTask(qc, taskId) 仍合法（skipDetail 可选），但确认类必须传 skipDetail。
    const code = stripComments(read(TASKS))
    for (const fn of ['useApprovePlan', 'useReviewTodo']) {
      expect(fnBlock(code, fn)).toMatch(/invalidateTask\(qc, taskId, \{ skipDetail: true \}\)/)
    }
  })
})
