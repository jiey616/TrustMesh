import { useEffect, useMemo } from 'react'
import { useQueries } from '@tanstack/react-query'
import * as tasksApi from '@/api/tasks'
import { useAgents } from '@/hooks/useAgents'
import { useProjects } from '@/hooks/useProjects'
import { subscribeRealtimeEvents } from '@/realtime/emitter'
import type { TaskDetail } from '@/types'
import { officeSimulation } from './simulation'

// ─── 办公室数据管道 ───
// 快照种子：agents 花名册 + 各项目任务列表 → 活跃任务详情
// 实时更新：订阅 RealtimeProvider 广播的 SSE 事件写入模拟状态
// 查询 key 与任务页共用（['tasks', projectId]、['tasks','detail',id]），共享缓存。

const ACTIVE_TASK_STATUSES = new Set([
  'planning',
  'review',
  'pending',
  'in_progress',
  'awaiting_review',
  'waiting_user',
])

const MAX_ACTIVE_TASK_DETAILS = 24

function normalizeTaskDetail(task: TaskDetail): TaskDetail {
  return {
    ...task,
    todos: task.todos ?? [],
    artifacts: task.artifacts ?? [],
    messages: task.messages ?? [],
  }
}

export function useOfficeData() {
  const { data: agents } = useAgents()
  const { data: projects } = useProjects()

  const activeProjectIds = useMemo(
    () => (projects ?? []).filter((p) => p.status === 'active').map((p) => p.id),
    [projects],
  )

  // 1. 花名册 → 座位与在线状态
  useEffect(() => {
    if (!agents) return
    officeSimulation.seedAgents(agents.filter((a) => !a.archived))
  }, [agents])

  // 2. 各项目任务列表（与项目工作台共用缓存 key）
  const taskListQueries = useQueries({
    queries: activeProjectIds.map((projectId) => ({
      queryKey: ['tasks', projectId, undefined],
      queryFn: async () => {
        const res = await tasksApi.listProjectTasks(projectId)
        return res.data.items
      },
      staleTime: 30_000,
    })),
  })

  const activeTaskIds = useMemo(() => {
    const ids: { id: string; updated_at: string }[] = []
    for (const query of taskListQueries) {
      for (const task of query.data ?? []) {
        if (ACTIVE_TASK_STATUSES.has(task.status)) {
          ids.push({ id: task.id, updated_at: task.updated_at })
        }
      }
    }
    return ids
      .sort((a, b) => b.updated_at.localeCompare(a.updated_at))
      .slice(0, MAX_ACTIVE_TASK_DETAILS)
      .map((item) => item.id)
  }, [taskListQueries])

  // 3. 活跃任务详情（todos 才带 assignee，列表项不够）
  const detailQueries = useQueries({
    queries: activeTaskIds.map((taskId) => ({
      queryKey: ['tasks', 'detail', taskId],
      queryFn: async () => {
        const res = await tasksApi.getTask(taskId)
        return normalizeTaskDetail(res.data)
      },
      staleTime: 5_000,
    })),
  })

  useEffect(() => {
    for (const query of detailQueries) {
      if (query.data) {
        officeSimulation.applyTaskDetail(query.data)
      }
    }
  }, [detailQueries])

  // 4. SSE 事件 → 模拟状态（真实驱动，不表演）
  useEffect(() => {
    return subscribeRealtimeEvents((event) => {
      officeSimulation.applyRealtimeEvent(event)
    })
  }, [])

  const loading = !agents || (activeProjectIds.length > 0 && taskListQueries.some((q) => q.isLoading))

  return { loading }
}
