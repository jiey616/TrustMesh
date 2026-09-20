import { apiClient } from './client'
import type { ApiResponse } from '@/types'

// ─── 移动端安装包（Android APK，平台命名空间，权限点 platform.mobileapp.mgr）───
//
// 单条 current 记录，上传即覆盖。公开 meta/下载（/mobile/app/latest|download）
// 在 lib/appDownload.ts 里以裸 fetch 调用（登录页未登录场景）。

export interface MobileAppReleaseView {
  version: string
  file_name: string
  size: number
  sha512?: string
  updated_by?: string
  updated_at: string
}

export async function getMobileAppRelease() {
  return apiClient
    .get('platform/mobile-app')
    .json<ApiResponse<{ mobile_app: MobileAppReleaseView | null }>>()
}

export async function uploadMobileAppRelease(file: File, version: string) {
  const form = new FormData()
  form.append('file', file)
  form.append('version', version)
  return apiClient
    .post('platform/mobile-app', { body: form })
    .json<ApiResponse<{ mobile_app: Omit<MobileAppReleaseView, 'sha512'> }>>()
}

export async function deleteMobileAppRelease() {
  return apiClient.delete('platform/mobile-app').json<ApiResponse<{ deleted: boolean }>>()
}
