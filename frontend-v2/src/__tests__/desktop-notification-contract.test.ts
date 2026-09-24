/// <reference types="node" />
// 契约测试要读源码做断言，因此显式引入 node 类型（原因同 manual-block-contract.test.ts）。
import fs from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

// 桌面端「系统通知」的跨层契约守卫。
//
// 🔴 为什么需要这个测试：
//  2026-09-23 排查发现，这条链路**主进程 / preload / 类型声明三段都在，唯独渲染端 0 调用**
//  （`git log -S 'showNotification'` 只有 1 个 commit，即只被加入过、从未删除 ⇒ 不是回归，
//  是从一开始就没接上）。「写了 IPC 却没人调」是这类功能的典型死法，而且完全静默：
//  类型里声明成可选、调用方不调，编译、lint、类型检查全都不会说话。所以必须由测试钉死
//  「渲染端确实在 notification.created 分支里请求弹通知」这一环。
//
//  另外两条同样是静默失败：
//   - 通知请求若排在 `EVENT_INVALIDATIONS` 的提前 return **之后**，永远不执行；
//   - 「前台不弹」的判定若放在渲染端只会有 `document.hidden`，无法区分「被别的应用盖住」
//     与「正在操作本应用」⇒ 该判定必须在主进程用 `BrowserWindow.isFocused()`。

const ROOT = path.resolve(__dirname, '../../..')
const FE = path.join(ROOT, 'frontend-v2')

// 🔴 必须归一化换行符：本仓库混行尾，凡「匹配到空行/到文件尾」的跨块正则在 CRLF 文件上恒不命中。
function read(p: string) {
  return fs.readFileSync(p, 'utf8').replace(/\r\n/g, '\n')
}

// 注释里提到组件名/函数名是正常且必要的，所以「不许出现 X」这类断言必须先剥注释。
function stripComments(src: string): string {
  return src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '')
}

const RT = path.join(FE, 'src/hooks/useRealtimeEvents.ts')
const MAIN = path.join(FE, 'electron/main.cjs')
const PRELOAD = path.join(FE, 'electron/preload.cjs')
const LAYOUT = path.join(FE, 'src/layouts/MainLayout.tsx')
const INBOX = path.join(FE, 'src/pages/InboxPage.tsx')
const DTYPES = path.join(FE, 'src/types/desktop.d.ts')
const LIB = path.join(FE, 'src/lib/notifications.ts')

describe('桌面端系统通知 · 渲染端触发', () => {
  it('🔴 notification.created 分支确实请求弹系统通知（曾经整块缺失的一环）', () => {
    const src = read(RT)
    expect(src).toMatch(/window\.desktop\?\.showNotification/)
    expect(src).toMatch(/type === 'notification\.created'/)
    expect(src).toMatch(/notifyViaDesktopShell\(n\)/)
  })

  it('🔴 触发必须排在 EVENT_INVALIDATIONS 提前 return 之前（否则永不执行）', () => {
    const src = read(RT)
    const iNotify = src.indexOf("type === 'notification.created'")
    const iReturn = src.indexOf('const inv = EVENT_INVALIDATIONS[type]')
    expect(iNotify).toBeGreaterThan(-1)
    expect(iReturn).toBeGreaterThan(-1)
    expect(iNotify).toBeLessThan(iReturn)
  })

  it('只在桌面壳存在时弹（Web / 移动端静默 no-op，老版本壳也不能崩）', () => {
    const code = stripComments(read(RT))
    expect(code).toMatch(/typeof show !== 'function'/)
    expect(code).toMatch(/if \(!n\.title\) return/)
  })

  it('点击跳转目标与收件箱共用同一份规则（不许各算一套）', () => {
    const src = read(RT)
    expect(src).toMatch(/from '@\/lib\/notifications'/)
    expect(src).toMatch(/notificationTarget\(n\)/)
  })
})

describe('桌面端系统通知 · 主进程闸门', () => {
  it('🔴 窗口在前台时不弹：isFocused() 判定且排在 show() 之前', () => {
    const code = stripComments(read(MAIN))
    const blk = code.match(/ipcMain\.handle\('tm:show-notification'[\s\S]*?\n {4}\}/)
    if (!blk) throw new Error('tm:show-notification handler not found')
    const body = blk[0]
    expect(body).toMatch(/isFocused\(\)\)\s*return false/)
    expect(body.indexOf('isFocused()')).toBeLessThan(body.indexOf('n.show()'))
  })

  it('点击回传的载荷就是请求时给的 clickTarget（渲染端据此导航）', () => {
    const code = stripComments(read(MAIN))
    expect(code).toMatch(/const \{ title, body, clickTarget \} = options/)
    expect(code).toMatch(/send\('tm:notification-clicked', clickTarget\)/)
  })
})

describe('桌面端系统通知 · 点击跳转装配', () => {
  it('MainLayout 订阅 onNotificationClicked 并 navigate（兜底收件箱）', () => {
    const src = read(LAYOUT)
    expect(src).toMatch(/window\.desktop\?\.onNotificationClicked/)
    expect(src).toMatch(/navigate\(clickTarget \|\| '\/inbox'\)/)
  })

  it('InboxPage 不再自持一份跳转规则（规则已提取到 lib）', () => {
    const src = read(INBOX)
    expect(src).toMatch(/import \{\s*groupNotificationsByDate,\s*notificationTarget\s*\}/)
    expect(src).toMatch(/notificationTarget\(item\)/)
    // 原文里那串内联分支（agent → `/agents/${...}`）必须已经消失，否则等于两份实现
    expect(stripComments(src)).not.toMatch(/navigate\(`\/agents\/\$\{/)
  })
})

describe('桌面端系统通知 · 桥接声明', () => {
  it('preload 暴露 showNotification / onNotificationClicked 两条通道', () => {
    const src = read(PRELOAD)
    expect(src).toMatch(/showNotification: \(options\)/)
    expect(src).toMatch(/onNotificationClicked: \(callback\)/)
    expect(src).toMatch(/'tm:notification-clicked'/)
  })

  it('类型声明为**可选**（线上仍有老版本壳，其 preload 不含这两条）', () => {
    const src = read(DTYPES)
    expect(src).toMatch(/showNotification\?: \(options/)
    expect(src).toMatch(/onNotificationClicked\?: \(callback/)
    expect(src).toMatch(/clickTarget\?: string/)
  })
})

describe('桌面端系统通知 · 只弹白名单两类（来源键判定，非标题）', () => {
  // 用户口径：只要「需要人工确认的」与「任务完成/失败」两类弹系统横幅。
  // 这条筛选也是静默失败型的：漏了筛选只是"多弹"，但**判据取错**（用标题文本）
  // 会在后端改一次文案后整段失效，且不报错。
  it('🔴 渲染端必须先过白名单，再请求弹通知', () => {
    const code = stripComments(read(RT))
    const iFilter = code.indexOf('shouldNotifyDesktop(n)')
    const iShow = code.indexOf('show({ title:')
    expect(iFilter).toBeGreaterThan(-1)
    expect(iShow).toBeGreaterThan(-1)
    expect(iFilter).toBeLessThan(iShow)
  })

  it('🔴 判据只有 source_event 一个：不许按 title / body 文本匹配', () => {
    const code = stripComments(read(LIB))
    expect(code).toMatch(/item\?\.source_event/)
    expect(code).toMatch(/DESKTOP_NOTIFY_SOURCES\.includes/)
    // 白名单函数体里不该出现 title / body（注释已剥掉，命中的就是真代码）
    const fn = code.match(/export function shouldNotifyDesktop[\s\S]*?\n\}/)
    if (!fn) throw new Error('shouldNotifyDesktop not found')
    expect(fn[0]).not.toMatch(/title|body/)
  })

  it('白名单恰好是这 9 个来源键（与后端 TestNotificationSourceEvent 配对契约）', () => {
    // 🔴 必须先剥注释再抽键：源码里那大段说明性注释本身引用了这些键名，
    //    不剥的话正则会把注释里的键也当成数组项（顺序对不上则红得莫名其妙）。
    const blk = stripComments(read(LIB)).match(/DESKTOP_NOTIFY_SOURCES[^=]*=\s*\[([\s\S]*?)\]/)
    if (!blk) throw new Error('DESKTOP_NOTIFY_SOURCES not found')
    const keys = [...blk[1].matchAll(/'([^']+)'/g)].map((m) => m[1])
    expect(keys).toEqual([
      'task_status_changed.done',
      'task_status_changed.failed',
      'todo_hard_deadline_failed',
      'todo_awaiting_review',
      'todo_ask_received',
      'task_plan_ready',
      'todo_rework_exhausted',
      'todo_remind_escalated',
      'planning_reply.needs_input',
    ])
  })

  it('类型声明补上了 source_event（否则渲染端拿不到判据）', () => {
    expect(read(LIB)).toMatch(/source_event\?: string/)
  })

  it('🔴 白名单每个键的基类型都能在后端 switch 里找到（跨仓契约机械化）', () => {
    // 前后端是两套语言、两个仓库，白名单写错一个字**不会**有任何编译期或运行期报错，
    // 只会静默不弹。所以这里直接把后端 Go 源码当数据源对一遍：
    // 白名单 `a.b` 的基类型 `a` 必须出现在 `case "a":` 中。
    const go = read(path.join(ROOT, 'backend/internal/store/store_notification_internal.go'))
    const goCases = new Set([...go.matchAll(/\n\t*case "([^"]+)":/g)].map((m) => m[1]))
    expect(goCases.size).toBeGreaterThan(5) // 正则不匹配时不要静默通过
    const keys = [...stripComments(read(LIB)).matchAll(/'([a-z_]+(?:\.[a-z]+)?)'/g)]
      .map((m) => m[1])
      .filter((k) => k.includes('_'))
    expect(keys.length).toBeGreaterThan(0)
    for (const k of keys) {
      const base = k.split('.')[0]
      expect(goCases.has(base), `${k} 的基类型 "${base}" 在后端 switch 里不存在`).toBe(true)
    }
  })
})
