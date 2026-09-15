import { apiClient } from './client'
import type { ApiResponse, ApiListResponse, TaskDetail, TaskListItem, TaskPriority, Workflow, Event, Comment, AppendTaskMessageRequest, AddTaskCommentInput, TaskArtifact, ChatAttachment, WorkflowTemplate } from '@/types'

export interface CreateTaskInput {
  title: string
  description: string
  priority?: TaskPriority
  assignee_agent_id: string
  file_ids?: string[]
  workflow?: Workflow
  // Create against a slice of the project's primary workflow (总流程).
  workflow_index?: number
  step_from?: number
  step_to?: number
}

export async function createTask(projectId: string, input: CreateTaskInput) {
  return apiClient.post(`projects/${projectId}/tasks`, { json: input }).json<ApiResponse<TaskDetail>>()
}

export async function createTaskFromText(
  projectId: string,
  input: { content: string; agent_id?: string; file_ids?: string[]; workflow?: Workflow; workflow_index?: number; step_from?: number; step_to?: number; attachments?: ChatAttachment[] },
) {
  return apiClient
    .post(`projects/${projectId}/tasks/from-text`, { json: input })
    .json<ApiResponse<TaskDetail>>()
}

export async function listProjectTasks(projectId: string, status?: string) {
  const searchParams: Record<string, string> = {}
  if (status) searchParams.status = status
  return apiClient.get(`projects/${projectId}/tasks`, { searchParams }).json<ApiListResponse<TaskListItem>>()
}

export async function getTask(id: string) {
  return apiClient.get(`tasks/${id}`).json<ApiResponse<TaskDetail>>()
}

export async function appendTaskMessage(taskId: string, input: AppendTaskMessageRequest) {
  return apiClient.post(`tasks/${taskId}/messages`, { json: input }).json<ApiResponse<TaskDetail>>()
}

export async function listTaskEvents(id: string) {
  return apiClient.get(`tasks/${id}/events`).json<ApiListResponse<Event>>()
}

export async function listTaskComments(taskId: string) {
  return apiClient.get(`tasks/${taskId}/comments`).json<ApiListResponse<Comment>>()
}

export async function addTaskComment(taskId: string, input: AddTaskCommentInput) {
  return apiClient
    .post(`tasks/${taskId}/comments`, { json: input })
    .json<ApiResponse<{ id: string; content: string; mention_deliveries?: Array<{ agent_id: string; agent_name: string; status: string }> }>>()
}

export async function cancelTask(taskId: string, reason: string) {
  return apiClient.post(`tasks/${taskId}/cancel`, { json: { reason } }).json<ApiResponse<TaskDetail>>()
}

// ─── 规划审批 ───

export async function approvePlan(taskId: string) {
  return apiClient.post(`tasks/${taskId}/approve`).json<ApiResponse<TaskDetail>>()
}

export async function rejectPlan(taskId: string, input: { feedback: string }) {
  return apiClient.post(`tasks/${taskId}/reject`, { json: input }).json<ApiResponse<TaskDetail>>()
}

// ─── Todo 管理与人工确认 ───

export interface AddTodoInput {
  title: string
  description?: string
  assignee_id: string
}

export interface UpdateTodoInput {
  title?: string
  description?: string
  assignee_id?: string
}

export async function addTaskTodo(taskId: string, input: AddTodoInput) {
  return apiClient.post(`tasks/${taskId}/todos`, { json: input }).json<ApiResponse<TaskDetail>>()
}

export async function insertTaskTodo(taskId: string, todoId: string, input: AddTodoInput) {
  return apiClient.post(`tasks/${taskId}/todos/${todoId}/insert`, { json: input }).json<ApiResponse<TaskDetail>>()
}

export async function updateTaskTodo(taskId: string, todoId: string, input: UpdateTodoInput) {
  return apiClient.patch(`tasks/${taskId}/todos/${todoId}`, { json: input }).json<ApiResponse<TaskDetail>>()
}

export async function removeTaskTodo(taskId: string, todoId: string) {
  return apiClient.delete(`tasks/${taskId}/todos/${todoId}`).json<ApiResponse<TaskDetail>>()
}

export async function reorderTaskTodos(taskId: string, todoIds: string[]) {
  return apiClient.put(`tasks/${taskId}/todos/reorder`, { json: { todo_ids: todoIds } }).json<ApiResponse<TaskDetail>>()
}

export async function dispatchTodo(taskId: string, todoId: string) {
  return apiClient.post(`tasks/${taskId}/todos/${todoId}/dispatch`).json<ApiResponse<TaskDetail>>()
}

// 重开失败/已取消/已完成的 todo（回到 in_progress）。
// 注意：重开不会自动派发给执行员工（防双跑设计），需要用户到任务评论里 @ 执行员工唤醒。
export async function reopenTodo(taskId: string, todoId: string, reason?: string) {
  return apiClient.post(`tasks/${taskId}/todos/${todoId}/reopen`, { json: { reason: reason ?? '' } }).json<ApiResponse<TaskDetail>>()
}

export async function reviewTodo(taskId: string, todoId: string, action: 'approve' | 'reject', reason?: string) {
  return apiClient.post(`tasks/${taskId}/todos/${todoId}/review`, { json: { action, reason: reason ?? '' } }).json<ApiResponse<TaskDetail>>()
}

export async function answerTodo(taskId: string, todoId: string, input: { question_id: string; answer: string }) {
  return apiClient.post(`tasks/${taskId}/todos/${todoId}/answer`, { json: input }).json<ApiResponse<TaskDetail>>()
}

// ─── 交付物 ───

export async function getTaskArtifactContent(taskId: string, transferId: string) {
  return apiClient.get(`tasks/${taskId}/artifacts/${transferId}/content`).blob()
}

// 把已归档的过程文件提升为某工作流步骤的最终交付物（手工救场）。
// `artifact_id` 就是后端的 transfer_id（Mongo bson:_id 同值）。
export async function bindArtifactOutput(
  taskId: string,
  todoId: string,
  input: { artifact_id: string; output_name: string },
) {
  return apiClient
    .post(`tasks/${taskId}/todos/${todoId}/outputs/bind`, { json: input })
    .json<ApiResponse<TaskArtifact>>()
}

// T1.9：把一个已完成任务一键沉淀为全局工作流模板。
// 后端挂在 /tasks/:id/distill-template；body 可选，缺省时模板名回退为任务标题。
export async function distillTaskWorkflowTemplate(
  taskId: string,
  input?: { name?: string; description?: string },
) {
  return apiClient
    .post(`tasks/${taskId}/distill-template`, { json: input ?? {} })
    .json<ApiResponse<WorkflowTemplate>>()
}
