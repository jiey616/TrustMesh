import { apiClient } from './client'
import type { ApiResponse, AuthSuccessData } from '@/types'

export async function login(input: { email: string; password: string }): Promise<AuthSuccessData> {
  const res = await apiClient
    .post('auth/login', { json: input })
    .json<ApiResponse<AuthSuccessData>>()
  return res.data
}

export async function refresh(refreshToken: string) {
  const res = await apiClient
    .post('auth/refresh', { json: { refresh_token: refreshToken } })
    .json<ApiResponse<{ access_token: string; refresh_token: string }>>()
  return res.data
}
