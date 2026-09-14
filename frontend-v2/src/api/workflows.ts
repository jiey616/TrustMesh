import { apiClient } from './client'
import type {
  ApiResponse,
  ApiListResponse,
  Project,
  WorkflowTemplate,
  WorkflowSyncDiff,
} from '@/types'

// ---------- 全局工作流模板 ----------

export async function listWorkflowTemplates() {
  return apiClient.get('workflow-templates').json<ApiListResponse<WorkflowTemplate>>()
}

export async function getWorkflowTemplate(id: string) {
  return apiClient.get(`workflow-templates/${id}`).json<ApiResponse<WorkflowTemplate>>()
}

export async function createWorkflowTemplate(input: {
  name: string
  description?: string
  steps: WorkflowTemplate['steps']
}) {
  return apiClient.post('workflow-templates', { json: input }).json<ApiResponse<WorkflowTemplate>>()
}

export async function updateWorkflowTemplate(
  id: string,
  input: { name: string; description?: string; steps: WorkflowTemplate['steps'] },
) {
  return apiClient.patch(`workflow-templates/${id}`, { json: input }).json<ApiResponse<WorkflowTemplate>>()
}

export async function copyWorkflowTemplate(id: string) {
  return apiClient.post(`workflow-templates/${id}/copy`).json<ApiResponse<WorkflowTemplate>>()
}

export async function deleteWorkflowTemplate(id: string) {
  return apiClient.delete(`workflow-templates/${id}`).json<ApiResponse<WorkflowTemplate>>()
}

// ---------- 策展标记（T1.9） ----------

export async function curateWorkflowTemplate(id: string, curated: boolean) {
  return apiClient
    .post(`workflow-templates/${id}/curate`, { json: { curated } })
    .json<ApiResponse<WorkflowTemplate>>()
}

// ---------- 项目继承 / 同步 ----------

export async function inheritWorkflowTemplate(projectId: string, templateId: string) {
  return apiClient
    .post(`projects/${projectId}/workflows/inherit`, { json: { template_id: templateId } })
    .json<ApiResponse<Project>>()
}

export async function getWorkflowSyncDiff(projectId: string, workflowId: string) {
  return apiClient
    .get(`projects/${projectId}/workflows/${workflowId}/sync-diff`)
    .json<ApiResponse<WorkflowSyncDiff>>()
}

export async function applyWorkflowSync(projectId: string, workflowId: string, removeSteps: string[]) {
  return apiClient
    .post(`projects/${projectId}/workflows/${workflowId}/sync`, { json: { remove_steps: removeSteps } })
    .json<ApiResponse<Project>>()
}

export async function detachWorkflowTemplate(projectId: string, workflowId: string) {
  return apiClient
    .post(`projects/${projectId}/workflows/${workflowId}/detach`)
    .json<ApiResponse<Project>>()
}
