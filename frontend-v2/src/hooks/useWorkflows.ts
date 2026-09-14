import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as workflowsApi from '@/api/workflows'
import type { WorkflowSyncDiff, WorkflowStep } from '@/types'

type TemplateInput = { name: string; description?: string; steps: WorkflowStep[] }

// ---------- 全局工作流模板 ----------

export function useWorkflowTemplates() {
  return useQuery({
    queryKey: ['workflow-templates'],
    queryFn: async () => {
      const res = await workflowsApi.listWorkflowTemplates()
      return res.data.items
    },
  })
}

export function useWorkflowTemplate(id: string | undefined) {
  return useQuery({
    queryKey: ['workflow-templates', id],
    queryFn: async () => {
      const res = await workflowsApi.getWorkflowTemplate(id!)
      return res.data
    },
    enabled: !!id,
  })
}

export function useCreateWorkflowTemplate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: TemplateInput) => workflowsApi.createWorkflowTemplate(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['workflow-templates'] }),
  })
}

export function useUpdateWorkflowTemplate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: TemplateInput }) =>
      workflowsApi.updateWorkflowTemplate(id, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['workflow-templates'] }),
  })
}

export function useCopyWorkflowTemplate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => workflowsApi.copyWorkflowTemplate(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['workflow-templates'] }),
  })
}

export function useDeleteWorkflowTemplate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => workflowsApi.deleteWorkflowTemplate(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['workflow-templates'] }),
  })
}

export function useCurateWorkflowTemplate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, curated }: { id: string; curated: boolean }) =>
      workflowsApi.curateWorkflowTemplate(id, curated),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['workflow-templates'] }),
  })
}

// ---------- 项目继承 / 同步 ----------

export function useInheritWorkflowTemplate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ projectId, templateId }: { projectId: string; templateId: string }) =>
      workflowsApi.inheritWorkflowTemplate(projectId, templateId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['projects'] }),
  })
}

export function useWorkflowSyncDiff(projectId: string | undefined, workflowId: string | undefined) {
  return useQuery({
    queryKey: ['workflow-sync-diff', projectId, workflowId],
    queryFn: async (): Promise<WorkflowSyncDiff> => {
      const res = await workflowsApi.getWorkflowSyncDiff(projectId!, workflowId!)
      return res.data
    },
    enabled: !!projectId && !!workflowId,
  })
}

export function useApplyWorkflowSync() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ projectId, workflowId, removeSteps }: { projectId: string; workflowId: string; removeSteps: string[] }) =>
      workflowsApi.applyWorkflowSync(projectId, workflowId, removeSteps),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['projects'] }),
  })
}

export function useDetachWorkflowTemplate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ projectId, workflowId }: { projectId: string; workflowId: string }) =>
      workflowsApi.detachWorkflowTemplate(projectId, workflowId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['projects'] }),
  })
}
