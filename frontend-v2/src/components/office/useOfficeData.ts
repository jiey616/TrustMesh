import { useEffect, useMemo } from 'react'
import { useQueries } from '@tanstack/react-query'
import * as tasksApi from '@/api/tasks'
import { useAgents } from '@/hooks/useAgents'
import { useProjects } from '@/hooks/useProjects'
import { subscribeRealtimeEvents } from '@/lib/realtimeBus'
import { useOfficeStore } from '@/stores/officeStore'

// ─── 办公室数据管道 ───
// 种子：agents 花名册 + 各项目活跃任务详情快照（权威状态）
// 实时：订阅 realtimeBus 广播的 SSE 事件（真实驱动，不表演）
// 查询 key 与任务页共用（['tasks', projectId] / ['tasks','detail',id]），共享缓存。

const ACTIVE_TASK_STATUSES = new Set([
  'planning',
  'review',
  'pending',
  'in_progress',
  'awaiting_review',
  'waiting_user',
])

const MAX_ACTIVE_TASK_DETAILS = 24

/** 办公室侧栏「执行中任务」用的轻量任务行 */
export interface OfficeTaskRow {
  id: string
  projectId: string
  title: string
  status: string
  updatedAt: string
}

/**
 * 各活跃项目的进行中任务（与 useOfficeData 共用同一批 query 缓存，
 * 在办公室页两个 hook 同时调用也只发一次请求）。
 */
export function useActiveOfficeTasks(): OfficeTaskRow[] {
  const { data: projects } = useProjects()

  const activeProjectIds = useMemo(
    () => (projects ?? []).filter((p) => p.status === 'active').map((p) => p.id),
    [projects],
  )

  const taskListQueries = useQueries({
    queries: activeProjectIds.map((projectId) => ({
      queryKey: ['tasks', projectId, undefined],
      queryFn: async () => (await tasksApi.listProjectTasks(projectId)).data.items,
      staleTime: 30_000,
      // SSE 实时为主，低频轮询兜底：办公室是全站唯一没有其他轮询来源的实时页面，
      // SSE 断链期间（重连窗/token 过期）没有兜底就会冻结到刷新页面。
      refetchInterval: 30_000,
    })),
  })

  return useMemo(() => {
    const rows: OfficeTaskRow[] = []
    for (const query of taskListQueries) {
      for (const task of query.data ?? []) {
        if (ACTIVE_TASK_STATUSES.has(task.status)) {
          rows.push({
            id: task.id,
            projectId: task.project_id,
            title: task.title || '未命名任务',
            status: task.status,
            updatedAt: task.updated_at,
          })
        }
      }
    }
    return rows.sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
  }, [taskListQueries])
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
    useOfficeStore.getState().seedAgents(agents.filter((a) => !a.archived))
  }, [agents])

  // 2. 各项目任务列表（与项目工作台共用缓存 key）
  const taskListQueries = useQueries({
    queries: activeProjectIds.map((projectId) => ({
      queryKey: ['tasks', projectId, undefined],
      queryFn: async () => (await tasksApi.listProjectTasks(projectId)).data.items,
      staleTime: 30_000,
      // 同上：办公室无其他轮询来源，SSE 断链期间靠它兜底
      refetchInterval: 30_000,
    })),
  })

  // 3. 最近更新的活跃任务详情 —— 权威快照，可覆盖/修复事件推导出的状态。
  //    这也是 SSE 30 分钟断连（后端 sseMaxDuration）后的状态兜底。
  const activeTaskIds = useMemo(() => {
    const rows: { id: string; updated_at: string }[] = []
    for (const query of taskListQueries) {
      for (const task of query.data ?? []) {
        if (ACTIVE_TASK_STATUSES.has(task.status)) {
          rows.push({ id: task.id, updated_at: task.updated_at })
        }
      }
    }
    return rows
      .sort((a, b) => b.updated_at.localeCompare(a.updated_at))
      .slice(0, MAX_ACTIVE_TASK_DETAILS)
      .map((row) => row.id)
  }, [taskListQueries])

  const detailQueries = useQueries({
    queries: activeTaskIds.map((taskId) => ({
      queryKey: ['tasks', 'detail', taskId],
      queryFn: async () => (await tasksApi.getTask(taskId)).data,
      staleTime: 5_000,
      // 权威快照兜底轮询：SSE 断链时 applyTaskDetail 靠它修复事件推导出的状态
      // （对齐任务详情页 useTask 的活跃任务轮询策略；SSE 健康时它只是低频校准）
      refetchInterval: 10_000,
    })),
  })

  // 依赖用 dataUpdatedAt 拼接的稳定字符串：useQueries 每次渲染返回新数组，
  // 直接以 detailQueries 为依赖会让 effect 在每次渲染后重跑；配合 applyTaskDetail
  // 的幂等性双保险，彻底杜绝「快照 → set → 重渲染 → 快照」的无限循环（React #185）。
  const detailStamp = detailQueries.map((q) => q.dataUpdatedAt).join(',')

  useEffect(() => {
    const store = useOfficeStore.getState()
    for (const query of detailQueries) {
      if (query.data) store.applyTaskDetail(query.data)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [detailStamp])

  // 4. SSE 事件 → 可视化状态
  useEffect(
    () =>
      subscribeRealtimeEvents((event) => {
        useOfficeStore.getState().applyRealtimeEvent(event)
      }),
    [],
  )

  const loading =
    !agents || (activeProjectIds.length > 0 && taskListQueries.some((q) => q.isLoading))

  return { loading }
}
