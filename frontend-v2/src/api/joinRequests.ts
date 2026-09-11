import { apiClient } from '@/api/client'
import type {
  ApiResponse,
  ApiListResponse,
  Agent,
  InvitePrompt,
  JoinRequest,
  JoinRequestOverrides,
} from '@/types'

export async function getInvitePrompt() {
  return apiClient.get('agents/invite-prompt').json<ApiResponse<InvitePrompt>>()
}

export async function listJoinRequests(status?: string) {
  const searchParams: Record<string, string> = {}
  if (status) searchParams.status = status
  return apiClient.get('agents/join-requests', { searchParams }).json<ApiListResponse<JoinRequest>>()
}

export async function approveJoinRequest(id: string, overrides?: JoinRequestOverrides) {
  return apiClient.post(`agents/join-requests/${id}/approve`, { json: overrides ?? {} }).json<ApiResponse<Agent>>()
}

export async function rejectJoinRequest(id: string) {
  await apiClient.post(`agents/join-requests/${id}/reject`)
}
