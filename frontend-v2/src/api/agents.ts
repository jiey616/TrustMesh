import { apiClient } from './client'
import type {
  ApiResponse,
  ApiListResponse,
  Agent,
  AgentInsights,
  AgentStats,
  AgentTaskItem,
  CapabilityInfo,
  CreateAgentRequest,
  CronExecutionsResult,
  Event,
  SetCapabilityRequest,
  SetCapabilityResult,
  UpdateAgentRequest,
} from '@/types'

export async function createAgent(input: CreateAgentRequest) {
  return apiClient.post('agents', { json: input }).json<ApiResponse<Agent>>()
}

export async function listAgents() {
  return apiClient.get('agents').json<ApiListResponse<Agent>>()
}

export async function getAgent(id: string) {
  return apiClient.get(`agents/${id}`).json<ApiResponse<Agent>>()
}

export async function updateAgent(id: string, input: UpdateAgentRequest) {
  return apiClient.patch(`agents/${id}`, { json: input }).json<ApiResponse<Agent>>()
}

export async function deleteAgent(id: string) {
  await apiClient.delete(`agents/${id}`)
}

export async function getAgentStats(id: string) {
  return apiClient.get(`agents/${id}/stats`).json<ApiResponse<AgentStats>>()
}

export async function getAgentEvents(id: string, limit = 50) {
  const searchParams = { limit: String(limit) }
  return apiClient.get(`agents/${id}/events`, { searchParams }).json<ApiListResponse<Event>>()
}

export async function getAgentInsights(id: string) {
  return apiClient.get(`agents/${id}/insights`).json<ApiResponse<AgentInsights>>()
}

export async function listAgentTasks(id: string, status?: string) {
  const searchParams: Record<string, string> = {}
  if (status) searchParams.status = status
  return apiClient.get(`agents/${id}/tasks`, { searchParams }).json<ApiListResponse<AgentTaskItem>>()
}

export async function getAgentCapabilities(id: string) {
  return apiClient.get(`agents/${id}/capabilities`).json<ApiResponse<CapabilityInfo>>()
}

export async function setAgentCapabilities(id: string, input: SetCapabilityRequest) {
  return apiClient.post(`agents/${id}/capabilities`, { json: input }).json<ApiResponse<SetCapabilityResult>>()
}

export async function getCronExecutions(id: string, jobId?: string, limit = 20) {
  const searchParams: Record<string, string> = {}
  if (jobId) searchParams.jobId = jobId
  searchParams.limit = String(limit)
  return apiClient.get(`agents/${id}/cron/executions`, { searchParams }).json<ApiResponse<CronExecutionsResult>>()
}

export async function uploadSkillFile(id: string, file: File) {
  const form = new FormData()
  form.append('file', file)
  return apiClient.post(`agents/${id}/skills/upload`, { body: form }).json<ApiResponse<{ fileId: string }>>()
}
