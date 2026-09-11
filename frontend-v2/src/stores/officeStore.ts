import { create } from 'zustand'
import { subscribeWithSelector } from 'zustand/middleware'
import type { Agent, TaskDetail } from '@/types'
import type { AgentVisual, AgentVisualState, RealtimeEvent } from '@/types/office'
import { assignDesks, OFFICE_THEMES, type DeskSlot, type OfficePalette, type OfficeTheme } from '@/components/office/officeLayout'
import { DEFAULT_TALK_SEC, type Mission } from '@/components/office/mission'

// ─── 办公室逻辑状态 ───
// 刻意不进 TanStack Query 缓存：这是高频、瞬时的视图派生状态，
// 种子 = agents 列表 + 活跃任务详情快照，更新 = realtimeBus 转发的 SSE 事件。
//
// 重要：本 store 只存「离散状态」和「目标位置」，不存每帧坐标。
// 每帧的插值运算在渲染层（useFrame + useRef）完成，绝不触发 React re-render。

const STICKER_DURATION_MS = 6_000
const BUBBLE_DURATION_MS = 8_000
const PROGRESS_THROTTLE_MS = 10_000

/**
 * 消息流条目：3D 气泡只放得下短摘要，完整消息沉淀在这里，
 * 由页面「最新动态」卡片滚动展示（可读 + 可滚动 + 可点击跳任务）。
 */
export interface OfficeFeedItem {
  id: string
  agentId: string
  agentName: string
  kind: 'normal' | 'question'
  text: string
  taskId?: string
  /** 任务所属项目（用于「点击跳到该任务」的 deep-link） */
  projectId?: string
  ts: number
}

/** 消息流最多保留的条数 */
const FEED_LIMIT = 40

/**
 * taskId → projectId。事件里只有 task_id，没有 project_id，
 * 而 deep-link 到任务需要 `/projects/:pid?task=:id`，
 * 所以在任务详情快照进来时顺手记下映射（不进 state，避免无谓 re-render）。
 */
const taskProjectMap = new Map<string, string>()

function truncate(text: string | null | undefined, max = 44): string {
  if (!text) return ''
  const clean = text.replace(/\s+/g, ' ').trim()
  return clean.length > max ? `${clean.slice(0, max)}…` : clean
}

function newVisual(id: string): AgentVisual {
  return {
    id,
    name: id,
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
}

interface OfficeState {
  /** agentId → 可视化状态 */
  agents: Record<string, AgentVisual>
  /** agentId → 工位（座位不足的 agent 不在此映射中） */
  seating: Record<string, DeskSlot>
  /** 待命区：工位不够时放不下的 agent */
  benched: string[]
  /** 当前聚焦的 Agent（相机对准它）；null = 全景视角 */
  focusAgentId: string | null
  /** agentId → 进行中的走动任务 */
  missions: Record<string, Mission>
  /** 最新消息流（最新在前；3D 气泡只放短摘要，完整文本沉淀在这里） */
  feed: OfficeFeedItem[]

  seedAgents(agents: Agent[], activeIds?: Set<string>): void
  setPresence(
    agentId: string,
    presence: AgentVisual['presence'],
    name?: string,
    role?: string,
  ): void
  applyTaskDetail(task: TaskDetail): void
  applyRealtimeEvent(event: RealtimeEvent): void
  showBubble(
    agentId: string,
    text: string,
    kind: 'normal' | 'question',
    throttle?: boolean,
    taskId?: string,
  ): void
  setState(agentId: string, state: AgentVisualState): void
  setFocus(agentId: string | null): void
  /** 发起工位拜访：visitor 走到 host 工位旁说话，说完回自己工位 */
  startVisit(visitorId: string, hostId: string, message: string): void
  /** 推进任务阶段（渲染层 useFrame 调用，低频，非每帧） */
  updateMission(agentId: string, patch: Partial<Mission>): void
  /** 结束任务 */
  endMission(agentId: string): void
  /** 场景主题：夜晚（暗系）/ 白天（亮系） */
  theme: OfficeTheme
  setTheme(theme: OfficeTheme): void
}

/** 主题持久化 key（记住用户上次的日/夜选择） */
const THEME_STORAGE_KEY = 'trustmesh-office-theme'

function loadInitialTheme(): OfficeTheme {
  try {
    const saved = localStorage.getItem(THEME_STORAGE_KEY)
    if (saved === 'night' || saved === 'day') return saved
  } catch {
    /* localStorage 不可用时静默回退 */
  }
  return 'night'
}

/** todo_progress 节流记录（不进 state，避免无谓的订阅通知） */
const lastProgressBubbleAt = new Map<string, number>()

export const useOfficeStore = create<OfficeState>()(
  subscribeWithSelector((set, get) => ({
    agents: {},
    seating: {},
    benched: [],
    focusAgentId: null,
    missions: {},
    feed: [],
    theme: loadInitialTheme(),

    setTheme(theme) {
      set({ theme })
      try {
        localStorage.setItem(THEME_STORAGE_KEY, theme)
      } catch {
        /* 忽略持久化失败 */
      }
    },

    setFocus(agentId) {
      set({ focusAgentId: agentId })
    },

    startVisit(visitorId, hostId, message) {
      const state = get()
      // 前置校验：双方都得在花名册里、都得有工位、不能自己找自己
      if (visitorId === hostId) return
      if (!state.agents[visitorId] || !state.seating[visitorId]) return
      if (!state.agents[hostId] || !state.seating[hostId]) return
      // 已有任务在身就不打断，避免走到一半被拉去别处
      if (state.missions[visitorId]) return

      set({
        missions: {
          ...state.missions,
          [visitorId]: {
            kind: 'visit',
            phase: 'goto',
            targetAgentId: hostId,
            message,
            talkDuration: DEFAULT_TALK_SEC,
            meetingSeatIndex: null,
          },
        },
      })
    },

    updateMission(agentId, patch) {
      const prev = get().missions[agentId]
      if (!prev) return
      set({ missions: { ...get().missions, [agentId]: { ...prev, ...patch } } })
    },

    endMission(agentId) {
      const next = { ...get().missions }
      delete next[agentId]
      set({ missions: next })
    },

    seedAgents(agents, activeIds) {
      const prev = get().agents
      const next: Record<string, AgentVisual> = {}

      for (const agent of agents) {
        next[agent.id] = {
          ...(prev[agent.id] ?? newVisual(agent.id)),
          name: agent.name,
          role: agent.role,
          presence: agent.status,
        }
      }

      // 座位分配：PM 优先独立办公室，其余「活跃优先 + id 哈希稳定」。
      // 关键：已落座的 Agent 尽量保持原位，只给新加入的填空位 ——
      // 否则任务活跃度一变就会导致全场座位重排、视觉上不停跳动。
      const candidates = agents.map((a) => ({
        id: a.id,
        role: a.role,
        active: activeIds ? activeIds.has(a.id) : false,
      }))
      const assignment = assignDesks(candidates)
      const prevSeating = get().seating

      const seating: Record<string, DeskSlot> = {}
      const used = new Set<string>()

      for (const agent of agents) {
        const prev = prevSeating[agent.id]
        if (prev && !used.has(prev.id)) {
          seating[agent.id] = prev
          used.add(prev.id)
        }
      }

      for (const agent of agents) {
        if (seating[agent.id]) continue
        const slot = assignment.get(agent.id)
        if (slot && !used.has(slot.id)) {
          seating[agent.id] = slot
          used.add(slot.id)
        }
      }

      const benched = agents.filter((a) => !seating[a.id]).map((a) => a.id)

      set({ agents: next, seating, benched })
    },

    setPresence(agentId, presence, name, role) {
      const prev = get().agents[agentId] ?? newVisual(agentId)
      set({
        agents: {
          ...get().agents,
          [agentId]: {
            ...prev,
            presence,
            name: name ?? prev.name,
            role: role ?? prev.role,
          },
        },
      })
    },

    setState(agentId, state) {
      const prev = get().agents[agentId]
      if (!prev || prev.state === state) return
      set({ agents: { ...get().agents, [agentId]: { ...prev, state } } })
    },

    showBubble(agentId, text, kind, throttle = false, taskId?: string) {
      if (!text) return
      const now = Date.now()
      if (throttle) {
        const last = lastProgressBubbleAt.get(agentId) ?? 0
        if (now - last < PROGRESS_THROTTLE_MS) return
        lastProgressBubbleAt.set(agentId, now)
      }
      const prev = get().agents[agentId]
      if (!prev) return

      // 完整文本沉淀到消息流（3D 气泡只显示短摘要，HTML 卡片可滚动看全文）
      const item: OfficeFeedItem = {
        id: `${agentId}-${now}-${Math.random().toString(36).slice(2, 7)}`,
        agentId,
        agentName: prev.name,
        kind,
        text,
        taskId,
        projectId: taskId ? taskProjectMap.get(taskId) : undefined,
        ts: now,
      }

      set({
        agents: {
          ...get().agents,
          [agentId]: {
            ...prev,
            bubble: text,
            bubbleKind: kind,
            bubbleUntil: now + BUBBLE_DURATION_MS,
          },
        },
        feed: [item, ...get().feed].slice(0, FEED_LIMIT),
      })
    },

    /** 任务详情快照 → 每个参与者的派生状态（权威数据，覆盖事件推导） */
    applyTaskDetail(task) {
      // 记下 taskId → projectId，供消息流 deep-link 使用
      if (task.id && task.project_id) taskProjectMap.set(task.id, task.project_id)

      const agents = { ...get().agents }
      const states = new Map<string, AgentVisualState>()
      const bubbles = new Map<string, { text: string; kind: 'normal' | 'question' }>()

      // 兜底：任务已终结（完成/失败/取消）时，所有参与者回到 idle。
      // 否则 agent 会一直停在 working —— 事件流一旦中断（任务结束不再推送
      // todo_* 事件），「状态不实时更新」就表现为卡在执行中。
      const finished =
        task.status === 'done' || task.status === 'failed' || task.status === 'canceled'

      for (const todo of task.todos ?? []) {
        const assigneeId = todo.assignee?.agent_id
        if (!assigneeId) continue
        if (finished) {
          states.set(assigneeId, 'idle')
          continue
        }
        const unanswered = (todo.questions ?? []).find((q) => !q.answer)

        if (todo.status === 'waiting_user' || unanswered) {
          // 未答问题最需要用户注意，优先级最高
          states.set(assigneeId, 'asking')
          bubbles.set(assigneeId, {
            text: truncate(unanswered?.question ?? todo.title),
            kind: 'question',
          })
        } else if (todo.status === 'in_progress') {
          if (states.get(assigneeId) !== 'asking') states.set(assigneeId, 'working')
        }
      }

      // PM 在任务规划/评审阶段进入思考
      const pmId = task.pm_agent?.id
      if (pmId && (task.status === 'planning' || task.status === 'review')) {
        states.set(pmId, 'thinking')
      }

      let changed = false
      for (const [agentId, state] of states) {
        const prev = agents[agentId] ?? newVisual(agentId)
        const bubble = bubbles.get(agentId)
        // 问题气泡常驻直到被回答；其余保留原样。
        // 关键：必须做幂等比较 —— waiting_user 的问题在每次任务快照里都会出现，
        // 若「有 bubble 就算 changed」，快照轮询会无限 set() → React #185
        // （Maximum update depth exceeded）直接把办公室页面打崩。
        let nextBubble: { bubble: string | null; bubbleKind: 'normal' | 'question'; bubbleUntil: number } | null = null
        if (bubble) {
          if (
            prev.bubble !== bubble.text ||
            prev.bubbleKind !== bubble.kind ||
            prev.bubbleUntil !== 0
          ) {
            nextBubble = { bubble: bubble.text, bubbleKind: bubble.kind, bubbleUntil: 0 }
          }
        } else if (prev.bubbleKind === 'question') {
          nextBubble = { bubble: null, bubbleKind: 'normal' as const, bubbleUntil: 0 }
        }

        if (prev.state !== state || nextBubble) {
          agents[agentId] = {
            ...prev,
            state,
            taskId: task.id,
            ...(nextBubble ?? {}),
          }
          changed = true
        }
      }

      if (changed) set({ agents })
    },

    applyRealtimeEvent(event) {
      switch (event.type) {
        case 'agent.status.changed': {
          const agent = event.payload.agent
          get().setPresence(agent.id, agent.status, agent.name, agent.role)
          return
        }
        case 'task.updated': {
          get().applyTaskDetail(event.payload.task)
          return
        }
        case 'task.event.created': {
          applyTaskEvent(get, set, event.payload.event)
          return
        }
        default:
          return
      }
    },
  })),
)

type Get = () => OfficeState
// 注意：不要命名为 Set —— 会遮蔽内置的 Set<T> 泛型，导致 `Set<string>` 报
// "Type 'Set' is not generic"（本文件 seedAgents 就用到了 Set<string>）。
type StoreSet = (partial: Partial<OfficeState>) => void

/**
 * 找某个角色已落座的 Agent。
 * 优先返回坐在该角色专属工位上的（PM 有独立办公室），否则返回任意一个该角色。
 */
function findAgentIdByRole(state: OfficeState, role: string): string | null {
  const seated = Object.keys(state.seating)
  for (const id of seated) {
    if (state.agents[id]?.role === role && state.seating[id]?.kind === 'pm') return id
  }
  for (const id of seated) {
    if (state.agents[id]?.role === role) return id
  }
  return null
}

/**
 * 确定「谁走过去」。
 * 后端不少事件是系统代发的：actor_id 可能是 'system' 或角色名（如 'reviewer'），
 * 并非真实 agent id —— 这时要还原成对应角色的真人，否则没人走动。
 */
function resolveVisitor(state: OfficeState, actorId: string): string | null {
  if (state.agents[actorId] && state.seating[actorId]) return actorId
  const byRole = findAgentIdByRole(state, actorId)
  if (byRole) return byRole
  return findAgentIdByRole(state, 'pm')
}

/**
 * 系统/用户代发的事件（如派单 todo_assigned 的 actor 是 system）没有真实
 * agent id，需要从 metadata 里还原接单人。此前 `actor_type !== 'agent'`
 * 直接 return，导致派单既不显示气泡也不会走动 —— 这里改为按 metadata 定位。
 */
function resolveEventAgent(
  event: { actor_type: string; actor_id: string; metadata: Record<string, unknown> },
): string {
  if (event.actor_type === 'agent') return event.actor_id
  const fromMeta = event.metadata?.assignee_agent_id ?? event.metadata?.agent_id
  return typeof fromMeta === 'string' ? fromMeta : ''
}

/** SSE 任务事件 → 气泡 / 贴纸 / 状态 */
function applyTaskEvent(
  get: Get,
  set: StoreSet,
  event: {
    actor_type: string
    actor_id: string
    actor_name?: string
    event_type: string
    content: string | null
    task_id?: string
    metadata: Record<string, unknown>
  },
) {
  const agentId = resolveEventAgent(event)
  const taskId = event.task_id

  // 没有明确 agent 的系统/用户事件：只有「任务创建」需要 PM 接需求。
  // 其余（如 task_status_changed）在办公室里没有对应表现，直接忽略。
  if (!agentId) {
    if (event.event_type === 'task_created') {
      const pmId = findAgentIdByRole(get(), 'pm')
      if (pmId) {
        get().setState(pmId, 'thinking')
        const title =
          typeof event.metadata?.task_title === 'string' ? event.metadata.task_title : ''
        get().showBubble(pmId, `新需求：${truncate(title || event.content || '', 20)}`, 'normal')
      }
    }
    return
  }

  const prev = get().agents[agentId]
  if (!prev) return // 该 agent 不在花名册内（未落座），忽略

  const now = Date.now()
  const todoTitle =
    typeof event.metadata?.todo_title === 'string' ? event.metadata.todo_title : ''
  const base = { ...prev, name: event.actor_name ?? prev.name, taskId: event.task_id ?? prev.taskId }

  switch (event.event_type) {
    case 'todo_progress':
      set({ agents: { ...get().agents, [agentId]: { ...base, state: 'working' } } })
      get().showBubble(agentId, truncate(event.content), 'normal', true, taskId)
      return

    case 'todo_started':
      set({ agents: { ...get().agents, [agentId]: { ...base, state: 'working' } } })
      get().showBubble(agentId, truncate(event.content || todoTitle), 'normal', false, taskId)
      return

    case 'todo_completed':
    case 'todo_failed': {
      const failed = event.event_type === 'todo_failed'
      set({
        agents: {
          ...get().agents,
          [agentId]: {
            ...base,
            state: 'idle',
            sticker: failed ? 'failed' : 'celebrate',
            stickerUntil: now + STICKER_DURATION_MS,
          },
        },
      })
      get().showBubble(agentId, truncate(event.content), 'normal', false, taskId)
      return
    }

    case 'todo_ask_received': {
      const question = truncate(event.content || todoTitle) || '需要你的输入'
      set({
        agents: {
          ...get().agents,
          [agentId]: {
            ...base,
            state: 'asking',
            bubble: question,
            bubbleKind: 'question',
            bubbleUntil: 0, // 常驻直到被回答
          },
        },
      })
      // 走到 PM 工位当面问（问题最需要被看见）
      const pmId = findAgentIdByRole(get(), 'pm')
      if (pmId && pmId !== agentId) get().startVisit(agentId, pmId, question)
      return
    }

    case 'todo_answer_received':
      set({
        agents: {
          ...get().agents,
          [agentId]: {
            ...base,
            state: 'working',
            bubble: '收到答复，继续执行',
            bubbleKind: 'normal',
            bubbleUntil: now + BUBBLE_DURATION_MS,
          },
        },
      })
      // 答复到了，结束拜访回自己工位
      if (get().missions[agentId]) get().updateMission(agentId, { phase: 'return' })
      return

    case 'todo_assigned': {
      get().setState(agentId, 'working')
      get().showBubble(
        agentId,
        todoTitle ? `新任务：${truncate(todoTitle, 20)}` : truncate(event.content),
        'normal',
        false,
        taskId,
      )
      // 派单方走到接单方工位当面交代。
      // 退回重做同样走这条路：后端先发 todo_rework_requested，紧接着发 todo_assigned。
      //
      // 注意：todo_assigned 由「系统」代发（actor_type = system），此时 event.actor_id
      // 不是真人，resolveVisitor 会回退到 PM —— 由 PM 走过去派活。
      // 判断条件必须比较「走动的人」与「被派单的人」，不能拿 agentId 比较
      // （agentId 本身就是接单人，比出来永远相等，走动会被跳过）。
      const assigneeId = event.metadata?.assignee_agent_id
      if (typeof assigneeId === 'string') {
        const visitor = resolveVisitor(get(), event.actor_id)
        if (visitor && visitor !== assigneeId) {
          get().startVisit(
            visitor,
            assigneeId,
            todoTitle ? `派活：${truncate(todoTitle, 14)}` : '有新任务',
          )
        }
      }
      return
    }

    case 'todo_awaiting_review': {
      get().showBubble(agentId, '交付待评审', 'normal', false, taskId)
      const reviewerId = findAgentIdByRole(get(), 'reviewer')
      if (reviewerId && reviewerId !== agentId) {
        get().startVisit(agentId, reviewerId, '交付待评审')
      }
      return
    }

    case 'todo_review_approved':
    case 'todo_review_rejected':
      // 评审结束，回到自己工位
      if (get().missions[agentId]) get().updateMission(agentId, { phase: 'return' })
      return

    case 'planning_reply':
    case 'task_comment':
      get().showBubble(agentId, truncate(event.content), 'normal', false, taskId)
      return

    case 'task_created':
      // 任务刚创建，PM 开始思考（PM 身份由 task.updated 快照修正）
      get().setState(agentId, 'thinking')
      return

    case 'task_plan_ready':
      get().setState(agentId, 'idle')
      return

    default:
      return
  }
}

// 开发期调试出口：便于在浏览器控制台 / 自动化冒烟里检查走动状态机。
// 只在 DEV 生效，生产构建会被 tree-shake 掉。
if (import.meta.env.DEV) {
  ;(window as unknown as Record<string, unknown>).__officeStore = useOfficeStore
}

/** 订阅当前主题并拿到对应色板（各 3D 组件统一从这里取色） */
export function useOfficePalette(): OfficePalette {
  const theme = useOfficeStore((s) => s.theme)
  return OFFICE_THEMES[theme]
}
