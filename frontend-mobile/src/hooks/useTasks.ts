import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { listProjectTasks, listProjects } from '@/api/projects'
import {
  addTaskComment,
  answerTodo,
  appendTaskMessage,
  approvePlan,
  getTask,
  listTaskComments,
  listTaskEvents,
  rejectPlan,
  reviewTodo,
} from '@/api/tasks'
import { collectPendingItems, pendingKindOrder, type PendingItem } from '@/lib/pendingItems'
import { useWorkspaceStore } from '@/stores/workspaceStore'
import type { TaskStatus } from '@/types'

/** 还"活着"、可能需要人介入的任务状态 */
const ACTIVE_STATUSES: TaskStatus[] = [
  'planning',
  'review',
  'pending',
  'in_progress',
  'awaiting_review',
  'waiting_user',
]

function useReady() {
  return useWorkspaceStore((s) => s.calibrated)
}

export function useProjects() {
  const ready = useReady()
  return useQuery({ queryKey: ['projects'], queryFn: listProjects, enabled: ready })
}

export function useProjectTasks(projectId: string) {
  const ready = useReady()
  return useQuery({
    queryKey: ['projectTasks', projectId],
    queryFn: () => listProjectTasks(projectId),
    enabled: ready && Boolean(projectId),
    // SSE 实时为主，低频轮询兜底（桌面端同款 30s）
    refetchInterval: 30_000,
  })
}

/** 全量任务平铺（按更新时间倒序）：任务 Tab 直接展示，点击进详情，不再经项目中间页。 */
export function useAllTasks() {
  const ready = useReady()
  const projects = useProjects()
  const projectIds = projects.data?.map((p) => p.id) ?? []

  const taskLists = useQueries({
    queries: projectIds.map((id) => ({
      queryKey: ['projectTasks', id],
      queryFn: () => listProjectTasks(id),
      enabled: ready,
      refetchInterval: 30_000,
    })),
  })

  const data = taskLists
    .flatMap((q) => q.data ?? [])
    .sort((a, b) => b.updated_at.localeCompare(a.updated_at))

  const isLoading = projects.isLoading || (projectIds.length > 0 && taskLists.some((q) => q.isLoading))
  return { data, isLoading }
}

/** 任务处于这些状态时视为「活跃」：详情与事件流保持轮询（桌面端同款策略）。 */
const LIVE_TASK_STATUSES: TaskStatus[] = [
  'planning',
  'review',
  'pending',
  'in_progress',
  'awaiting_review',
  'waiting_user',
]

export function useTask(taskId: string) {
  return useQuery({
    queryKey: ['task', taskId],
    queryFn: () => getTask(taskId),
    enabled: Boolean(taskId),
    staleTime: 5_000,
    // SSE 实时为主，轮询兜底：活跃任务 3s（桌面端同款），完结自动停
    refetchInterval: (currentQuery) => {
      const task = currentQuery.state.data
      return task && LIVE_TASK_STATUSES.includes(task.status) ? 3_000 : false
    },
  })
}

export function useTaskEvents(taskId: string) {
  const qc = useQueryClient()
  return useQuery({
    queryKey: ['taskEvents', taskId],
    queryFn: () => listTaskEvents(taskId),
    enabled: Boolean(taskId),
    staleTime: 3_000,
    // 与 useTask 一致：任务活跃才轮询事件流，避免对话刷新了、执行过程没刷新的错位
    refetchInterval: () => {
      const task = qc.getQueryData(['task', taskId]) as { status: TaskStatus } | undefined
      return task && LIVE_TASK_STATUSES.includes(task.status) ? 4_000 : false
    },
  })
}

export function useTaskComments(taskId: string) {
  return useQuery({
    queryKey: ['taskComments', taskId],
    queryFn: () => listTaskComments(taskId),
    enabled: Boolean(taskId),
  })
}

/**
 * 工作台待办流：项目 → 任务 →（详情 + 事件）→ 汇总待确认事项。
 *
 * 只扫**最近更新的 N 个活跃任务**（默认 8），避免内部工具在手机上打爆 N+1 请求；
 * 这是一个明确的取舍：很久没动过的任务不会出现在移动端工作台。
 */
export function usePendingFeed(limit = 8) {
  const ready = useReady()
  const projects = useProjects()
  const projectIds = projects.data?.map((p) => p.id) ?? []

  const taskLists = useQueries({
    queries: projectIds.map((id) => ({
      queryKey: ['projectTasks', id],
      queryFn: () => listProjectTasks(id),
      enabled: ready,
    })),
  })

  const candidates = taskLists
    .flatMap((q) => q.data ?? [])
    .filter((t) => ACTIVE_STATUSES.includes(t.status))
    .sort((a, b) => b.updated_at.localeCompare(a.updated_at))
    .slice(0, limit)

  const details = useQueries({
    queries: candidates.map((t) => ({
      queryKey: ['task', t.id],
      queryFn: () => getTask(t.id),
    })),
  })

  const events = useQueries({
    queries: candidates.map((t) => ({
      queryKey: ['taskEvents', t.id],
      queryFn: () => listTaskEvents(t.id),
    })),
  })

  const items: PendingItem[] = candidates
    .flatMap((_t, i) => collectPendingItems(details[i]?.data, events[i]?.data))
    .sort(
      (a, b) =>
        pendingKindOrder.indexOf(a.kind) - pendingKindOrder.indexOf(b.kind) ||
        a.createdAt.localeCompare(b.createdAt),
    )

  const isLoading =
    projects.isLoading || taskLists.some((q) => q.isLoading) || details.some((q) => q.isLoading)

  return { isLoading, items, activeTasks: candidates }
}

// ─── 写操作 ───

function useTaskMutation<TVars>(
  fn: (vars: TVars) => Promise<void>,
  invalidate: (vars: TVars) => string[][],
) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: (_data, vars) => {
      for (const key of invalidate(vars)) void qc.invalidateQueries({ queryKey: key })
    },
  })
}

type TaskVars = { taskId: string }
type TodoVars = { taskId: string; todoId: string }

export function useApprovePlan() {
  return useTaskMutation<TaskVars>(
    ({ taskId }) => approvePlan(taskId),
    ({ taskId }) => [['task', taskId], ['projectTasks']],
  )
}

export function useRejectPlan() {
  return useTaskMutation<TaskVars & { feedback: string }>(
    ({ taskId, feedback }) => rejectPlan(taskId, feedback),
    ({ taskId }) => [['task', taskId], ['projectTasks']],
  )
}

export function useReviewTodo() {
  return useTaskMutation<TodoVars & { action: 'approve' | 'reject'; reason?: string }>(
    ({ taskId, todoId, action, reason }) => reviewTodo(taskId, todoId, action, reason),
    ({ taskId }) => [['task', taskId], ['taskEvents', taskId], ['projectTasks']],
  )
}

export function useAnswerTodo() {
  return useTaskMutation<TodoVars & { questionId: string; answer: string }>(
    ({ taskId, todoId, questionId, answer }) => answerTodo(taskId, todoId, questionId, answer),
    ({ taskId }) => [['task', taskId], ['taskEvents', taskId]],
  )
}

export function useAppendMessage() {
  return useTaskMutation<
    TaskVars & { content: string; uiResponse?: { blocks: Record<string, unknown> } }
  >(
    ({ taskId, content, uiResponse }) => appendTaskMessage(taskId, content, uiResponse as never),
    ({ taskId }) => [['task', taskId], ['taskEvents', taskId], ['projectTasks']],
  )
}

export function useAddComment() {
  return useTaskMutation<TaskVars & { content: string }>(
    ({ taskId, content }) => addTaskComment(taskId, content),
    ({ taskId }) => [['taskComments', taskId]],
  )
}
