import { useMemo } from 'react'
import { AdditiveBlending } from 'three'
import { MEETING_TABLE } from './officeLayout'
import { makeLabelTexture, textureAspect } from './sprites'
import { useOfficePalette } from '@/stores/officeStore'

// ─── 会议室区域 ───
// 会议桌 + 环绕落座点。会议进行中时桌面上方浮现发光环与标牌。

function Seat({ x, z }: { x: number; z: number }) {
  const p = useOfficePalette()
  const { center } = MEETING_TABLE
  // three 中物体默认朝 +Z，绕 Y 旋转后 +Z 变为 (sinθ, 0, cosθ)。
  // 让椅子正面朝向桌心：θ = atan2(dx, dz)
  const facing = Math.atan2(center.x - x, center.z - z)

  return (
    <group position={[x, 0, z]} rotation={[0, facing, 0]}>
      <mesh position={[0, 0.5, 0]} castShadow>
        <cylinderGeometry args={[0.34, 0.3, 0.1, 12]} />
        <meshStandardMaterial color={p.chair} roughness={0.85} />
      </mesh>
      <mesh position={[0, 0.25, 0]}>
        <cylinderGeometry args={[0.06, 0.06, 0.5, 8]} />
        <meshStandardMaterial color={p.chair} />
      </mesh>
      {/* 靠背在身后（-Z 侧） */}
      <mesh position={[0, 0.8, -0.3]} castShadow>
        <boxGeometry args={[0.58, 0.5, 0.09]} />
        <meshStandardMaterial color={p.chair} roughness={0.85} />
      </mesh>
    </group>
  )
}

export function MeetingTable({ active = false }: { active?: boolean }) {
  const p = useOfficePalette()
  const { center, radiusX, radiusZ, seats } = MEETING_TABLE

  const labelTex = useMemo(
    () =>
      makeLabelTexture(active ? '会议进行中' : '会议室', {
        fontSize: 44,
        color: active ? '#8b7ff8' : 'rgba(255,255,255,0.48)',
        background: active ? 'rgba(109,95,245,0.22)' : 'rgba(10,10,18,0.7)',
        borderColor: active ? 'rgba(109,95,245,0.65)' : 'rgba(255,255,255,0.12)',
      }),
    [active],
  )
  const aspect = textureAspect(labelTex)

  return (
    <group>
      {/* 椭圆桌面：cylinder 是圆的，靠 scale.x 拉成椭圆 */}
      <mesh
        position={[center.x, 0.75, center.z]}
        scale={[radiusX / radiusZ, 1, 1]}
        castShadow
        receiveShadow
      >
        <cylinderGeometry args={[radiusZ, radiusZ * 0.96, 0.09, 40]} />
        <meshStandardMaterial color={p.deskTop} roughness={0.66} metalness={0.12} />
      </mesh>

      {/* 中央桌腿 */}
      <mesh position={[center.x, 0.36, center.z]}>
        <cylinderGeometry args={[0.3, 0.42, 0.72, 14]} />
        <meshStandardMaterial color={p.deskEdge} roughness={0.8} />
      </mesh>

      {seats.map((seat, i) => (
        <Seat key={i} x={seat.x} z={seat.z} />
      ))}

      {/* 会议进行中：桌面上方发光环（加色混合，无需后处理） */}
      {active && (
        <mesh
          position={[center.x, 0.86, center.z]}
          rotation={[-Math.PI / 2, 0, 0]}
          scale={[radiusX / radiusZ, 1, 1]}
        >
          <ringGeometry args={[radiusZ * 0.72, radiusZ * 0.86, 40]} />
          <meshBasicMaterial
            color={p.monitorGlow}
            transparent
            opacity={0.55}
            blending={AdditiveBlending}
            depthWrite={false}
          />
        </mesh>
      )}

      {/* 标牌：Canvas 贴图，支持中文 */}
      <mesh position={[center.x, 2.15, center.z]}>
        <planeGeometry args={[0.55 * aspect, 0.55]} />
        <meshBasicMaterial map={labelTex} transparent depthWrite={false} />
      </mesh>

      {/* 会议进行时的紫色补光 */}
      {active && (
        <pointLight
          position={[center.x, 2.6, center.z]}
          color={p.monitorGlow}
          intensity={6}
          distance={9}
          decay={2}
        />
      )}
    </group>
  )
}
