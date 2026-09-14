import type { Agent, TaskDetail } from '@/types'
import type { RealtimeEvent } from '@/realtime/types'

// ─── 办公室模拟状态机 ───
// 独立的衍生视图状态（刻意不进 TanStack Query 缓存）：
//   种子 = agents 列表 + 活跃任务详情快照
//   更新 = RealtimeProvider 转发的 SSE 事件（真实驱动，不表演）
// 场景每帧读取 getState() 渲染；sticker/bubble 带 until 时间戳自动过期。

export type AgentVisualState = 'working' | 'asking' | 'thinking' | 'idle'
export type AgentPresence = 'online' | 'offline' | 'busy'
export type AgentSticker = 'celebrate' | 'failed'

export interface AgentVisual {
  id: string
  name: string
  role: string
  presence: AgentPresence
  /** 派生状态：asking(有未答问题) > working(有 in_progress todo) > thinking(PM 规划中) > idle */
  state: AgentVisualState
  sticker: AgentSticker | null
  stickerUntil: number
  bubble: string | null
  /** epoch ms；0 表示常驻（等待用户回答），到点自动消失 */
  bubbleUntil: number
  bubbleKind: 'normal' | 'question'
  /** 最近关联的任务，供点击跳转 */
  taskId: string | null
}

const STICKER_DURATION_MS = 6_000
const BUBBLE_DURATION_MS = 8_000
const PROGRESS_THROTTLE_MS = 10_000

function truncate(text: string | null | undefined, max = 44): string {
  if (!text) return ''
  const clean = text.replace(/\s+/g, ' ').trim()
  return clean.length > max ? `${clean.slice(0, max)}…` : clean
}

class OfficeSimulationStore {
  private agents = new Map<string, AgentVisual>()
  private listeners = new Set<() => void>()
  private snapshot: AgentVisual[] = []
  private lastProgressBubbleAt = new Map<string, number>()

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener)
    return () => {
      this.listeners.delete(listener)
    }
  }

  getState = (): AgentVisual[] => this.snapshot

  private touch(agentId: string): AgentVisual {
    let agent = this.agents.get(agentId)
    if (!agent) {
      agent = {
        id: agentId,
        name: agentId,
        role: 'custom',
        presence: 'online',
        state: 'idle',
        sticker: null,
        stickerUntil: 0,
        bubble: null,
        bubbleUntil: 0,
        bubbleKind: 'normal',
        taskId: null,
      }
      this.agents.set(agentId, agent)
    }
    return agent
  }

  private commit() {
    this.snapshot = [...this.agents.values()].map((agent) => ({ ...agent }))
    for (const listener of this.listeners) listener()
  }

  /** 用 agents 列表重建花名册（保留已衍生出的状态） */
  seedAgents(agents: Agent[]) {
    const known = new Set(agents.map((a) => a.id))
    for (const id of [...this.agents.keys()]) {
      if (!known.has(id)) this.agents.delete(id)
    }
    for (const agent of agents) {
      const visual = this.touch(agent.id)
      visual.name = agent.name
      visual.role = agent.role
      visual.presence = agent.status
    }
    this.commit()
  }

  setPresence(agentId: string, presence: AgentPresence, name?: string, role?: string) {
    const visual = this.touch(agentId)
    visual.presence = presence
    if (name) visual.name = name
    if (role) visual.role = role
    this.commit()
  }

  /** 任务详情快照 → 每个参与者的派生状态（权威数据，覆盖事件推导） */
  applyTaskDetail(task: TaskDetail) {
    const states = new Map<string, AgentVisualState>()
    const bubbles = new Map<string, { text: string; kind: 'normal' | 'question' }>()

    for (const todo of task.todos ?? []) {
      const assigneeId = todo.assignee?.agent_id
      if (!assigneeId) continue
      const unanswered = (todo.questions ?? []).find((q) => !q.answer)

      if (todo.status === 'waiting_user' || unanswered) {
        // 未答问题是最需要用户注意的状态，优先级最高
        const question = unanswered?.question ?? todo.title
        states.set(assigneeId, 'asking')
        bubbles.set(assigneeId, { text: truncate(question), kind: 'question' })
      } else if (todo.status === 'in_progress') {
        if (states.get(assigneeId) !== 'asking') states.set(assigneeId, 'working')
      }
    }

    // PM 在任务规划/评审阶段进入思考
    const pmId = task.pm_agent?.id
    if (pmId && (task.status === 'planning' || task.status === 'review')) {
      states.set(pmId, 'thinking')
    }

    for (const [agentId, state] of states) {
      const visual = this.touch(agentId)
      if (visual.state !== state) visual.state = state
      const taskRef = task.id
      visual.taskId = taskRef

      const bubble = bubbles.get(agentId)
      if (bubble) {
        visual.bubble = bubble.text
        visual.bubbleKind = bubble.kind
        visual.bubbleUntil = 0 // 常驻直到问题被回答
      } else if (visual.bubbleKind === 'question') {
        // 问题已答（权威快照里没有未答问题了），清理常驻气泡
        visual.bubble = null
        visual.bubbleKind = 'normal'
        visual.bubbleUntil = 0
      }
    }
    this.commit()
  }

  /** SSE 事件 → 气泡/贴纸/状态覆盖 */
  applyRealtimeEvent(event: RealtimeEvent, now = Date.now()) {
    switch (event.type) {
      case 'agent.status.changed': {
        const agent = event.payload.agent
        this.setPresence(agent.id, agent.status, agent.name, agent.role)
        return
      }
      case 'task.updated': {
        this.applyTaskDetail(event.payload.task)
        return
      }
      case 'task.event.created': {
        this.applyTaskEvent(event.payload.event, now)
        return
      }
      default:
        return
    }
  }

  private applyTaskEvent(event: {
    actor_type: string
    actor_id: string
    actor_name?: string
    event_type: string
    content: string | null
    task_id?: string
    todo_id?: string
    metadata: Record<string, unknown>
  }, now: number) {
    if (event.actor_type !== 'agent') {
      // 用户/系统发起的事件：任务创建后 PM 开始思考
      if (event.event_type === 'task_created' && event.task_id) {
        // PM 身份未知，等 task.updated 快照修正；这里仅记录任务关联
        return
      }
      return
    }

    const agentId = event.actor_id
    const visual = this.touch(agentId)
    if (event.actor_name) visual.name = event.actor_name
    visual.taskId = event.task_id ?? visual.taskId

    const todoTitle = typeof event.metadata?.todo_title === 'string' ? event.metadata.todo_title : ''

    switch (event.event_type) {
      case 'todo_progress': {
        visual.state = 'working'
        this.showBubble(agentId, truncate(event.content), 'normal', now, true)
        break
      }
      case 'todo_started': {
        visual.state = 'working'
        this.showBubble(agentId, truncate(event.content || todoTitle), 'normal', now, false)
        break
      }
      case 'todo_completed': {
        visual.state = 'idle'
        visual.sticker = 'celebrate'
        visual.stickerUntil = now + STICKER_DURATION_MS
        this.showBubble(agentId, truncate(event.content), 'normal', now, false)
        break
      }
      case 'todo_failed': {
        visual.state = 'idle'
        visual.sticker = 'failed'
        visual.stickerUntil = now + STICKER_DURATION_MS
        this.showBubble(agentId, truncate(event.content), 'normal', now, false)
        break
      }
      case 'todo_ask_received': {
        visual.state = 'asking'
        visual.bubble = truncate(event.content || todoTitle) || '需要你的输入'
        visual.bubbleKind = 'question'
        visual.bubbleUntil = 0 // 常驻直到回答
        break
      }
      case 'todo_answer_received': {
        visual.state = 'working'
        visual.bubble = '收到答复，继续执行'
        visual.bubbleKind = 'normal'
        visual.bubbleUntil = now + BUBBLE_DURATION_MS
        break
      }
      case 'todo_assigned': {
        this.showBubble(agentId, todoTitle ? `新任务：${truncate(todoTitle, 36)}` : truncate(event.content), 'normal', now, false)
        break
      }
      case 'planning_reply':
      case 'task_comment': {
        this.showBubble(agentId, truncate(event.content), 'normal', now, false)
        break
      }
      default:
        return
    }
    // 事件已修改状态，统一提交快照并通知订阅者
    this.commit()
  }

  private showBubble(agentId: string, text: string, kind: 'normal' | 'question', now: number, throttle: boolean) {
    if (!text) return
    if (throttle) {
      const last = this.lastProgressBubbleAt.get(agentId) ?? 0
      if (now - last < PROGRESS_THROTTLE_MS) return
      this.lastProgressBubbleAt.set(agentId, now)
    }
    const visual = this.agents.get(agentId)
    if (!visual) return
    visual.bubble = text
    visual.bubbleKind = kind
    visual.bubbleUntil = now + BUBBLE_DURATION_MS
  }
}

export const officeSimulation = new OfficeSimulationStore()
