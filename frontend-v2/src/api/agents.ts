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

/**
 * 写回节点能力（技能 / 模型 / cron）。
 *
 * 🔴 这个端点必须单独放宽超时：ky 的默认 `timeout` 是 10s，而写回要经
 * daemon → NATS `capability.set` 穿透到远端节点、同步等待回执，并重启该节点 gateway，
 * **生产实测单次 9.97s**（logging 中间件的 http_request latency）—— 紧贴 10s。
 * 结果就是前端必然先超时报错，而后端其实已经改完 ⇒ 用户看到「超时」、刷新却发现已成功。
 *
 * 取值刻意大于后端 ctx 的 60s，保证「后端先拿到节点的真实结果」而不是两边同时放弃；
 * 真正失败时由 daemon 按契约返回 200 + ok=false，前端走 applyWritebackResult 提示。
 */
const CAPABILITY_WRITE_TIMEOUT_MS = 70_000

export async function setAgentCapabilities(id: string, input: SetCapabilityRequest) {
  return apiClient
    .post(`agents/${id}/capabilities`, { json: input, timeout: CAPABILITY_WRITE_TIMEOUT_MS })
    .json<ApiResponse<SetCapabilityResult>>()
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
