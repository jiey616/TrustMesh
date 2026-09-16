import { Navigate } from 'react-router-dom'
import { usePermStore } from '@/stores/permStore'
import type { ReactNode } from 'react'

interface Props {
  children: ReactNode
  /** 需要的权限点：任一命中即可（缺省 = 不做权限点判断） */
  perms?: readonly string[]
  /** 身份层：platform = 仅平台管理员；business = 仅业务账号（平台管理员被拦） */
  audience?: 'platform' | 'business'
}

/**
 * 路由级权限兜底（设计文档 §7）：越权统一跳 403 页，而不是渲染出必然 403 的白屏页面。
 *
 * 这只是**界面投影**，不是安全边界 —— 后端 API 鉴权才是唯一权威。
 * 权限视图未就绪（/users/me 尚未返回）时放行，由后端 403 兜底：
 * 绝不因权限视图失败把界面锁死（与 F2 门控 fail-open 同源）。
 *
 * 身份层不匹配（平台管理员访问业务页）不算「越权」，而是走错了命名空间 →
 * 直接送回他自己的首页，避免落到一个只能点返回的 403 死胡同。
 */
export function PermRoute({ children, perms, audience }: Props) {
  const ready = usePermStore((s) => s.ready)
  const permissions = usePermStore((s) => s.permissions)
  const isPlatformAdmin = usePermStore((s) => s.isPlatformAdmin)

  if (!ready) return <>{children}</>

  if (audience === 'platform' && !isPlatformAdmin) return <Navigate to="/403" replace />
  if (audience === 'business' && isPlatformAdmin) {
    return <Navigate to="/platform/orgs" replace />
  }
  if (perms && perms.length > 0 && !perms.some((p) => permissions.includes(p))) {
    return <Navigate to="/403" replace />
  }

  return <>{children}</>
}