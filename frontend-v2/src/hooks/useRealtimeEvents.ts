import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useAuthStore } from '@/stores/authStore'
import { getApiBase } from '@/stores/serverConfigStore'
import { emitRealtimeEvent } from '@/lib/realtimeBus'
import { notificationTarget, type NotificationTargetSource } from '@/lib/notifications'
import type { RealtimeEvent } from '@/types/office'

const SSE_URL = `${getApiBase()}events/stream`

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

/** 事件类型 → 需要失效的 React Query 前缀 */
const EVENT_INVALIDATIONS: Record<string, ReadonlyArray<readonly unknown[]>> = {
  'notification.created': [
    ['notifications'],
    ['notifications', 'unread-count'],
  ],
  'notification.read': [
    ['notifications'],
    ['notifications', 'unread-count'],
  ],
  'notifications.all_read': [
    ['notifications'],
    ['notifications', 'unread-count'],
  ],
  'task.updated': [['tasks']],
  'task.comment.created': [['tasks']],
  'agent.status.changed': [['agents']],
  'join_request.created': [['join-requests']],
}

/**
 * 桌面端：把站内通知弹成**系统通知**（Windows 通知中心横幅）。
 * Web / 移动端没有 `window.desktop` 桥 ⇒ 静默 no-op（移动端的原生通知在
 * `frontend-mobile` 里另有一套，走 Capacitor LocalNotifications）。
 *
 * 🔴 为什么信 `notification.created`、而不是像移动端那样自己判 `task.updated` /
 * `task.event.created`：后端 `store_notification_internal.go` **已经**判定过「哪些事件
 * 值得通知」并生成了通知实体（标题/正文/分类/优先级/来源 id 齐全），客户端再判一遍
 * 就是把同一套规则实现两次，筛选口径与文案必然漂移。这里只做搬运。
 *
 * 窗口是否在前台由**主进程**判（`tm:show-notification` 里的 `BrowserWindow.isFocused()`）：
 * 用户正看着界面时不该再叠一条系统横幅，且那个判断在渲染端做不准。
 */
function notifyViaDesktopShell(n: NotificationTargetSource): void {
  const show = typeof window === 'undefined' ? undefined : window.desktop?.showNotification
  if (typeof show !== 'function') return
  if (!n.title) return
  // clickTarget 由主进程原样回传（见 preload 的 onNotificationClicked）。
  // 点击后落到哪一页与收件箱共用同一套规则，兜底为收件箱。
  void show({ title: n.title, body: n.body ?? '', clickTarget: notificationTarget(n) ?? '/inbox' })
}

/** 后端 SSE 实时事件订阅：收到事件后失效对应 query，实现页面实时刷新 */
export function useRealtimeEvents() {
  const qc = useQueryClient()
  // accessToken 不在 zustand persist 中（只持久化 refreshToken），页面加载时初始为 null，
  // 需等 401→refresh 或登录后才就绪。必须订阅它，否则 token 就绪后 SSE 连接不会建立，
  // 所有实时刷新（通知/任务/智能体对话）都失效——这是此前"浏览器从不发起 SSE"的根因。
  const accessToken = useAuthStore((s) => s.accessToken)

  useEffect(() => {
    if (!accessToken) return
    let disposed = false
    let retryTimer: number | undefined
    let controller: AbortController | null = null

    const handlePayload = (payload: Record<string, unknown>) => {
      // 广播原始事件给需要它的消费者（如办公室可视化）。
      // 必须放在最前面：下面的分支有提前 return（如 task.event.created 缺字段时），
      // 若放在后面会漏掉这部分事件。
      emitRealtimeEvent(payload as RealtimeEvent)
      const type = typeof payload.type === 'string' ? payload.type : ''

      // 桌面端系统通知。🔴 必须放在下面 `EVENT_INVALIDATIONS` 的提前 `return` **之前**：
      // notification.created 在那张表里有条目，先走失效分支就会被 return 掉，通知永不触发。
      if (type === 'notification.created') {
        const n = (payload.payload as { notification?: NotificationTargetSource } | undefined)?.notification
        if (n) notifyViaDesktopShell(n)
      }

      const inv = EVENT_INVALIDATIONS[type]
      if (inv) {
        for (const key of inv) void qc.invalidateQueries({ queryKey: key })
        return
      }
      // agent_chat.updated：智能体在详情页「对话」中回复后，后端会推送该事件并携带完整 chat。
      // 注意查询 key 是 ['agents', agentId, 'chat']（不是 'agent-chat'），必须按 payload 的 agent_id
      // 精准写入，才能让会话列表/详情页的回复实时出现，否则不刷新页面看不到回复。
      if (type === 'agent_chat.updated') {
        const chat = (payload.payload as { chat?: { agent_id?: string; messages?: unknown[] } } | undefined)?.chat
        const agentId = chat?.agent_id
        if (agentId) {
          // 直接写入最新会话，免去一次往返请求，回复即时上屏
          qc.setQueryData(['agents', agentId, 'chat'], chat)
          void qc.invalidateQueries({ queryKey: ['agents', agentId, 'chat', 'sessions'] })
        }
        return
      }
      // task.event.created 按具体事件类型过滤高频噪声。
      // 后端 payload 结构：{ payload: { task_id, project_id, event: { event_type, ... } } }，
      // event_type 在 payload.payload.event 内层 —— 此前少套了一层，eventType 恒为
      // undefined，该分支永远提前 return，任务事件从不失效 query（任务详情页有轮询
      // 兜底看不出来；办公室页无兜底，表现为状态冻结到刷新页面）。
      if (type === 'task.event.created') {
        const envelope = payload.payload as
          | { task_id?: string; event?: { event_type?: string; task_id?: string } }
          | undefined
        const eventType = envelope?.event?.event_type
        const taskId = envelope?.task_id ?? envelope?.event?.task_id
        if (!eventType || !taskId) return
        // todo_progress 是执行过程 feed 的主要内容，但高频：仅精准失效该任务的事件流查询，
        // 避免任务详情（含对话 messages）被高频拉取风暴波及，同时让「执行过程」近实时更新。
        if (eventType === 'todo_progress') {
          void qc.invalidateQueries({ queryKey: ['tasks', 'detail', taskId, 'events'] })
          return
        }
        // 其余事件（状态变更 / 智能体对话回复 / 规划回复(planning_reply) / 评论 等）：
        // 同时刷新「执行过程」与「任务详情」（任务详情含需求对话 messages 与状态），
        // 保证界面实时同步，避免此前 planning_reply 等非 IMPORTANT 事件被 SSE 忽略导致不刷新。
        void qc.invalidateQueries({ queryKey: ['tasks', 'detail', taskId, 'events'] })
        void qc.invalidateQueries({ queryKey: ['tasks', 'detail', taskId] })
      }
    }

    const connect = async () => {
      // 每次重连都从 store 现取 token，不能用闭包捕获的 accessToken：
      // token TTL(15m) < 服务端 SSE 最长连接(30m)，30min 强制断开后的重连若用旧
      // token 必 401，且 catch 分支会拿同一个过期 token 每 5s 死循环重试 ——
      // 标签页失焦时轮询暂停（refetchIntervalInBackground 默认 false）、无 API 调用
      // 触发刷新，SSE 便永久死亡，办公室等纯 SSE 页面冻结到刷新页面。
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
          // 后端 gin SSEvent 使用 CRLF(\r\n) 行尾，事件帧分隔为 \r\n\r\n。
          // 用 \n\n 切分永远匹配不到（\r 夹在中间），会导致事件帧无法被解析、
          // handlePayload 永不触发——这是实时刷新在浏览器里失效的根因。
          // 这里对齐老版本的做法：按 \r?\n\r?\n 切分，兼容 CRLF 与 LF。
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
        // 服务端「干净关闭」（如 30 分钟 sseMaxDuration 到期后正常结束流）会走到
        // 这里 —— done=true 退出循环，不抛错、不进 catch。此前没有重连，
        // SSE 静默死亡，之后所有事件（todo_completed 等）都收不到，
        // 表现为「办公室/任务状态不实时更新，只有刷新页面才能更新」。
        if (!disposed) scheduleReconnect()
      } catch {
        if (disposed) return
        scheduleReconnect()
      }
    }

    const scheduleReconnect = () => {
      if (disposed) return
      retryTimer = window.setTimeout(() => void connect(), 5000)
    }

    void connect()
    return () => {
      disposed = true
      controller?.abort()
      if (retryTimer) window.clearTimeout(retryTimer)
    }
  }, [accessToken, qc])
}
