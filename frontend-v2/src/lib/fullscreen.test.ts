import { describe, expect, it } from 'vitest'

import { isElementFullscreen } from '@/lib/fullscreen'
// 用 Vite 的 `?raw` 取源码文本（tsconfig 的 types 只含 vite/client，没有 node 类型，
// 故不引入 node:fs / process；该导入在单测与 tsc 下均可用）。
import componentSource from '@/components/external/ExternalAppFrame.tsx?raw'

/**
 * 回归护栏：首次进入外部应用时按钮误显示「退出全屏」。
 *
 * 根因是 `null === null` 这个 JavaScript 陷阱：外部应用在拿到 launch_url 之前渲染的是
 * `<Spin>` 分支，容器 `stageRef` 尚未挂载（`current === null`），而此刻
 * `document.fullscreenElement` 也是 `null`，于是
 * `document.fullscreenElement === stageRef.current` 求值为 **true**，
 * 组件误判自己已在全屏。
 *
 * 这里测的是抽出来的纯函数（组件生产路径调用的就是它），而不是像素。
 */

describe('isElementFullscreen：容器未挂载时不得判定为全屏', () => {
  it('两侧同为 null（首次进入、容器未挂载）→ false（本 bug 的直接复现点）', () => {
    expect(isElementFullscreen(null, null)).toBe(false)
  })

  it('容器未挂载、但浏览器有别的全屏元素 → false', () => {
    const other = {} as Element
    expect(isElementFullscreen(null, other)).toBe(false)
  })

  it('容器已挂载且就是当前全屏元素 → true', () => {
    const el = {} as Element
    expect(isElementFullscreen(el, el)).toBe(true)
  })

  it('容器已挂载但不是当前全屏元素 → false', () => {
    const el = {} as Element
    const other = {} as Element
    expect(isElementFullscreen(el, other)).toBe(false)
  })

  it('容器已挂载、浏览器未全屏 → false', () => {
    const el = {} as Element
    expect(isElementFullscreen(el, null)).toBe(false)
  })
})

/**
 * 源码级契约：组件不得再内联那种「两侧都可能为 null 的相等比较」。
 * 一旦有人把 `=== stageRef.current` 写回去，本用例立刻变红。
 */
describe('ExternalAppFrame：全屏判定必须走 isElementFullscreen', () => {
  it('引用了 isElementFullscreen', () => {
    expect(componentSource).toContain("from '@/lib/fullscreen'")
    expect(componentSource).toContain('isElementFullscreen(')
  })

  it('不再直接拿 document.fullscreenElement 与 ref 比较', () => {
    expect(componentSource).not.toMatch(/document\.fullscreenElement\s*===\s*stageRef\.current/)
  })

  it('按钮文案随 isFullscreen 切换（默认「全屏」，全屏时「退出全屏」）', () => {
    expect(componentSource).toMatch(/isFullscreen\s*\?\s*'退出全屏'\s*:\s*'全屏'/)
  })
})
