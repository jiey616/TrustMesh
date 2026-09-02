import { api } from './client'
import type {
  AddTaskCommentResult,
  AppendTaskMessageRequest,
  ApiResponse,
  ApiListResponse,
  CreatePlanningTaskRequest,
  RejectPlanRequest,
  TaskListItem,
  TaskDetail,
  TaskPriority,
  Event,
  Comment,
  ListProjectTasksQuery,
  Workflow,
} from '@/types'

export interface CreateTaskInput {
  title: string
  description: string
  priority?: TaskPriority
  assignee_agent_id: string
  file_ids?: string[]
  workflow?: Workflow
}

export async function createTask(projectId: string, input: CreateTaskInput) {
  return api.post(`projects/${projectId}/tasks`, { json: input }).json<ApiResponse<TaskDetail>>()
}

export async function createPlanningTask(projectId: string, input: CreatePlanningTaskRequest & { file_ids?: string[] }) {
  return api.post(`projects/${projectId}/tasks/planning`, { json: input }).json<ApiResponse<TaskDetail>>()
}

export async function createTaskFromText(projectId: string, content: string, agentId?: string, fileIds?: string[], workflow?: Workflow) {
  return api
    .post(`projects/${projectId}/tasks/from-text`, {
      json: { content, agent_id: agentId ?? '', file_ids: fileIds ?? [], workflow: workflow ?? undefined },
    })
    .json<ApiResponse<TaskDetail>>()
}

export async function listProjectTasks(projectId: string, query?: ListProjectTasksQuery) {
  const searchParams: Record<string, string> = {}
  if (query?.status) searchParams.status = query.status
  return api
    .get(`projects/${projectId}/tasks`, { searchParams })
    .json<ApiListResponse<TaskListItem>>()
}

export async function getTask(id: string) {
  return api.get(`tasks/${id}`).json<ApiResponse<TaskDetail>>()
}

export async function appendTaskMessage(taskId: string, input: AppendTaskMessageRequest) {
  return api.post(`tasks/${taskId}/messages`, { json: input }).json<ApiResponse<TaskDetail>>()
}

export async function listTaskEvents(id: string) {
  return api.get(`tasks/${id}/events`).json<ApiListResponse<Event>>()
}

export async function dispatchTodo(taskId: string, todoId: string) {
  return api.post(`tasks/${taskId}/todos/${todoId}/dispatch`).json<ApiResponse<TaskDetail>>()
}

export async function reviewTodo(taskId: string, todoId: string, action: 'approve' | 'reject', reason?: string) {
  return api.post(`tasks/${taskId}/todos/${todoId}/review`, { json: { action, reason: reason ?? '' } }).json<ApiResponse<TaskDetail>>()
}

export async function answerTodo(taskId: string, todoId: string, questionId: string, answer: string) {
  return api.post(`tasks/${taskId}/todos/${todoId}/answer`, { json: { question_id: questionId, answer } }).json<ApiResponse<{ question_id: string }>>()
}

export async function cancelTask(taskId: string, reason: string) {
  return api.post(`tasks/${taskId}/cancel`, { json: { reason } }).json<ApiResponse<TaskDetail>>()
}

export async function approvePlan(taskId: string) {
  return api.post(`tasks/${taskId}/approve`).json<ApiResponse<TaskDetail>>()
}

export async function rejectPlan(taskId: string, input: RejectPlanRequest) {
  return api.post(`tasks/${taskId}/reject`, { json: input }).json<ApiResponse<TaskDetail>>()
}

export async function listTaskComments(taskId: string) {
  return api.get(`tasks/${taskId}/comments`).json<ApiListResponse<Comment>>()
}

export interface AddTaskCommentInput {
  content: string
  todo_id?: string
  mentions?: Array<{ agent_id: string }>
}

export async function addTaskComment(taskId: string, input: AddTaskCommentInput) {
  return api
    .post(`tasks/${taskId}/comments`, {
      json: input,
    })
    .json<ApiResponse<AddTaskCommentResult>>()
}

export async function getTaskArtifactContent(taskId: string, transferId: string) {
  return api.get(`tasks/${taskId}/artifacts/${transferId}/content`).blob()
}

// ─── Dynamic TODO management ───

export interface AddTodoInput {
  title: string
  description?: string
  assignee_id: string
}

export interface InsertTodoInput {
  title: string
  description?: string
  assignee_id: string
  before_todo_id: string
}

export interface UpdateTodoInput {
  title?: string
  description?: string
  assignee_id?: string
}

export async function addTaskTodo(taskId: string, input: AddTodoInput) {
  return api.post(`tasks/${taskId}/todos`, { json: input }).json<ApiResponse<TaskDetail>>()
}

export async function insertTaskTodo(taskId: string, todoId: string, input: AddTodoInput) {
  return api.post(`tasks/${taskId}/todos/${todoId}/insert`, { json: input }).json<ApiResponse<TaskDetail>>()
}

export async function updateTaskTodo(taskId: string, todoId: string, input: UpdateTodoInput) {
  return api.patch(`tasks/${taskId}/todos/${todoId}`, { json: input }).json<ApiResponse<TaskDetail>>()
}

export async function removeTaskTodo(taskId: string, todoId: string) {
  return api.delete(`tasks/${taskId}/todos/${todoId}`).json<ApiResponse<TaskDetail>>()
}

export async function reorderTaskTodos(taskId: string, todoIds: string[]) {
  return api.put(`tasks/${taskId}/todos/reorder`, { json: { todo_ids: todoIds } }).json<ApiResponse<TaskDetail>>()
}
