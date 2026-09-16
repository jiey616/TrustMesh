import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as projectsApi from '@/api/projects'
import type { CreateProjectRequest, UpdateProjectRequest, WorkflowProgress } from '@/types'

// enabled 供 F2 门控使用：MainLayout 校准前传 false 挡住无头请求；默认 true 保持既有调用点零改动。
export function useProjects(enabled = true) {
  return useQuery({
    queryKey: ['projects'],
    queryFn: async () => {
      const res = await projectsApi.listProjects()
      return res.data.items
    },
    enabled,
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
export type BindStepOutputInput = {
  file_id?: string
  artifact_id?: string
  output_name: string
  replace?: boolean
}

// useBindStepOutput 手工把任意文件绑定为某个步骤的交付物（项目流程 · 手工绑定）。
export function useBindStepOutput(projectId: string | undefined) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ stepIndex, input }: { stepIndex: number; input: BindStepOutputInput }) =>
      projectsApi.bindStepOutput(projectId!, stepIndex, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['workflow-progress', projectId] })
      qc.invalidateQueries({ queryKey: ['project-files', projectId] })
      qc.invalidateQueries({ queryKey: ['project-files-browse', projectId] })
      qc.invalidateQueries({ queryKey: ['projects', projectId] })
    },
  })
}
