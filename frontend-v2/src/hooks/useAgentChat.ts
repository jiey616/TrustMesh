import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as agentChatApi from '@/api/agentChat'
import type { AgentChatDetail, AgentChatSessionSummary } from '@/types'

function normalizeChat(chat: AgentChatDetail | null): AgentChatDetail | null {
  if (!chat) return null
  return {
    ...chat,
    messages: (chat.messages ?? []).map((m) => ({ ...m, content: m.content })),
  }
}

export function useAgentChat(agentId: string | undefined) {
  return useQuery({
    queryKey: ['agents', agentId, 'chat'],
    queryFn: async () => {
      const res = await agentChatApi.getAgentChat(agentId!)
      return normalizeChat(res.data)
    },
    enabled: !!agentId,
    staleTime: 15_000,
  })
}

export function useAgentChatSessions(agentId: string | undefined) {
  return useQuery({
    queryKey: ['agents', agentId, 'chat', 'sessions'],
    queryFn: async () => {
      const res = await agentChatApi.getAgentChatSessions(agentId!)
      return res.data as AgentChatSessionSummary[]
    },
    enabled: !!agentId,
    staleTime: 15_000,
  })
}

export function useAgentChatSession(
  agentId: string | undefined,
  sessionId: string | undefined,
  enabled = true,
) {
  return useQuery({
    queryKey: ['agents', agentId, 'chat', 'session', sessionId],
    queryFn: async () => {
      const res = await agentChatApi.getAgentChatSession(agentId!, sessionId!)
      return normalizeChat(res.data)
    },
    enabled: !!agentId && !!sessionId && enabled,
    staleTime: 15_000,
  })
}

export function useSendAgentChatMessage() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ agentId, content }: { agentId: string; content: string }) =>
      agentChatApi.sendAgentChatMessage(agentId, content),
    onSuccess: (data, variables) => {
      qc.setQueryData(['agents', variables.agentId, 'chat'], normalizeChat(data.data))
      qc.invalidateQueries({ queryKey: ['agents', variables.agentId, 'chat', 'sessions'] })
    },
    onSettled: (_data, _error, variables) => {
      if (!variables) return
      void qc.invalidateQueries({ queryKey: ['agents', variables.agentId, 'chat'] })
      void qc.invalidateQueries({ queryKey: ['agents', variables.agentId, 'chat', 'sessions'] })
    },
  })
}

export function useResetAgentChat() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (agentId: string) => agentChatApi.resetAgentChat(agentId),
    onSuccess: (_data, agentId) => {
      qc.setQueryData(['agents', agentId, 'chat'], null)
      qc.invalidateQueries({ queryKey: ['agents', agentId, 'chat', 'sessions'] })
    },
  })
}
