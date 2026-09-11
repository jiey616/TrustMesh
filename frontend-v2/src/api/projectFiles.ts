import { apiClient } from './client'
import type { ApiResponse, ApiListResponse, ProjectFile, BatchDeleteResult, BrowseFilesResult, CreateProjectFolderRequest } from '@/types'

export async function uploadProjectFile(projectId: string, formData: FormData) {
  return apiClient.post(`projects/${projectId}/files`, { body: formData }).json<ApiResponse<ProjectFile>>()
}

export async function listProjectFiles(projectId: string) {
  return apiClient.get(`projects/${projectId}/files`).json<ApiListResponse<ProjectFile>>()
}

/** 按目录浏览文件（根目录 parent_id 传空字符串） */
export async function browseProjectFiles(projectId: string, parentId: string) {
  return apiClient
    .get(`projects/${projectId}/files/browse`, { searchParams: { parent_id: parentId } })
    .json<ApiResponse<BrowseFilesResult>>()
}

export async function createProjectFolder(projectId: string, input: CreateProjectFolderRequest) {
  return apiClient.post(`projects/${projectId}/folders`, { json: input }).json<ApiResponse<ProjectFile>>()
}

export async function renameProjectFile(projectId: string, fileId: string, name: string) {
  return apiClient.patch(`projects/${projectId}/files/${fileId}/rename`, { json: { name } }).json<ApiResponse<ProjectFile>>()
}

export async function deleteProjectFile(projectId: string, fileId: string) {
  return apiClient.delete(`projects/${projectId}/files/${fileId}`).json<ApiResponse<ProjectFile>>()
}

export async function downloadProjectFile(projectId: string, fileId: string): Promise<Blob> {
  const res = await apiClient.get(`projects/${projectId}/files/${fileId}/content`)
  return res.blob()
}

export async function batchDeleteProjectFiles(projectId: string, fileIds: string[]) {
  return apiClient
    .post(`projects/${projectId}/files/batch-delete`, { json: { file_ids: fileIds } })
    .json<ApiResponse<BatchDeleteResult>>()
}
