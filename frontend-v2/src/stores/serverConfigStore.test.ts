// 「信任自签名 / 私有 CA 证书」默认值与迁移语义（2026-09-22）
//
// 🔴 背景：桌面端默认 false 时，新装客户端**连登录接口都打不通**
//   （渲染进程只拿到笼统的 `Failed to fetch`），用户不知道要去服务器设置里勾选。
//   改为默认 true 之后，最容易写错的两点必须有测试钉住：
//     1. **Web 端不能跟着变 true**（浏览器证书策略不受页面控制，勾了没用）；
//     2. **用户主动关闭的选择必须保留**（不能因为改默认值就把人家的收紧动作重置）。
//
// ⚠️ 这里只测纯函数 + 可注入的 isDesktop。**不要**在 jsdom 里伪造 `window.process`
//    来假装 Electron —— 那会连带打断 vitest 自身（09-19 移动端踩过同一个坑）。
import { beforeEach, describe, expect, it } from 'vitest'
import {
  defaultTrustInsecureTls,
  migrateServerConfig,
  normalizeServerUrl,
  useServerConfigStore,
} from './serverConfigStore'

describe('defaultTrustInsecureTls · 默认值按运行环境分流', () => {
  it('桌面端默认信任（自签后端否则登录必失败）', () => {
    expect(defaultTrustInsecureTls(true)).toBe(true)
  })

  it('Web 端默认不信任（浏览器证书策略不受页面控制）', () => {
    expect(defaultTrustInsecureTls(false)).toBe(false)
  })

  it('默认参数 = 当前运行环境；本单测跑在 jsdom 里 ⇒ 非桌面 ⇒ false', () => {
    expect(defaultTrustInsecureTls()).toBe(false)
  })
})

describe('migrateServerConfig · v2 → v3 迁移', () => {
  it('旧记录里的 false 是「旧默认值」而非用户选择 ⇒ 桌面端改用新默认 true', () => {
    const out = migrateServerConfig({ serverUrl: 'https://10.0.0.1', trustInsecureTls: false }, true)
    expect(out.trustInsecureTls).toBe(true)
    expect(out.trustInsecureTlsExplicit).toBe(false)
  })

  it('同一份旧记录在 Web 端仍是 false', () => {
    const out = migrateServerConfig({ serverUrl: null, trustInsecureTls: false }, false)
    expect(out.trustInsecureTls).toBe(false)
  })

  it('🔴 用户显式关闭过 ⇒ 桌面端也必须保持 false（不能被默认值覆盖）', () => {
    const out = migrateServerConfig(
      { serverUrl: null, trustInsecureTls: false, trustInsecureTlsExplicit: true },
      true,
    )
    expect(out.trustInsecureTls).toBe(false)
    expect(out.trustInsecureTlsExplicit).toBe(true)
  })

  it('用户显式开启过 ⇒ Web 端也保持 true（不拿环境去覆盖用户选择）', () => {
    const out = migrateServerConfig(
      { serverUrl: null, trustInsecureTls: true, trustInsecureTlsExplicit: true },
      false,
    )
    expect(out.trustInsecureTls).toBe(true)
  })

  it('空 / undefined 持久化数据不抛异常，落到默认值', () => {
    expect(migrateServerConfig(undefined, true)).toEqual({
      serverUrl: null,
      trustInsecureTls: true,
      trustInsecureTlsExplicit: false,
    })
    expect(migrateServerConfig(null, false)).toEqual({
      serverUrl: null,
      trustInsecureTls: false,
      trustInsecureTlsExplicit: false,
    })
  })

  it('serverUrl 脏数据仍照旧清洗（空串 / 相对路径 → null，带 /api/v1 后缀 → 去掉）', () => {
    expect(migrateServerConfig({ serverUrl: '' }, true).serverUrl).toBe(null)
    expect(migrateServerConfig({ serverUrl: '/api/v1' }, true).serverUrl).toBe(
      normalizeServerUrl('/api/v1'),
    )
    expect(
      migrateServerConfig({ serverUrl: 'https://x.example:8443/api/v1/' }, true).serverUrl,
    ).toBe('https://x.example:8443')
  })
})

describe('useServerConfigStore · 开关置为「显式选择」', () => {
  beforeEach(() => {
    // store 是模块单例，逐例复位到「未显式选择」状态
    useServerConfigStore.setState({ trustInsecureTls: false, trustInsecureTlsExplicit: false })
  })

  it('关闭开关 ⇒ 值变 false 且被标记为显式', () => {
    useServerConfigStore.getState().setTrustInsecureTls(false)
    const s = useServerConfigStore.getState()
    expect(s.trustInsecureTls).toBe(false)
    expect(s.trustInsecureTlsExplicit).toBe(true)
  })

  it('开启开关 ⇒ 值变 true 且被标记为显式', () => {
    useServerConfigStore.getState().setTrustInsecureTls(true)
    const s = useServerConfigStore.getState()
    expect(s.trustInsecureTls).toBe(true)
    expect(s.trustInsecureTlsExplicit).toBe(true)
  })

  it('非桌面环境调用不抛异常（window.desktop 不存在时静默跳过）', () => {
    expect(() => useServerConfigStore.getState().setTrustInsecureTls(false)).not.toThrow()
  })
})
