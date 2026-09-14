import { useQuery } from '@tanstack/react-query'
import * as dashboardApi from '@/api/dashboard'

export function useDashboardStats() {
  return useQuery({
    queryKey: ['dashboard', 'stats'],
    queryFn: async () => {
      const res = await dashboardApi.getDashboardStats()
      return res.data
    },
    staleTime: 30_000,
  })
}

export function useDashboardEvents(limit = 10) {
  return useQuery({
    queryKey: ['dashboard', 'events', limit],
    queryFn: async () => {
      const res = await dashboardApi.getDashboardEvents(limit)
      return res.data.items
    },
    staleTime: 15_000,
  })
}

export function useDashboardTasks(limit = 10) {
  return useQuery({
    queryKey: ['dashboard', 'tasks', limit],
    queryFn: async () => {
      const res = await dashboardApi.getDashboardTasks(limit)
      return res.data.items
    },
    staleTime: 15_000,
  })
}

export function useAgentEvents(agentId: string | undefined, limit = 50) {
  return useQuery({
    queryKey: ['agents', agentId, 'events', limit],
    queryFn: async () => {
      const res = await dashboardApi.getAgentEvents(agentId!, limit)
      return res.data.items
    },
    enabled: !!agentId,
    staleTime: 15_000,
  })
}
