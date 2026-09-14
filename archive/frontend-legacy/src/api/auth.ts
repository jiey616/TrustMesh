import { api } from './client'
import ky from 'ky'
import { apiBaseUrl } from '@/lib/apiBase'
import type {
  ApiResponse,
  AuthRegisterRequest,
  AuthLoginRequest,
  AuthSuccessData,
  RefreshSuccessData,
} from '@/types'

export async function register(input: AuthRegisterRequest) {
  return api.post('auth/register', { json: input }).json<ApiResponse<AuthSuccessData>>()
}

export async function login(input: AuthLoginRequest) {
  return api.post('auth/login', { json: input }).json<ApiResponse<AuthSuccessData>>()
}

// Use a standalone ky instance to avoid the interceptor loop in client.ts
export async function refresh(refreshToken: string) {
  return ky
    .post('auth/refresh', {
      prefixUrl: apiBaseUrl,
      json: { refresh_token: refreshToken },
    })
    .json<ApiResponse<RefreshSuccessData>>()
}
