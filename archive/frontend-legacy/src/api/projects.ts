import { api } from './client'
import type {
  ApiResponse,
  ApiListResponse,
  Project,
  CreateProjectRequest,
  UpdateProjectRequest,
} from '@/types'

export async function createProject(input: CreateProjectRequest) {
  return api.post('projects', { json: input }).json<ApiResponse<Project>>()
}

export async function listProjects() {
  return api.get('projects').json<ApiListResponse<Project>>()
}

export async function getProject(id: string) {
  return api.get(`projects/${id}`).json<ApiResponse<Project>>()
}

export async function updateProject(id: string, input: UpdateProjectRequest) {
  return api.patch(`projects/${id}`, { json: input }).json<ApiResponse<Project>>()
}

export async function archiveProject(id: string) {
  return api.delete(`projects/${id}`).json<ApiResponse<Project>>()
}
