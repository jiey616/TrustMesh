import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as tasksApi from '@/api/tasks'
import type { CreateTaskInput, AddTodoInput, UpdateTodoInput } from '@/api/tasks'
import type { UIResponse, Workflow, ChatAttachment } from '@/types'

export function useTasks(projectId: string | undefined, status?: string) {
  return useQuery({
    queryKey: ['tasks', projectId, status],
    queryFn: async () => {
      const res = await tasksApi.listProjectTasks(projectId!, status)
      return res.data.items
    },
    enabled: !!projectId,
    staleTime: 30_000,
    // SSE 实时为主，低频轮询兜底（任务执行过程/SSE 断线时也能更新列表）
    refetchInterval: 30_000,
  })
}

export function useTask(id: string | undefined) {
  return useQuery({
    queryKey: ['tasks', 'detail', id],
    queryFn: async () => {
      const res = await tasksApi.getTask(id!)
      return res.data
    },
    enabled: !!id,
    staleTime: 5_000,
    refetchInterval: (currentQuery) => {
      const task = currentQuery.state.data as { status: string } | undefined
      // 含 awaiting_review / waiting_user：待人工确认/等待用户时对话与状态仍需刷新，
      // 否则头部状态会与执行过程事件流对不齐。
      return !task || ['planning', 'review', 'pending', 'in_progress', 'awaiting_review', 'waiting_user'].includes(task.status) ? 3_000 : false
    },
  })
}

// 任务处于这些状态时视为「活跃」：对话(useTask)与执行过程(useTaskEvents)应同步刷新。
const LIVE_TASK_STATUSES = ['planning', 'review', 'pending', 'in_progress', 'awaiting_review', 'waiting_user']

export function useTaskEvents(id: string | undefined) {
  const qc = useQueryClient()
  return useQuery({
    queryKey: ['tasks', 'detail', id, 'events'],
    queryFn: async () => {
      const res = await tasksApi.listTaskEvents(id!)
      return res.data.items
    },
    enabled: !!id,
    staleTime: 3_000,
    refetchInterval: () => {
      // 与 useTask 保持一致：只要任务活跃就轮询执行过程，避免「对话刷新了、执行过程没刷新」的错位。
      const task = qc.getQueryData(['tasks', 'detail', id]) as { status: string } | undefined
      return task && LIVE_TASK_STATUSES.includes(task.status) ? 4_000 : false
    },
  })
}

export function useTaskComments(id: string | undefined) {
  return useQuery({
    queryKey: ['tasks', 'detail', id, 'comments'],
    queryFn: async () => {
      const res = await tasksApi.listTaskComments(id!)
      return res.data.items
    },
    enabled: !!id,
    staleTime: 10_000,
  })
}

export function useAppendTaskMessage() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, content, uiResponse, attachments }: { taskId: string; content: string; uiResponse?: UIResponse; attachments?: ChatAttachment[] }) =>
      tasksApi.appendTaskMessage(taskId, { content, ui_response: uiResponse, attachments }),
    onSuccess: (_res, { taskId }) => {
      invalidateTask(qc, taskId)
    },
  })
}

export function useAddTaskComment() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, content, mentions, attachments }: { taskId: string; content: string; mentions?: Array<{ agent_id: string }>; attachments?: ChatAttachment[] }) =>
      tasksApi.addTaskComment(taskId, { content, mentions, attachments }),
    onSuccess: (_res, { taskId }) => {
      invalidateTask(qc, taskId)
    },
  })
}

export function useCreateTask() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ projectId, input }: { projectId: string; input: CreateTaskInput }) =>
      tasksApi.createTask(projectId, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['tasks'] })
      qc.invalidateQueries({ queryKey: ['projects'] })
      qc.invalidateQueries({ queryKey: ['workflow-progress'] })
    },
  })
}

export function useCreateTaskFromText() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { projectId: string; content: string; agent_id?: string; file_ids?: string[]; workflow?: Workflow; workflow_index?: number; step_from?: number; step_to?: number; attachments?: ChatAttachment[] }) =>
      tasksApi.createTaskFromText(input.projectId, {
        content: input.content,
        agent_id: input.agent_id,
        file_ids: input.file_ids,
        workflow: input.workflow,
        workflow_index: input.workflow_index,
        step_from: input.step_from,
        step_to: input.step_to,
        attachments: input.attachments,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['tasks'] })
      qc.invalidateQueries({ queryKey: ['projects'] })
      qc.invalidateQueries({ queryKey: ['workflow-progress'] })
    },
  })
}

export function useCancelTask() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, reason }: { taskId: string; reason: string }) => tasksApi.cancelTask(taskId, reason),
    onSuccess: (_res, { taskId }) => {
      qc.invalidateQueries({ queryKey: ['tasks'] })
      qc.invalidateQueries({ queryKey: ['tasks', 'detail', taskId] })
      qc.invalidateQueries({ queryKey: ['projects'] })
    },
  })
}

export function useApprovePlan() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId }: { taskId: string }) => tasksApi.approvePlan(taskId),
    onSuccess: (_res, { taskId }) => invalidateTask(qc, taskId),
  })
}

export function useRejectPlan() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, feedback }: { taskId: string; feedback: string }) => tasksApi.rejectPlan(taskId, { feedback }),
    onSuccess: (_res, { taskId }) => invalidateTask(qc, taskId),
  })
}

export function useReviewTodo() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, todoId, action, reason }: { taskId: string; todoId: string; action: 'approve' | 'reject'; reason?: string }) =>
      tasksApi.reviewTodo(taskId, todoId, action, reason),
    onSuccess: (_res, { taskId }) => invalidateTask(qc, taskId),
  })
}

export function useDispatchTodo() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, todoId }: { taskId: string; todoId: string }) => tasksApi.dispatchTodo(taskId, todoId),
    onSuccess: (_res, { taskId }) => invalidateTask(qc, taskId),
  })
}

export function useAnswerTodo() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, todoId, questionId, answer }: { taskId: string; todoId: string; questionId: string; answer: string }) =>
      tasksApi.answerTodo(taskId, todoId, { question_id: questionId, answer }),
    onSuccess: (_res, { taskId }) => invalidateTask(qc, taskId),
  })
}

export function useAddTaskTodo() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, input }: { taskId: string; input: AddTodoInput }) => tasksApi.addTaskTodo(taskId, input),
    onSuccess: (_res, { taskId }) => invalidateTask(qc, taskId),
  })
}

export function useUpdateTaskTodo() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, todoId, input }: { taskId: string; todoId: string; input: UpdateTodoInput }) =>
      tasksApi.updateTaskTodo(taskId, todoId, input),
    onSuccess: (_res, { taskId }) => invalidateTask(qc, taskId),
  })
}

export function useRemoveTaskTodo() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, todoId }: { taskId: string; todoId: string }) => tasksApi.removeTaskTodo(taskId, todoId),
    onSuccess: (_res, { taskId }) => invalidateTask(qc, taskId),
  })
}

// 把归档的过程文件提升为某工作流步骤的交付物。绑定成功后会触发任务详情刷新，
// 让文件 badge 从「过程」变成「交付 · 输出位名」，并把它接到工作流图的下游步骤上。
export function useBindArtifactOutput() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      taskId,
      todoId,
      input,
    }: {
      taskId: string
      todoId: string
      input: { artifact_id: string; output_name: string }
    }) => tasksApi.bindArtifactOutput(taskId, todoId, input),
    onSuccess: (_res, { taskId }) => invalidateTask(qc, taskId),
  })
}

function invalidateTask(qc: ReturnType<typeof useQueryClient>, taskId: string) {
  qc.invalidateQueries({ queryKey: ['tasks'] })
  qc.invalidateQueries({ queryKey: ['tasks', 'detail', taskId] })
  qc.invalidateQueries({ queryKey: ['tasks', 'detail', taskId, 'events'] })
  qc.invalidateQueries({ queryKey: ['tasks', 'detail', taskId, 'comments'] })
  qc.invalidateQueries({ queryKey: ['projects'] })
}

export function useDistillTaskWorkflowTemplate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, name, description }: { taskId: string; name?: string; description?: string }) =>
      tasksApi.distillTaskWorkflowTemplate(taskId, { name, description }),
    // 沉淀会新建一个全局模板 → 让模板列表页立即刷新
    onSuccess: () => qc.invalidateQueries({ queryKey: ['workflow-templates'] }),
  })
}
