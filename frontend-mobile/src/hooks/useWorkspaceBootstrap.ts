import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useAuthStore } from '@/stores/authStore'
import { runWorkspaceCalibration, useWorkspaceStore } from '@/stores/workspaceStore'
import { listOrganizations } from '@/api/orgs'

/**
 * 登录后校准工作区：确定个人空间 ID → 落到当前空间 → 清缓存 → 放开渲染。
 *
 * 🔴 顺序由 `runWorkspaceCalibration` 单点保证（写反会静默复发，静态检查发现不了）。
 * 🔴 全程在 await 之后才 setState，避免 effect 内同步 setState 触发级联渲染。
 */
export function useWorkspaceBootstrap(): void {
  const queryClient = useQueryClient()
  const hasSession = useAuthStore((s) => Boolean(s.refreshToken))

  useEffect(() => {
    if (!hasSession) return
    let cancelled = false

    void (async () => {
      try {
        const orgs = await listOrganizations()
        if (cancelled) return

        const personal = orgs.find((o) => o.kind === 'personal') ?? null
        const { activeOrgId, setPersonalOrgId, setActiveOrgId, setCalibrated } =
          useWorkspaceStore.getState()

        runWorkspaceCalibration(
          {
            setPersonalOrgId,
            setActiveOrgId,
            clearQueries: () => queryClient.removeQueries(),
            setCalibrated,
          },
          {
            personalOrgId: personal?.id ?? null,
            activeOrgId: activeOrgId ?? personal?.id ?? null,
          },
        )
      } catch {
        // 组织拉取失败不阻断使用：放开渲染，让各页面自己展示空态/重试。
        if (!cancelled) useWorkspaceStore.getState().setCalibrated(true)
      }
    })()

    return () => {
      cancelled = true
    }
  }, [hasSession, queryClient])
}
