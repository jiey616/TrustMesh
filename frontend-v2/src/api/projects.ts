import { apiClient } from './client'
import type { ApiResponse, ApiListResponse, Project, CreateProjectRequest, UpdateProjectRequest, WorkflowProgress, TaskArtifact } from '@/types'

export async function createProject(input: CreateProjectRequest) {
  return apiClient.post('projects', { json: input }).json<ApiResponse<Project>>()
}

export async function listProjects() {
  return apiClient.get('projects').json<ApiListResponse<Project>>()
}

export async function getProject(id: string) {
  return apiClient.get(`projects/${id}`).json<ApiResponse<Project>>()
}

export async function updateProject(id: string, input: UpdateProjectRequest) {
  return apiClient.patch(`projects/${id}`, { json: input }).json<ApiResponse<Project>>()
}

export async function archiveProject(id: string) {
  return apiClient.delete(`projects/${id}`).json<ApiResponse<Project>>()
}

// getWorkflowProgress returns the project's overall pipeline (总流程) progress.
export async function getWorkflowProgress(projectId: string) {
  return apiClient.get(`projects/${projectId}/workflow-progress`).json<ApiResponse<WorkflowProgress>>()
}
// bindStepOutput 把项目里任意文件绑定为总流程某个步骤的输出位交付物。
// 地址用 project + stepIndex，由后端解析承载任务与 todo，所以项目文件区、
// 流程条等任意入口都能直接调用，不必先知道该步骤此刻对应哪个 todoId。
// file_id 指向项目文件（含用户手工上传的），artifact_id 指向已有产物，二选一。
export async function bindStepOutput(
  projectId: string,
  stepIndex: number,
  input: { file_id?: string; artifact_id?: string; output_name: string; replace?: boolean },
) {
  return apiClient
    .post(`projects/${projectId}/workflow/steps/${stepIndex}/outputs/bind`, { json: input })
    .json<ApiResponse<TaskArtifact>>()
}
