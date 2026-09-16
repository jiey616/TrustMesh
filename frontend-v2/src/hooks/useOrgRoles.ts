import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as rolesApi from '@/api/roles'
import { permKeys } from '@/hooks/usePermView'
import type { CreateOrgRoleRequest, UpdateOrgRoleRequest } from '@/types'

export const roleKeys = {
  all: ['orgRoles'] as const,
  list: (orgId: string) => [...roleKeys.all, orgId] as const,
}

/** 企业角色列表（需 org.member.mgr）。 */
export function useOrgRoles(orgId: string | undefined, enabled = true) {
  return useQuery({
    queryKey: roleKeys.list(orgId ?? ''),
    queryFn: async () => (await rolesApi.listOrgRoles(orgId!)).data.items,
    enabled: enabled && !!orgId,
  })
}

export function useCreateOrgRole(orgId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateOrgRoleRequest) => rolesApi.createOrgRole(orgId, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: roleKeys.list(orgId) }),
  })
}

export function useUpdateOrgRole(orgId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ roleId, input }: { roleId: string; input: UpdateOrgRoleRequest }) =>
      rolesApi.updateOrgRole(orgId, roleId, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: roleKeys.list(orgId) }),
  })
}

export function useDeleteOrgRole(orgId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (roleId: string) => rolesApi.deleteOrgRole(orgId, roleId),
    onSuccess: () => qc.invalidateQueries({ queryKey: roleKeys.list(orgId) }),
  })
}

/** 角色变更后刷新权限视图（当前账号若被改角色，菜单应立即跟着变）。 */
export function useRefreshPermView() {
  const qc = useQueryClient()
  return () => qc.invalidateQueries({ queryKey: permKeys.all })
}