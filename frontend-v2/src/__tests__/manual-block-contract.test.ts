/// <reference types="node" />
// 契约测试要读源码做断言，因此显式引入 node 类型。
// 不能靠 `types` 白名单：tsconfig.app.json 刻意只放 `vite/client`，把 node 全局
// 变量挡在业务代码之外（否则 `process` / `Buffer` 会被误用而不报错）。
import fs from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

// 平台操作手册「块类型契约」守卫。
//
// 🔴 为什么需要这个测试：块类型是**跨语言契约** —— 后端
// store_platform_manual.go 的 manualBlockTypeAllowed 决定能存什么，
// 前端 ManualBlocks.tsx 的 switch 决定能渲染什么。两边漂移的后果是
// **静默的**：后端放行了一个新类型而前端没实现，管理员能保存成功，
// 但用户看到的是「内容凭空消失」（渲染器 default 分支返回 null）；
// 反过来前端支持了后端不认的类型，保存直接 400。
//
// 两侧编译器都不报错，只有真实点开手册才会发现。因此在这里用
// 「读源码 + 比对白名单」的方式把契约钉死，任何一边新增类型而另一边
// 没跟上，CI 立刻红。

const ROOT = path.resolve(__dirname, '../../..')
const BE = path.join(ROOT, 'backend/internal/store/store_platform_manual.go')
const FE = path.join(ROOT, 'frontend-v2/src/pages/manual/ManualBlocks.tsx')

/** 从后端 Go 源码里抠出 manualBlockTypeAllowed 的 case 列表。 */
function backendTypes(): string[] {
  const src = fs.readFileSync(BE, 'utf8')
  const fn = src.match(/func manualBlockTypeAllowed\(t string\) bool \{[\s\S]*?\n\}/)
  if (!fn) throw new Error('manualBlockTypeAllowed not found in backend source')
  const cases = [...fn[0].matchAll(/"([a-z_]+)"/g)].map((m) => m[1])
  return [...new Set(cases)].sort()
}

/** 从前端渲染器的 switch 里抠出 case 列表（排除 default）。 */
function frontendTypes(): string[] {
  const src = fs.readFileSync(FE, 'utf8')
  const fn = src.match(/export function ManualBlockView[\s\S]*?\n\}/)
  if (!fn) throw new Error('ManualBlockView not found in frontend source')
  const cases = [...fn[0].matchAll(/case '([a-z_]+)':/g)].map((m) => m[1])
  return [...new Set(cases)].sort()
}

describe('平台操作手册 · 块类型前后端契约', () => {
  it('后端白名单与前端渲染分支完全一致', () => {
    const be = backendTypes()
    const fe = frontendTypes()
    expect(be.length).toBeGreaterThan(0)
    expect(fe).toEqual(be)
  })

  it('覆盖已知的 9 种类型（防止任一侧被误删后测试仍「通过」）', () => {
    const expected = ['flow', 'heading', 'image', 'paragraph', 'qa', 'steps', 'table', 'tip', 'warn'].sort()
    expect(backendTypes()).toEqual(expected)
    expect(frontendTypes()).toEqual(expected)
  })
})

// 前端 API 客户端里的 ManualBlockType 联合类型也必须与上述一致，
// 否则编辑器的类型下拉框会漏掉某些类型。
describe('平台操作手册 · 前端类型声明', () => {
  it('ManualBlockType 联合类型覆盖全部 9 种', () => {
    const src = fs.readFileSync(path.join(ROOT, 'frontend-v2/src/api/platformManual.ts'), 'utf8')
    const decl = src.match(/export type ManualBlockType =[\s\S]*?\n\n/)
    if (!decl) throw new Error('ManualBlockType not found')
    const members = [...decl[0].matchAll(/'([a-z_]+)'/g)].map((m) => m[1]).sort()
    expect(members).toEqual(backendTypes())
  })
})

// 平台操作手册 · 读接口按身份分流（2026-09-21 线上事故回归）
//
// 🔴 事故经过：ManualPage 曾对所有身份一律用 getManual（GET /manual）。
// 本部署设了 PLATFORM_ADMIN_EMAILS（种子模式），后端**刻意反向拒绝**平台管理员
// 访问业务 API：
//   403 "platform admin account cannot access business APIs; use /api/v1/platform/*"
// ⇒ 平台管理员一打开手册页就 403，界面显示「手册加载失败」。
// 后端两条路都是对的（防越权设计），错在前端没分流。
//
// 反向也要守住：普通用户走 /platform/manual 会 403 "platform admin only"。
describe('平台操作手册 · 读接口按身份分流', () => {
  const PAGE = path.join(ROOT, 'frontend-v2/src/pages/manual/ManualPage.tsx')

  it('ManualPage 同时持有两个读接口，并按 isPlatformAdmin 分流', () => {
    const src = fs.readFileSync(PAGE, 'utf8')
    // 两个函数都要 import
    expect(src).toMatch(/import\s*\{[^}]*\bgetManual\b[^}]*\}\s*from\s*'@\/api\/platformManual'/)
    expect(src).toMatch(/import\s*\{[^}]*\bgetPlatformManual\b[^}]*\}\s*from\s*'@\/api\/platformManual'/)
    // queryFn 必须是三元分流，不能写死成 getManual
    expect(src).toMatch(/queryFn:\s*isPlatformAdmin\s*\?\s*getPlatformManual\s*:\s*getManual/)
  })

  it('queryKey 必须带身份区分，避免两个身份共用缓存', () => {
    const src = fs.readFileSync(PAGE, 'utf8')
    expect(src).toMatch(/queryKey:\s*\['manual',\s*isPlatformAdmin\s*\?\s*'platform'\s*:\s*'user'\]/)
  })

  it('两个读接口的 URL 前缀截然不同（getManual=业务、getPlatformManual=platform）', () => {
    const src = fs.readFileSync(path.join(ROOT, 'frontend-v2/src/api/platformManual.ts'), 'utf8')
    const g = src.match(/export async function getManual\(\)[\s\S]*?\n\}/)
    const gp = src.match(/export async function getPlatformManual\(\)[\s\S]*?\n\}/)
    if (!g || !gp) throw new Error('manual api fns not found')
    expect(g[0]).toMatch(/\.get\('manual'\)/)
    expect(g[0]).not.toMatch(/\.get\('platform\/manual'\)/)
    expect(gp[0]).toMatch(/\.get\('platform\/manual'\)/)
  })
})

// 平台操作手册 · 配图地址必须按运行环境解析（2026-09-22「桌面版看手册图片不显示」回归）
//
// 🔴 事故经过：后端入库的配图地址是**根相对路径** `/api/v1/manual/images/{id}`。
//   · Web 端页面 origin 就是服务端 ⇒ `/<path>` 解析正确，看起来一切正常；
//   · 桌面端页面由主进程 `win.loadFile(dist-v2/index.html)` 以 **file://** 打开
//     ⇒ 根相对路径被解析成 `file:///api/v1/manual/images/{id}` ⇒ 图片**全部不显示**。
// 修法：`ImageBlock` 的 src 必须过 `resolveAssetUrl()`（按 getApiBase() 的 origin 补全）。
// 这里把「必须过解析器」钉死，任何回退成 `src={d.url}` 都会让本测试立刻红。
describe('平台操作手册 · 配图地址按运行环境解析', () => {
  it('ImageBlock 的 src 必须经 resolveAssetUrl（禁止直接喂 d.url）', () => {
    const fn = fs.readFileSync(FE, 'utf8').match(/function ImageBlock\([\s\S]*?\n\}/)
    if (!fn) throw new Error('ImageBlock not found in frontend source')
    expect(fn[0]).toMatch(/src=\{resolveAssetUrl\(d\.url\)\}/)
    expect(fn[0]).not.toMatch(/src=\{d\.url\}/)
  })

  it('后端存的确实是根相对路径（哪天改成绝对地址这条会红，提醒重新审视本修法）', () => {
    const be = fs.readFileSync(BE, 'utf8')
    expect(be).toMatch(/URL:\s*"\/api\/v1\/manual\/images\/"\s*\+\s*id/)
  })
})
