// ─── 3D 办公室布局 ───
// 世界坐标：XZ 平面为地面，Y 为高度（沙盘悬浮在深色空间中，y=0 为沙盘地面）。
// 分区：背面一侧（z 负半轴）集中放 PM 独立办公室 + 会议桌区（「管理 + 会议翼」），
// 前方（z 正半轴，靠近相机）为主工位区（4 列 × 3 行）。
// 这样主工位区不被 PM/会议夹在中间、整体横向跨度收窄到约 ±6，
// 相机可拉近、单个 Agent 在画面中明显更大。
// 沿用 V1 的「稳定入座」思想：同一 agent 每次进页面座位不变（按 id 哈希）。

import type { AgentRole } from '@/types'
import type { AgentVisualState } from '@/types/office'

// ─── 沙盘尺寸 ───

/** 沙盘地面尺寸（three 单位，约等于米）
 *  尺度说明：人高 2.0，沙盘 18×15。主工位区前置、PM/会议合并到背面，
 *  包围盒约 12 宽 × 12.5 深（比旧 22 宽更方），默认机位拉到距离 ~13 即可全收，
 *  单个 Agent 约占画面高度 30%+，既清晰又不丢全局。 */
export const SANDBOX = {
  width: 18,
  depth: 15,
  /** 沙盘悬浮高度 */
  y: 0,
} as const

/** Agent 立体身高（Y）：Q 版小人总高，名牌等文字层挂在其上方。 */
export const AGENT_HEIGHT = 1.6

// ─── 工位 ───

export interface DeskSlot {
  id: string
  kind: 'worker' | 'pm'
  /** 座位点（地面 XZ） */
  seat: { x: number; z: number }
  /** 桌面中心（地面 XZ） */
  desk: { x: number; z: number }
  /** 家具朝向（绕 Y 轴弧度） */
  facing: number
}

// 家具按真实办公尺寸（约米），不要再放大 —— 家具过大会把人衬得很小
const DESK_WIDTH = 1.8
const DESK_DEPTH = 0.9

/** 工位到人/桌的相对偏移：人坐桌后（+Z），桌在人前（-Z） */
const SEAT_OFFSET_Z = 0.62
const DESK_OFFSET_Z = -0.42

function makeWorkerDesk(index: number, col: number, row: number): DeskSlot {
  // 主工位区前置（z 正半轴，靠近相机）：4 列 × 3 行，更方的 footprint。
  const x = -4.2 + col * 2.8
  const z = 1.2 + row * 2.7
  return {
    id: `desk-${index + 1}`,
    kind: 'worker',
    seat: { x, z: z + SEAT_OFFSET_Z },
    desk: { x, z: z + DESK_OFFSET_Z },
    // 人面朝 -Z（朝向桌子），three 默认物体朝 +Z，故旋转 π
    facing: Math.PI,
  }
}

/** 主工位区：4 列 × 3 行 = 12 位（前置） */
export const WORKER_DESKS: DeskSlot[] = Array.from({ length: 12 }, (_, i) =>
  makeWorkerDesk(i, i % 4, Math.floor(i / 4)),
)

/** PM 独立办公室（背面一侧，左） */
export const PM_DESK: DeskSlot = {
  id: 'desk-pm',
  kind: 'pm',
  seat: { x: -5.2, z: -3.2 },
  desk: { x: -5.2, z: -4.4 },
  facing: Math.PI,
}

export const DESK_SLOTS: DeskSlot[] = [PM_DESK, ...WORKER_DESKS]

// ─── 会议区（背面一侧，右） ───

export const MEETING_TABLE = {
  center: { x: 4.2, z: -3.2 },
  radiusX: 2.0,
  radiusZ: 1.4,
  /** 落座点（环绕会议桌，最多 6 位） */
  seats: [
    { x: 4.2, z: -1.4 },
    { x: 6.2, z: -2.5 },
    { x: 6.2, z: -3.9 },
    { x: 4.2, z: -5.0 },
    { x: 2.2, z: -3.9 },
    { x: 2.2, z: -2.5 },
  ] as ReadonlyArray<{ x: number; z: number }>,
} as const

// ─── 房间分区（用于地面材质与地毯） ───

export const ZONES = {
  pmOffice: { x: -5.2, z: -3.2, w: 4.4, d: 4.0 },
  meeting: { x: 4.2, z: -3.2, w: 5.4, d: 4.4 },
  workArea: { x: 0, z: 3.9, w: 11, d: 8 },
} as const

/** 每个分区的霓虹描边色（多巴胺色板） */
export const ZONE_GLOW = {
  pmOffice: '#6d5ff5',
  meeting: '#22d3ee',
  workArea: '#8b5cf6',
} as const

// ─── 稳定入座 ───
// PM 优先坐独立办公室；其余按「有活跃任务优先」+「id 哈希稳定」分配。
// 超出工位数的 agent 不返回映射（由调用方折进待命区）。

export interface SeatCandidate {
  id: string
  role: string
  /** 有活跃任务，优先上座 */
  active: boolean
}

function hashId(id: string): number {
  let hash = 0
  for (let i = 0; i < id.length; i++) {
    hash = (hash * 31 + id.charCodeAt(i)) >>> 0
  }
  return hash
}

export function assignDesks(agents: SeatCandidate[]): Map<string, DeskSlot> {
  const assignment = new Map<string, DeskSlot>()
  const pmAgents = agents.filter((a) => a.role === 'pm')
  const others = agents.filter((a) => a.role !== 'pm')

  // 1. 第一个 PM 坐独立办公室，其余 PM 混入主工位区
  pmAgents.forEach((agent, i) => {
    if (i === 0) assignment.set(agent.id, PM_DESK)
  })
  const leftoverPms = pmAgents.slice(1)

  // 2. 其余按 active 优先，同档内按 id 哈希稳定排序
  const queue = [...leftoverPms, ...others].sort((a, b) => {
    if (a.active !== b.active) return a.active ? -1 : 1
    return hashId(a.id) - hashId(b.id)
  })

  const taken = new Set([...assignment.values()].map((s) => s.id))
  const free = WORKER_DESKS.filter((s) => !taken.has(s.id))
  for (let i = 0; i < queue.length && i < free.length; i++) {
    assignment.set(queue[i]!.id, free[i]!)
  }

  return assignment
}

// ─── 角色外观 ───
// V2 的 AgentRole = 'pm' | 'developer' | 'reviewer' | 'custom'。
// 每种角色一张基础精灵图，同角色的多个 agent 用 tint 色相偏移区分。

export type OfficeRoleKind = AgentRole

/** 角色主色（与平台主色 #6d5ff5 呼应的紫留给 PM） */
export const ROLE_TINT: Record<OfficeRoleKind, string> = {
  pm: '#6d5ff5',
  developer: '#22d3ee',
  reviewer: '#f59e0b',
  custom: '#10b981',
}

/** 多巴胺调色板：每个 Agent 一个专属色（12 色循环，深色底上够亮够区分） */
export const AGENT_PALETTE = [
  '#6d5ff5', // 规范紫
  '#4ecdc4', // 青
  '#f59e0b', // 琥珀
  '#f43f5e', // 玫红
  '#10b981', // 翠绿
  '#3b82f6', // 蓝
  '#ec4899', // 粉
  '#a78bfa', // 浅紫
  '#f97316', // 橙
  '#14b8a6', // 蓝绿
  '#84cc16', // 黄绿
  '#eab308', // 黄
] as const

/** 给一组 agent 稳定分配专属颜色：按 id 排序后循环取色板（≤12 人互不撞色） */
export function assignAgentColors(ids: string[]): Record<string, string> {
  const map: Record<string, string> = {}
  ;[...ids].sort().forEach((id, i) => {
    map[id] = AGENT_PALETTE[i % AGENT_PALETTE.length]
  })
  return map
}

/** 同角色内按 id 哈希做色相偏移，保证个体可区分又不破坏风格统一 */
export function roleTintFor(role: string, agentId: string): string {
  const base = ROLE_TINT[role as OfficeRoleKind] ?? ROLE_TINT.custom
  if (role === 'pm') return base // PM 保持规范紫，不做偏移
  const offset = (hashId(agentId) % 41) - 20 // ±20°
  return shiftHue(base, offset)
}

function shiftHue(hex: string, degrees: number): string {
  const r = parseInt(hex.slice(1, 3), 16) / 255
  const g = parseInt(hex.slice(3, 5), 16) / 255
  const b = parseInt(hex.slice(5, 7), 16) / 255
  const max = Math.max(r, g, b)
  const min = Math.min(r, g, b)
  const l = (max + min) / 2
  const d = max - min
  if (d === 0) return hex
  const s = l > 0.5 ? d / (2 - max - min) : d / (max + min)
  let h = 0
  if (max === r) h = ((g - b) / d + (g < b ? 6 : 0)) / 6
  else if (max === g) h = ((b - r) / d + 2) / 6
  else h = ((r - g) / d + 4) / 6
  h = (h + degrees / 360 + 1) % 1
  return hslToHex(h, s, l)
}

function hslToHex(h: number, s: number, l: number): string {
  const hue2rgb = (p: number, q: number, t: number) => {
    let tt = t
    if (tt < 0) tt += 1
    if (tt > 1) tt -= 1
    if (tt < 1 / 6) return p + (q - p) * 6 * tt
    if (tt < 1 / 2) return q
    if (tt < 2 / 3) return p + (q - p) * (2 / 3 - tt) * 6
    return p
  }
  const q = l < 0.5 ? l * (1 + s) : l + s - l * s
  const p = 2 * l - q
  const to = (v: number) =>
    Math.round(v * 255)
      .toString(16)
      .padStart(2, '0')
  return `#${to(hue2rgb(p, q, h + 1 / 3))}${to(hue2rgb(p, q, h))}${to(hue2rgb(p, q, h - 1 / 3))}`
}

// ─── 尺寸常量（家具用） ───

export const FURNITURE = {
  deskWidth: DESK_WIDTH,
  deskDepth: DESK_DEPTH,
  deskHeight: 0.75,
  monitorWidth: 0.85,
  monitorHeight: 0.55,
  chairRadius: 0.28,
  chairHeight: 0.45,
} as const

// ─── 配色：深色空间 + 多巴胺点缀 ───
// 对齐 V2 的 Dopamine Design System（青 #22d3ee / 琥珀 #f59e0b / 绿 #10b981 /
// 紫 #6d5ff5 / 玫红 #f43f5e）。地面与家具保持低饱和深色调做「底」，
// 高饱和多巴胺色只用在发光件与状态件上，避免整体过花。

/** 办公室主题：夜晚（暗系，当前默认）/ 白天（亮系，模拟开灯后的日光） */
export type OfficeTheme = 'night' | 'day'

export interface OfficePalette {
  /** 沙盘之外的背景空间 */
  void: string
  /** 沙盘地面 */
  floor: string
  /** 地面网格线 */
  grid: string
  /** 网格/描边不透明度（白天浅色底上要更淡） */
  gridOpacity: number
  /** 沙盘外缘发光边框 */
  rim: string
  rugPm: string
  rugMeeting: string
  rugWork: string
  /** 桌面 */
  deskTop: string
  /** 桌沿 */
  deskEdge: string
  /** 显示器机身 */
  monitor: string
  /** 屏幕发光（青，会按 Agent 状态覆盖） */
  monitorGlow: string
  /** 座椅 */
  chair: string
  /** 外围墙 */
  wall: string
  wallTop: string
  windowA: string
  windowB: string
  /** 置物柜 */
  counter: string
  counterTop: string
  counterLine: string
  /** 绿植 */
  pot: string
  potRim: string
  leaf1: string
  leaf2: string
  leaf3: string
  leafEmissive: string
  /** 叶簇自发光强度（夜晚高、白天低） */
  leafGlow: number
  /** 灯光：环境光强度/颜色、主光强度、冷色补光强度 */
  ambientIntensity: number
  ambientColor: string
  keyLightIntensity: number
  fillLightIntensity: number
}

export const OFFICE_THEMES: Record<OfficeTheme, OfficePalette> = {
  night: {
    void: '#0a0a12',
    floor: '#1a1d2e',
    grid: '#6d5ff5',
    gridOpacity: 0.16,
    rim: '#6d5ff5',
    rugPm: '#2b2450',
    rugMeeting: '#123a42',
    rugWork: '#20233a',
    deskTop: '#2f3450',
    deskEdge: '#3d4468',
    monitor: '#11131c',
    monitorGlow: '#22d3ee',
    chair: '#2a2f4a',
    wall: '#1d2336',
    wallTop: '#6d5ff5',
    windowA: '#6d5ff5',
    windowB: '#22d3ee',
    counter: '#232941',
    counterTop: '#2e3552',
    counterLine: '#454b66',
    pot: '#3a3f55',
    potRim: '#454b66',
    leaf1: '#2f9e6e',
    leaf2: '#37b37f',
    leaf3: '#268a5e',
    leafEmissive: '#1c5c40',
    leafGlow: 0.4,
    ambientIntensity: 0.6,
    ambientColor: '#6b7280',
    keyLightIntensity: 1.15,
    fillLightIntensity: 0.4,
  },
  day: {
    void: '#e2e5ee',
    floor: '#f1efe9',
    grid: '#c3c7d6',
    gridOpacity: 0.5,
    rim: '#6d5ff5',
    rugPm: '#e7e2f9',
    rugMeeting: '#def1ee',
    rugWork: '#edeaf6',
    deskTop: '#ffffff',
    deskEdge: '#d6d2c6',
    monitor: '#3a3f4c',
    monitorGlow: '#22d3ee',
    chair: '#e9e7e1',
    wall: '#f6f4ef',
    wallTop: '#6d5ff5',
    windowA: '#8a7bff',
    windowB: '#22b8cf',
    counter: '#e4ddcf',
    counterTop: '#f2efe8',
    counterLine: '#c0b8a7',
    pot: '#d9d3c7',
    potRim: '#c6c0b2',
    leaf1: '#3f9e68',
    leaf2: '#4cb378',
    leaf3: '#358a58',
    leafEmissive: '#2e7d52',
    leafGlow: 0.15,
    ambientIntensity: 1.25,
    ambientColor: '#ffffff',
    keyLightIntensity: 1.5,
    fillLightIntensity: 0.12,
  },
}

/** 状态色（对齐 V2 色板，用于光环 / 屏幕 / 气泡描边） */
export const STATE_COLORS = {
  working: '#22d3ee',
  asking: '#f59e0b',
  thinking: '#8b5cf6',
  meeting: '#f43f5e',
  /** 空闲 = 绿色（V2 主绿），表示「在线待命、可以派活」 */
  idle: '#10b981',
  offline: '#3f4450',
} as const

/** 状态中文标签（显示在员工名牌下方） */
export const STATUS_LABEL: Record<AgentVisualState, string> = {
  working: '执行中',
  asking: '等你答复',
  thinking: '规划中',
  meeting: '会议中',
  idle: '空闲',
}
