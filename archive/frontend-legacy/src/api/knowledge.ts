import { api } from './client'
import type {
  ApiResponse,
  ApiListResponse,
  KnowledgeDocument,
  KnowledgeChunk,
  KnowledgeSearchResult,
  KnowledgeSearchRequest,
  UpdateKnowledgeDocRequest,
} from '@/types'

export async function uploadDocument(formData: FormData) {
  return api.post('knowledge/documents', { body: formData }).json<ApiResponse<KnowledgeDocument>>()
}

export async function listDocuments(params?: { project_id?: string; status?: string; tag?: string }) {
  const searchParams = new URLSearchParams()
  if (params?.project_id) searchParams.set('project_id', params.project_id)
  if (params?.status) searchParams.set('status', params.status)
  if (params?.tag) searchParams.set('tag', params.tag)
  const query = searchParams.toString()
  return api.get(`knowledge/documents${query ? `?${query}` : ''}`).json<ApiListResponse<KnowledgeDocument>>()
}

export async function getDocument(id: string) {
  return api.get(`knowledge/documents/${id}`).json<ApiResponse<KnowledgeDocument>>()
}

export async function updateDocument(id: string, input: UpdateKnowledgeDocRequest) {
  return api.patch(`knowledge/documents/${id}`, { json: input }).json<ApiResponse<KnowledgeDocument>>()
}

export async function deleteDocument(id: string) {
  await api.delete(`knowledge/documents/${id}`)
}

export async function listChunks(docId: string) {
  return api.get(`knowledge/documents/${docId}/chunks`).json<ApiListResponse<KnowledgeChunk>>()
}

export async function reprocessDocument(id: string) {
  return api.post(`knowledge/documents/${id}/reprocess`).json<ApiResponse<{ status: string }>>()
}

export async function searchKnowledge(input: KnowledgeSearchRequest) {
  return api.post('knowledge/search', { json: input }).json<ApiListResponse<KnowledgeSearchResult>>()
}
