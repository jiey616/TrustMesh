import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useAuthStore } from '@/stores/authStore'
import { API_BASE } from '@/api/client'
import { notifyTaskEvent } from '@/lib/native'

// 照搬桌面端 hooks/useRealtimeEvents.ts（SSE → 失效 React Query → 页面实时刷新），
// 四个已踩过的坑必须保留：CRLF 切帧、token 过期重连死循环防护、
// 30min 干净关闭后的重连、task.event.created 的双层 payload 结构。

const SSE_URL = `${API_BASE}events/stream`

interface SseEvent {
  event: string
  data: string
}

/** 解析 SSE 文本块为事件列表（兼容多条 event/data 及多行 data） */
function parseSSE(chunk: string): SseEvent[] {
  const events: SseEvent[] = []
  let event = 'message'
  let dataLines: string[] = []
  for (const line of chunk.split('\n')) {
    if (line === '') {
      if (dataLines.length) {
        events.push({ event, data: dataLines.join('\n') })
        dataLines = []
      }
      event = 'message'
    } else if (line.startsWith('event:')) {
      event = line.slice(6).trim()
    } else if (line.startsWith('data:')) {
      dataLines.push(line.slice(5).replace(/^ /, ''))
    }
  }
  if (dataLines.length) events.push({ event, data: dataLines.join('\n') })
  return events
}

/** 事件类型 → 需要失效的移动端 React Query 前缀。
 *  移动端任务键是 ['task', id]（单任务）+ ['projectTasks', projectId]（列表），
 *  与桌面端 ['tasks', ...] 不同，映射按移动端命名来。 */
function invalidationsFor(type: string): ReadonlyArray<readonly unknown[]> {
  switch (type) {
    case 'notification.created':
    case 'notification.read':
    case 'notifications.all_read':
      return [['notifications'], ['unreadCount']]
    case 'task.updated':
    case 'task.comment.created':
      return [['task'], ['projectTasks'], ['workflowProgress']]
    default:
      return []
  }
}

/** 后端 SSE 实时事件订阅：收到事件后失效对应 query，实现页面实时刷新。 */
export function useRealtimeEvents() {
  const qc = useQueryClient()
  // access token 不持久化，页面加载时初始为 null：必须订阅它，
  // 否则 token 就绪后 SSE 连接不会建立、实时刷新全部失效（桌面端同款根因）。
  const accessToken = useAuthStore((s) => s.accessToken)

  useEffect(() => {
    if (!accessToken) return
    let disposed = false
    let retryTimer: ReturnType<typeof setTimeout> | undefined
    let controller: AbortController | null = null

    const handlePayload = (payload: Record<string, unknown>) => {
      const type = typeof payload.type === 'string' ? payload.type : ''

      // P3.2：任务终态（完成/失败）弹本地通知（原生壳内；Web no-op）
      if (type === 'task.updated') {
        const task = (payload.payload as { task?: { id?: string; title?: string; status?: string } } | undefined)?.task
        if (task?.id) {
          if (task.status === 'done') {
            notifyTaskEvent({ title: '任务已完成', body: task.title ? `「${task.title}」已完成` : '任务已完成', taskId: task.id })
          } else if (task.status === 'failed') {
            notifyTaskEvent({ title: '任务失败', body: task.title ? `「${task.title}」执行失败` : '任务执行失败', taskId: task.id })
          }
        }
      }

      const inv = invalidationsFor(type)
      if (inv.length > 0) {
        for (const key of inv) void qc.invalidateQueries({ queryKey: key })
        return
      }

      // task.event.created：payload 为 { payload: { task_id, event: { event_type, ... } } }（双层）。
      if (type === 'task.event.created') {
        const envelope = payload.payload as
          | { task_id?: string; event?: { event_type?: string; task_id?: string } }
          | undefined
        const eventType = envelope?.event?.event_type
        const taskId = envelope?.task_id ?? envelope?.event?.task_id
        if (!eventType || !taskId) return
        // P3.2：原生壳内把需要人关注的事件弹成本地通知（Web 上 no-op）
        const todoTitle = (envelope?.event as { todo_title?: string } | undefined)?.todo_title ?? ''
        if (eventType === 'todo_completed') {
          notifyTaskEvent({ title: '步骤已完成', body: todoTitle ? `「${todoTitle}」已完成` : '有步骤已完成', taskId })
        } else if (eventType === 'todo_ask_received') {
          notifyTaskEvent({ title: '需要你确认', body: todoTitle ? `「${todoTitle}」需要你确认` : '有事项需要你确认', taskId })
        } else if (eventType === 'todo_failed') {
          notifyTaskEvent({ title: '执行失败', body: todoTitle ? `「${todoTitle}」执行失败` : '有步骤执行失败', taskId })
        }
        if (eventType === 'todo_progress') {
          // 高频：只精准失效该任务的事件流，避免详情页轮询风暴
          void qc.invalidateQueries({ queryKey: ['taskEvents', taskId] })
          return
        }
        void qc.invalidateQueries({ queryKey: ['taskEvents', taskId] })
        void qc.invalidateQueries({ queryKey: ['task', taskId] })
      }
    }

    const scheduleReconnect = () => {
      if (disposed) return
      retryTimer = setTimeout(() => void connect(), 5000)
    }

    const connect = async () => {
      // 每次重连都从 store 现取 token：token TTL(15m) < 服务端 SSE 最长连接(30m)，
      // 用闭包里的旧 token 重连必 401 → 5s 死循环（桌面端踩过的坑）。
      const token = useAuthStore.getState().accessToken
      if (!token) {
        scheduleReconnect()
        return
      }
      controller = new AbortController()
      try {
        const res = await fetch(SSE_URL, {
          headers: { Authorization: `Bearer ${token}`, Accept: 'text/event-stream' },
          signal: controller.signal,
        })
        if (!res.ok || !res.body) throw new Error(`SSE status ${res.status}`)

        const reader = res.body.getReader()
        const decoder = new TextDecoder()
        let buffer = ''
        for (;;) {
          const { done, value } = await reader.read()
          if (done) break
          buffer += decoder.decode(value, { stream: true })
          // 后端 gin SSEvent 用 CRLF 行尾，帧分隔 \r\n\r\n：按 \r?\n\r?\n 切分兼容两种。
          const frames = buffer.split(/\r?\n\r?\n/)
          buffer = frames.pop() ?? ''
          for (const raw of frames) {
            for (const ev of parseSSE(raw)) {
              if (ev.event !== 'snapshot') continue
              try {
                handlePayload(JSON.parse(ev.data) as Record<string, unknown>)
              } catch {
                // 忽略无法解析的数据
              }
            }
          }
        }
        // 服务端 30 分钟到期后「干净关闭」也会走到这里：必须重连，否则 SSE 静默死亡。
        if (!disposed) scheduleReconnect()
      } catch {
        if (disposed) return
        scheduleReconnect()
      }
    }

    void connect()
    return () => {
      disposed = true
      controller?.abort()
      if (retryTimer) clearTimeout(retryTimer)
    }
  }, [accessToken, qc])
}
