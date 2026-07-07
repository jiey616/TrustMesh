import type { QueryClient } from '@tanstack/react-query'
import type { Agent, Project, ProjectTaskSummary, ProjectWorkStatus, TaskDetail, TaskStatus } from '@/types'
import { normalizeProject } from '@/lib/projects'

function clampNonNegative(value: number) {
  return value < 0 ? 0 : value
}

function incrementStatusCount(summary: ProjectTaskSummary, status: TaskStatus) {
  switch (status) {
    case 'planning':
    case 'pending':
      summary.pending_count += 1
      break
    case 'in_progress':
      summary.in_progress_count += 1
      break
    case 'done':
      summary.done_count += 1
      break
    case 'failed':
      summary.failed_count += 1
      break
    case 'canceled':
      summary.canceled_count += 1
      break
  }
}

function decrementStatusCount(summary: ProjectTaskSummary, status: TaskStatus) {
  switch (status) {
    case 'planning':
    case 'pending':
      summary.pending_count = clampNonNegative(summary.pending_count - 1)
      break
    case 'in_progress':
      summary.in_progress_count = clampNonNegative(summary.in_progress_count - 1)
      break
    case 'done':
      summary.done_count = clampNonNegative(summary.done_count - 1)
      break
    case 'failed':
      summary.failed_count = clampNonNegative(summary.failed_count - 1)
      break
    case 'canceled':
      summary.canceled_count = clampNonNegative(summary.canceled_count - 1)
      break
  }
}

function deriveProjectWorkStatus(project: Project, summary: ProjectTaskSummary): ProjectWorkStatus {
  if (project.status === 'archived') {
    return 'archived'
  }
  if (summary.task_total === 0) {
    return 'empty'
  }
  if (summary.in_progress_count > 0) {
    return 'running'
  }
  if (summary.failed_count > 0) {
    return 'attention'
  }
  if (summary.pending_count > 0) {
    return 'queued'
  }
  return 'idle'
}

function applyTaskSummaryChange(
  project: Project,
  task: TaskDetail,
  previousTaskStatus: TaskStatus | null,
  isNewTask: boolean
): Project {
  const normalizedProject = normalizeProject(project)
  const summary: ProjectTaskSummary = {
    ...normalizedProject.task_summary,
  }

  if (isNewTask) {
    summary.task_total += 1
    incrementStatusCount(summary, task.status)
  } else if (previousTaskStatus && previousTaskStatus !== task.status) {
    decrementStatusCount(summary, previousTaskStatus)
    incrementStatusCount(summary, task.status)
  }

  if (!summary.latest_task_at || summary.latest_task_at.localeCompare(task.updated_at) < 0) {
    summary.latest_task_at = task.updated_at
  }
  summary.work_status = deriveProjectWorkStatus(normalizedProject, summary)

  return {
    ...normalizedProject,
    task_summary: summary,
  }
}

function patchProjectCaches(
  queryClient: QueryClient,
  projectId: string,
  updater: (project: Project) => Project
) {
  queryClient.setQueryData<Project | undefined>(['projects', projectId], (project) => (
    project ? updater(project) : project
  ))

  queryClient.setQueryData<Project[] | undefined>(['projects'], (projects) => {
    // Guard: if the list cache is undefined or empty, don't overwrite.
    // During SSE-heavy periods, invalidateQueries may have cleared it temporarily.
    if (!projects || projects.length === 0) {
      return projects
    }
    return projects.map((project) => (project.id === projectId ? updater(project) : project))
  })
}

export function applyProjectTaskUpdated(
  queryClient: QueryClient,
  input: {
    task: TaskDetail
    previousTaskStatus: TaskStatus | null
    isNewTask: boolean
    hasEnoughContext: boolean
  }
) {
  const { task, previousTaskStatus, isNewTask, hasEnoughContext } = input

  if (!hasEnoughContext) {
    // Only invalidate the specific project detail — not the full project list.
    // The full list invalidation every time a task has insufficient context
    // can cause the project list to briefly disappear during frequent SSE events.
    void queryClient.invalidateQueries({ queryKey: ['projects', task.project_id] })
    return
  }

  patchProjectCaches(
    queryClient,
    task.project_id,
    (project) => applyTaskSummaryChange(project, task, previousTaskStatus, isNewTask)
  )
}

export function applyProjectAgentStatusChanged(queryClient: QueryClient, agent: Agent) {
  const updateProject = (project: Project) => {
    const normalizedProject = normalizeProject(project)
    if (normalizedProject.pm_agent.id !== agent.id) {
      return normalizedProject
    }

    return {
      ...normalizedProject,
      pm_agent: {
        id: agent.id,
        name: agent.name,
        node_id: agent.node_id,
        status: agent.status,
      },
    }
  }

  queryClient.setQueryData<Project[] | undefined>(['projects'], (projects) => (
    projects?.map(updateProject)
  ))

  const projectQueries = queryClient.getQueriesData<Project>({ queryKey: ['projects'] })
  for (const [queryKey, project] of projectQueries) {
    if (!Array.isArray(queryKey) || queryKey.length !== 2 || queryKey[0] !== 'projects' || typeof queryKey[1] !== 'string') {
      continue
    }
    const normalizedProject = project ? normalizeProject(project) : project
    if (!normalizedProject || normalizedProject.pm_agent.id !== agent.id) {
      continue
    }
    queryClient.setQueryData<Project>(queryKey, updateProject(normalizedProject))
  }
}
