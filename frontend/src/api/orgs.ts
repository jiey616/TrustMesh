import { api } from './client'
import type {
  ApiResponse,
  ApiListResponse,
  OrgView,
  OrgMemberView,
  CreateOrgRequest,
  AddOrgMemberRequest,
} from '@/types'

// ─── 多租户组织 API（阶段 4-A 后端：/api/v1/organizations） ───

export async function listOrganizations() {
  return api.get('/api/v1/organizations').json<ApiListResponse<OrgView>>()
}

export async function createOrganization(input: CreateOrgRequest) {
  return api.post('/api/v1/organizations', { json: input }).json<ApiResponse<OrgView>>()
}

export async function getOrganization(id: string) {
  return api.get(`/api/v1/organizations/${id}`).json<ApiResponse<OrgView>>()
}

export async function listOrgMembers(id: string) {
  return api.get(`/api/v1/organizations/${id}/members`).json<ApiListResponse<OrgMemberView>>()
}

export async function addOrgMember(id: string, input: AddOrgMemberRequest) {
  return api
    .post(`/api/v1/organizations/${id}/members`, { json: input })
    .json<ApiResponse<OrgMemberView>>()
}

export async function updateOrgMemberRole(id: string, userId: string, role: 'admin' | 'member') {
  return api
    .patch(`/api/v1/organizations/${id}/members/${userId}`, { json: { role } })
    .json<ApiResponse<OrgMemberView>>()
}

export async function removeOrgMember(id: string, userId: string) {
  return api
    .delete(`/api/v1/organizations/${id}/members/${userId}`)
    .json<ApiResponse<{ removed: string }>>()
}
