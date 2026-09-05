import { apiClient } from './client'
import type { ApiResponse, User } from '@/types'

// ─── 当前账号 API（/api/v1/users/me） ───
// 账号是 user 维度资源，与当前工作区（X-Org-Id）无关。

export async function getMe() {
  return apiClient.get('/api/v1/users/me').json<ApiResponse<User>>()
}

export async function updateProfile(input: { name: string }) {
  return apiClient.patch('/api/v1/users/me', { json: input }).json<ApiResponse<User>>()
}

export async function changePassword(input: { old_password: string; new_password: string }) {
  return apiClient
    .post('/api/v1/users/me/password', { json: input })
    .json<ApiResponse<{ ok: boolean }>>()
}
