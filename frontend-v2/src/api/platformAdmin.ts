import { apiClient } from './client'
import type {
  ApiListResponse,
  ApiResponse,
  AuditLogQuery,
  AuditLogView,
  CreatePlatformOrgRequest,
  PlatformGlobalConfig,
  PlatformOrgMemberView,
  PlatformOrgView,
  PlatformUsage,
  PlatformUserView,
  ResetPasswordResult,
} from '@/types'

// ─── 平台管理 API（设计文档 §3.2 / §6.4，/api/v1/platform/*） ───
//
// 独立命名空间：只认平台管理员标记（env 种子账号），**不看企业角色** ——
// 企业用户（含 owner）调此处一律 403。

export async function listPlatformOrgs(params?: { keyword?: string; status?: string }) {
  const searchParams: Record<string, string> = {}
  if (params?.keyword) searchParams.keyword = params.keyword
  if (params?.status) searchParams.status = params.status
  return apiClient.get('platform/orgs', { searchParams }).json<ApiListResponse<PlatformOrgView>>()
}

export async function getPlatformOrg(id: string) {
  return apiClient.get(`platform/orgs/${id}`).json<ApiResponse<PlatformOrgView>>()
}

export async function createPlatformOrg(input: CreatePlatformOrgRequest) {
  return apiClient.post('platform/orgs', { json: input }).json<ApiResponse<PlatformOrgView>>()
}

export async function disablePlatformOrg(id: string) {
  return apiClient
    .post(`platform/orgs/${id}/disable`)
    .json<ApiResponse<PlatformOrgView>>()
}

export async function restorePlatformOrg(id: string) {
  return apiClient
    .post(`platform/orgs/${id}/restore`)
    .json<ApiResponse<PlatformOrgView>>()
}

export async function getPlatformConfig() {
  return apiClient.get('platform/config').json<ApiResponse<{ config: PlatformGlobalConfig }>>()
}

export async function updatePlatformConfig(input: PlatformGlobalConfig) {
  return apiClient
    .put('platform/config', { json: input })
    .json<ApiResponse<{ config: PlatformGlobalConfig }>>()
}

export async function listAuditLogs(query?: AuditLogQuery) {
  const searchParams: Record<string, string> = {}
  if (query?.scope) searchParams.scope = query.scope
  if (query?.action) searchParams.action = query.action
  if (query?.actor_user_id) searchParams.actor_user_id = query.actor_user_id
  if (query?.limit) searchParams.limit = String(query.limit)
  return apiClient
    .get('platform/audit-logs', { searchParams })
    .json<ApiListResponse<AuditLogView>>()
}

export async function getPlatformUsage() {
  return apiClient.get('platform/usage').json<ApiResponse<{ usage: PlatformUsage }>>()
}

// ─── 平台侧用户管理 ───

export async function listPlatformUsers(params?: {
  keyword?: string
  org_id?: string
  status?: string
}) {
  const searchParams: Record<string, string> = {}
  if (params?.keyword) searchParams.keyword = params.keyword
  if (params?.org_id) searchParams.org_id = params.org_id
  if (params?.status) searchParams.status = params.status
  return apiClient
    .get('platform/users', { searchParams })
    .json<ApiListResponse<PlatformUserView>>()
}

export async function getPlatformUser(id: string) {
  return apiClient.get(`platform/users/${id}`).json<ApiResponse<PlatformUserView>>()
}

/** 平台管理员重置账号密码：临时密码只在本次响应里返回一次，不落库、不写日志 */
export async function resetPlatformUserPassword(id: string) {
  return apiClient
    .post(`platform/users/${id}/reset-password`)
    .json<ApiResponse<ResetPasswordResult>>()
}

/** 禁用账号：登录 / refresh 一律拒绝（已发 access token 自然过期） */
export async function setPlatformUserDisabled(id: string, disabled: boolean) {
  const action = disabled ? 'disable' : 'enable'
  return apiClient
    .post(`platform/users/${id}/${action}`)
    .json<ApiResponse<PlatformUserView>>()
}

/** 企业成员列表（平台视角）：仅企业租户，个人租户返回 404 */
export async function listPlatformOrgMembers(orgId: string) {
  return apiClient
    .get(`platform/orgs/${orgId}/members`)
    .json<ApiListResponse<PlatformOrgMemberView>>()
}