import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as agentChatApi from '@/api/agentChat'
import type { AgentChatDetail, AgentChatSessionSummary, ChatAttachment } from '@/types'
import { normalizeChatMessageContent, normalizeEscapedText } from '@/lib/utils'

function normalizeAgentChatDetail(chat: AgentChatDetail | null): AgentChatDetail | null {
  if (!chat) {
    return null
  }

  return {
    ...chat,
    messages: (chat.messages ?? []).map((message) => ({
      ...message,
      content: normalizeChatMessageContent(message.content),
    })),
  }
}

export function useAgentChat(agentId: string | undefined) {
  return useQuery({
    queryKey: ['agents', agentId, 'chat'],
    queryFn: async () => {
      const res = await agentChatApi.getAgentChat(agentId!)
      return normalizeAgentChatDetail(res.data)
    },
    enabled: !!agentId,
    staleTime: 15_000,
  })
}

function normalizeAgentChatSessions(sessions: AgentChatSessionSummary[]): AgentChatSessionSummary[] {
  return (sessions ?? []).map((session) => ({
    ...session,
    last_message_preview: normalizeEscapedText(normalizeChatMessageContent(session.last_message_preview)).replace(/\s+/g, ' ').trim(),
  }))
}

export function useAgentChatSessions(agentId: string | undefined) {
  return useQuery({
    queryKey: ['agents', agentId, 'chat', 'sessions'],
    queryFn: async () => {
      const res = await agentChatApi.getAgentChatSessions(agentId!)
      return normalizeAgentChatSessions(res.data)
    },
    enabled: !!agentId,
    staleTime: 15_000,
  })
}

export function useAgentChatSession(agentId: string | undefined, sessionId: string | undefined, enabled = true) {
  return useQuery({
    queryKey: ['agents', agentId, 'chat', 'session', sessionId],
    queryFn: async () => {
      const res = await agentChatApi.getAgentChatSession(agentId!, sessionId!)
      return normalizeAgentChatDetail(res.data)
    },
    enabled: !!agentId && !!sessionId && enabled,
    staleTime: 15_000,
  })
}

export function useSendAgentChatMessage() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ agentId, content, attachments }: { agentId: string; content: string; attachments?: ChatAttachment[] }) =>
      agentChatApi.sendAgentChatMessage(agentId, content, attachments),
    onSuccess: (data, variables) => {
      qc.setQueryData(['agents', variables.agentId, 'chat'], normalizeAgentChatDetail(data.data))
      qc.invalidateQueries({ queryKey: ['agents', variables.agentId, 'chat', 'sessions'] })
      qc.setQueryData(['agents', variables.agentId, 'chat', 'session', data.data.id], normalizeAgentChatDetail(data.data))
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
