import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as externalAppsApi from '@/api/externalApps'
import type {
  CreateExternalAppRequest,
  ExternalAppView,
  UpdateExternalAppRequest,
} from '@/types'

export const externalAppKeys = {
  all: ['external-apps'] as const,
}

// enabled 供 F2 门控使用：MainLayout 校准前传 false 挡住无头请求；默认 true 保持既有调用点零改动。
export function useExternalApps(enabled = true) {
  return useQuery({
    queryKey: externalAppKeys.all,
    queryFn: async () => (await externalAppsApi.listExternalApps()).data.external_apps,
    enabled,
  })
}

export function useCreateExternalApp() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateExternalAppRequest) => externalAppsApi.createExternalApp(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: externalAppKeys.all }),
  })
}

export function useUpdateExternalApp() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdateExternalAppRequest }) =>
      externalAppsApi.updateExternalApp(id, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: externalAppKeys.all }),
  })
}

export function useDeleteExternalApp() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => externalAppsApi.deleteExternalApp(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: externalAppKeys.all }),
  })
}

export interface LaunchArgs {
  id: string
  projectId?: string
  taskId?: string
}

export function useLaunchExternalApp() {
  return useMutation({
    mutationFn: async ({ id, projectId, taskId }: LaunchArgs) => {
      const res = await externalAppsApi.launchExternalApp(id, { project_id: projectId, task_id: taskId })
      return res.data
    },
  })
}

export type { ExternalAppView }
