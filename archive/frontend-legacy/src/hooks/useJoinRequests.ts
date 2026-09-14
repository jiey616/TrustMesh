import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as joinRequestsApi from '@/api/joinRequests'
import type { JoinRequestOverrides } from '@/types'

export function useInvitePrompt(enabled: boolean) {
  return useQuery({
    queryKey: ['invite-prompt'],
    queryFn: async () => {
      const res = await joinRequestsApi.getInvitePrompt()
      return res.data
    },
    enabled,
    staleTime: 5 * 60_000,
  })
}

export function useJoinRequests(status?: string) {
  return useQuery({
    queryKey: ['join-requests', status ?? 'all'],
    queryFn: async () => {
      const res = await joinRequestsApi.listJoinRequests(status)
      return res.data.items
    },
    refetchInterval: 15_000,
  })
}

export function useApproveJoinRequest() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, overrides }: { id: string; overrides?: JoinRequestOverrides }) =>
      joinRequestsApi.approveJoinRequest(id, overrides),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['join-requests'] })
      void qc.invalidateQueries({ queryKey: ['agents'] })
    },
  })
}

export function useRejectJoinRequest() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => joinRequestsApi.rejectJoinRequest(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['join-requests'] })
    },
  })
}
