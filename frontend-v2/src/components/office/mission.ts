import type { DeskSlot } from './officeLayout'
import { visitStandPoint, type NavPoint } from './navGraph'
import { planWalk, type NavPathContext } from './pathfinding'

// ─── 走动任务（mission）───
// 状态机：goto（走过去）→ talk（站着说话）→ return（回自己工位）。
//
// 与参考实现的关键差异：ai-office 用模块级 dispatcher + 全局锁调度，
// 它有并发时序 bug（其 issue #1「POST /actions 无动画」）。
// 这里改为：mission 只描述「目标与阶段」，推进完全由渲染层的 useFrame 完成，
// 没有任何锁，也就不存在 busy→idle 的竞态窗口。

/** 步行速度（单位/秒）。场景 22 单位宽，横穿约 3~4 秒，节奏合适 */
export const WALK_SPEED = 2.4

/** 默认说话时长（秒） */
export const DEFAULT_TALK_SEC = 3.2

export type MissionPhase = 'goto' | 'talk' | 'return'

export interface Mission {
  kind: 'visit' | 'meeting'
  phase: MissionPhase
  /** 拜访对象（visit）；会议任务为 null */
  targetAgentId: string | null
  /** talk 阶段展示的话术 */
  message: string
  talkDuration: number
  /** 会议落座序号（kind='meeting' 时用） */
  meetingSeatIndex: number | null
}

/** 规划：从当前位置走到被访者工位旁的站位点 */
export function buildVisitPath(
  from: NavPoint,
  hostSlot: DeskSlot,
  ctx?: NavPathContext,
): NavPoint[] {
  const stand = visitStandPoint(hostSlot, from)
  return planWalk(from, stand, ctx)
}

/** 规划：走回自己工位 */
export function buildReturnPath(
  from: NavPoint,
  homeSlot: DeskSlot,
  ctx?: NavPathContext,
): NavPoint[] {
  return planWalk(from, homeSlot.seat, ctx)
}

export interface AdvanceResult {
  pos: NavPoint
  index: number
  done: boolean
}

/**
 * 沿折线推进一段距离（按 delta 时间 × 速度）。
 * 纯函数，不修改入参；返回新位置、新的路径下标、是否已走完。
 */
export function advanceAlongPath(
  pos: NavPoint,
  path: NavPoint[],
  index: number,
  speed: number,
  delta: number,
): AdvanceResult {
  let x = pos.x
  let z = pos.z
  let i = index
  let remaining = speed * delta

  while (i < path.length && remaining > 0) {
    const target = path[i]!
    const dx = target.x - x
    const dz = target.z - z
    const d = Math.hypot(dx, dz)
    if (d <= remaining) {
      x = target.x
      z = target.z
      remaining -= d
      i += 1
    } else {
      x += (dx / d) * remaining
      z += (dz / d) * remaining
      remaining = 0
    }
  }

  return { pos: { x, z }, index: i, done: i >= path.length }
}
