import { useEffect, useMemo, useRef } from 'react'
import { useFrame } from '@react-three/fiber'
import type { Group, Mesh, MeshBasicMaterial } from 'three'
import type { AgentVisual } from '@/types/office'
import { AGENT_HEIGHT, STATE_COLORS, STATUS_LABEL } from './officeLayout'
import type { DeskSlot } from './officeLayout'
import { makeBubbleTexture, makeLabelTexture, textureAspect, textureSize } from './sprites'
import { useOfficeStore } from '@/stores/officeStore'
import { AgentFigure } from './AgentFigure'
import {
  WALK_SPEED,
  advanceAlongPath,
  buildReturnPath,
  buildVisitPath,
  type MissionPhase,
} from './mission'
import type { NavPoint } from './navGraph'

// ─── 单个 Agent 的 3D 呈现 ───
// 立体小人（AgentFigure）+ 脚下状态光环 + 名牌 + 状态胶囊 + 气泡 + 贴纸，
// 以及在办公室里真的走起来（goto → talk → return）。
//
// 层次约定：
//   - 身体（figureRef）：走动时朝移动方向，静止时缓缓转向相机 —— 立体模型任何视角都可见
//   - 文字层（labelRef）：名牌/状态/气泡/贴纸永远 Y 轴面向相机（可读性优先）
//
// 性能约定：位置、朝向、微动作、脉动、走动推进全部在 useFrame 里
// 直接操作 Object3D，不 setState —— 动画期间 React 完全不 re-render。
// 唯一会引发 re-render 的是 mission 的阶段切换（一次任务只有 3~4 次）。

/** 角度插值（处理 ±π 环绕，避免转身时绕远路） */
function lerpAngle(a: number, b: number, k: number): number {
  let diff = b - a
  while (diff > Math.PI) diff -= Math.PI * 2
  while (diff < -Math.PI) diff += Math.PI * 2
  return a + diff * k
}

// ─── 气泡尺寸 ───
// 关键 1：用「贴图像素 → 世界单位」的固定比例，而不是固定高度。
// 固定高度会让长消息（多行）每行被压扁，字高掉到几个像素完全读不了。
// 固定比例则保证任何长度的消息在场景里字号一致，行数只影响气泡变得更高。
// 66 → 88：从「30px 过大」回调到中间档，屏幕文字高度约 20px（最初 15px 太小、30px 太大）。
const BUBBLE_PX_PER_UNIT = 88
/**
 * 关键 2：距离 + 视口双重补偿，让气泡在屏幕上大小恒定。
 * 屏幕像素/世界单位 = H_px / (2 · d · tan(fov/2))，
 * 所以同一世界尺寸在大屏/近距离下显得大、小屏/远距离下显得小。
 * 这里把两者一起归一化到「全景距离 15 + 视口高 700」这个设计基准，
 * 无论用户窗口多大、镜头拉多远，气泡文字高度都稳定在约 30px（清晰可读）。
 */
const BUBBLE_BASE_DIST = 15
const BUBBLE_BASE_VIEWPORT_H = 700
// 下限抬高到 0.72：大窗口 / 拉近时不再被压成细条（之前 0.5 是“太小”的主因之一）
const BUBBLE_SCALE_MIN = 0.72
const BUBBLE_SCALE_MAX = 2.0
/** 缩放后的世界尺寸硬上限：极端小窗下也不让气泡横跨整个办公室 */
const BUBBLE_MAX_W = 8.0
const BUBBLE_MAX_H = 4.2
/** 气泡底边固定的高度（在状态胶囊之上），气泡向上生长 */
const BUBBLE_BOTTOM_Y = AGENT_HEIGHT + 1.1
/** 名牌/状态胶囊用同一套补偿但幅度更保守，只补偿远近差异、不喧宾夺主 */
const BADGE_SCALE_MIN = 0.55
const BADGE_SCALE_MAX = 1.25

/** 由 id 派生稳定相位，避免所有 Agent 动作同步 */
function phaseOf(id: string): number {
  let h = 0
  for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) >>> 0
  return ((h % 1000) / 1000) * Math.PI * 2
}

export interface AgentSpriteProps {
  visual: AgentVisual
  slot: DeskSlot
  /** 该 Agent 的专属颜色（按 id 稳定分配，每个小人一色） */
  color: string
  onClick?: (agentId: string) => void
}

export function AgentSprite({ visual, slot, color, onClick }: AgentSpriteProps) {
  const rootRef = useRef<Group>(null)
  const figureRef = useRef<Group>(null)
  const labelRef = useRef<Group>(null)
  const ringRef = useRef<Mesh>(null)
  const ringMatRef = useRef<MeshBasicMaterial>(null)
  const bubbleRef = useRef<Group>(null)
  const stickerRef = useRef<Group>(null)
  const nameRef = useRef<Group>(null)
  const statusRef = useRef<Group>(null)

  const isFocused = useOfficeStore((s) => s.focusAgentId === visual.id)
  // 订阅 mission 只为拿到 talk 话术做气泡；位置推进不走 React
  const mission = useOfficeStore((s) => s.missions[visual.id])
  const phase = useMemo(() => phaseOf(visual.id), [visual.id])

  // ─── 走动状态（全部在 ref 里） ───
  const posRef = useRef<NavPoint>({ x: slot.seat.x, z: slot.seat.z })
  const pathRef = useRef<NavPoint[]>([])
  const pathIndexRef = useRef(0)
  const talkLeftRef = useRef(0)
  const phaseRef = useRef<MissionPhase | null>(null)
  /** 走动时朝向移动方向 */
  const headingRef = useRef<number | null>(null)
  /** 面谈时朝向交谈对象（进入 talk 阶段时锁定） */
  const talkFacingRef = useRef<number | null>(null)

  // 座位变动（花名册变化/重排）且当前没在走动时，把人放回座位
  useEffect(() => {
    if (!useOfficeStore.getState().missions[visual.id]) {
      posRef.current = { x: slot.seat.x, z: slot.seat.z }
    }
  }, [slot.seat.x, slot.seat.z, visual.id])

  const nameTex = useMemo(
    () =>
      makeLabelTexture(visual.name, {
        fontSize: 38,
        color: visual.presence === 'offline' ? 'var(--text-tertiary)' : 'var(--text-primary)',
        background: 'rgba(10,10,18,0.74)',
        borderColor: 'var(--line-strong)',
        bold: true,
      }),
    [visual.name, visual.presence],
  )
  const nameAspect = textureAspect(nameTex)

  const stateColor = STATE_COLORS[visual.state] ?? STATE_COLORS.idle
  const dimmed = visual.presence === 'offline'

  /** 状态胶囊：实心状态色 + 深色字 */
  const statusTex = useMemo(
    () =>
      makeLabelTexture(
        mission
          ? mission.phase === 'goto'
            ? '走过去'
            : mission.phase === 'talk'
              ? '沟通中'
              : '回工位'
          : STATUS_LABEL[visual.state],
        {
          fontSize: 30,
          color: 'var(--text-inverse)',
          background: dimmed ? 'var(--surface-raised)' : stateColor,
          borderColor: dimmed ? 'var(--line-strong)' : stateColor,
          paddingX: 18,
          paddingY: 8,
          bold: true,
        },
      ),
    [visual.state, stateColor, dimmed, mission?.phase],
  )
  const statusAspect = textureAspect(statusTex)

  // talk 阶段优先展示话术，否则展示事件产生的气泡
  const bubbleText =
    mission?.phase === 'talk' ? mission.message : visual.bubble

  const bubbleTex = useMemo(
    () =>
      bubbleText
        ? makeBubbleTexture(bubbleText, {
            color: visual.bubbleKind === 'question' ? 'var(--text-inverse)' : 'var(--text-primary)',
            background:
              visual.bubbleKind === 'question' ? '#f59e0b' : 'rgba(20,20,34,0.92)',
            borderColor: visual.bubbleKind === 'question' ? 'var(--warning)' : stateColor,
          })
        : null,
    [bubbleText, visual.bubbleKind, stateColor],
  )
  // 实际世界尺寸在 useFrame 里按「距离 + 视口」逐帧算，这里只给首帧一个近似值
  const bubblePx = bubbleTex ? textureSize(bubbleTex) : null
  const bubbleW0 = bubblePx
    ? Math.min(BUBBLE_MAX_W, bubblePx.width / BUBBLE_PX_PER_UNIT)
    : 1
  const bubbleH0 = bubblePx
    ? Math.min(BUBBLE_MAX_H, bubblePx.height / BUBBLE_PX_PER_UNIT)
    : 0.5

  const stickerTex = useMemo(
    () =>
      visual.sticker
        ? makeLabelTexture(visual.sticker === 'failed' ? '！' : '✓', {
            fontSize: 64,
            color: visual.sticker === 'failed' ? 'var(--error)' : 'var(--success)',
            background: visual.sticker === 'failed' ? '#7f1d1d' : '#065f46',
            borderColor: visual.sticker === 'failed' ? 'var(--error)' : 'var(--success)',
          })
        : null,
    [visual.sticker],
  )

  useFrame(({ camera, clock, size }, delta) => {
    const t = clock.elapsedTime
    const now = Date.now()
    const dt = Math.min(delta, 0.1) // 切回前台时别让一帧跳太远
    const store = useOfficeStore.getState()

    // ─── 走动状态机 ───
    if (mission) {
      // 阶段切换：重建路径 / 重置说话计时
      if (mission.phase !== phaseRef.current) {
        phaseRef.current = mission.phase
        if (mission.phase === 'goto' && mission.targetAgentId) {
          const hostSlot = store.seating[mission.targetAgentId]
          pathRef.current = hostSlot ? buildVisitPath(posRef.current, hostSlot) : []
          pathIndexRef.current = 0
        } else if (mission.phase === 'talk') {
          talkLeftRef.current = mission.talkDuration
          pathRef.current = []
          // 到达后转身面见面谈对象
          const hostSlot = mission.targetAgentId
            ? store.seating[mission.targetAgentId]
            : undefined
          talkFacingRef.current = hostSlot
            ? Math.atan2(
                hostSlot.seat.x - posRef.current.x,
                hostSlot.seat.z - posRef.current.z,
              )
            : null
        } else if (mission.phase === 'return') {
          pathRef.current = buildReturnPath(posRef.current, slot)
          pathIndexRef.current = 0
        }
      }

      const walking = mission.phase === 'goto' || mission.phase === 'return'

      if (walking) {
        const beforeX = posRef.current.x
        const beforeZ = posRef.current.z
        const r = advanceAlongPath(
          posRef.current,
          pathRef.current,
          pathIndexRef.current,
          WALK_SPEED,
          dt,
        )
        posRef.current = r.pos
        pathIndexRef.current = r.index

        const dx = r.pos.x - beforeX
        const dz = r.pos.z - beforeZ
        if (Math.hypot(dx, dz) > 1e-4) headingRef.current = Math.atan2(dx, dz)

        if (r.done) {
          // 到达：要么开口说话，要么任务结束回座
          if (mission.phase === 'goto') {
            store.updateMission(visual.id, { phase: 'talk' })
          } else {
            store.endMission(visual.id)
          }
        }
      } else if (mission.phase === 'talk') {
        talkLeftRef.current -= dt
        if (talkLeftRef.current <= 0) {
          store.updateMission(visual.id, { phase: 'return' })
        }
      }
    } else if (phaseRef.current !== null) {
      // 任务结束，收起走动状态
      phaseRef.current = null
      pathRef.current = []
      headingRef.current = null
      talkFacingRef.current = null
    }

    // ─── 位置 ───
    const root = rootRef.current
    if (root) {
      root.position.x = posRef.current.x
      root.position.z = posRef.current.z
    }

    // ─── 朝向：走动朝移动方向 → 面谈朝对方 → 静止在工位面朝桌上显示器 ───
    if (figureRef.current) {
      const target =
        headingRef.current ?? talkFacingRef.current ?? slot.facing
      figureRef.current.rotation.y = lerpAngle(
        figureRef.current.rotation.y,
        target,
        1 - Math.exp(-8 * dt),
      )
      // 走动时整体轻微上下起伏，强化步伐感
      const walkingNow = Boolean(mission) && mission.phase !== 'talk'
      figureRef.current.position.y = walkingNow
        ? Math.abs(Math.sin(t * 10 + phase)) * 0.045
        : 0
    }
    if (labelRef.current) {
      // 文字层完全跟随相机四元数：无论怎么旋转/俯视，文字永远正对屏幕
      labelRef.current.quaternion.copy(camera.quaternion)
    }

    // ─── 状态光环：忙碌时脉动，聚焦时更亮更大 ───
    const busy =
      !dimmed &&
      (visual.state === 'working' ||
        visual.state === 'asking' ||
        visual.state === 'thinking')
    if (ringMatRef.current) {
      const base = dimmed ? 0.18 : isFocused ? 0.95 : visual.state === 'idle' ? 0.38 : 0.72
      const pulse = busy ? Math.sin(t * 3.2 + phase) * 0.2 : 0
      ringMatRef.current.opacity = base + pulse
    }
    if (ringRef.current) {
      const s = busy ? 1 + Math.sin(t * 3.2 + phase) * 0.09 : 1
      const focusScale = isFocused ? 1.18 : 1
      ringRef.current.scale.set(s * focusScale, s * focusScale, 1)
    }

    // ─── 文字层距离 + 视口补偿：屏幕上保持恒定可读大小 ───
    const dist = rootRef.current
      ? camera.position.distanceTo(rootRef.current.position)
      : BUBBLE_BASE_DIST
    const rawK =
      (dist / BUBBLE_BASE_DIST) *
      (BUBBLE_BASE_VIEWPORT_H / Math.max(size.height, 120))
    const k = Math.min(BUBBLE_SCALE_MAX, Math.max(BUBBLE_SCALE_MIN, rawK))
    // 名牌/状态胶囊用更保守的幅度：只抹平远近差异，不喧宾夺主
    const kBadge = Math.min(BADGE_SCALE_MAX, Math.max(BADGE_SCALE_MIN, k))
    if (nameRef.current) nameRef.current.scale.setScalar(kBadge)
    if (statusRef.current) statusRef.current.scale.setScalar(kBadge)

    // ─── 气泡 / 贴纸过期：直接改 visible ───
    if (bubbleRef.current && bubblePx && bubblePx.width > 0) {
      // 按「像素 × 补偿系数」换算世界尺寸，再受硬上限约束等比回缩
      let w = (bubblePx.width * k) / BUBBLE_PX_PER_UNIT
      let h = (bubblePx.height * k) / BUBBLE_PX_PER_UNIT
      if (w > BUBBLE_MAX_W || h > BUBBLE_MAX_H) {
        const f = Math.min(BUBBLE_MAX_W / w, BUBBLE_MAX_H / h)
        w *= f
        h *= f
      }
      bubbleRef.current.scale.set(w, h, 1)
      // 底边固定、向上生长：气泡变高时不会盖住状态胶囊
      bubbleRef.current.position.y = BUBBLE_BOTTOM_Y + h / 2
      // talk 阶段由 mission 驱动，超时逻辑不适用
      bubbleRef.current.visible =
        mission?.phase === 'talk'
          ? true
          : Boolean(visual.bubble) && (visual.bubbleUntil === 0 || now < visual.bubbleUntil)
    }
    if (stickerRef.current) {
      stickerRef.current.visible = Boolean(visual.sticker) && now < visual.stickerUntil
    }
  })

  return (
    <group
      ref={rootRef}
      position={[slot.seat.x, 0, slot.seat.z]}
      onClick={(e) => {
        e.stopPropagation()
        onClick?.(visual.id)
      }}
    >
      {/* 脚下状态光环 */}
      <mesh ref={ringRef} rotation={[-Math.PI / 2, 0, 0]} position={[0, 0.03, 0]}>
        <ringGeometry args={[0.5, 0.66, 28]} />
        <meshBasicMaterial
          ref={ringMatRef}
          color={dimmed ? STATE_COLORS.offline : stateColor}
          transparent
          opacity={dimmed ? 0.18 : visual.state === 'idle' ? 0.38 : 0.72}
          depthWrite={false}
        />
      </mesh>

      {/* 聚焦时的额外外环 */}
      {isFocused && (
        <mesh rotation={[-Math.PI / 2, 0, 0]} position={[0, 0.025, 0]}>
          <ringGeometry args={[0.78, 0.92, 32]} />
          <meshBasicMaterial
            color={stateColor}
            transparent
            opacity={0.35}
            depthWrite={false}
          />
        </mesh>
      )}

      {/* 立体小人：初始面朝工位显示器；走动朝移动方向、面谈朝对方 */}
      <group ref={figureRef} rotation={[0, slot.facing, 0]}>
        <AgentFigure
          role={visual.role}
          color={color}
          dimmed={dimmed}
          walking={Boolean(mission) && mission.phase !== 'talk'}
          typing={visual.state === 'working'}
          phase={phase}
        />
      </group>

      {/* 文字层：名牌/状态/气泡/贴纸永远面向相机（可读性优先） */}
      <group ref={labelRef}>
        {/* 名牌（比状态胶囊略小，避免抢占视觉） */}
        <group ref={nameRef} position={[0, AGENT_HEIGHT + 0.4, 0]}>
          <mesh>
            <planeGeometry args={[0.36 * nameAspect, 0.36]} />
            <meshBasicMaterial map={nameTex} transparent depthWrite={false} />
          </mesh>
        </group>

        {/* 状态胶囊 */}
        <group ref={statusRef} position={[0, AGENT_HEIGHT + 0.92, 0]}>
          <mesh>
            <planeGeometry args={[0.32 * statusAspect, 0.32]} />
            <meshBasicMaterial
              map={statusTex}
              transparent
              depthWrite={false}
              opacity={dimmed ? 0.6 : 1}
            />
          </mesh>
        </group>

        {/* 气泡：尺寸由 useFrame 按距离+视口动态算，plane 保持 1×1 */}
        {bubbleTex && (
          <group
            ref={bubbleRef}
            position={[0, BUBBLE_BOTTOM_Y + bubbleH0 / 2, 0]}
            scale={[bubbleW0, bubbleH0, 1]}
          >
            <mesh renderOrder={999}>
              <planeGeometry args={[1, 1]} />
              <meshBasicMaterial
                map={bubbleTex}
                transparent
                depthWrite={false}
                depthTest={false}
              />
            </mesh>
          </group>
        )}

        {/* 结果贴纸 */}
        {stickerTex && (
          <group ref={stickerRef} position={[0.72, AGENT_HEIGHT + 0.18, 0]}>
            <mesh>
              <planeGeometry args={[0.42, 0.42]} />
              <meshBasicMaterial map={stickerTex} transparent depthWrite={false} />
            </mesh>
          </group>
        )}
      </group>
    </group>
  )
}
