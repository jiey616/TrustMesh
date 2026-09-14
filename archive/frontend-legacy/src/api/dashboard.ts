import { api } from './client'
import type {
  ApiResponse,
  ApiListResponse,
  DashboardStats,
  Event,
  TaskListItem,
} from '@/types'

export async function getDashboardStats() {
  return api.get('dashboard/stats').json<ApiResponse<DashboardStats>>()
}

export async function getDashboardEvents(limit = 20) {
  return api
    .get('dashboard/events', { searchParams: { limit: String(limit) } })
    .json<ApiListResponse<Event>>()
}

export async function getDashboardTasks(limit = 10) {
  return api
    .get('dashboard/tasks', { searchParams: { limit: String(limit) } })
    .json<ApiListResponse<TaskListItem>>()
}

export async function getAgentEvents(agentId: string, limit = 50) {
  return api
    .get(`agents/${agentId}/events`, { searchParams: { limit: String(limit) } })
    .json<ApiListResponse<Event>>()
}
