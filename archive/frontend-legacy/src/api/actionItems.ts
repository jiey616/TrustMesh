import { api } from './client'
import type { ActionItemRefDTO } from '@/types'

export async function listActionItems(projectId: string) {
  return api
    .get('action-items', { searchParams: { project_id: projectId, status: 'awaiting_confirmation' } })
    .json<{ data: { items: ActionItemRefDTO[]; total: number } }>()
}

export async function convertActionItems(projectId: string, itemKeys: string[], assignees: Record<string, string> = {}) {
  return api
    .post('action-items/convert', {
      searchParams: { project_id: projectId },
      json: { item_ids: itemKeys, assignees },
    })
    .json<{ data: { created_task_ids: string[]; count: number } }>()
}
