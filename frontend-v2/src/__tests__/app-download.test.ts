import { afterAll, describe, expect, it, vi } from 'vitest'
import { fetchLatestMobileRelease, normalizeApiPath, parseLatestYML } from '@/lib/appDownload'

const API_BASE = '/api/v1/'

// 与后端 handler.RenderLatestYML 的输出逐字段对齐（带双引号的 YAML 安全子集）
const SAMPLE = [
  'version: "0.2.3"',
  'files:',
  '  - url: "TrustMesh-Setup-0.2.3.exe"',
  '    sha512: "abc123=="',
  '    size: 91234567',
  'path: "TrustMesh-Setup-0.2.3.exe"',
  'sha512: "abc123=="',
  'releaseDate: "2026-09-20T10:00:00.000Z"',
  '',
].join('\n')

describe('parseLatestYML', () => {
  it('解析 version 与 path 并拼出下载地址', () => {
    const info = parseLatestYML(SAMPLE, API_BASE)
    expect(info).not.toBeNull()
    expect(info?.version).toBe('0.2.3')
    expect(info?.fileName).toBe('TrustMesh-Setup-0.2.3.exe')
    expect(info?.downloadUrl).toBe('/api/v1/desktop/releases/feed/TrustMesh-Setup-0.2.3.exe')
  })

  it('下载地址对文件名做 URI 编码', () => {
    const info = parseLatestYML(
      'version: "1.0.0"\npath: "TrustMesh Setup 1.0.0.exe"\n',
      API_BASE,
    )
    expect(info?.downloadUrl).toBe('/api/v1/desktop/releases/feed/TrustMesh%20Setup%201.0.0.exe')
  })

  it('缺 version 返回 null（隐藏下载入口）', () => {
    expect(parseLatestYML('path: "x.exe"\n', API_BASE)).toBeNull()
  })

  it('缺 path 返回 null', () => {
    expect(parseLatestYML('version: "1.0.0"\n', API_BASE)).toBeNull()
  })

  it('空文本返回 null', () => {
    expect(parseLatestYML('', API_BASE)).toBeNull()
  })
})

// 移动端安装包 meta（公开端点 /mobile/app/latest）：JSON 解析 + 绝对下载地址拼装。
describe('fetchLatestMobileRelease', () => {
  afterAll(() => {
    vi.unstubAllGlobals()
  })

  it('解析 meta 并拼出绝对下载地址', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            data: {
              mobile_app: {
                version: '1.0.0',
                file_name: 'TrustMesh-1.0.0.apk',
                download_path: '/api/v1/mobile/app/download',
              },
            },
          }),
          { status: 200 },
        ),
      ),
    )
    const info = await fetchLatestMobileRelease()
    expect(info?.version).toBe('1.0.0')
    expect(info?.fileName).toBe('TrustMesh-1.0.0.apk')
    // 🔴 必须断言「完整路径且只有一层 /api/v1」——2026-09-21 事故：
    //    旧断言用 /\/api\/v1\/mobile\/app\/download$/（未锚定开头），
    //    于是 `.../api/v1/api/v1/mobile/app/download` 也能“匹配”通过，
    //    扫码 404 的 bug 就这样逃过了单测。这里改为锚定整个 path。
    expect(info?.downloadUrl).toMatch(/^https?:\/\/[^/]+\/api\/v1\/mobile\/app\/download$/)
    expect(info?.downloadUrl).not.toMatch(/\/api\/v1\/api\/v1\//)
  })

  it('normalizeApiPath：带/不带 /api/v1 前缀都收敛成唯一一层', () => {
    expect(normalizeApiPath('/api/v1/mobile/app/download')).toBe('/api/v1/mobile/app/download')
    expect(normalizeApiPath('/mobile/app/download')).toBe('/api/v1/mobile/app/download')
    expect(normalizeApiPath('mobile/app/download')).toBe('/api/v1/mobile/app/download')
    expect(normalizeApiPath('/api/v1/api/v1/mobile/app/download')).toBe('/api/v1/mobile/app/download')
  })

  it('无包（mobile_app:null）返回 null，扫码入口隐藏', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(new Response(JSON.stringify({ data: { mobile_app: null } }), { status: 200 })),
    )
    await expect(fetchLatestMobileRelease()).resolves.toBeNull()
  })

  it('HTTP 错误返回 null（不抛出）', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('not found', { status: 404 })))
    await expect(fetchLatestMobileRelease()).resolves.toBeNull()
  })
})
