import { apiClient } from './client'
import type { ApiResponse, ApiListResponse, Project, CreateProjectRequest, UpdateProjectRequest, WorkflowProgress } from '@/types'

export async function createProject(input: CreateProjectRequest) {
  return apiClient.post('/api/v1/projects', { json: input }).json<ApiResponse<Project>>()
}

export async function listProjects() {
  return apiClient.get('/api/v1/projects').json<ApiListResponse<Project>>()
}

export async function getProject(id: string) {
  return apiClient.get(`/api/v1/projects/${id}`).json<ApiResponse<Project>>()
}

export async function updateProject(id: string, input: UpdateProjectRequest) {
  return apiClient.patch(`/api/v1/projects/${id}`, { json: input }).json<ApiResponse<Project>>()
}

export async function archiveProject(id: string) {
  return apiClient.delete(`/api/v1/projects/${id}`).json<ApiResponse<Project>>()
}

// getWorkflowProgress returns the project's overall pipeline (总流程) progress.
export async function getWorkflowProgress(projectId: string) {
  return apiClient.get(`/api/v1/projects/${projectId}/workflow-progress`).json<ApiResponse<WorkflowProgress>>()
}
