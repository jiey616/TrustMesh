import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as desktopReleasesApi from '@/api/desktopReleases'
import type { DesktopReleaseUploadInput } from '@/types'

export const desktopReleaseKeys = {
  all: ['desktop-releases'] as const,
}

/** enabled 供路由级门控使用（默认 true，保持调用点零改动）。 */
export function useDesktopReleases(enabled = true) {
  return useQuery({
    queryKey: desktopReleaseKeys.all,
    queryFn: async () => (await desktopReleasesApi.listDesktopReleases()).data.desktop_releases,
    enabled,
  })
}

export function usePublishDesktopRelease() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => desktopReleasesApi.publishDesktopRelease(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: desktopReleaseKeys.all }),
  })
}

export function useRollbackDesktopRelease() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => desktopReleasesApi.rollbackDesktopRelease(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: desktopReleaseKeys.all }),
  })
}

export function useDeleteDesktopRelease() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => desktopReleasesApi.deleteDesktopRelease(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: desktopReleaseKeys.all }),
  })
}

export interface UploadDesktopReleaseArgs {
  input: DesktopReleaseUploadInput
  onProgress?: (percent: number) => void
}

/**
 * 上传发行版。进度回调刻意透传到调用方（而非存进 mutation 状态）：
 * 进度是**瞬时 UI**，放进全局缓存只会引入无谓的重渲染与失效语义。
 */
export function useUploadDesktopRelease() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ input, onProgress }: UploadDesktopReleaseArgs) =>
      desktopReleasesApi.uploadDesktopRelease(input, onProgress),
    onSuccess: () => qc.invalidateQueries({ queryKey: desktopReleaseKeys.all }),
  })
}
