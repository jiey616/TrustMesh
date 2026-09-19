import { useEffect, useState } from 'react'
import { fetchOrganizationLogo } from '@/api/orgs'

/**
 * 组织 logo 的「浏览器可直接用」URL（object URL）。
 *
 * 为什么不能把 `org.logo_url` 直接塞给 `<img src>`：
 *   后端 `GET /api/v1/organizations/:id/logo` 挂在**鉴权路由组**上，必须携带
 *   `Authorization: Bearer <token>`（middleware.RequireAuth）。而 `<img>` / antd
 *   `<Avatar src>` 走浏览器原生图片加载，**无法附加请求头** ⇒ 必然 401，
 *   现象就是「上传提示已更新，但图标不显示」。
 *   这里用 apiClient 取回字节（自动带 Authorization + X-Org-Id，且复用 401 刷新重放），
 *   再转成 object URL 交给 `<img>`。
 *
 * 生命周期：每次重取都新建 object URL，并在依赖变化 / 卸载时 **revoke 上一个**，
 *   避免 blob 常驻内存（与 useChatAttachments 等处的既有约定一致）。
 *
 * 实现注意：**不在 effect 体内同步 setState**（react-hooks/set-state-in-effect：
 *   同步 setState 会触发级联渲染）。改用「状态里带 key、返回值按 key 派生」的写法——
 *   无 logo / 切换组织时不需要主动清空，返回的 `undefined` 自然让 Avatar 回落首字母占位。
 */

/** 已取回的 logo：key 绑定「哪个组织的哪个 logo_url」，防止切组织时串号。 */
interface LogoEntry {
  key: string
  url: string
}

/**
 * @param orgId      组织 ID；为空则不请求。
 * @param logoUrl    后端返回的 logo 地址；为空串 = 该组织未设置 logo → 返回 undefined，
 *                   由调用方回落首字母占位（**不可保留上一次的值**：切到无 logo 的组织时
 *                   保留旧图会串号，故返回值按 key 严格匹配）。
 * @param refreshKey 重新上传后自增的 nonce。logo_url 不随重复上传变化，靠它触发 effect 重跑，
 *                   否则设置页上传成功后预览仍是旧图（同一会话内不刷新页面就看不到新图）。
 */
export function useOrgLogoObjectUrl(
  orgId?: string | null,
  logoUrl?: string | null,
  refreshKey = 0,
): string | undefined {
  const [entry, setEntry] = useState<LogoEntry | undefined>(undefined)

  // key 为空串 = 该组织没有 logo（或 orgId 未知）→ 不请求，且派生结果必为 undefined。
  const key = orgId && logoUrl ? `${orgId}|${logoUrl}` : ''

  useEffect(() => {
    if (!key || !orgId) return

    let cancelled = false
    let created: string | undefined

    fetchOrganizationLogo(orgId)
      .then((blob) => {
        // 依赖已变（或已卸载）：本次结果作废，别建 object URL 造成泄漏。
        if (cancelled) return
        created = URL.createObjectURL(blob)
        setEntry({ key, url: created })
      })
      .catch(() => {
        // 失败（401/网络/文件缺失）→ 交回 undefined，由 Avatar 回落首字母占位。
        if (!cancelled) setEntry(undefined)
      })

    return () => {
      cancelled = true
      if (created) URL.revokeObjectURL(created)
    }
  }, [key, orgId, refreshKey])

  // 严格按 key 匹配：key 变了（换组织 / logo 被清空）→ 立即不返回旧图，无需在 effect 里清状态。
  return entry?.key === key ? entry.url : undefined
}
