import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as orgsApi from '@/api/orgs'
import { useAuthStore } from '@/stores/authStore'
import { permKeys } from '@/hooks/usePermView'
import type { AddOrgMemberRequest, CreateOrgRequest } from '@/types'

export const orgKeys = {
  all: ['organizations'] as const,
  mine: () => [...orgKeys.all, 'mine'] as const,
  detail: (id: string) => [...orgKeys.all, 'detail', id] as const,
  members: (id: string) => [...orgKeys.all, 'members', id] as const,
}

// 我所属的全部租户（个人 + 企业）。仅登录后调用。
export function useOrganizations(enabled = true) {
  const refreshToken = useAuthStore((s) => s.refreshToken)
  return useQuery({
    queryKey: orgKeys.mine(),
    queryFn: async () => (await orgsApi.listOrganizations()).data.items,
    enabled: enabled && !!refreshToken,
  })
}

export function useOrgMembers(orgId: string | undefined) {
  return useQuery({
    queryKey: orgKeys.members(orgId ?? ''),
    queryFn: async () => (await orgsApi.listOrgMembers(orgId!)).data.items,
    enabled: !!orgId,
  })
}

export function useCreateOrg() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateOrgRequest) => orgsApi.createOrganization(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: orgKeys.all }),
  })
}

export function useAddOrgMember(orgId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: AddOrgMemberRequest) => orgsApi.addOrgMember(orgId, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: orgKeys.members(orgId) }),
  })
}

export function useUpdateOrgMemberRole(orgId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ userId, ref }: { userId: string; ref: { role?: string; role_id?: string } }) =>
      orgsApi.updateOrgMemberRole(orgId, userId, ref),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: orgKeys.members(orgId) })
      // 被改角色的可能就是当前账号：权限视图必须跟着刷新（不依赖进程内缓存）。
      qc.invalidateQueries({ queryKey: permKeys.all })
    },
  })
}

export function useRemoveOrgMember(orgId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (userId: string) => orgsApi.removeOrgMember(orgId, userId),
    onSuccess: () => qc.invalidateQueries({ queryKey: orgKeys.members(orgId) }),
  })
}

/** 企业级菜单覆盖（只能缩小）：保存后刷新权限视图，菜单立即跟着变。 */
export function useUpdateOrgMenuOverrides(orgId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (menuOverrides: string[]) => orgsApi.updateOrgMenuOverrides(orgId, menuOverrides),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: orgKeys.all })
      qc.invalidateQueries({ queryKey: permKeys.all })
    },
  })
}
