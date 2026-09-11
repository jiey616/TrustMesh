import { apiClient } from './client'
import type { ApiResponse } from '@/types'
import type { OpsIncident } from '@/types/ops'

export async function listOpsIncidents(status?: string) {
  const searchParams = new URLSearchParams()
  if (status && status !== 'all') searchParams.set('status', status)
  const qs = searchParams.toString()
  return apiClient
    .get(`ops/incidents${qs ? `?${qs}` : ''}`)
    .json<ApiResponse<{ incidents: OpsIncident[] }>>()
}

export async function getOpsIncident(id: string) {
  return apiClient
    .get(`ops/incidents/${id}`)
    .json<ApiResponse<{ incident: OpsIncident }>>()
}

/** 人工忽略：之后不再自动干预该问题（同主体复发会新开单） */
export async function ignoreOpsIncident(id: string, reason?: string) {
  return apiClient
    .post(`ops/incidents/${id}/ignore`, { json: { reason: reason ?? '' } })
    .json<ApiResponse<{ incident: OpsIncident }>>()
}

/** 人工关闭：语义等同 resolved */
export async function closeOpsIncident(id: string, reason?: string) {
  return apiClient
    .post(`ops/incidents/${id}/close`, { json: { reason: reason ?? '' } })
    .json<ApiResponse<{ incident: OpsIncident }>>()
}
