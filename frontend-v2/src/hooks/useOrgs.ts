import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as orgsApi from '@/api/orgs'
import { useAuthStore } from '@/stores/authStore'
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
    mutationFn: ({ userId, role }: { userId: string; role: 'admin' | 'member' }) =>
      orgsApi.updateOrgMemberRole(orgId, userId, role),
    onSuccess: () => qc.invalidateQueries({ queryKey: orgKeys.members(orgId) }),
  })
}

export function useRemoveOrgMember(orgId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (userId: string) => orgsApi.removeOrgMember(orgId, userId),
    onSuccess: () => qc.invalidateQueries({ queryKey: orgKeys.members(orgId) }),
  })
}
