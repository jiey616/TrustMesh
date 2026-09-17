import { apiClient } from './client'
import type { ApiResponse, MeResponse, User } from '@/types'

// ─── 当前账号 API（users/me） ───
// 账号是 user 维度资源，与当前工作区（X-Org-Id）无关；
// 但 /users/me 额外回传的权限视图（permissions/menu_overrides）**按 X-Org-Id 解析**。

export async function getMe() {
  return apiClient.get('users/me').json<ApiResponse<MeResponse>>()
}

// 后端 PATCH /users/me 返回 {"data":{"user":{...}}}（与 GET /users/me 的 {user, permissions, ...} 信封一致）。
// 🔴 必须在 API 边界解包：曾把 `data` 直接当 `User` 用，导致 authStore.user 被写成 {user:{...}}，
// user.id / user.name 变 undefined → 工作区校准 resolveWorkspaceTarget 恒回落个人空间（切空间失效），
// 且脏对象经 persist 落盘、跨刷新存活。类型与真实契约必须一致。
export async function updateProfile(input: { name: string }): Promise<User> {
  const res = await apiClient
    .patch('users/me', { json: input })
    .json<ApiResponse<{ user: User }>>()
  return res.data.user
}

export async function changePassword(input: { old_password: string; new_password: string }) {
  return apiClient
    .post('users/me/password', { json: input })
    .json<ApiResponse<{ ok: boolean }>>()
}
