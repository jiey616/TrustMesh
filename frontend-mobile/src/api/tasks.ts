import { apiClient } from './client'
import type {
  ApiListResponse,
  ApiResponse,
  Comment,
  TaskDetail,
  TaskEvent,
  UIBlockResponse,
} from '@/types'

export async function getTask(id: string): Promise<TaskDetail> {
  const res = await apiClient.get(`tasks/${id}`).json<ApiResponse<TaskDetail>>()
  return res.data
}

/** 一段话建任务（桌面端 from-text 同款接口）：PM agent 自行拆解规划，会真实触发生产流程。
 *  传 slice 时按项目总流程的步骤区间建任务（workflow_index + step_from/to，闭区间）。 */
export async function createTaskFromText(
  projectId: string,
  content: string,
  slice?: { workflowIndex: number; stepFrom: number; stepTo: number },
): Promise<TaskDetail> {
  const json: Record<string, unknown> = { content }
  if (slice) {
    json.workflow_index = slice.workflowIndex
    json.step_from = slice.stepFrom
    json.step_to = slice.stepTo
  }
  const res = await apiClient
    .post(`projects/${projectId}/tasks/from-text`, { json })
    .json<ApiResponse<TaskDetail>>()
  return res.data
}

export async function listTaskEvents(id: string): Promise<TaskEvent[]> {
  const res = await apiClient.get(`tasks/${id}/events`).json<ApiListResponse<TaskEvent>>()
  return res.data.items ?? []
}

export async function listTaskComments(taskId: string): Promise<Comment[]> {
  const res = await apiClient
    .get(`tasks/${taskId}/comments`)
    .json<ApiListResponse<Comment>>()
  return res.data.items ?? []
}

export async function addTaskComment(taskId: string, content: string): Promise<void> {
  await apiClient.post(`tasks/${taskId}/comments`, { json: { content } })
}

/** 规划澄清作答：把 ui_blocks 的答案连同正文一起回给 PM。 */
export async function appendTaskMessage(
  taskId: string,
  content: string,
  uiResponse?: { blocks: Record<string, UIBlockResponse> },
): Promise<void> {
  await apiClient.post(`tasks/${taskId}/messages`, {
    json: { content, ...(uiResponse ? { ui_response: uiResponse } : {}) },
  })
}

export async function approvePlan(taskId: string): Promise<void> {
  await apiClient.post(`tasks/${taskId}/approve`)
}

export async function rejectPlan(taskId: string, feedback: string): Promise<void> {
  await apiClient.post(`tasks/${taskId}/reject`, { json: { feedback } })
}

export async function reviewTodo(
  taskId: string,
  todoId: string,
  action: 'approve' | 'reject',
  reason?: string,
): Promise<void> {
  await apiClient.post(`tasks/${taskId}/todos/${todoId}/review`, {
    json: { action, reason: reason ?? '' },
  })
}

export async function answerTodo(
  taskId: string,
  todoId: string,
  questionId: string,
  answer: string,
): Promise<void> {
  await apiClient.post(`tasks/${taskId}/todos/${todoId}/answer`, {
    json: { question_id: questionId, answer },
  })
}

/**
 * 取交付物字节（**必须走 apiClient**：该接口挂在鉴权路由组上，
 * `<img src>` 直连带不上 Authorization ⇒ 恒 401）。
 */
export async function getArtifactBlob(taskId: string, transferId: string): Promise<Blob> {
  return apiClient.get(`tasks/${taskId}/artifacts/${transferId}/content`).blob()
}
