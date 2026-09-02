import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as projectFilesApi from '@/api/projectFiles'
import type { CreateProjectFolderRequest } from '@/types'

export function useProjectFiles(projectId: string | undefined) {
  return useQuery({
    queryKey: ['project-files', projectId],
    queryFn: async () => {
      const res = await projectFilesApi.listProjectFiles(projectId!)
      return res.data.items
    },
    enabled: !!projectId,
  })
}

/** 按目录浏览（parentId 为 '' 表示根目录） */
export function useBrowseProjectFiles(projectId: string | undefined, parentId: string) {
  return useQuery({
    queryKey: ['project-files-browse', projectId, parentId],
    queryFn: async () => {
      const res = await projectFilesApi.browseProjectFiles(projectId!, parentId)
      return res.data
    },
    enabled: !!projectId,
  })
}

export function useUploadProjectFile(projectId: string | undefined, parentId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (formData: FormData) => projectFilesApi.uploadProjectFile(projectId!, formData),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['project-files-browse', projectId, parentId] })
      qc.invalidateQueries({ queryKey: ['project-files', projectId] })
    },
  })
}

export function useCreateProjectFolder(projectId: string | undefined, parentId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateProjectFolderRequest) => projectFilesApi.createProjectFolder(projectId!, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['project-files-browse', projectId, parentId] })
      qc.invalidateQueries({ queryKey: ['project-files', projectId] })
    },
  })
}

export function useRenameProjectFile(projectId: string | undefined, parentId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ fileId, name }: { fileId: string; name: string }) =>
      projectFilesApi.renameProjectFile(projectId!, fileId, name),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['project-files-browse', projectId, parentId] })
      qc.invalidateQueries({ queryKey: ['project-files', projectId] })
    },
  })
}

export function useDeleteProjectFile(projectId: string | undefined, parentId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (fileId: string) => projectFilesApi.deleteProjectFile(projectId!, fileId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['project-files-browse', projectId, parentId] })
      qc.invalidateQueries({ queryKey: ['project-files', projectId] })
    },
  })
}

export function useBatchDeleteProjectFiles(projectId: string | undefined) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (fileIds: string[]) => projectFilesApi.batchDeleteProjectFiles(projectId!, fileIds),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['project-files', projectId] }),
  })
}
