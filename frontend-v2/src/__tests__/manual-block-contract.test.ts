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
