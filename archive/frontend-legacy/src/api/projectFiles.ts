import { api } from './client'
import type {
  ApiResponse,
  ApiListResponse,
  ProjectFile,
  ProjectFileTree,
  ListProjectFilesQuery,
  CreateProjectFolderRequest,
  RenameProjectFileRequest,
  MoveProjectFileRequest,
  BatchDeleteProjectFilesRequest,
  BatchDeleteResult,
  BrowseFilesResult,
  ArtifactTaskGroup,
} from '@/types'

export async function uploadProjectFile(projectId: string, formData: FormData) {
  return api.post(`projects/${projectId}/files`, { body: formData }).json<ApiResponse<ProjectFile>>()
}

export async function listProjectFiles(projectId: string, params?: ListProjectFilesQuery) {
  const searchParams = new URLSearchParams()
  if (params?.source) searchParams.set('source', params.source)
  if (params?.task_id) searchParams.set('task_id', params.task_id)
  if (params?.agent_id) searchParams.set('agent_id', params.agent_id)
  const query = searchParams.toString()
  return api.get(`projects/${projectId}/files${query ? `?${query}` : ''}`).json<ApiListResponse<ProjectFile>>()
}

export async function getProjectFileTree(projectId: string) {
  return api.get(`projects/${projectId}/files/tree`).json<ApiResponse<ProjectFileTree>>()
}

export async function createProjectFolder(projectId: string, req: CreateProjectFolderRequest) {
  return api.post(`projects/${projectId}/folders`, { json: req }).json<ApiResponse<ProjectFile>>()
}

export async function deleteProjectFile(projectId: string, fileId: string) {
  return api.delete(`projects/${projectId}/files/${fileId}`).json<ApiResponse<ProjectFile>>()
}

export async function downloadProjectFile(projectId: string, fileId: string): Promise<Blob> {
  const res = await api.get(`projects/${projectId}/files/${fileId}/content`)
  return res.blob()
}

export async function renameProjectFile(projectId: string, fileId: string, req: RenameProjectFileRequest) {
  return api.patch(`projects/${projectId}/files/${fileId}/rename`, { json: req }).json<ApiResponse<ProjectFile>>()
}

export async function moveProjectFile(projectId: string, fileId: string, req: MoveProjectFileRequest) {
  return api.patch(`projects/${projectId}/files/${fileId}/move`, { json: req }).json<ApiResponse<ProjectFile>>()
}

export async function batchDeleteProjectFiles(projectId: string, req: BatchDeleteProjectFilesRequest) {
  return api.post(`projects/${projectId}/files/batch-delete`, { json: req }).json<ApiResponse<BatchDeleteResult>>()
}

// ─── v2 browse / artifacts ───

export async function browseProjectFiles(projectId: string, parentId?: string) {
  const params = parentId ? `?parent_id=${encodeURIComponent(parentId)}` : ''
  return api.get(`projects/${projectId}/files/browse${params}`).json<ApiResponse<BrowseFilesResult>>()
}

export async function listProjectArtifacts(projectId: string) {
  return api.get(`projects/${projectId}/files/artifacts`).json<ApiResponse<ArtifactTaskGroup[]>>()
}
