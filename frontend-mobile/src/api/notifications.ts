import { apiClient } from './client'
import type { ApiListResponse, ApiResponse, Notification } from '@/types'

/** filter: 'recent' | 'unread'（后端约定，与桌面端一致） */
export async function listNotifications(filter = 'recent', limit = 50): Promise<Notification[]> {
  const res = await apiClient
    .get('notifications', { searchParams: { filter, limit: String(limit) } })
    .json<ApiListResponse<Notification>>()
  return res.data.items ?? []
}

export async function getUnreadCount(): Promise<number> {
  const res = await apiClient
    .get('notifications/unread-count')
    .json<ApiResponse<{ count: number }>>()
  return res.data.count ?? 0
}

export async function markNotificationRead(id: string): Promise<void> {
  await apiClient.patch(`notifications/${id}/read`)
}

export async function markAllNotificationsRead(): Promise<void> {
  await apiClient.post('notifications/mark-all-read')
}
