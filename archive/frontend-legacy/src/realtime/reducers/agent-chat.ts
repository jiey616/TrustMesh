import type { QueryClient } from '@tanstack/react-query'
import type { AgentChatDetail } from '@/types'
import { normalizeChatMessageContent } from '@/lib/utils'

export function applyAgentChatUpdated(
  queryClient: QueryClient,
  payload: { chat: AgentChatDetail }
) {
  queryClient.setQueryData(['agents', payload.chat.agent_id, 'chat'], {
    ...payload.chat,
    messages: (payload.chat.messages ?? []).map((message) => ({
      ...message,
      content: normalizeChatMessageContent(message.content),
    })),
  })
}
