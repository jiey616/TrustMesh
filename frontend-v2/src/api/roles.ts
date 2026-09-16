import { apiClient } from './client'
import type {
  ApiListResponse,
  ApiResponse,
  CreateOrgRoleRequest,
  OrgRoleView,
  UpdateOrgRoleRequest,
} from '@/types'

// ─── 企业角色 API（设计文档 §5） ───
//
// 读（列表）走 org.member.mgr（成员改角色的下拉框需要它），
// 增删改走 org.role.mgr（仅 owner）。
// 错误码：409 BUILTIN_ROLE_LOCKED / ROLE_IN_USE(details.member_count) / ROLE_NAME_TAKEN，
// 422 VALIDATION_ERROR(details.permission)，403（非法角色引用）。

export async function listOrgRoles(orgId: string) {
  return apiClient.get(`organizations/${orgId}/roles`).json<ApiListResponse<OrgRoleView>>()
}

export async function createOrgRole(orgId: string, input: CreateOrgRoleRequest) {
  return apiClient
    .post(`organizations/${orgId}/roles`, { json: input })
    .json<ApiResponse<OrgRoleView>>()
}

export async function updateOrgRole(orgId: string, roleId: string, input: UpdateOrgRoleRequest) {
  return apiClient
    .patch(`organizations/${orgId}/roles/${roleId}`, { json: input })
    .json<ApiResponse<OrgRoleView>>()
}

export async function deleteOrgRole(orgId: string, roleId: string) {
  return apiClient
    .delete(`organizations/${orgId}/roles/${roleId}`)
    .json<ApiResponse<{ deleted: string }>>()
}