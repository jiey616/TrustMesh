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
  org: (id: string) => [...platformKeys.all, 'org', id] as const,
  orgMembers: (id: string) => [...platformKeys.all, 'orgMembers', id] as const,
  users: (params: { keyword?: string; org_id?: string; status?: string }) =>
    [...platformKeys.all, 'users', params] as const,
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

/** 单个企业详情（平台视角元数据），供企业详情页使用。 */
export function usePlatformOrg(id: string | undefined, enabled = true) {
  return useQuery({
    queryKey: platformKeys.org(id ?? ''),
    queryFn: async () => (await platformApi.getPlatformOrg(id as string)).data,
    enabled: enabled && !!id,
  })
}

/** 账号列表（平台视角），支持关键字 / 所属企业 / 状态过滤。 */
export function usePlatformUsers(
  params: { keyword?: string; org_id?: string; status?: string },
  enabled = true,
) {
  return useQuery({
    queryKey: platformKeys.users(params),
    queryFn: async () => (await platformApi.listPlatformUsers(params)).data.items,
    enabled,
  })
}

/** 企业成员列表（平台视角，含账号禁用态）。 */
export function usePlatformOrgMembers(orgId: string | undefined, enabled = true) {
  return useQuery({
    queryKey: platformKeys.orgMembers(orgId ?? ''),
    queryFn: async () => (await platformApi.listPlatformOrgMembers(orgId as string)).data.items,
    enabled: enabled && !!orgId,
  })
}

/** 禁用/启用账号：登录与 refresh 一律拒绝；已发 access token 自然过期。 */
export function useSetPlatformUserDisabled() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, disabled }: { id: string; disabled: boolean }) =>
      platformApi.setPlatformUserDisabled(id, disabled),
    onSuccess: () => qc.invalidateQueries({ queryKey: platformKeys.all }),
  })
}

/** 重置账号密码：临时密码只在返回值里出现一次，界面必须提示「只显示一次」。 */
export function useResetPlatformUserPassword() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => platformApi.resetPlatformUserPassword(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: platformKeys.all }),
  })
}