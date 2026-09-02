import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as projectsApi from '@/api/projects'
import type { CreateProjectRequest, UpdateProjectRequest, WorkflowProgress } from '@/types'

export function useProjects() {
  return useQuery({
    queryKey: ['projects'],
    queryFn: async () => {
      const res = await projectsApi.listProjects()
      return res.data.items
    },
  })
}

// useWorkflowProgress fetches the project's overall pipeline (总流程) progress.
export function useWorkflowProgress(projectId: string | undefined) {
  return useQuery({
    queryKey: ['workflow-progress', projectId],
    queryFn: async (): Promise<WorkflowProgress> => {
      const res = await projectsApi.getWorkflowProgress(projectId!)
      return res.data
    },
    enabled: !!projectId,
    staleTime: 15_000,
    refetchInterval: 30_000,
  })
}

export function useProject(id: string | undefined) {
  return useQuery({
    queryKey: ['projects', id],
    queryFn: async () => {
      const res = await projectsApi.getProject(id!)
      return res.data
    },
    enabled: !!id,
  })
}

export function useCreateProject() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateProjectRequest) => projectsApi.createProject(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['projects'] }),
  })
}

export function useUpdateProject() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdateProjectRequest }) =>
      projectsApi.updateProject(id, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['projects'] }),
  })
}

export function useArchiveProject() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => projectsApi.archiveProject(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['projects'] }),
  })
}
