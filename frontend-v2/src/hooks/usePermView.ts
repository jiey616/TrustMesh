import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getMe } from '@/api/user'
import { useAuthStore } from '@/stores/authStore'
import { usePermStore } from '@/stores/permStore'
import type { MeResponse } from '@/types'

export const permKeys = {
  all: ['perm'] as const,
  /** 权限视图按租户上下文解析：queryKey 带 activeOrgId，切工作区即重取 */
  me: (orgId: string | null) => [...permKeys.all, 'me', orgId] as const,
}

/**
 * 拉取 `/users/me` 的权限视图并灌入 permStore。
 *
 * ️ 必须挂 F2 门控（`enabled = workspaceCalibrated`）：权限集按 `X-Org-Id` 解析，
 * 校准完成前发出的请求会缺少（或带错）租户头，解析结果不可信。
 *
 * 不做跨请求缓存（staleTime=0）：权限/角色变更必须即时生效（多实例下更是如此）。
 */
export function usePermView(enabled = true) {
  const refreshToken = useAuthStore((s) => s.refreshToken)
  const activeOrgId = useAuthStore((s) => s.activeOrgId)
  const query = useQuery<MeResponse>({
    queryKey: permKeys.me(activeOrgId),
    queryFn: async () => (await getMe()).data,
    enabled: enabled && !!refreshToken,
    staleTime: 0,
  })

  const data = query.data
  useEffect(() => {
    if (data) usePermStore.getState().setPermView(data)
  }, [data])

  return query
}