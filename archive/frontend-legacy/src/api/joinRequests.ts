import { api } from './client'
import type {
  ApiResponse,
  ApiListResponse,
  Agent,
  InvitePrompt,
  JoinRequest,
  JoinRequestOverrides,
} from '@/types'

export async function getInvitePrompt() {
  return api.get('agents/invite-prompt').json<ApiResponse<InvitePrompt>>()
}

export async function listJoinRequests(status?: string) {
  const searchParams: Record<string, string> = {}
  if (status) searchParams.status = status
  return api.get('agents/join-requests', { searchParams }).json<ApiListResponse<JoinRequest>>()
}

export async function approveJoinRequest(id: string, overrides?: JoinRequestOverrides) {
  return api.post(`agents/join-requests/${id}/approve`, { json: overrides ?? {} }).json<ApiResponse<Agent>>()
}

export async function rejectJoinRequest(id: string) {
  await api.post(`agents/join-requests/${id}/reject`)
}
