import { apiClient } from './client'
import type { ApiResponse, ApiListResponse, Notification } from '@/types'

export async function listNotifications(filter = 'recent', limit = 50) {
  return apiClient
    .get('notifications', { searchParams: { filter, limit: String(limit) } })
    .json<ApiListResponse<Notification>>()
}

export async function getUnreadCount() {
  return apiClient.get('notifications/unread-count').json<ApiResponse<{ count: number }>>()
}

export async function markNotificationRead(id: string) {
  return apiClient.patch(`notifications/${id}/read`).json<ApiResponse<{ status: string }>>()
}

export async function markAllNotificationsRead() {
  return apiClient.post('notifications/mark-all-read').json<ApiResponse<{ marked: number }>>()
}
