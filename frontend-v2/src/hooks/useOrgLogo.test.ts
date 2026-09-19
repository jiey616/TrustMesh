import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { fetchOrganizationLogo } from '@/api/orgs'
import { useAuthStore } from '@/stores/authStore'
// 源码级契约（与 lib/fullscreen.test.ts 同法）：tsconfig 的 types 只含 vite/client，
// 没有 node 类型，故用 Vite 的 `?raw` 取源码文本而不是 node:fs。
import apiSource from '@/api/orgs.ts?raw'
import orgLogoSrc from '@/components/shared/OrgLogo.tsx?raw'
import hookSource from '@/hooks/useOrgLogo.ts?raw'
import layoutSource from '@/layouts/MainLayout.tsx?raw'
import orgSettingsSource from '@/pages/OrgSettingsPage.tsx?raw'

/**
 * 回归护栏：组织 logo「提示已更新，但图标不显示」。
 *
 * 根因（已实证，非推测）：`GET /api/v1/organizations/:id/logo` 注册在**鉴权路由组**上
 * （router.go 的 `orgs` 组挂了 middleware.RequireAuth），必须带 `Authorization` 头；
 * 而 `<img src>` / antd `<Avatar src>` 走浏览器原生图片加载，**无法附加请求头** ⇒
 * 浏览器发出的那个 GET 恒 401：
 *     curl 无头  → 401 {"error":{"code":"UNAUTHORIZED",...}}
 *     curl 带头  → 200 bytes=13502 ctype=image/png
 * 于是上传接口 200、Mongo 里 logo_file_id 也写入了，但界面永远看不到图。
 *
 * 修法：改由 apiClient 取回 blob → URL.createObjectURL → 交给 <img>。
 * 下面第一部分是**行为级**断言（真跑 ky + stub fetch，挡住「看起来对但请求头/缓存不对」），
 * 第二部分是**源码级**契约（挡住有人又把 logo_url 直接塞回 src）。
 */

interface Captured {
  url: string
  method: string
  headers: Record<string, string>
  cache: string
}

let calls: Captured[]
let responder: () => Response

function pngResponse(): Response {
  return new Response(new Uint8Array([137, 80, 78, 71]), {
    status: 200,
    headers: { 'content-type': 'image/png' },
  })
}

beforeEach(() => {
  calls = []
  responder = pngResponse
  vi.stubGlobal('fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
    const req = input instanceof Request ? input : new Request(input as string, init)
    const headers: Record<string, string> = {}
    req.headers.forEach((value, key) => {
      headers[key.toLowerCase()] = value
    })
    calls.push({ url: req.url, method: req.method, headers, cache: req.cache })
    return responder()
  })
  useAuthStore.setState({
    accessToken: null,
    refreshToken: null,
    user: null,
    activeOrgId: null,
    personalOrgId: null,
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('fetchOrganizationLogo：必须带鉴权取字节（而不是让 <img> 裸请求）', () => {
  it('打到 /organizations/:id/logo，且带上 Authorization 与当前租户头', async () => {
    useAuthStore.setState({
      accessToken: 'tok-1',
      activeOrgId: 'org-enterprise',
      personalOrgId: 'org-personal',
    })

    await fetchOrganizationLogo('org-enterprise')

    expect(calls).toHaveLength(1)
    expect(calls[0].method).toBe('GET')
    expect(calls[0].url).toContain('/organizations/org-enterprise/logo')
    // 🔴 本 bug 的核心：没有这个头就是 401。断言它会随请求一起发出。
    expect(calls[0].headers['authorization']).toBe('Bearer tok-1')
    expect(calls[0].headers['x-org-id']).toBe('org-enterprise')
  })

  it('显式禁用浏览器 HTTP 缓存（后端 max-age=3600 且 URL 不随重传变化）', async () => {
    useAuthStore.setState({ accessToken: 'tok-1', personalOrgId: 'org-personal' })

    await fetchOrganizationLogo('org-1')

    // 不禁用缓存 ⇒ 重新上传后最长 1 小时仍拿旧字节，又是「上传了没生效」
    expect(calls[0].cache).toBe('no-store')
  })

  it('返回的是图片 blob（可直接 createObjectURL）', async () => {
    useAuthStore.setState({ accessToken: 'tok-1', personalOrgId: 'org-personal' })

    const blob = await fetchOrganizationLogo('org-1')

    // 注意：不要用 toBeInstanceOf(Blob) —— 单测环境里 Response 来自 Node(undici)，
    // 其 .blob() 产出的是 undici 的 Blob，与 jsdom 的全局 Blob 不是同一个 realm，
    // 断言必假红。这里改断言「可直接喂给 createObjectURL」的能力特征。
    expect(typeof blob.arrayBuffer).toBe('function')
    expect(blob.size).toBeGreaterThan(0)
    expect(blob.type).toBe('image/png')
  })
})

describe('object URL 生命周期：必须释放，避免 blob 常驻内存', () => {
  it('创建后依赖变化/卸载时 revoke 上一个 URL', () => {
    expect(hookSource).toContain('URL.createObjectURL(blob)')
    expect(hookSource).toContain('URL.revokeObjectURL(created)')
    // 清理函数必须真的被 effect 返回（return () => {...}），否则等于没释放
    expect(hookSource).toMatch(/return \(\) => \{\s*cancelled = true/)
  })

  it('重传后必须能重取：effect 依赖包含 refreshKey', () => {
    // 少了这一条，logo_url 不变 ⇒ effect 不重跑 ⇒ 上传成功却仍是旧图
    expect(hookSource).toMatch(/\}, \[key, orgId, refreshKey\]\)/)
  })

  it('返回值按 key 严格匹配（切组织/清空 logo 时不得沿用旧图）', () => {
    expect(hookSource).toContain("return entry?.key === key ? entry.url : undefined")
    expect(hookSource).toContain('`${orgId}|${logoUrl}`')
  })
})

describe('源码级契约：logo 只能经 OrgLogo 渲染，且不得裁切 / 不得有背景色', () => {
  it('API 层保留 no-store 的取字节入口', () => {
    expect(apiSource).toContain('export async function fetchOrganizationLogo')
    expect(apiSource).toContain("cache: 'no-store'")
  })

  it('MainLayout 品牌区（收起 + 展开两处）都走 OrgLogo 组件', () => {
    expect(layoutSource).toContain("from '@/components/shared/OrgLogo'")
    expect((layoutSource.match(/<OrgLogo/g) || []).length).toBe(2)
    // 一旦有人把 logo_url 塞回 src（= 线上 401 白图 bug），或绕过组件直连 hook，本用例立刻变红
    expect(layoutSource).not.toMatch(/src=\{activeOrg/)
    expect(layoutSource).not.toContain('useOrgLogoObjectUrl')
  })

  it('OrgSettingsPage 走 OrgLogo，并在重传后触发重取', () => {
    expect(orgSettingsSource).toContain("from '@/components/shared/OrgLogo'")
    expect(orgSettingsSource).toMatch(/<OrgLogo[\s\S]{0,200}refreshKey=\{logoRefreshKey\}/)
    expect(orgSettingsSource).toContain('setLogoRefreshKey((k) => k + 1)')
    expect(orgSettingsSource).not.toMatch(/src=\{org\.logo_url\}/)
  })

  it('OrgLogo：有 logo 时渲染原生 <img>，contain 完整显示、无圆角、无背景色', () => {
    // 方形 logo 被圆形裁掉 = 用户报的「显示不全」；根因是 antd Avatar 的 50% 圆角
    expect(orgLogoSrc).toContain("objectFit: 'contain'")
    expect(orgLogoSrc).toContain('borderRadius: 0')
    expect(orgLogoSrc).toContain("background: 'transparent'")
    // 🔴 占位 Avatar 绝不能接 src —— 那正是「圆形紫底 + 被裁 logo」的老写法
    expect(orgLogoSrc).toMatch(/if \(!configured\) \{[\s\S]*?<Avatar/)
    expect(orgLogoSrc).not.toMatch(/<Avatar[\s\S]{0,200}?src=/)
    // 有 logo 的分支必须是 <img>（且带 contain 样式）
    expect(orgLogoSrc).toMatch(/<img[\s\S]*?objectFit: 'contain'/)
  })

  it('OrgLogo 只在未设置 logo 时才用圆形占位（不再拿它当 logo 容器）', () => {
    expect(orgLogoSrc).toContain('const configured = !!logoUrl')
    // 圆形占位（Avatar）出现次数 = 1，且位于 !configured 分支内
    expect((orgLogoSrc.match(/<Avatar/g) || []).length).toBe(1)
  })
})
