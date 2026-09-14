import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as projectFilesApi from '@/api/projectFiles'
import type {
  ListProjectFilesQuery,
  CreateProjectFolderRequest,
} from '@/types'

export function useProjectFiles(projectId: string | undefined, params?: ListProjectFilesQuery) {
  return useQuery({
    queryKey: ['project-files', projectId, params],
    queryFn: async () => {
      const res = await projectFilesApi.listProjectFiles(projectId!, params)
      return res.data.items
    },
    enabled: !!projectId,
  })
}

export function useProjectFileTree(projectId: string | undefined) {
  return useQuery({
    queryKey: ['project-files', projectId, 'tree'],
    queryFn: async () => {
      const res = await projectFilesApi.getProjectFileTree(projectId!)
      return res.data
    },
    enabled: !!projectId,
  })
}

export function useUploadProjectFile(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (formData: FormData) => projectFilesApi.uploadProjectFile(projectId, formData),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['project-files', projectId] })
    },
  })
}

export function useCreateProjectFolder(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: CreateProjectFolderRequest) => projectFilesApi.createProjectFolder(projectId, req),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['project-files', projectId] })
    },
  })
}

export function useDeleteProjectFile(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (fileId: string) => projectFilesApi.deleteProjectFile(projectId, fileId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['project-files', projectId] })
    },
  })
}

export function useRenameProjectFile(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ fileId, name }: { fileId: string; name: string }) =>
      projectFilesApi.renameProjectFile(projectId, fileId, { name }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['project-files', projectId] })
    },
  })
}

export function useMoveProjectFile(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ fileId, parentId }: { fileId: string; parentId?: string }) =>
      projectFilesApi.moveProjectFile(projectId, fileId, { parent_id: parentId }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['project-files', projectId] })
    },
  })
}

export function useBatchDeleteProjectFiles(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (ids: string[]) => projectFilesApi.batchDeleteProjectFiles(projectId, { ids }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['project-files', projectId] })
    },
  })
}

// ─── v2 browse / artifacts ───

export function useBrowseFiles(projectId: string | undefined, parentId?: string) {
  return useQuery({
    queryKey: ['project-files', projectId, 'browse', parentId ?? 'root'],
    queryFn: async () => {
      const res = await projectFilesApi.browseProjectFiles(projectId!, parentId)
      return res.data
    },
    enabled: !!projectId,
  })
}

export function useProjectArtifacts(projectId: string | undefined) {
  return useQuery({
    queryKey: ['project-files', projectId, 'artifacts'],
    queryFn: async () => {
      const res = await projectFilesApi.listProjectArtifacts(projectId!)
      return res.data
    },
    enabled: !!projectId,
  })
}
