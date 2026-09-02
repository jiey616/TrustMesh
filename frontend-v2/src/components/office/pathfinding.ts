import { allSeatPoints, getNavGraph, nearestNodeId, type NavPoint } from './navGraph'

// ─── 寻路 ───
// 移植自 ai-office-react 的 navPathfinding（MIT），坐标由 (x,y) 改为 3D 的 (x,z)。
// 只在「发起一次走动」时计算一次，不是每帧计算。

/**
 * 座椅禁区半径：路径线段距某座位小于此值即视为穿座，该边不可走。
 *
 * 上限受布局约束：横向过道（Z=0 等）到最近座位的距离只有 0.78，
 * 半径再大就会把所有横向过道都判为不可走，导致找不到路径。
 */
const SEAT_BLOCK_RADIUS = 0.62

export type NavPathContext = {
  /** 额外规避的动态障碍（例如正原地工作、不便穿过的其他 Agent） */
  dynamicObstacles?: NavPoint[]
  /** 不计入障碍的座位点（通常是起点与终点本身） */
  exclude?: NavPoint[]
}

function distPointToSegment(
  px: number,
  pz: number,
  ax: number,
  az: number,
  bx: number,
  bz: number,
): number {
  const dx = bx - ax
  const dz = bz - az
  const len2 = dx * dx + dz * dz
  if (len2 < 1e-6) return Math.hypot(px - ax, pz - az)
  let t = ((px - ax) * dx + (pz - az) * dz) / len2
  t = Math.max(0, Math.min(1, t))
  return Math.hypot(px - (ax + t * dx), pz - (az + t * dz))
}

function isExcluded(p: NavPoint, exclude?: NavPoint[]): boolean {
  if (!exclude) return false
  return exclude.some((e) => Math.hypot(e.x - p.x, e.z - p.z) < 0.01)
}

/** 线段是否穿过任一座椅禁区 */
export function segmentCrossesSeat(
  a: NavPoint,
  b: NavPoint,
  ctx?: NavPathContext,
): boolean {
  for (const seat of allSeatPoints()) {
    if (isExcluded(seat, ctx?.exclude)) continue
    if (distPointToSegment(seat.x, seat.z, a.x, a.z, b.x, b.z) < SEAT_BLOCK_RADIUS) {
      return true
    }
  }
  for (const o of ctx?.dynamicObstacles ?? []) {
    if (isExcluded(o, ctx?.exclude)) continue
    if (distPointToSegment(o.x, o.z, a.x, a.z, b.x, b.z) < SEAT_BLOCK_RADIUS) {
      return true
    }
  }
  return false
}

/** Dijkstra 最短路（权重为欧氏距离），跳过穿座椅的边 */
export function shortestPath(
  fromId: string,
  toId: string,
  ctx?: NavPathContext,
): string[] | null {
  if (fromId === toId) return [fromId]

  const { nodes, adj } = getNavGraph()
  const dist = new Map<string, number>()
  const prev = new Map<string, string>()
  const visited = new Set<string>()

  for (const id of nodes.keys()) dist.set(id, Infinity)
  dist.set(fromId, 0)

  while (visited.size < nodes.size) {
    let u: string | null = null
    let best = Infinity
    for (const [id, d] of dist) {
      if (!visited.has(id) && d < best) {
        best = d
        u = id
      }
    }
    if (u == null || best === Infinity) break
    if (u === toId) break
    visited.add(u)

    const nu = nodes.get(u)!
    for (const vId of adj.get(u) ?? []) {
      if (visited.has(vId)) continue
      const nv = nodes.get(vId)!
      // 关键：排除边自身的两个端点所落在的座椅点。
      // 否则 seat→aisle 这条边「起点就是座位」，会被 segmentCrossesSeat 误判为
      // 穿过自己的座位而整条边作废，导致所有座位节点被孤立、Dijkstra 找不到路、
      // planWalk 退化为两点直线（小人直接穿桌走过去）。
      const edgeExclude: NavPoint[] = [
        { x: nu.x, z: nu.z },
        { x: nv.x, z: nv.z },
        ...(ctx?.exclude ?? []),
      ]
      if (segmentCrossesSeat(nu, nv, { ...ctx, exclude: edgeExclude })) continue

      const alt = best + Math.hypot(nv.x - nu.x, nv.z - nu.z)
      if (alt < (dist.get(vId) ?? Infinity)) {
        dist.set(vId, alt)
        prev.set(vId, u)
      }
    }
  }

  if ((dist.get(toId) ?? Infinity) === Infinity) return null

  const ids: string[] = []
  let p: string | undefined = toId
  while (p) {
    ids.unshift(p)
    p = prev.get(p)
  }
  return ids.length > 0 ? ids : null
}

function dedupe(points: NavPoint[]): NavPoint[] {
  const out: NavPoint[] = []
  for (const p of points) {
    const prev = out[out.length - 1]
    if (prev && Math.hypot(prev.x - p.x, prev.z - p.z) < 0.05) continue
    out.push(p)
  }
  return out
}

/**
 * 规划从 from 到 to 的折线路径。
 * 找不到路时退化为两点直线（宁可偶尔穿模，也不要卡住不动）。
 */
export function planWalk(
  from: NavPoint,
  to: NavPoint,
  ctx?: NavPathContext,
): NavPoint[] {
  const { nodes } = getNavGraph()
  const fromId = nearestNodeId(from)
  const toId = nearestNodeId(to)
  const ids = shortestPath(fromId, toId, ctx)

  const points: NavPoint[] = [from]
  if (ids) {
    for (const id of ids) {
      const n = nodes.get(id)
      if (n) points.push({ x: n.x, z: n.z })
    }
  }
  points.push(to)

  return dedupe(points)
}
