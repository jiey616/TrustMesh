import { apiClient } from './client'
import type {
  ApiResponse,
  ApiListResponse,
  OrgView,
  OrgMemberView,
  CreateOrgRequest,
  AddOrgMemberRequest,
} from '@/types'

// ─── 多租户组织 API（阶段 4-A 后端：organizations） ───

export async function listOrganizations() {
  return apiClient.get('organizations').json<ApiListResponse<OrgView>>()
}

export async function createOrganization(input: CreateOrgRequest) {
  return apiClient.post('organizations', { json: input }).json<ApiResponse<OrgView>>()
}

export async function getOrganization(id: string) {
  return apiClient.get(`organizations/${id}`).json<ApiResponse<OrgView>>()
}

export async function listOrgMembers(id: string) {
  return apiClient
    .get(`organizations/${id}/members`)
    .json<ApiListResponse<OrgMemberView>>()
}

export async function addOrgMember(id: string, input: AddOrgMemberRequest) {
  return apiClient
    .post(`organizations/${id}/members`, { json: input })
    .json<ApiResponse<OrgMemberView>>()
}

/** 改成员角色：传 role_id（自定义角色优先）或内置角色串 role。 */
export async function updateOrgMemberRole(
  id: string,
  userId: string,
  ref: { role?: string; role_id?: string },
) {
  return apiClient
    .patch(`organizations/${id}/members/${userId}`, { json: ref })
    .json<ApiResponse<OrgMemberView>>()
}

export async function removeOrgMember(id: string, userId: string) {
  return apiClient
    .delete(`organizations/${id}/members/${userId}`)
    .json<ApiResponse<{ removed: string }>>()
}

/** 企业级菜单覆盖（只能缩小）：被点名的菜单键对全员隐藏（需 org.settings）。 */
export async function updateOrgMenuOverrides(id: string, menuOverrides: string[]) {
  return apiClient
    .patch(`organizations/${id}/menu-overrides`, { json: { menu_overrides: menuOverrides } })
    .json<ApiResponse<OrgView>>()
}
