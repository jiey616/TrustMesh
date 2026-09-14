import type { QueryClient } from '@tanstack/react-query'
import type { Comment, Event, TaskDetail, TaskListItem, TaskStatus } from '@/types'
import { applyProjectTaskUpdated } from './projects'

function sortEventsAscending(items: Event[]) {
  return items.slice().sort((left, right) => left.created_at.localeCompare(right.created_at))
}

function sortEventsDescending(items: Event[]) {
  return items.slice().sort((left, right) => right.created_at.localeCompare(left.created_at))
}

function sortCommentsAscending(items: Comment[]) {
  return items.slice().sort((left, right) => left.created_at.localeCompare(right.created_at))
}

function sortTasksDescending(items: TaskListItem[]) {
  return items.slice().sort((left, right) => right.updated_at.localeCompare(left.updated_at))
}

function matchesTaskFilter(task: TaskDetail, filter: unknown) {
  if (!filter || typeof filter !== 'object') {
    return true
  }
  const status = (filter as { status?: unknown }).status
  return typeof status !== 'string' || task.status === status
}

function toTaskListItem(task: TaskDetail): TaskListItem {
  const completedTodoCount = task.todos.filter((todo) => todo.status === 'done').length
  const failedTodoCount = task.todos.filter((todo) => todo.status === 'failed').length

  return {
    id: task.id,
    project_id: task.project_id,
    title: task.title,
    description: task.description,
    status: task.status,
    priority: task.priority,
    pm_agent: task.pm_agent,
    todo_count: task.todos.length,
    completed_todo_count: completedTodoCount,
    failed_todo_count: failedTodoCount,
    created_at: task.created_at,
    updated_at: task.updated_at,
  }
}

function findPreviousTaskStatus(
  task: TaskDetail,
  taskQueries: Array<[readonly unknown[], TaskListItem[] | undefined]>,
  previousTaskDetail: TaskDetail | undefined
): TaskStatus | null {
  if (previousTaskDetail) {
    return previousTaskDetail.status
  }

  for (const [, items] of taskQueries) {
    const previousTask = items?.find((item) => item.id === task.id)
    if (previousTask) {
      return previousTask.status
    }
  }

  return null
}

function hasFullProjectTaskListSnapshot(
  task: TaskDetail,
  taskQueries: Array<[readonly unknown[], TaskListItem[] | undefined]>
) {
  return taskQueries.some(([queryKey, items]) => (
    Array.isArray(queryKey) &&
    queryKey[0] === 'tasks' &&
    queryKey[1] === task.project_id &&
    queryKey.length >= 3 &&
    queryKey[2] === undefined &&
    Array.isArray(items)
  ))
}

export function applyTaskUpdated(queryClient: QueryClient, payload: { task: TaskDetail }) {
  const { task } = payload
  const listItem = toTaskListItem(task)
  const previousTaskDetail = queryClient.getQueryData<TaskDetail>(['tasks', 'detail', task.id])
  const taskQueries = queryClient.getQueriesData<TaskListItem[]>({ queryKey: ['tasks', task.project_id] })
  const previousTaskStatus = findPreviousTaskStatus(task, taskQueries, previousTaskDetail)
  const hasFullListSnapshot = hasFullProjectTaskListSnapshot(task, taskQueries)
  const isNewTask = previousTaskStatus === null && hasFullListSnapshot

  // Cancel any in-flight refetch so it doesn't overwrite this SSE-delivered data
  void queryClient.cancelQueries({ queryKey: ['tasks', 'detail', task.id] })
  queryClient.setQueryData(['tasks', 'detail', task.id], task)
  for (const [queryKey, items] of taskQueries) {
    if (!Array.isArray(queryKey) || queryKey[0] !== 'tasks' || queryKey[1] !== task.project_id || queryKey.length < 3) {
      continue
    }
    const filter = queryKey[2]
    queryClient.setQueryData<TaskListItem[] | undefined>(queryKey, () => {
      if (!items) {
        return items
      }
      const nextItems = items.filter((item) => item.id !== task.id)
      if (!matchesTaskFilter(task, filter)) {
        return sortTasksDescending(nextItems)
      }
      return sortTasksDescending([...nextItems, listItem])
    })
  }

  const dashboardTaskQueries = queryClient.getQueriesData<TaskListItem[]>({ queryKey: ['dashboard', 'tasks'] })
  for (const [queryKey, items] of dashboardTaskQueries) {
    if (!Array.isArray(queryKey) || queryKey[0] !== 'dashboard' || queryKey[1] !== 'tasks' || typeof queryKey[2] !== 'number') {
      continue
    }
    queryClient.setQueryData<TaskListItem[] | undefined>(queryKey, () => {
      if (!items) {
        return items
      }
      const next = sortTasksDescending([...items.filter((item) => item.id !== task.id), listItem])
      return next.slice(0, queryKey[2])
    })
  }

  applyProjectTaskUpdated(queryClient, {
    task,
    previousTaskStatus,
    isNewTask,
    hasEnoughContext: previousTaskStatus !== null || isNewTask,
  })

  void queryClient.invalidateQueries({ queryKey: ['dashboard', 'stats'] })
}

export function applyTaskEventCreated(
  queryClient: QueryClient,
  payload: { task_id: string; project_id: string; event: Event }
) {
  queryClient.setQueryData<Event[] | undefined>(['tasks', 'events', payload.task_id], (items) => {
    if (!items) {
      return items
    }
    if (items.some((item) => item.id === payload.event.id)) {
      return items
    }
    return sortEventsAscending([...items, payload.event])
  })

  const dashboardEventQueries = queryClient.getQueriesData<Event[]>({ queryKey: ['dashboard', 'events'] })
  for (const [queryKey, items] of dashboardEventQueries) {
    if (!Array.isArray(queryKey) || queryKey[0] !== 'dashboard' || queryKey[1] !== 'events' || typeof queryKey[2] !== 'number') {
      continue
    }
    queryClient.setQueryData<Event[] | undefined>(queryKey, () => {
      if (!items) {
        return items
      }
      if (items.some((item) => item.id === payload.event.id)) {
        return items
      }
      return sortEventsDescending([payload.event, ...items]).slice(0, queryKey[2])
    })
  }

  if (payload.event.actor_type === 'agent') {
    const agentEventQueries = queryClient.getQueriesData<Event[]>({
      queryKey: ['agents', payload.event.actor_id, 'events'],
    })
    for (const [queryKey, items] of agentEventQueries) {
      if (
        !Array.isArray(queryKey) ||
        queryKey[0] !== 'agents' ||
        queryKey[1] !== payload.event.actor_id ||
        queryKey[2] !== 'events' ||
        typeof queryKey[3] !== 'number'
      ) {
        continue
      }
      queryClient.setQueryData<Event[] | undefined>(queryKey, () => {
        if (!items) {
          return items
        }
        if (items.some((item) => item.id === payload.event.id)) {
          return items
        }
        return sortEventsDescending([payload.event, ...items]).slice(0, queryKey[3])
      })
    }
  }
}

export function applyTaskCommentCreated(
  queryClient: QueryClient,
  payload: { task_id: string; project_id: string; comment: Comment }
) {
  queryClient.setQueryData<Comment[] | undefined>(['tasks', 'comments', payload.task_id], (items) => {
    if (!items) {
      return items
    }
    if (items.some((item) => item.id === payload.comment.id)) {
      return items
    }
    return sortCommentsAscending([...items, payload.comment])
  })
}
