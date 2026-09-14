import { api } from './client'
import type { ApiListResponse, ApiResponse, MarketDeptSummary, MarketRoleDetail, MarketRoleListItem } from '@/types'

export async function listDepts() {
  return api.get('market/departments').json<ApiListResponse<MarketDeptSummary>>()
}

export interface ListRolesParams {
  dept?: string
  q?: string
}

export async function listRoles(params?: ListRolesParams) {
  const searchParams: Record<string, string> = {}
  if (params?.dept) searchParams.dept = params.dept
  if (params?.q) searchParams.q = params.q
  return api.get('market/roles', { searchParams }).json<ApiListResponse<MarketRoleListItem>>()
}

export async function getRole(id: string) {
  return api.get(`market/roles/${id}`).json<ApiResponse<MarketRoleDetail>>()
}

export async function downloadRole(id: string) {
  const blob = await api.get(`market/roles/${id}/download`).blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${id}.zip`
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}
