import { apiClient } from './client'
import type { ApiResponse } from '@/types'

// ─── 平台「使用引导」（单篇全局 HTML）───
//
// 阅读（GET /guide）是登录基础权限；管理三动作（GET/PUT/DELETE /platform/guide）
// 走权限点 platform.guide.mgr。上传即覆盖（后端固定 _id="global"）。

export interface PlatformGuidePayload {
  file_name: string
  size: number
  updated_by?: string
  updated_at: string
  /** HTML 原文。渲染必须走 sandbox iframe（禁脚本），绝不注入 DOM。 */
  html: string
}

export async function getGuide() {
  return apiClient.get('guide').json<ApiResponse<{ guide: PlatformGuidePayload | null }>>()
}

export async function getPlatformGuide() {
  return apiClient
    .get('platform/guide')
    .json<ApiResponse<{ guide: PlatformGuidePayload | null }>>()
}

export async function uploadPlatformGuide(file: File) {
  const form = new FormData()
  form.append('file', file)
  return apiClient
    .put('platform/guide', { body: form })
    .json<ApiResponse<{ guide: Omit<PlatformGuidePayload, 'html'> }>>()
}

export async function deletePlatformGuide() {
  return apiClient.delete('platform/guide').json<ApiResponse<{ deleted: boolean }>>()
}
