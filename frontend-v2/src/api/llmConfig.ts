import { apiClient } from './client'
import type { ApiResponse } from '@/types'

// ========== 平台 / 租户 LLM 配置（A1+B2+D1） ==========

export interface LLMConfigView {
  org_id?: string
  api_url: string
  /** write-only：只回掩码（sk-***1234），空 = 该层未配 key */
  api_key_masked: string
  model: string
  ops_model?: string
  /** org | platform | env：解析后的实际生效来源 */
  source: string
  /** 该层是否存在落库配置（false = 展示的是回退值） */
  has_override: boolean
  updated_at?: string
  updated_by?: string
}

export interface UpdateLLMConfigRequest {
  api_url: string
  /** 留空 = 保持已有 key 不变；该层尚无 key 时必填 */
  api_key?: string
  model: string
  ops_model?: string
  reset_api_key?: boolean
}

export interface LLMConfigTestResult {
  ok: boolean
  error?: string
  model?: string
  source?: string
  latency_ms?: number
}

// ---- 平台默认层（平台管理员） ----

export async function getPlatformLLMConfig() {
  return apiClient
    .get('platform/llm-config')
    .json<ApiResponse<{ config: LLMConfigView }>>()
}

export async function updatePlatformLLMConfig(input: UpdateLLMConfigRequest) {
  return apiClient
    .put('platform/llm-config', { json: input })
    .json<ApiResponse<{ config: LLMConfigView }>>()
}

export async function deletePlatformLLMConfig() {
  return apiClient.delete('platform/llm-config').json<ApiResponse<{ deleted: boolean }>>()
}

// ---- 租户覆盖层（org owner/admin） ----

export async function getOrgLLMConfig(orgId: string) {
  return apiClient
    .get(`organizations/${orgId}/llm-config`)
    .json<ApiResponse<{ config: LLMConfigView }>>()
}

export async function updateOrgLLMConfig(orgId: string, input: UpdateLLMConfigRequest) {
  return apiClient
    .put(`organizations/${orgId}/llm-config`, { json: input })
    .json<ApiResponse<{ config: LLMConfigView }>>()
}

export async function deleteOrgLLMConfig(orgId: string) {
  return apiClient
    .delete(`organizations/${orgId}/llm-config`)
    .json<ApiResponse<{ deleted: boolean }>>()
}

// ---- 连接测试 / 模型列表（字段可选：缺省测生效配置） ----

export interface LLMConfigScopeInput {
  /** personal | org | platform；空 = 按 org_id 推断（默认平台层） */
  scope?: string
  org_id?: string
  api_url?: string
  api_key?: string
  model?: string
}

export async function testLLMConfig(input?: LLMConfigScopeInput) {
  return apiClient
    .post('llm-config/test', { json: input ?? {} })
    .json<ApiResponse<{ test: LLMConfigTestResult }>>()
}

export interface LLMModelsResult {
  ok: boolean
  error?: string
  items?: string[]
}

/** 拉取 provider 可用模型列表（OpenAI 兼容 GET {api_url}/models）。 */
export async function fetchLLMModels(input?: LLMConfigScopeInput) {
  return apiClient
    .post('llm-config/models', { json: input ?? {} })
    .json<ApiResponse<{ models: LLMModelsResult }>>()
}

// ---- 个人空间层（任何登录用户） ----

export async function getPersonalLLMConfig() {
  return apiClient
    .get('llm-config/personal')
    .json<ApiResponse<{ config: LLMConfigView }>>()
}

export async function updatePersonalLLMConfig(input: UpdateLLMConfigRequest) {
  return apiClient
    .put('llm-config/personal', { json: input })
    .json<ApiResponse<{ config: LLMConfigView }>>()
}

export async function deletePersonalLLMConfig() {
  return apiClient
    .delete('llm-config/personal')
    .json<ApiResponse<{ deleted: boolean }>>()
}
