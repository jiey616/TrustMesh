import { Avatar } from 'antd'

import { useOrgLogoObjectUrl } from '@/hooks/useOrgLogo'

/**
 * 组织 logo 展示块。
 *
 * 🔴 刻意**不**用 antd Avatar 的 `src` 来显示 logo：Avatar 默认 `shape="circle"`
 *   （`border-radius: 50%` + `overflow: hidden`），会把方形 logo 的四角裁掉；
 *   再叠上我们原先给的渐变背景色，就变成「圆形紫底 + 被裁的 logo」——
 *   即用户反馈的「logo 显示不全 + 有背景颜色」。
 *   有 logo 时一律渲染原生 `<img>`：`object-fit: contain`（等比完整装进方框、不裁剪）、
 *   **无圆角、无背景色**，logo 是什么就是什么。
 *
 * 仅当该组织**没有设置 logo** 时才回落首字母圆形占位（保持既有观感；
 * 它是「无 logo」的占位，不是 logo 的容器）。
 *
 * `logoUrl` 传的是后端 `org.logo_url`（鉴权接口地址），本组件内部经
 * `useOrgLogoObjectUrl` 换取 object URL；调用方不必自己取 blob。
 */
export function OrgLogo({
  orgId,
  logoUrl,
  fallbackText,
  size,
  refreshKey,
}: {
  /** 组织 ID（取 logo 字节时要用） */
  orgId?: string | null
  /** 后端返回的 logo 地址；为空串 / undefined = 该组织未设置 logo */
  logoUrl?: string | null
  /** 未设置 logo 时展示的首字（通常取组织显示名的第 1 个字） */
  fallbackText: string
  /** 展示边长（px），正方形 */
  size: number
  /** 重新上传后自增的 nonce，用于强制重取字节 */
  refreshKey?: number
}) {
  const configured = !!logoUrl
  const src = useOrgLogoObjectUrl(orgId, logoUrl, refreshKey)

  // 未设置 logo：首字母占位（唯一使用圆形的场景）
  if (!configured) {
    return (
      <Avatar
        size={size}
        style={{
          background: 'linear-gradient(135deg, var(--signal), var(--signal))',
          flexShrink: 0,
        }}
      >
        {fallbackText.slice(0, 1)}
      </Avatar>
    )
  }

  // 已设置 logo：永远渲染原生 img。字节未就绪时 `src` 为空 ⇒ 浏览器不画任何东西，
  // 只占位 —— 不闪圆形字母、也不留背景色，避免「先假后真」的跳变。
  return (
    <img
      src={src || undefined}
      alt=""
      width={size}
      height={size}
      style={{
        width: size,
        height: size,
        // 等比完整显示：只缩不放裁，绝不裁切
        objectFit: 'contain',
        display: 'block',
        flexShrink: 0,
        // 明确：无圆角、无背景色
        borderRadius: 0,
        background: 'transparent',
      }}
    />
  )
}
