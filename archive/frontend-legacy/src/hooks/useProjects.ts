import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as projectsApi from '@/api/projects'
import type { CreateProjectRequest, Project, UpdateProjectRequest } from '@/types'
import { normalizeProject } from '@/lib/projects'

export function useProjects() {
  return useQuery({
    queryKey: ['projects'],
    queryFn: async () => {
      const res = await projectsApi.listProjects()
      return res.data.items.map(normalizeProject)
    },
  })
}

export function useProject(id: string | undefined) {
  return useQuery({
    queryKey: ['projects', id],
    queryFn: async () => {
      const res = await projectsApi.getProject(id!)
      return normalizeProject(res.data)
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
    onSuccess: (res, { id }) => {
      const updatedProject = normalizeProject(res.data)
      qc.setQueryData<Project | undefined>(['projects', id], updatedProject)
      qc.setQueryData<Project[] | undefined>(['projects'], (projects) =>
        projects?.map((project) => (project.id === updatedProject.id ? updatedProject : project)),
      )
      qc.invalidateQueries({ queryKey: ['projects'] })
    },
  })
}

export function useArchiveProject() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => projectsApi.archiveProject(id),
    onSuccess: (res, id) => {
      const archivedProject = normalizeProject(res.data)
      qc.setQueryData<Project | undefined>(['projects', id], archivedProject)
      qc.setQueryData<Project[] | undefined>(['projects'], (projects) =>
        projects?.map((project) => (project.id === archivedProject.id ? archivedProject : project)),
      )
      qc.invalidateQueries({ queryKey: ['projects'] })
    },
  })
}
