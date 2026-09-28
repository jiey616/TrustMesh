/// <reference types="node" />
// 契约测试要读源码做断言，因此显式引入 node 类型（原因同 desktop-notification-contract.test.ts）。
import fs from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

// 事件流增量轮询 + 限流读路径放宽（2026-09-28）的契约守卫。
//
// 🔴 为什么需要它：
//  「轮询只传增量」这件事天然有两个静默失败方向，且都不报错：
//   1. **漏**：游标写松了（`>`）、或前端截断历史 ⇒「执行过程」少几条，
//      更严重的是 `collectPendingItems` 会漏判「数字员工在等你回答」的待办；
//   2. **重**：游标写紧了又不做去重 ⇒ 渲染出重复条目。
//  这两条都只有用户能看出来。所以必须把「`>=` 语义 + 按 id 去重 + 不截断历史」
//  三件事同时钉死在测试里，任何一边被改坏都会立刻变红。
//
//  限流那条同理：放宽读路径是为了修「429 ⇒ 查询失败 ⇒ UI 静默停在旧数据」，
//  但如果有人顺手把写路径阈值也一起调大，就悄悄削弱了写侧防护。

const ROOT = path.resolve(__dirname, '../../..')
const FE = path.join(ROOT, 'frontend-v2')
const BE = path.join(ROOT, 'backend')

function read(p: string) {
  return fs.readFileSync(p, 'utf8').replace(/\r\n/g, '\n')
}

function stripComments(src: string): string {
  return src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '')
}

/** 抽出一段匹配的源码块；找不到就抛（避免正则失效时静默通过）。 */
function grab(source: string, re: RegExp, what: string): string {
  const m = source.match(re)
  if (!m) throw new Error(`pattern not found for ${what}`)
  return m[0]
}

const TASKS = path.join(FE, 'src/hooks/useTasks.ts')
const API = path.join(FE, 'src/api/tasks.ts')
const APP = path.join(FE, 'src/App.tsx')
const PENDING = path.join(FE, 'src/lib/pendingItems.ts')
const GO_STORE = path.join(BE, 'internal/store/store_task.go')
const GO_HANDLER = path.join(BE, 'internal/handler/task.go')
const GO_RATE = path.join(BE, 'internal/middleware/rate_limit.go')

describe('事件流增量 · 后端游标', () => {
  it('🔴 零值游标 = 全量（向后兼容红线：frontend-mobile 也在调这个端点）', () => {
    const fn = grab(
      stripComments(read(GO_STORE)),
      /func \(s \*Store\) ListTaskEvents\([\s\S]*?\n\}/,
      'ListTaskEvents',
    )
    expect(fn).toMatch(/since time\.Time/)
    // 只在游标非零时才过滤 —— 这就是「缺省全量」的实现形态
    expect(fn).toMatch(/!since\.IsZero\(\)\s*&&\s*\w+\.CreatedAt\.Before\(since\)/)
  })

  it('🔴 游标语义是 >= 而不是 >（> 会吞掉与游标同毫秒的边界事件）', () => {
    const fn = grab(
      stripComments(read(GO_STORE)),
      /func \(s \*Store\) ListTaskEvents\([\s\S]*?\n\}/,
      'ListTaskEvents',
    )
    // >= 的正确写法就是「早于游标才跳过」；出现 After(since) 说明被改成了 > 语义
    expect(fn).toMatch(/CreatedAt\.Before\(since\)/)
    expect(fn).not.toMatch(/CreatedAt\.After\(since\)/)
  })

  it('handler 解析 since（RFC3339）且**解析失败按缺省处理**，不返回 4xx', () => {
    const fn = grab(
      stripComments(read(GO_HANDLER)),
      /func \(h \*TaskHandler\) ListEvents\([\s\S]*?\n\}/,
      'ListEvents',
    )
    expect(fn).toMatch(/c\.Query\("since"\)/)
    expect(fn).toMatch(/time\.Parse\(time\.RFC3339Nano/)
    // 这只是个优化参数：一个格式错误不该把「看执行过程」整条路搞挂
    expect(fn).not.toMatch(/BadRequest|Validation/)
  })
})

describe('事件流增量 · 前端消费', () => {
  it('API 层支持 since，且缺省不发送该参数（缺省即全量）', () => {
    const fn = grab(
      stripComments(read(API)),
      /export async function listTaskEvents\([\s\S]*?\n\}/,
      'listTaskEvents',
    )
    expect(fn).toMatch(/since\?: string/)
    expect(fn).toMatch(/since \? \{ since \} : \{\}/)
  })

  it('🔴 轮询带上「上次最后一条」的 created_at 作游标', () => {
    const fn = grab(
      stripComments(read(TASKS)),
      /export function useTaskEvents\([\s\S]*?\n\}/,
      'useTaskEvents',
    )
    expect(fn).toMatch(/prev\[prev\.length - 1\]\?\.created_at/)
    expect(fn).toMatch(/listTaskEvents\(id!, since\)/)
  })

  it('🔴 增量按 id 去重合并（游标是 >=，边界那条会重复回来）', () => {
    const fn = grab(
      stripComments(read(TASKS)),
      /function mergeEventsById\([\s\S]*?\n\}/,
      'mergeEventsById',
    )
    expect(fn).toMatch(/new Set\(prev\.map\(\(e\) => e\.id\)\)/)
    expect(fn).toMatch(/!seen\.has\(e\.id\)/)
  })

  it('🔴 绝不截断历史：pendingItems 靠遍历完整事件流找未答复的 todo_ask', () => {
    const fn = grab(
      stripComments(read(TASKS)),
      /export function useTaskEvents\([\s\S]*?\n\}/,
      'useTaskEvents',
    )
    // 一旦出现「只取最近 N 条」，历史里未答复的提问就会从待确认列表里消失
    expect(fn).not.toMatch(/\.slice\(-\d+\)/)
    expect(fn).not.toMatch(/\.slice\(0,\s*\d+\)/)

    // 反向确认这个约束确实存在（否则上面两条断言等于自说自话）
    const p = stripComments(read(PENDING))
    expect(p).toMatch(/for \(const ev of events \?\? \[\]\)/)
    expect(p).toMatch(/if \(ev\.metadata\?\.answer != null\) continue/)
  })
})

describe('限流 · 读路径放宽', () => {
  const rateSrc = () => stripComments(read(GO_RATE))
  const constOf = (name: string): number => {
    const m = rateSrc().match(new RegExp(`\\b${name}\\s*=\\s*(\\d+)`))
    if (!m) throw new Error(`${name} not found`)
    return Number(m[1])
  }

  it('🔴 读路径阈值严格大于写路径（写侧保持原值）', () => {
    expect(constOf('readIPRateLimit')).toBeGreaterThan(constOf('ipRateLimit'))
    expect(constOf('readGlobalRateLimit')).toBeGreaterThan(constOf('globalRateLimit'))
    // 写侧必须还是原值：放宽读路径不能顺手削弱写防护
    expect(constOf('ipRateLimit')).toBe(20)
    expect(constOf('globalRateLimit')).toBe(100)
  })

  it('GET/HEAD 归读路径，其它方法归写路径', () => {
    const code = rateSrc()
    expect(code).toMatch(/http\.MethodGet \|\| c\.Request\.Method == http\.MethodHead/)
    expect(code).toMatch(/limiter\.allow\(ip, read\)/)
  })

  it('429 带 Retry-After（客户端 ky 靠它算退避，缺了就变盲目重试）', () => {
    expect(rateSrc()).toMatch(/c\.Header\("Retry-After", "1"\)/)
  })
})

describe('429 · 不在 React Query 层二次重试', () => {
  it('🔴 429 的 ApiRequestError 直接判「不重试」', () => {
    const fn = grab(
      stripComments(read(APP)),
      /retry: \(failureCount, error\) => \{[\s\S]*?\n\s*\}/,
      'queryClient.retry',
    )
    expect(fn).toMatch(/error\.status === 429/)
    expect(fn).toMatch(/return false/)
  })

  it('必须真的 import 了 ApiRequestError（否则 instanceof 恒 false = 改动等于没做）', () => {
    expect(read(APP)).toMatch(/import \{ ApiRequestError \} from/)
  })
})
