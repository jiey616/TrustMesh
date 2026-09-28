import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useAuthStore } from '@/stores/authStore'
import { getApiBase } from '@/stores/serverConfigStore'
import { emitRealtimeEvent } from '@/lib/realtimeBus'
import { notificationTarget, shouldNotifyDesktop, type NotificationTargetSource } from '@/lib/notifications'
import { ensureAccessTokenReady, refreshAccessTokenOnce } from '@/api/client'
import type { RealtimeEvent } from '@/types/office'

const SSE_URL = `${getApiBase()}events/stream`

/**
 * SSE 断线重连的退避区间。
 * 固定间隔在故障期会让大批客户端同频重连（生产实测 48h 内 79,658 次无谓重连），
 * 退避把压力摊开，同时保证单机网络抖动时能快速恢复。
 */
const SSE_INITIAL_RETRY_MS = 1_000
const SSE_MAX_RETRY_MS = 30_000

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
  // 🔴 白名单筛选：用户只要「待人工确认」与「任务完成/失败」两类系统横幅，其余只进收件箱。
  //    判据是后端的 `source_event`（机器可读），不是标题文本 —— 见 `shouldNotifyDesktop`。
  if (!shouldNotifyDesktop(n)) return
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
    // 重连退避：0 表示尚未失败（下次失败用初始延迟）。
    let retryDelay = 0
    // 本轮重连是否已尝试过刷新 token —— 一轮只刷一次，防 refresh/401 风暴。
    let refreshedInCycle = false

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
      try {
        // 🔴 冷启动门闩：accessToken 刻意不持久化（只持久化 refreshToken），整页刷新后
        // 首轮为空。必须复用 apiClient 的同一原语（client.ts 当初就是为 XHR / 裸 fetch
        // 这类绕过 hooks 的通道导出的），否则 token 就绪前 SSE 只会空转。
        await ensureAccessTokenReady()
        if (disposed) return

        // 每次重连都从 store 现取 token，不能用闭包捕获的 accessToken：
        // token TTL(15m) < 服务端 SSE 最长连接(30m)，30min 强制断开后的重连若用旧
        // token 必 401。
        const token = useAuthStore.getState().accessToken
        if (!token) {
          scheduleReconnect()
          return
        }

        controller = new AbortController()
        const res = await fetch(SSE_URL, {
          headers: { Authorization: `Bearer ${token}`, Accept: 'text/event-stream' },
          signal: controller.signal,
        })

        // 🔴 401 自愈（2026-09-24）。SSE 走裸 fetch，绕过了 apiClient 的
        // 「401 → 单飞刷新 → 重放一次」链路，此前只能拿同一个过期 token 每 5s 死循环
        // 重试；而窗口被托盘/失焦时 RQ 会暂停全部轮询（refetchIntervalInBackground 默认
        // false）⇒ 没有业务请求 ⇒ 触发不了刷新 ⇒ 自锁。生产实测：0.2.5 客户端
        // 57,561 次 401 / 0 次成功，某僵尸机每小时精确 600 次、连跑 48h 不停。
        if (res.status === 401) {
          if (res.body) void res.body.cancel().catch(() => {})
          if (!refreshedInCycle) {
            refreshedInCycle = true
            await refreshAccessTokenOnce().catch(() => {
              // refreshToken 也失效时刷新会抛错：交给重连退避兜住，不在此登出。
            })
          }
          scheduleReconnect()
          return
        }

        if (!res.ok || !res.body) throw new Error(`SSE status ${res.status}`)

        // 连上了 ⇒ 重置退避与「本轮已刷新」标记。
        retryDelay = 0
        refreshedInCycle = false

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

    // 指数退避：1s → 2s → 4s → … → 30s 封顶；连接成功后由 connect 内重置为 0。
    const scheduleReconnect = () => {
      if (disposed) return
      retryDelay = retryDelay === 0 ? SSE_INITIAL_RETRY_MS : Math.min(retryDelay * 2, SSE_MAX_RETRY_MS)
      retryTimer = window.setTimeout(() => void connect(), retryDelay)
    }

    void connect()
    return () => {
      disposed = true
      controller?.abort()
      if (retryTimer) window.clearTimeout(retryTimer)
    }
  }, [accessToken, qc])
}
