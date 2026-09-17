import { apiClient } from './client'
import type {
  ApiResponse,
  CreateExternalAppRequest,
  ExternalAppView,
  LaunchExternalAppResponse,
  UpdateExternalAppRequest,
} from '@/types'

export async function listExternalApps() {
  return apiClient
    .get('external-apps')
    .json<ApiResponse<{ external_apps: ExternalAppView[] }>>()
}

// 创建后后端会一次性返回 client_secret（仅此一次），需在前端提示用户保存。
export async function createExternalApp(input: CreateExternalAppRequest) {
  return apiClient
    .post('external-apps', { json: input })
    .json<ApiResponse<{ external_app: ExternalAppView; client_secret: string }>>()
}

export async function getExternalApp(id: string) {
  return apiClient.get(`external-apps/${id}`).json<ApiResponse<{ external_app: ExternalAppView }>>()
}

export async function updateExternalApp(id: string, input: UpdateExternalAppRequest) {
  return apiClient
    .patch(`external-apps/${id}`, { json: input })
    .json<ApiResponse<{ external_app: ExternalAppView }>>()
}

export async function deleteExternalApp(id: string) {
  return apiClient.delete(`external-apps/${id}`).json<ApiResponse<{ deleted: boolean }>>()
}

export async function launchExternalApp(
  id: string,
  input?: { project_id?: string; task_id?: string },
) {
  return apiClient
    .post(`external-apps/${id}/launch`, { json: input ?? {} })
    .json<ApiResponse<LaunchExternalAppResponse>>()
}

// ========== 全局级（平台命名空间，仅平台管理员；企业角色一律 403）==========

export async function listGlobalExternalApps() {
  return apiClient
    .get('platform/external-apps')
    .json<ApiResponse<{ external_apps: ExternalAppView[] }>>()
}

export async function createGlobalExternalApp(input: CreateExternalAppRequest) {
  return apiClient
    .post('platform/external-apps', { json: input })
    .json<ApiResponse<{ external_app: ExternalAppView; client_secret: string }>>()
}

export async function updateGlobalExternalApp(id: string, input: UpdateExternalAppRequest) {
  return apiClient
    .patch(`platform/external-apps/${id}`, { json: input })
    .json<ApiResponse<{ external_app: ExternalAppView }>>()
}

export async function deleteGlobalExternalApp(id: string) {
  return apiClient
    .delete(`platform/external-apps/${id}`)
    .json<ApiResponse<{ deleted: boolean }>>()
}
