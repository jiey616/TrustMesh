import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as knowledgeApi from '@/api/knowledge'
import type { KnowledgeSearchRequest, UpdateKnowledgeDocRequest } from '@/types'

export function useKnowledgeDocs(params?: { project_id?: string; status?: string; tag?: string }) {
  return useQuery({
    queryKey: ['knowledge-docs', params],
    queryFn: async () => {
      const res = await knowledgeApi.listDocuments(params)
      return res.data.items
    },
  })
}

export function useKnowledgeDoc(id: string | undefined) {
  return useQuery({
    queryKey: ['knowledge-docs', id],
    queryFn: async () => {
      const res = await knowledgeApi.getDocument(id!)
      return res.data
    },
    enabled: !!id,
  })
}

export function useKnowledgeChunks(docId: string | undefined) {
  return useQuery({
    queryKey: ['knowledge-docs', docId, 'chunks'],
    queryFn: async () => {
      const res = await knowledgeApi.listChunks(docId!)
      return res.data.items
    },
    enabled: !!docId,
  })
}

export function useUploadDocument() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (formData: FormData) => knowledgeApi.uploadDocument(formData),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['knowledge-docs'] }),
  })
}

export function useUpdateDocument() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdateKnowledgeDocRequest }) =>
      knowledgeApi.updateDocument(id, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['knowledge-docs'] }),
  })
}

export function useDeleteDocument() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => knowledgeApi.deleteDocument(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['knowledge-docs'] }),
  })
}

export function useReprocessDocument() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => knowledgeApi.reprocessDocument(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['knowledge-docs'] }),
  })
}

export function useKnowledgeSearch() {
  return useMutation({
    mutationFn: (input: KnowledgeSearchRequest) => knowledgeApi.searchKnowledge(input),
  })
}
