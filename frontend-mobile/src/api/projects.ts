import { apiClient } from './client'
import type {
  ApiListResponse,
  ApiResponse,
  ProjectView,
  TaskListItem,
  TaskStatus,
  WorkflowProgress,
} from '@/types'

export async function listProjects(): Promise<ProjectView[]> {
  const res = await apiClient.get('projects').json<ApiListResponse<ProjectView>>()
  return res.data.items ?? []
}

/** 项目总流程流水线进度（每步状态 + 关联任务 + 步骤产出）。 */
export async function getWorkflowProgress(projectId: string): Promise<WorkflowProgress> {
  const res = await apiClient
    .get(`projects/${projectId}/workflow-progress`)
    .json<ApiResponse<WorkflowProgress>>()
  return res.data
}

export async function listProjectTasks(
  projectId: string,
  status?: TaskStatus,
): Promise<TaskListItem[]> {
  const searchParams: Record<string, string> = {}
  if (status) searchParams.status = status
  const res = await apiClient
    .get(`projects/${projectId}/tasks`, { searchParams })
    .json<ApiListResponse<TaskListItem>>()
  return res.data.items ?? []
}
