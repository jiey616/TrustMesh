import { apiClient } from './client'
import type {
  AgentChatDetail,
  AgentChatSessionSummary,
  ApiResponse,
} from '@/types'

export async function getAgentChat(agentId: string) {
  return apiClient.get(`/api/v1/agents/${agentId}/chat`).json<ApiResponse<AgentChatDetail | null>>()
}

export async function getAgentChatSessions(agentId: string) {
  return apiClient
    .get(`/api/v1/agents/${agentId}/chat/sessions`)
    .json<ApiResponse<AgentChatSessionSummary[]>>()
}

export async function getAgentChatSession(agentId: string, sessionId: string) {
  return apiClient
    .get(`/api/v1/agents/${agentId}/chat/sessions/${sessionId}`)
    .json<ApiResponse<AgentChatDetail>>()
}

export async function sendAgentChatMessage(agentId: string, content: string) {
  return apiClient
    .post(`/api/v1/agents/${agentId}/chat/messages`, { json: { content } })
    .json<ApiResponse<AgentChatDetail>>()
}

export async function resetAgentChat(agentId: string) {
  return apiClient.post(`/api/v1/agents/${agentId}/chat/reset`).json<ApiResponse<null>>()
}
