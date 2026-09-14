import { api } from './client'
import type { AgentChatDetail, AgentChatSessionSummary, ApiResponse, ChatAttachment } from '@/types'

export async function getAgentChat(agentId: string) {
  return api.get(`agents/${agentId}/chat`).json<ApiResponse<AgentChatDetail | null>>()
}

export async function getAgentChatSessions(agentId: string) {
  return api.get(`agents/${agentId}/chat/sessions`).json<ApiResponse<AgentChatSessionSummary[]>>()
}

export async function getAgentChatSession(agentId: string, sessionId: string) {
  return api.get(`agents/${agentId}/chat/sessions/${sessionId}`).json<ApiResponse<AgentChatDetail>>()
}

export async function sendAgentChatMessage(agentId: string, content: string, attachments?: ChatAttachment[]) {
  const body: { content: string; attachments?: ChatAttachment[] } = { content }
  if (attachments && attachments.length > 0) {
    body.attachments = attachments
  }
  return api.post(`agents/${agentId}/chat/messages`, { json: body }).json<ApiResponse<AgentChatDetail>>()
}

export async function resetAgentChat(agentId: string) {
  return api.post(`agents/${agentId}/chat/reset`).json<ApiResponse<null>>()
}
