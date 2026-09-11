import { apiClient } from './client'
import type { ApiResponse, ApiListResponse, DashboardStats, Event, TaskListItem } from '@/types'

export async function getDashboardStats() {
  return apiClient.get('dashboard/stats').json<ApiResponse<DashboardStats>>()
}

export async function getDashboardEvents(limit = 20) {
  return apiClient
    .get('dashboard/events', { searchParams: { limit: String(limit) } })
    .json<ApiListResponse<Event>>()
}

export async function getDashboardTasks(limit = 10) {
  return apiClient
    .get('dashboard/tasks', { searchParams: { limit: String(limit) } })
    .json<ApiListResponse<TaskListItem>>()
}
