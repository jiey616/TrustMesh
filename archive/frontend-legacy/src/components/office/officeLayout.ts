// ─── 场景尺寸（逻辑分辨率，等比缩放居中） ───

export const SCENE_WIDTH = 960
export const SCENE_HEIGHT = 640

// ─── 工位布局 ───
// M0 固定 10 个普通工位（2 排）+ PM 独立办公位（左上角房间）。
// 坐标均为「人物锚点」（头顶中心），家具相对锚点绘制。

export interface DeskSlot {
  id: string
  /** 人物锚点（坐着，头顶中心） */
  seatX: number
  seatY: number
  /** 桌面矩形（用于绘制与深度排序） */
  deskX: number
  deskY: number
  kind: 'worker' | 'pm'
}

const WORKER_DESKS: DeskSlot[] = Array.from({ length: 10 }, (_, i) => {
  const row = Math.floor(i / 5)
  const col = i % 5
  const x = 150 + col * 165
  const y = row === 0 ? 300 : 480
  return {
    id: `desk-${i + 1}`,
    seatX: x,
    seatY: y - 14,
    deskX: x - 62,
    deskY: y,
    kind: 'worker' as const,
  }
})

const PM_DESK: DeskSlot = {
  id: 'desk-pm',
  seatX: 170,
  seatY: 128,
  deskX: 108,
  deskY: 142,
  kind: 'pm',
}

export const DESK_SLOTS: DeskSlot[] = [PM_DESK, ...WORKER_DESKS]

export const MEETING_TABLE = { x: 660, y: 66, w: 240, h: 130 }

// ─── 稳定入座：PM 优先坐 PM 位，其余按 agent.id 哈希稳定分配 ───

export interface SeatCandidate {
  id: string
  role: string
}

export function assignDesks(agents: SeatCandidate[]): Map<string, DeskSlot> {
  const assignment = new Map<string, DeskSlot>()
  const pmAgents = agents.filter((a) => a.role === 'pm')
  const others = agents.filter((a) => a.role !== 'pm')

  const workerSlots = WORKER_DESKS
  let workerCursor = 0
  let pmCursor = 0

  // PM 坐独立办公室，坐满后混入普通工位
  for (const agent of pmAgents) {
    if (pmCursor < 1) {
      assignment.set(agent.id, PM_DESK)
      pmCursor += 1
    } else if (workerCursor < workerSlots.length) {
      assignment.set(agent.id, workerSlots[workerCursor])
      workerCursor += 1
    }
  }

  // 非 PM 按 id 哈希稳定入座（同一 agent 每次进页面座位不变）
  const taken = new Set([...assignment.values()].map((s) => s.id))
  const freeSlots = workerSlots.filter((s) => !taken.has(s.id))
  const order = [...others].sort((a, b) => hashId(a.id) - hashId(b.id))
  for (let i = 0; i < order.length && i < freeSlots.length; i++) {
    assignment.set(order[i].id, freeSlots[i])
  }
  return assignment
}

function hashId(id: string): number {
  let hash = 0
  for (let i = 0; i < id.length; i++) {
    hash = (hash * 31 + id.charCodeAt(i)) >>> 0
  }
  return hash
}

export function seatIndexOf(agentId: string): number {
  return hashId(agentId) % CHARACTER_PALETTES.length
}

// ─── 角色调色板（头发/上衣/肤色） ───

export interface CharacterPalette {
  hair: number
  shirt: number
  skin: number
  accent: number
}

export const CHARACTER_PALETTES: CharacterPalette[] = [
  { hair: 0x3b2f2a, shirt: 0x4f7cc0, skin: 0xf2c9a1, accent: 0x2d5a94 },
  { hair: 0x1f1f24, shirt: 0xc05a4f, skin: 0xe8b48c, accent: 0x8f3a32 },
  { hair: 0x6b4a2f, shirt: 0x5aa06b, skin: 0xf5d3ae, accent: 0x3d7a4c },
  { hair: 0x2a2a2e, shirt: 0x9a6bbf, skin: 0xeab992, accent: 0x74499a },
  { hair: 0x8a6d3b, shirt: 0xd29a3f, skin: 0xf2c9a1, accent: 0xa8781f },
  { hair: 0x444a52, shirt: 0x4fa3a5, skin: 0xe8b48c, accent: 0x33787a },
  { hair: 0x23303c, shirt: 0x5f6f8f, skin: 0xf5d3ae, accent: 0x42526b },
  { hair: 0x5c3a28, shirt: 0xc97a94, skin: 0xeab992, accent: 0x9c546f },
]

// ─── 房间配色 ───

export const COLORS = {
  floor: 0xefe9df,
  floorLine: 0xe2dacd,
  wallTop: 0xdcd4c6,
  wallSide: 0xcfc6b6,
  deskTop: 0xb98d5a,
  deskFront: 0xa17a4a,
  deskSide: 0x8f6a3f,
  monitorFrame: 0x3a3f46,
  monitorScreen: 0x9fd8e8,
  chair: 0x6e7480,
  rug: 0xd8e5dd,
  plant: 0x5f8f5a,
  pot: 0xb0703f,
}
