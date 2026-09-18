import { describe, expect, it } from 'vitest'
import { validateUploadPair } from '@/lib/desktopRelease'
import pageSource from '@/pages/platform/DesktopReleasesPage.tsx?raw'

const meta = {
  version: '0.3.0',
  file: 'TrustMesh-Setup-0.3.0.exe',
  sha512: 'U29tZUJhc2U2NARpZ2VzdA==',
}

describe('validateUploadPair（上传前的安装包/元数据配对校验）', () => {
  it('版本号与文件名一致时放行', () => {
    expect(validateUploadPair('TrustMesh-Setup-0.3.0.exe', meta)).toEqual({ ok: true })
  })

  it('缺 version 时拒绝', () => {
    const r = validateUploadPair('TrustMesh-Setup-0.3.0.exe', { ...meta, version: '' })
    expect(r.ok).toBe(false)
    expect(r.reason).toContain('version')
  })

  it('缺 sha512 时拒绝', () => {
    const r = validateUploadPair('TrustMesh-Setup-0.3.0.exe', { ...meta, sha512: '' })
    expect(r.ok).toBe(false)
    expect(r.reason).toContain('sha512')
  })

  // 最重的那个坑：元数据是 0.4.0、包是 0.3.0。发布出去 = 全体客户端装不上。
  it('release.json 的 file 与所选安装包不一致时拒绝', () => {
    const r = validateUploadPair('TrustMesh-Setup-0.3.0.exe', {
      ...meta,
      version: '0.4.0',
      file: 'TrustMesh-Setup-0.4.0.exe',
    })
    expect(r.ok).toBe(false)
    expect(r.reason).toContain('TrustMesh-Setup-0.4.0.exe')
    expect(r.reason).toContain('TrustMesh-Setup-0.3.0.exe')
  })

  it('file 字段缺失时退回「文件名必须含版本号」这条更弱的检查', () => {
    const r = validateUploadPair('TrustMesh-Setup-0.3.0.exe', { ...meta, file: '' })
    expect(r.ok).toBe(true)
    const bad = validateUploadPair('TrustMesh-Setup-0.2.0.exe', { ...meta, file: '' })
    expect(bad.ok).toBe(false)
    expect(bad.reason).toContain('0.3.0')
  })

  it('文件名不含版本号时拒绝（例如改了名或选错了包）', () => {
    // 必须把 file 置空才能走到这条分支：file 不一致是**更具体**的错误，
    // 会先命中（见上一条用例）—— 分支顺序刻意按「错误信息由具体到宽泛」排列。
    const r = validateUploadPair('setup.exe', { ...meta, file: '' })
    expect(r.ok).toBe(false)
    expect(r.reason).toContain('不含版本号')
  })
})

// 源码级契约（本仓不引 @testing-library，见 MEMORY 的测试策略）：
// 页面必须**委托**给上面这个被单测覆盖的纯函数，而不是就地再写一份没被覆盖的
// 校验副本 —— 那种副本一旦与纯函数漂移，测试不会红，而上线后果是全体客户端装不上。
describe('DesktopReleasesPage 的契约', () => {
  it('上传前调用 validateUploadPair，不内联重复实现', () => {
    expect(pageSource).toContain('validateUploadPair(')
    // 内联副本的标志：页面里直接做 file/version 的相等与包含判断。
    expect(pageSource).not.toContain('meta.file !== installer.name')
    expect(pageSource).not.toContain('installer.name.includes(meta.version)')
  })

  it('上传期间渲染进度条（87MB 无进度等于无可用性）', () => {
    expect(pageSource).toContain('<Progress')
  })
})
