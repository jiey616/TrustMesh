/// <reference types="node" />
// 契约测试要读源码做断言，因此显式引入 node 类型（见 manual-block-contract.test.ts 的说明）。
import fs from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

// 桌面端「信任自签名 / 私有 CA 证书」默认开启（2026-09-22 用户报「登录不上去」）
//
// 🔴 事故形态：新装桌面端默认不信任自签证书 ⇒ 渲染进程 fetch 拿到笼统的
//   `Failed to fetch`、主进程探针拿到 `ERR_CERT_AUTHORITY_INVALID`
//   ⇒ **登录不上去**，而修复入口藏在「服务器设置」的一个复选框里。
//
// 这个契约钉住三件事（任何一条被回退，CI 立刻红）：
//   1. 主进程两处默认值都是 true（`let desktopConfig` 与 `loadDesktopConfig` 的兜底）；
//   2. 渲染端 store 的初值走 `defaultTrustInsecureTls()`（桌面 true / Web false），
//      且 persist 版本 ≥ 3（v3 才带 `trustInsecureTlsExplicit` 语义）；
//   3. `probeServer` 不再引用不存在的 `window_clear`（会让「测试」按钮永久转圈）。

const ROOT = path.resolve(__dirname, '../../..')
const MAIN = path.join(ROOT, 'frontend-v2/electron/main.cjs')
const STORE = path.join(ROOT, 'frontend-v2/src/stores/serverConfigStore.ts')
const PROBE = path.join(ROOT, 'frontend-v2/src/lib/serverProbe.ts')

describe('桌面端 TLS 信任默认值 · 主进程（权威执行方）', () => {
  it('模块级默认值是 true', () => {
    expect(fs.readFileSync(MAIN, 'utf8')).toMatch(
      /let desktopConfig = \{ trustInsecureTls: true \}/,
    )
  })

  it('配置文件缺失 / 缺字段时兜底也是 true（不能退回旧的 false）', () => {
    const src = fs.readFileSync(MAIN, 'utf8')
    expect(src).toMatch(/desktopConfig = \{ trustInsecureTls: true, \.\.\.JSON\.parse\(raw\) \}/)
    // 旧的默认值写法一旦回来，说明有人把默认又关掉了
    expect(src).not.toMatch(/let desktopConfig = \{ trustInsecureTls: false \}/)
  })

  it('certificate-error 仍按开关决定放行 / 拒绝', () => {
    const src = fs.readFileSync(MAIN, 'utf8')
    const blk = src.match(/app\.on\('certificate-error'[\s\S]*?\n\}\)/)
    if (!blk) throw new Error("app.on('certificate-error') not found")
    expect(blk[0]).toMatch(/if \(desktopConfig\.trustInsecureTls\) \{/)
    expect(blk[0]).toMatch(/callback\(true\)/)
    expect(blk[0]).toMatch(/callback\(false\)/)
  })
})

describe('桌面端 TLS 信任默认值 · 渲染端 store', () => {
  it('初值来自 defaultTrustInsecureTls()（桌面 true / Web false），不是写死的 false', () => {
    const src = fs.readFileSync(STORE, 'utf8')
    expect(src).toMatch(/trustInsecureTls: defaultTrustInsecureTls\(\),/)
    expect(src).not.toMatch(/trustInsecureTls: false,\n\s+setServerUrl/)
  })

  it('persist 版本 ≥ 3（v3 才携带「用户是否显式选择」语义）', () => {
    const src = fs.readFileSync(STORE, 'utf8')
    expect(src).toMatch(/name: 'trustmesh-server-config',\s*\n\s*version: 3,/)
  })

  it('默认值实现按运行环境返回（桌面 true / Web false）', () => {
    const src = fs.readFileSync(STORE, 'utf8')
    const fn = src.match(/export function defaultTrustInsecureTls\([\s\S]*?\n\}/)
    if (!fn) throw new Error('defaultTrustInsecureTls not found')
    expect(fn[0]).toMatch(/return isDesktop/)
  })
})

describe('设置页探针 · 不再引用未定义标识符', () => {
  /** 去掉注释后再断言：注释里为了记录来由会提到那个坏名字，不该算命中。 */
  function stripComments(src: string): string {
    return src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '')
  }

  it('probeServer 的 net.request 异常分支用 clearTimer()，不是那个不存在的 window_clear()', () => {
    const src = fs.readFileSync(MAIN, 'utf8')
    expect(stripComments(src)).not.toMatch(/\bwindow_clear\b/)
    // 一直匹配到 catch 块的收尾（4 空格 + `}`），中间不能被 try 自己的 `} catch` 截断
    const blk = src.match(
      /req = net\.request\(\{ method: 'GET', url: target \}\)[\s\S]*?catch \(err\) \{[\s\S]*?\n {4}\}/,
    )
    if (!blk) throw new Error('net.request catch block not found')
    expect(blk[0]).toMatch(/clearTimer\(\)/)
  })

  it('证书类错误的文案指向「已默认开启」，而不是让用户去勾一个可能早就勾上的框', () => {
    const src = fs.readFileSync(PROBE, 'utf8')
    expect(src).toMatch(/信任自签名 \/ 私有 CA 证书/)
    expect(src).toMatch(/默认已开启/)
  })
})
