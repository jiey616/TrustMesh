import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as platformApi from '@/api/platformAdmin'
import type {
  AuditLogQuery,
  CreatePlatformOrgRequest,
  PlatformGlobalConfig,
} from '@/types'

export const platformKeys = {
  all: ['platformAdmin'] as const,
  orgs: (params: { keyword?: string; status?: string }) =>
    [...platformKeys.all, 'orgs', params] as const,
  config: () => [...platformKeys.all, 'config'] as const,
  audit: (query: AuditLogQuery) => [...platformKeys.all, 'audit', query] as const,
  usage: () => [...platformKeys.all, 'usage'] as const,
}

/** 平台侧企业列表（元数据），支持关键字/状态过滤。 */
export function usePlatformOrgs(params: { keyword?: string; status?: string }, enabled = true) {
  return useQuery({
    queryKey: platformKeys.orgs(params),
    queryFn: async () => (await platformApi.listPlatformOrgs(params)).data.items,
    enabled,
  })
}

export function useCreatePlatformOrg() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreatePlatformOrgRequest) => platformApi.createPlatformOrg(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: platformKeys.all }),
  })
}

/** 禁用/恢复企业：禁用即拦截（该企业成员的请求一律 403 ORG_DISABLED），恢复即时生效。 */
export function useSetPlatformOrgStatus() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, disabled }: { id: string; disabled: boolean }) =>
      disabled ? platformApi.disablePlatformOrg(id) : platformApi.restorePlatformOrg(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: platformKeys.all }),
  })
}

export function usePlatformConfig(enabled = true) {
  return useQuery({
    queryKey: platformKeys.config(),
    queryFn: async () => (await platformApi.getPlatformConfig()).data.config,
    enabled,
  })
}

export function useUpdatePlatformConfig() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: PlatformGlobalConfig) => platformApi.updatePlatformConfig(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: platformKeys.config() }),
  })
}

export function useAuditLogs(query: AuditLogQuery, enabled = true) {
  return useQuery({
    queryKey: platformKeys.audit(query),
    queryFn: async () => (await platformApi.listAuditLogs(query)).data.items,
    enabled,
  })
}

export function usePlatformUsage(enabled = true) {
  return useQuery({
    queryKey: platformKeys.usage(),
    queryFn: async () => (await platformApi.getPlatformUsage()).data.usage,
    enabled,
  })
}