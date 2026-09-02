import { api } from './client'
import type {
  ApiResponse,
  CreateExternalAppRequest,
  ExternalAppView,
  LaunchExternalAppResponse,
  UpdateExternalAppRequest,
} from '@/types'

export async function listExternalApps() {
  return api
    .get('external-apps')
    .json<ApiResponse<{ external_apps: ExternalAppView[] }>>()
}

// 创建后后端会一次性返回 client_secret（仅此一次），需在前端提示用户保存。
export async function createExternalApp(input: CreateExternalAppRequest) {
  return api
    .post('external-apps', { json: input })
    .json<ApiResponse<{ external_app: ExternalAppView; client_secret: string }>>()
}

export async function getExternalApp(id: string) {
  return api.get(`external-apps/${id}`).json<ApiResponse<{ external_app: ExternalAppView }>>()
}

export async function updateExternalApp(id: string, input: UpdateExternalAppRequest) {
  return api
    .patch(`external-apps/${id}`, { json: input })
    .json<ApiResponse<{ external_app: ExternalAppView }>>()
}

export async function deleteExternalApp(id: string) {
  return api.delete(`external-apps/${id}`).json<ApiResponse<{ deleted: boolean }>>()
}

export async function launchExternalApp(
  id: string,
  input?: { project_id?: string; task_id?: string },
) {
  return api
    .post(`external-apps/${id}/launch`, { json: input ?? {} })
    .json<ApiResponse<LaunchExternalAppResponse>>()
}
