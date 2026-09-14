import type { Project, ProjectTaskSummary } from '@/types'

export function getEmptyProjectTaskSummary(): ProjectTaskSummary {
  return {
    task_total: 0,
    pending_count: 0,
    in_progress_count: 0,
    done_count: 0,
    failed_count: 0,
    canceled_count: 0,
    work_status: 'empty',
    latest_task_at: null,
  }
}

export function normalizeProject(project: Project): Project {
  return {
    ...project,
    task_summary: project.task_summary ?? getEmptyProjectTaskSummary(),
  }
}
