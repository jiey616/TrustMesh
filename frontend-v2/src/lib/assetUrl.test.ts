import { describe, expect, it } from 'vitest'
import { resolveAssetUrl } from './assetUrl'

/** 生产手册里真实存在的配图地址形态（根相对，见 backend store_platform_manual.go）。 */
const MANUAL = '/api/v1/manual/images/0136cbf4a13a72524ad4cc33450cfd04'

// 桌面端 / Web 容器两种真实基址形态（分别对应 compose 与 electron-builder 的构建产物）。
const BASE_DESKTOP = 'https://175.27.135.91/api/v1/'
const BASE_WEB = '/api/v1/'

describe('resolveAssetUrl', () => {
  it('桌面端：把根相对的手册配图补成绝对地址（本次「桌面版看手册图片不显示」的修复点）', () => {
    expect(resolveAssetUrl(MANUAL, BASE_DESKTOP)).toBe(`https://175.27.135.91${MANUAL}`)
  })

  it('反例守卫：桌面端绝不能还是根相对（那会被解析成 file:///api/... ⇒ 必裂）', () => {
    const out = resolveAssetUrl(MANUAL, BASE_DESKTOP)
    expect(out.startsWith('/')).toBe(false)
    expect(out.startsWith('file://')).toBe(false)
    expect(out.startsWith('https://')).toBe(true)
  })

  it('Web（容器注入同源 /api/v1/）：保持根相对，行为与改动前完全一致', () => {
    expect(resolveAssetUrl(MANUAL, BASE_WEB)).toBe(MANUAL)
  })

  it('桌面端换服务端地址（设置页）：origin 跟着换', () => {
    expect(resolveAssetUrl(MANUAL, 'https://10.0.0.5:8443/api/v1/')).toBe(`https://10.0.0.5:8443${MANUAL}`)
  })

  it('基址带子路径也能正确取 origin（不留 /api/v1）', () => {
    expect(resolveAssetUrl(MANUAL, 'https://h.test/base/api/v1/')).toBe(`https://h.test/base${MANUAL}`)
  })

  it('绝对地址一律原样返回：手填外链 / data: / blob: / 协议相对', () => {
    const abs = [
      'https://cdn.example.com/a.png',
      'http://10.0.0.1/a.png',
      'data:image/png;base64,iVBORw0KGgo=',
      'blob:https://175.27.135.91/9a1b',
      '//cdn.example.com/a.png',
    ]
    for (const u of abs) expect(resolveAssetUrl(u, BASE_DESKTOP)).toBe(u)
  })

  it('无前导斜杠的相对地址也能补对，且不产生双斜杠', () => {
    expect(resolveAssetUrl('api/v1/x.png', BASE_WEB)).toBe('/api/v1/x.png')
    expect(resolveAssetUrl('api/v1/x.png', BASE_DESKTOP)).toBe('https://175.27.135.91/api/v1/x.png')
  })

  it('空值返回空串（ImageBlock 靠它早退，不渲染空 img）', () => {
    expect(resolveAssetUrl('', BASE_DESKTOP)).toBe('')
    expect(resolveAssetUrl('   ', BASE_DESKTOP)).toBe('')
    expect(resolveAssetUrl(undefined, BASE_DESKTOP)).toBe('')
    expect(resolveAssetUrl(null, BASE_DESKTOP)).toBe('')
  })

  it('默认参数取自运行环境（getApiBase）：相对地址必须被补成绝对，不会被原样返回', () => {
    // 单测环境下 vitest.config 把 VITE_API_BASE_URL 固定为绝对地址（模拟容器形态），
    // 故这里只做关系断言，不写死具体主机。
    const out = resolveAssetUrl(MANUAL)
    expect(out).not.toBe(MANUAL)
    expect(out.startsWith('/')).toBe(false)
    expect(out.endsWith(MANUAL)).toBe(true)
  })
})
