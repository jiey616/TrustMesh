import { useMemo } from 'react'
import { AdditiveBlending, BufferGeometry, Float32BufferAttribute } from 'three'
import { SANDBOX, ZONES, ZONE_GLOW } from './officeLayout'
import { useOfficePalette } from '@/stores/officeStore'

// ─── 全息沙盘底座 ───
// 办公室是悬浮在深色空间中的微缩模型：地面用低饱和深色做「底」，
// 三个功能区各配一块彩色地毯 + 霓虹描边，霓虹色取自 V2 多巴胺色板
// （工位区淡紫 / PM 办公室紫 / 会议区青）。

/** 生成矩形边框线段（用于霓虹描边） */
function useRectOutline(w: number, d: number, y: number): BufferGeometry {
  return useMemo(() => {
    const hw = w / 2
    const hd = d / 2
    const pts = [
      -hw, y, -hd, hw, y, -hd,
      hw, y, -hd, hw, y, hd,
      hw, y, hd, -hw, y, hd,
      -hw, y, hd, -hw, y, -hd,
    ]
    const geo = new BufferGeometry()
    geo.setAttribute('position', new Float32BufferAttribute(pts, 3))
    return geo
  }, [w, d, y])
}

/** 分区地毯 + 霓虹描边 */
function ZoneRug({
  x,
  z,
  w,
  d,
  color,
  glow,
  opacity = 0.5,
}: {
  x: number
  z: number
  w: number
  d: number
  color: string
  glow: string
  opacity?: number
}) {
  const outline = useRectOutline(w, d, 0.03)
  return (
    <group position={[x, 0, z]}>
      <mesh rotation={[-Math.PI / 2, 0, 0]} position={[0, 0.02, 0]}>
        <planeGeometry args={[w, d]} />
        <meshStandardMaterial color={color} roughness={0.95} />
      </mesh>
      <lineSegments geometry={outline}>
        <lineBasicMaterial
          color={glow}
          transparent
          opacity={opacity}
          blending={AdditiveBlending}
        />
      </lineSegments>
    </group>
  )
}

export function OfficeSandbox() {
  const p = useOfficePalette()
  const rim = useRectOutline(SANDBOX.width, SANDBOX.depth, 0.02)

  return (
    <group>
      {/* 沙盘地面 */}
      <mesh rotation={[-Math.PI / 2, 0, 0]} position={[0, 0, 0]} receiveShadow>
        <planeGeometry args={[SANDBOX.width, SANDBOX.depth]} />
        <meshStandardMaterial color={p.floor} roughness={0.92} metalness={0.06} />
      </mesh>

      {/* 发光网格（每格 1 单位，Z 方向按沙盘比例压缩） */}
      <gridHelper
        args={[SANDBOX.width, SANDBOX.width, p.grid, p.grid]}
        position={[0, 0.015, 0]}
        scale={[1, 1, SANDBOX.depth / SANDBOX.width]}
      >
        <lineBasicMaterial color={p.grid} transparent opacity={p.gridOpacity} />
      </gridHelper>

      {/* 三个功能区：地毯 + 霓虹描边（工位区最淡，避免抢人物） */}
      <ZoneRug
        x={ZONES.workArea.x}
        z={ZONES.workArea.z}
        w={ZONES.workArea.w}
        d={ZONES.workArea.d}
        color={p.rugWork}
        glow={ZONE_GLOW.workArea}
        opacity={0.3}
      />
      <ZoneRug
        x={ZONES.pmOffice.x}
        z={ZONES.pmOffice.z}
        w={ZONES.pmOffice.w}
        d={ZONES.pmOffice.d}
        color={p.rugPm}
        glow={ZONE_GLOW.pmOffice}
      />
      <ZoneRug
        x={ZONES.meeting.x}
        z={ZONES.meeting.z}
        w={ZONES.meeting.w}
        d={ZONES.meeting.d}
        color={p.rugMeeting}
        glow={ZONE_GLOW.meeting}
      />

      {/* 外缘发光边框（加色混合模拟辉光，无需后处理） */}
      <lineSegments geometry={rim}>
        <lineBasicMaterial
          color={p.rim}
          transparent
          opacity={0.85}
          blending={AdditiveBlending}
        />
      </lineSegments>
    </group>
  )
}
