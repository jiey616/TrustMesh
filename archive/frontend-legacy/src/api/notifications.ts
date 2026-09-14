import { api } from './client'
import type { ApiResponse, ApiListResponse, Notification } from '@/types'

export async function listNotifications(filter = 'recent', limit = 50) {
  return api
    .get('notifications', { searchParams: { filter, limit: String(limit) } })
    .json<ApiListResponse<Notification>>()
}

export async function getUnreadCount() {
  return api.get('notifications/unread-count').json<ApiResponse<{ count: number }>>()
}

export async function markNotificationRead(id: string) {
  return api.patch(`notifications/${id}/read`).json<ApiResponse<{ status: string }>>()
}

export async function markAllNotificationsRead() {
  return api.post('notifications/mark-all-read').json<ApiResponse<{ marked: number }>>()
}
