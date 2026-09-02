import { DESK_SLOTS, MEETING_TABLE, PM_DESK, WORKER_DESKS } from './officeLayout'
import type { DeskSlot } from './officeLayout'

// ─── 办公区导航图 ───
// 3D 场景用 XZ 平面做地面，故 NavPoint 是 { x, z }（不是 2D 版的 { x, y }）。
//
// 布局是规整的 3 列 × 4 行，所以导航图直接用规则网格生成，
// 不需要参考实现里那套 centroid 聚类（它要适配任意摆放的工位）。
//
// 过道刻意与工位错位：过道 X 取相邻两列的中点，过道 Z 取相邻两行的中点，
// 保证纵向/横向穿行时都不会压到桌子或椅子上。

export type NavPoint = { x: number; z: number }
export type NavNode = { id: string; x: number; z: number }
export type Adjacency = Map<string, string[]>

export type NavGraph = {
  nodes: Map<string, NavNode>
  adj: Adjacency
}

/** 纵向过道 X 坐标：覆盖主工位 4 列(x -4.2/-1.4/1.4/4.2)的两侧及两两中点，
 *  并向背面翼（PM 在 x-5.2、会议在 x4.2）延伸 */
const AISLE_X = [-6, -2.8, 0, 2.8, 6]
/** 横向过道 Z 坐标：主工位 3 行(z 1.2/3.9/6.6)的前后及间隔中点，
 *  加一条在背面翼之后(z-5.8)用于连接 PM/会议 */
const AISLE_Z = [-5.8, -0.5, 2.55, 5.25, 8.3]

/** 访客站在被访者侧面的横向偏移 */
const SIDE_OFFSET = 0.95

function dist(a: NavPoint, b: NavPoint): number {
  return Math.hypot(b.x - a.x, b.z - a.z)
}

/** 工位的左/右站位点（访客站这里，不占用本人座椅） */
export function sidePointOf(slot: DeskSlot, side: 'left' | 'right'): NavPoint {
  return {
    x: side === 'left' ? slot.seat.x - SIDE_OFFSET : slot.seat.x + SIDE_OFFSET,
    z: slot.seat.z,
  }
}

/** 访客应该站哪一侧：选离自己更近的一侧 */
export function pickVisitSide(
  hostSlot: DeskSlot,
  from: NavPoint,
): 'left' | 'right' {
  const left = sidePointOf(hostSlot, 'left')
  const right = sidePointOf(hostSlot, 'right')
  return dist(left, from) <= dist(right, from) ? 'left' : 'right'
}

export function visitStandPoint(hostSlot: DeskSlot, from: NavPoint): NavPoint {
  return sidePointOf(hostSlot, pickVisitSide(hostSlot, from))
}

function addEdge(adj: Adjacency, a: string, b: string) {
  if (a === b) return
  const list = adj.get(a)
  if (!list) {
    adj.set(a, [b])
    return
  }
  if (!list.includes(b)) list.push(b)
}

function link(adj: Adjacency, a: string, b: string) {
  addEdge(adj, a, b)
  addEdge(adj, b, a)
}

function aisleId(ix: number, iz: number): string {
  return `aisle-${ix}-${iz}`
}

let cached: NavGraph | null = null

/** 构建（并缓存）导航图。图只依赖静态布局，构建一次即可。 */
export function getNavGraph(): NavGraph {
  if (cached) return cached

  const nodes = new Map<string, NavNode>()
  const adj: Adjacency = new Map()

  // 1. 过道网格：横纵双向连通
  for (let ix = 0; ix < AISLE_X.length; ix++) {
    for (let iz = 0; iz < AISLE_Z.length; iz++) {
      const id = aisleId(ix, iz)
      nodes.set(id, { id, x: AISLE_X[ix]!, z: AISLE_Z[iz]! })
      adj.set(id, [])
    }
  }
  for (let ix = 0; ix < AISLE_X.length; ix++) {
    for (let iz = 0; iz < AISLE_Z.length; iz++) {
      if (ix + 1 < AISLE_X.length) {
        link(adj, aisleId(ix, iz), aisleId(ix + 1, iz))
      }
      if (iz + 1 < AISLE_Z.length) {
        link(adj, aisleId(ix, iz), aisleId(ix, iz + 1))
      }
    }
  }

  // 2. 每个工位的座位点入图，并接到最近的两个过道节点
  //    （接两个是为了让 Dijkstra 能挑更顺的一侧绕行）
  for (const slot of DESK_SLOTS) {
    const seatId = `seat-${slot.id}`
    nodes.set(seatId, { id: seatId, x: slot.seat.x, z: slot.seat.z })
    adj.set(seatId, [])

    const ranked = [...nodes.values()]
      .filter((n) => n.id.startsWith('aisle-'))
      .map((n) => ({ id: n.id, d: dist(n, slot.seat) }))
      .sort((a, b) => a.d - b.d)
      .slice(0, 2)
    for (const r of ranked) link(adj, seatId, r.id)
  }

  // 3. 会议区：一个 hub 接到最近过道，6 个落座点接到 hub
  const hubId = 'meeting-hub'
  const hubX = MEETING_TABLE.center.x
  const hubZ = MEETING_TABLE.center.z - MEETING_TABLE.radiusZ - 0.5
  nodes.set(hubId, { id: hubId, x: hubX, z: hubZ })
  adj.set(hubId, [])

  const hubRanked = [...nodes.values()]
    .filter((n) => n.id.startsWith('aisle-'))
    .map((n) => ({ id: n.id, d: dist(n, { x: hubX, z: hubZ }) }))
    .sort((a, b) => a.d - b.d)
    .slice(0, 2)
  for (const r of hubRanked) link(adj, hubId, r.id)

  MEETING_TABLE.seats.forEach((seat, i) => {
    const id = `meeting-seat-${i}`
    nodes.set(id, { id, x: seat.x, z: seat.z })
    adj.set(id, [])
    link(adj, id, hubId)
  })

  cached = { nodes, adj }
  return cached
}

/** 距离给定点最近的图节点 id */
export function nearestNodeId(point: NavPoint): string {
  let bestId = ''
  let bestD = Infinity
  for (const n of getNavGraph().nodes.values()) {
    const d = (n.x - point.x) ** 2 + (n.z - point.z) ** 2
    if (d < bestD) {
      bestD = d
      bestId = n.id
    }
  }
  return bestId
}

/** 工位对应的座位图节点 id（用于「从自己工位出发」与「回到自己工位」） */
export function seatNodeIdOf(slotId: string): string {
  return `seat-${slotId}`
}

/** 会议落座点的图节点 id */
export function meetingSeatNodeId(index: number): string {
  return `meeting-seat-${index}`
}

/** 所有需要规避的座椅点（访客绕行用） */
export function allSeatPoints(): NavPoint[] {
  return DESK_SLOTS.map((s) => ({ x: s.seat.x, z: s.seat.z }))
}

export { PM_DESK, WORKER_DESKS }
