import { useMemo } from 'react'
import { AdditiveBlending } from 'three'
import { FURNITURE } from './officeLayout'
import type { DeskSlot } from './officeLayout'
import { makeCodeScreenTexture, textureAspect } from './sprites'
import { useOfficePalette } from '@/stores/officeStore'

// ─── 程序化家具 ───
// 纯几何体拼装（Box / Cylinder / Plane），零外部模型资源、零版权风险。
// 人坐在 desk 的 +Z 侧、面朝 -Z，故显示器屏幕朝 +Z 面向人。

function Desk({ slot }: { slot: DeskSlot }) {
  const p = useOfficePalette()
  const { x, z } = slot.desk
  const w = FURNITURE.deskWidth
  const d = FURNITURE.deskDepth
  const h = FURNITURE.deskHeight

  // 每个工位按 slot.id 选一段代码，屏幕内容各不相同
  const codeTex = useMemo(() => makeCodeScreenTexture(slot.id), [slot.id])
  const screenW = FURNITURE.monitorWidth - 0.08
  const screenH = screenW / textureAspect(codeTex)

  return (
    <group position={[x, 0, z]}>
      {/* 桌面 */}
      <mesh position={[0, h, 0]} castShadow receiveShadow>
        <boxGeometry args={[w, 0.08, d]} />
        <meshStandardMaterial color={p.deskTop} roughness={0.7} metalness={0.1} />
      </mesh>

      {/* 左右侧板 */}
      {[-1, 1].map((side) => (
        <mesh key={side} position={[(side * (w - 0.12)) / 2, h / 2, 0]}>
          <boxGeometry args={[0.1, h, d]} />
          <meshStandardMaterial color={p.deskEdge} roughness={0.8} />
        </mesh>
      ))}

      {/* 显示器：机身 + 朝 +Z 的发光屏幕 */}
      <group position={[0, h + FURNITURE.monitorHeight / 2 + 0.12, -0.15]}>
        <mesh castShadow>
          <boxGeometry args={[FURNITURE.monitorWidth, FURNITURE.monitorHeight, 0.07]} />
          <meshStandardMaterial color={p.monitor} roughness={0.5} />
        </mesh>
        {/* 屏幕：贴代码屏纹理。meshBasicMaterial 本身不受光照影响，
            在深色场景里看起来就像屏幕自发光，无需后处理 */}
        <mesh position={[0, 0, 0.042]}>
          <planeGeometry args={[screenW, screenH]} />
          <meshBasicMaterial map={codeTex} toneMapped={false} />
        </mesh>
        {/* 屏幕辉光：一层加色混合的淡光晕，弱一点不至于糊住代码 */}
        <mesh position={[0, 0, 0.05]}>
          <planeGeometry args={[screenW * 1.08, screenH * 1.12]} />
          <meshBasicMaterial
            color={p.monitorGlow}
            transparent
            opacity={0.12}
            blending={AdditiveBlending}
            depthWrite={false}
          />
        </mesh>
        {/* 显示器支柱 */}
        <mesh position={[0, -FURNITURE.monitorHeight / 2 - 0.06, 0]}>
          <boxGeometry args={[0.16, 0.12, 0.16]} />
          <meshStandardMaterial color={p.monitor} />
        </mesh>
      </group>
    </group>
  )
}

function Chair({ slot }: { slot: DeskSlot }) {
  const p = useOfficePalette()
  const { x, z } = slot.seat
  const r = FURNITURE.chairRadius
  const h = FURNITURE.chairHeight

  return (
    <group position={[x, 0, z]}>
      {/* 座垫 */}
      <mesh position={[0, h, 0]} castShadow>
        <cylinderGeometry args={[r, r * 0.92, 0.1, 12]} />
        <meshStandardMaterial color={p.chair} roughness={0.85} />
      </mesh>
      {/* 中轴 */}
      <mesh position={[0, h / 2, 0]}>
        <cylinderGeometry args={[0.06, 0.06, h, 8]} />
        <meshStandardMaterial color={p.chair} />
      </mesh>
      {/* 靠背（人的身后，+Z 侧） */}
      <mesh position={[0, h + 0.3, r * 0.85]} castShadow>
        <boxGeometry args={[r * 1.7, 0.5, 0.09]} />
        <meshStandardMaterial color={p.chair} roughness={0.85} />
      </mesh>
    </group>
  )
}

export function DeskFurniture({ slot }: { slot: DeskSlot }) {
  return (
    <group>
      <Desk slot={slot} />
      <Chair slot={slot} />
    </group>
  )
}
