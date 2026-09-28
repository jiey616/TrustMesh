/// <reference types="node" />
// 契约测试要读源码做断言，因此显式引入 node 类型（原因同 desktop-notification-contract.test.ts）。
import fs from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

// SSE 断线自愈的跨层契约守卫。
//
// 🔴 为什么需要它（2026-09-24 生产实测）：
//  桌面端 SSE 曾**长期完全失效**且毫无报错。按客户端版本统计 /api/v1/events/stream：
//    0.2.5 → 57,561 次 401 / 0 次成功；0.2.4 → 28,777 / 0；某机每小时精确 600 次
//    连跑 48h。合计 79,658 次无谓重连。
//  根因是三件事叠加成的**自锁**，每一件单看都不报错：
//    1) SSE 用裸 fetch，绕过了 apiClient 的「401 → 单飞刷新 → 重放」链路；
//    2) token 过期后窗口在托盘 ⇒ RQ 暂停全部轮询（refetchIntervalInBackground
//       默认 false）⇒ 没有业务请求 ⇒ 没有任何 401 能触发刷新；
//    3) 重连是固定 5s ⇒ 拿同一个过期 token 无限空转。
//  「绕过了公共链路」+「只剩自己这一条路」是这类 bug 的通用形态，且类型 / lint /
//  构建全都不说话。所以必须由测试把「SSE 接上了刷新原语」「退避有上限」钉死。

const ROOT = path.resolve(__dirname, '../../..')
const FE = path.join(ROOT, 'frontend-v2')

// 🔴 必须归一化换行符：本仓库混行尾，跨块正则在 CRLF 文件上恒不命中。
function read(p: string) {
  return fs.readFileSync(p, 'utf8').replace(/\r\n/g, '\n')
}

// 注释里提到函数名是正常且必要的，所以「不许出现 X」类断言必须先剥注释。
function stripComments(src: string): string {
  return src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '')
}

const RT = path.join(FE, 'src/hooks/useRealtimeEvents.ts')
const CLIENT = path.join(FE, 'src/api/client.ts')

describe('SSE 自愈 · 复用刷新原语', () => {
  it('🔴 SSE 必须 import 并调用 apiClient 导出的两个原语（不许复制一份实现）', () => {
    const src = read(RT)
    expect(src).toMatch(/from '@\/api\/client'/)
    expect(src).toMatch(/ensureAccessTokenReady/)
    expect(src).toMatch(/refreshAccessTokenOnce/)
  })

  it('原语确实是 client.ts 为「绕过 hooks 的通道」导出的', () => {
    const code = stripComments(read(CLIENT))
    expect(code).toMatch(/export function ensureAccessTokenReady\(\): Promise<void>/)
    expect(code).toMatch(/export function refreshAccessTokenOnce\(\): Promise<void>/)
  })

  it('🔴 401 分支必须真的刷新，且排在调度重连之前', () => {
    const code = stripComments(read(RT))
    const i = code.indexOf('res.status === 401')
    expect(i).toBeGreaterThan(-1)
    const tail = code.slice(i)
    const iRefresh = tail.indexOf('await refreshAccessTokenOnce()')
    const iSchedule = tail.indexOf('scheduleReconnect()')
    expect(iRefresh).toBeGreaterThan(-1)
    expect(iSchedule).toBeGreaterThan(-1)
    expect(iRefresh).toBeLessThan(iSchedule)
  })

  it('🔴 一轮只刷新一次（防 refresh/401 风暴发散）', () => {
    const code = stripComments(read(RT))
    expect(code).toMatch(/let refreshedInCycle = false/)
    expect(code).toMatch(/if \(!refreshedInCycle\)/)
    expect(code).toMatch(/refreshedInCycle = true/)
  })

  it('🔴 冷启动门闩：fetch 之前先等 token 就绪', () => {
    const code = stripComments(read(RT))
    const iEnsure = code.indexOf('await ensureAccessTokenReady()')
    const iFetch = code.indexOf('await fetch(SSE_URL')
    expect(iEnsure).toBeGreaterThan(-1)
    expect(iFetch).toBeGreaterThan(-1)
    expect(iEnsure).toBeLessThan(iFetch)
  })

  it('刷新失败不得就地登出（交由退避兜住，避免网络抖动把人踢下线）', () => {
    const code = stripComments(read(RT))
    expect(code).toMatch(/refreshAccessTokenOnce\(\)\.catch\(/)
    expect(code).not.toMatch(/logout\(\)/)
  })
})

describe('SSE 自愈 · 退避', () => {
  it('🔴 固定 5s 重连已废除：改为带上下限的退避常量', () => {
    const code = stripComments(read(RT))
    expect(code).toMatch(/SSE_INITIAL_RETRY_MS\s*=\s*1_000/)
    expect(code).toMatch(/SSE_MAX_RETRY_MS\s*=\s*30_000/)
    // 旧实现：window.setTimeout(() => void connect(), 5000) —— 必须已消失
    expect(code).not.toMatch(/void connect\(\), 5000\)/)
  })

  it('重连延迟按指数增长并被上限夹住', () => {
    const code = stripComments(read(RT))
    expect(code).toMatch(/Math\.min\(retryDelay \* 2, SSE_MAX_RETRY_MS\)/)
  })

  it('🔴 连接成功后重置退避（否则一次抖动会永久停在 30s）', () => {
    const code = stripComments(read(RT))
    const iThrow = code.indexOf('throw new Error(`SSE status')
    expect(iThrow).toBeGreaterThan(-1)
    const iReset = code.indexOf('retryDelay = 0', iThrow)
    expect(iReset, '「连上了」之后没有重置 retryDelay').toBeGreaterThan(-1)
    const iResetFlag = code.indexOf('refreshedInCycle = false', iThrow)
    expect(iResetFlag, '「连上了」之后没有重置 refreshedInCycle').toBeGreaterThan(-1)
  })

  it('401 响应体必须被消费或取消（否则连接资源不释放）', () => {
    const code = stripComments(read(RT))
    expect(code).toMatch(/res\.body\.cancel\(\)/)
  })
})
