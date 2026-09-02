import { apiClient } from './client'
import type { ApiResponse, ApiListResponse, DashboardStats, Event, TaskListItem } from '@/types'

export async function getDashboardStats() {
  return apiClient.get('/api/v1/dashboard/stats').json<ApiResponse<DashboardStats>>()
}

export async function getDashboardEvents(limit = 20) {
  return apiClient
    .get('/api/v1/dashboard/events', { searchParams: { limit: String(limit) } })
    .json<ApiListResponse<Event>>()
}

export async function getDashboardTasks(limit = 10) {
  return apiClient
    .get('/api/v1/dashboard/tasks', { searchParams: { limit: String(limit) } })
    .json<ApiListResponse<TaskListItem>>()
}
