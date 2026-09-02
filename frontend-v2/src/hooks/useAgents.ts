import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as agentsApi from '@/api/agents'
import type { CreateAgentRequest, CronExecutionsResult, SetCapabilityRequest, UpdateAgentRequest } from '@/types'

export function useAgents() {
  return useQuery({
    queryKey: ['agents'],
    queryFn: async () => {
      const res = await agentsApi.listAgents()
      return res.data.items
    },
  })
}

export function useAgent(id: string | undefined) {
  return useQuery({
    queryKey: ['agents', id],
    queryFn: async () => {
      const res = await agentsApi.getAgent(id!)
      return res.data
    },
    enabled: !!id,
  })
}

export function useCreateAgent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateAgentRequest) => agentsApi.createAgent(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agents'] }),
  })
}

export function useUpdateAgent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdateAgentRequest }) => agentsApi.updateAgent(id, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agents'] }),
  })
}

export function useAgentTasks(id: string | undefined, status?: string) {
  return useQuery({
    queryKey: ['agents', id, 'tasks', status ?? 'all'],
    queryFn: async () => {
      const res = await agentsApi.listAgentTasks(id!, status)
      return res.data.items
    },
    enabled: !!id,
    staleTime: 30_000,
  })
}

export function useAgentStats(id: string | undefined) {
  return useQuery({
    queryKey: ['agents', id, 'stats'],
    queryFn: async () => {
      const res = await agentsApi.getAgentStats(id!)
      return res.data
    },
    enabled: !!id,
    staleTime: 30_000,
  })
}

export function useAgentEvents(id: string | undefined, limit = 50) {
  return useQuery({
    queryKey: ['agents', id, 'events', limit],
    queryFn: async () => {
      const res = await agentsApi.getAgentEvents(id!, limit)
      return res.data.items
    },
    enabled: !!id,
    staleTime: 15_000,
  })
}

export function useAgentInsights(id: string | undefined) {
  return useQuery({
    queryKey: ['agents', id, 'insights'],
    queryFn: async () => {
      const res = await agentsApi.getAgentInsights(id!)
      return res.data
    },
    enabled: !!id,
    staleTime: 30_000,
  })
}

export function useAgentCapabilities(id: string | undefined, enabled = true) {
  return useQuery({
    queryKey: ['agents', id, 'capabilities'],
    queryFn: async () => {
      const res = await agentsApi.getAgentCapabilities(id!)
      return res.data
    },
    enabled: !!id && enabled,
    staleTime: 30_000,
    retry: false,
  })
}

export function useDeleteAgent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => agentsApi.deleteAgent(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agents'] }),
  })
}

export function useSetAgentCapabilities(id: string | undefined) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: SetCapabilityRequest) => agentsApi.setAgentCapabilities(id!, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['agents', id, 'capabilities'] })
    },
  })
}

export function useUploadSkillFile(id: string | undefined) {
  return useMutation({
    mutationFn: (file: File) => agentsApi.uploadSkillFile(id!, file),
  })
}

export function useCronExecutions(id: string | undefined, jobId?: string, enabled = true) {
  return useQuery({
    queryKey: ['agents', id, 'cron-executions', jobId],
    queryFn: async () => {
      const res = await agentsApi.getCronExecutions(id!, jobId)
      return res.data as CronExecutionsResult
    },
    enabled: !!id && enabled,
    staleTime: 10_000,
  })
}
