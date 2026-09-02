import { apiClient } from './client'
import type { ApiResponse, AuthSuccessData, AuthLoginRequest, AuthRegisterRequest } from '@/types'

export async function login(data: AuthLoginRequest): Promise<AuthSuccessData> {
  const res = await apiClient.post('/api/v1/auth/login', { json: data }).json<ApiResponse<AuthSuccessData>>()
  return res.data
}

export async function register(data: AuthRegisterRequest): Promise<AuthSuccessData> {
  const res = await apiClient.post('/api/v1/auth/register', { json: data }).json<ApiResponse<AuthSuccessData>>()
  return res.data
}

export async function refreshToken(refreshToken: string): Promise<{ access_token: string; refresh_token: string }> {
  const res = await apiClient
    .post('/api/v1/auth/refresh', { json: { refresh_token: refreshToken } })
    .json<ApiResponse<{ access_token: string; refresh_token: string }>>()
  return res.data
}
