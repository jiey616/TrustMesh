import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as tasksApi from '@/api/tasks'
import { usePageVisibility } from './usePageVisibility'
import { useRealtimeStatus } from '@/realtime/hooks/useRealtimeStatus'
import type { CreateTaskInput } from '@/api/tasks'
import type { AppendTaskMessageRequest, CreatePlanningTaskRequest, ListProjectTasksQuery, RejectPlanRequest, TaskDetail, Workflow } from '@/types'

function normalizeTaskDetail(task: TaskDetail): TaskDetail {
  return {
    ...task,
    messages: task.messages ?? [],
    todos: task.todos ?? [],
    artifacts: task.artifacts ?? [],
  }
}

export function useTasks(projectId: string | undefined, query?: ListProjectTasksQuery) {
  return useQuery({
    queryKey: ['tasks', projectId, query],
    queryFn: async () => {
      const res = await tasksApi.listProjectTasks(projectId!, query)
      return res.data.items
    },
    enabled: !!projectId,
    staleTime: 30_000,
  })
}

export function useTask(id: string | undefined) {
  const isPageVisible = usePageVisibility()
  const realtimeStatus = useRealtimeStatus()

  return useQuery({
    queryKey: ['tasks', 'detail', id],
    queryFn: async () => {
      const res = await tasksApi.getTask(id!)
      return normalizeTaskDetail(res.data)
    },
    enabled: !!id,
    staleTime: 5_000,
    refetchInterval: (currentQuery) => {
      if (!isPageVisible || !id) {
        return false
      }
      if (realtimeStatus !== 'reconnecting' && realtimeStatus !== 'disconnected') {
        return false
      }

      const task = currentQuery.state.data as TaskDetail | undefined
      return !task || task.status === 'planning' || task.status === 'review' || task.status === 'pending' || task.status === 'in_progress' ? 3_000 : false
    },
    refetchIntervalInBackground: false,
  })
}

export function useTaskEvents(id: string | undefined) {
  return useQuery({
    queryKey: ['tasks', 'events', id],
    queryFn: async () => {
      const res = await tasksApi.listTaskEvents(id!)
      return res.data.items
    },
    enabled: !!id,
  })
}

export function useTaskComments(taskId: string | undefined) {
  return useQuery({
    queryKey: ['tasks', 'comments', taskId],
    queryFn: async () => {
      const res = await tasksApi.listTaskComments(taskId!)
      return res.data.items
    },
    enabled: !!taskId,
  })
}

export function useAddTaskComment() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, content, todoId, mentionAgentIds }: {
      taskId: string
      content: string
      todoId?: string
      mentionAgentIds?: string[]
    }) =>
      tasksApi.addTaskComment(taskId, {
        content,
        todo_id: todoId,
        mentions: mentionAgentIds?.map((agentId) => ({ agent_id: agentId })),
      }),
    onSuccess: (_res, { taskId }) => {
      qc.invalidateQueries({ queryKey: ['tasks', 'comments', taskId] })
      qc.invalidateQueries({ queryKey: ['tasks', 'events', taskId] })
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
      qc.invalidateQueries({ queryKey: ['dashboard', 'tasks'] })
      qc.invalidateQueries({ queryKey: ['dashboard', 'stats'] })
    },
  })
}

export function useCreatePlanningTask() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ projectId, input, fileIds }: { projectId: string; input: CreatePlanningTaskRequest; fileIds?: string[] }) =>
      tasksApi.createPlanningTask(projectId, { ...input, file_ids: fileIds }),
    onSuccess: (res) => {
      qc.setQueryData(['tasks', 'detail', res.data.id], normalizeTaskDetail(res.data))
      qc.invalidateQueries({ queryKey: ['tasks'] })
      qc.invalidateQueries({ queryKey: ['projects'] })
      qc.invalidateQueries({ queryKey: ['dashboard', 'tasks'] })
      qc.invalidateQueries({ queryKey: ['dashboard', 'stats'] })
    },
  })
}

export function useCreateTaskFromText() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ projectId, content, agentId, fileIds, workflow }: { projectId: string; content: string; agentId?: string; fileIds?: string[]; workflow?: Workflow }) =>
      tasksApi.createTaskFromText(projectId, content, agentId, fileIds, workflow),
    onSuccess: (res) => {
      qc.setQueryData(['tasks', 'detail', res.data.id], normalizeTaskDetail(res.data))
      qc.invalidateQueries({ queryKey: ['tasks'] })
      qc.invalidateQueries({ queryKey: ['projects'] })
      qc.invalidateQueries({ queryKey: ['dashboard', 'tasks'] })
      qc.invalidateQueries({ queryKey: ['dashboard', 'stats'] })
    },
  })
}

export function useAppendTaskMessage() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, input }: { taskId: string; input: AppendTaskMessageRequest }) =>
      tasksApi.appendTaskMessage(taskId, input),
    onSuccess: (res, { taskId }) => {
      qc.setQueryData(['tasks', 'detail', taskId], normalizeTaskDetail(res.data))
      qc.invalidateQueries({ queryKey: ['tasks'] })
    },
  })
}

export function useDispatchTodo() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, todoId }: { taskId: string; todoId: string }) =>
      tasksApi.dispatchTodo(taskId, todoId),
    onSuccess: (_res, { taskId }) => {
      qc.invalidateQueries({ queryKey: ['tasks'] })
      qc.invalidateQueries({ queryKey: ['tasks', 'detail', taskId] })
      qc.invalidateQueries({ queryKey: ['tasks', 'events', taskId] })
    },
  })
}

export function useApprovePlan() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId }: { taskId: string }) => tasksApi.approvePlan(taskId),
    onSuccess: (res, { taskId }) => {
      qc.setQueryData(['tasks', 'detail', taskId], normalizeTaskDetail(res.data))
      qc.invalidateQueries({ queryKey: ['tasks'] })
    },
  })
}

export function useRejectPlan() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, input }: { taskId: string; input: RejectPlanRequest }) =>
      tasksApi.rejectPlan(taskId, input),
    onSuccess: (res, { taskId }) => {
      qc.setQueryData(['tasks', 'detail', taskId], normalizeTaskDetail(res.data))
      qc.invalidateQueries({ queryKey: ['tasks'] })
    },
  })
}

// ─── Dynamic TODO management ───

export function useAddTaskTodo() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, input }: { taskId: string; input: tasksApi.AddTodoInput }) =>
      tasksApi.addTaskTodo(taskId, input),
    onSuccess: (res, { taskId }) => {
      qc.setQueryData(['tasks', 'detail', taskId], normalizeTaskDetail(res.data))
      qc.invalidateQueries({ queryKey: ['tasks'] })
      qc.invalidateQueries({ queryKey: ['tasks', 'events', taskId] })
    },
  })
}

export function useInsertTaskTodo() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, todoId, input }: { taskId: string; todoId: string; input: tasksApi.AddTodoInput }) =>
      tasksApi.insertTaskTodo(taskId, todoId, input),
    onSuccess: (res, { taskId }) => {
      qc.setQueryData(['tasks', 'detail', taskId], normalizeTaskDetail(res.data))
      qc.invalidateQueries({ queryKey: ['tasks'] })
      qc.invalidateQueries({ queryKey: ['tasks', 'events', taskId] })
    },
  })
}

export function useUpdateTaskTodo() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, todoId, input }: { taskId: string; todoId: string; input: tasksApi.UpdateTodoInput }) =>
      tasksApi.updateTaskTodo(taskId, todoId, input),
    onSuccess: (res, { taskId }) => {
      qc.setQueryData(['tasks', 'detail', taskId], normalizeTaskDetail(res.data))
      qc.invalidateQueries({ queryKey: ['tasks'] })
      qc.invalidateQueries({ queryKey: ['tasks', 'events', taskId] })
    },
  })
}

export function useRemoveTaskTodo() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, todoId }: { taskId: string; todoId: string }) =>
      tasksApi.removeTaskTodo(taskId, todoId),
    onSuccess: (_res, { taskId }) => {
      qc.invalidateQueries({ queryKey: ['tasks'] })
      qc.invalidateQueries({ queryKey: ['tasks', 'detail', taskId] })
      qc.invalidateQueries({ queryKey: ['tasks', 'events', taskId] })
    },
  })
}

export function useReorderTaskTodos() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, todoIds }: { taskId: string; todoIds: string[] }) =>
      tasksApi.reorderTaskTodos(taskId, todoIds),
    onSuccess: (_res, { taskId }) => {
      qc.invalidateQueries({ queryKey: ['tasks'] })
      qc.invalidateQueries({ queryKey: ['tasks', 'detail', taskId] })
    },
  })
}

export function useCancelTask() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, reason }: { taskId: string; reason: string }) =>
      tasksApi.cancelTask(taskId, reason),
    onSuccess: (res, { taskId }) => {
      qc.setQueryData(['tasks', 'detail', taskId], res.data)
      qc.invalidateQueries({ queryKey: ['tasks'] })
      qc.invalidateQueries({ queryKey: ['tasks', 'detail', taskId] })
      qc.invalidateQueries({ queryKey: ['tasks', 'events', taskId] })
      qc.invalidateQueries({ queryKey: ['projects'] })
      qc.invalidateQueries({ queryKey: ['dashboard', 'tasks'] })
      qc.invalidateQueries({ queryKey: ['dashboard', 'stats'] })
    },
  })
}
