import { apiClient } from './client'
import type {
  AgentChatDetail,
  AgentChatSessionSummary,
  ApiResponse,
  ChatAttachment,
} from '@/types'

export async function getAgentChat(agentId: string) {
  return apiClient.get(`agents/${agentId}/chat`).json<ApiResponse<AgentChatDetail | null>>()
}

export async function getAgentChatSessions(agentId: string) {
  return apiClient
    .get(`agents/${agentId}/chat/sessions`)
    .json<ApiResponse<AgentChatSessionSummary[]>>()
}

export async function getAgentChatSession(agentId: string, sessionId: string) {
  return apiClient
    .get(`agents/${agentId}/chat/sessions/${sessionId}`)
    .json<ApiResponse<AgentChatDetail>>()
}

export async function sendAgentChatMessage(agentId: string, content: string, attachments?: ChatAttachment[]) {
  return apiClient
    .post(`agents/${agentId}/chat/messages`, { json: { content, attachments } })
    .json<ApiResponse<AgentChatDetail>>()
}

export async function resetAgentChat(agentId: string) {
  return apiClient.post(`agents/${agentId}/chat/reset`).json<ApiResponse<null>>()
}
