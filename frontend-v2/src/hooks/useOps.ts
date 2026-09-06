import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as opsApi from '@/api/ops'
import type { OpsIncident } from '@/types/ops'

export const opsKeys = {
  all: ['ops-incidents'] as const,
  list: (status?: string) => ['ops-incidents', { status: status ?? 'all' }] as const,
}

export function useOpsIncidents(status?: string) {
  return useQuery({
    queryKey: opsKeys.list(status),
    queryFn: async () => (await opsApi.listOpsIncidents(status)).data.incidents,
  })
}

export function useOpsIncident(id: string | null) {
  return useQuery({
    queryKey: ['ops-incidents', 'detail', id],
    queryFn: async () => (await opsApi.getOpsIncident(id!)).data.incident,
    enabled: !!id,
  })
}

/** 忽略/关闭共用：成功后刷新列表 + 详情 */
function useOpsManualAction(
  fn: (args: { id: string; reason?: string }) => Promise<unknown>,
) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: opsKeys.all })
    },
  })
}

export function useIgnoreOpsIncident() {
  return useOpsManualAction(({ id, reason }) => opsApi.ignoreOpsIncident(id, reason))
}

export function useCloseOpsIncident() {
  return useOpsManualAction(({ id, reason }) => opsApi.closeOpsIncident(id, reason))
}

export type { OpsIncident }
