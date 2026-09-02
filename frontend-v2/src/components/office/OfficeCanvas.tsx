import { useMemo, useRef, type ComponentRef } from 'react'
import { Canvas } from '@react-three/fiber'
import { OrbitControls } from '@react-three/drei'
import { DESK_SLOTS } from './officeLayout'
import { OfficeSandbox } from './OfficeSandbox'
import { DeskFurniture } from './DeskFurniture'
import { MeetingTable } from './MeetingTable'
import { OfficeAgents } from './OfficeAgents'
import { OfficeWalls } from './OfficeWalls'
import { OfficeDecor } from './OfficeDecor'
import { CameraRig } from './CameraRig'
import { usePageVisibility } from '@/hooks/usePageVisibility'
import { useOfficePalette, useOfficeStore } from '@/stores/officeStore'

// ─── 3D 办公室画布 ───
// 相机为「受限轨道」：水平 ±35°、俯仰 25°~70°、可缩放。
// 既能获得空间感，又永远不会转到看不见内容的角度。
// 单击 Agent 由 CameraRig 平滑推近，点击空白处回到全景。

const DEG = Math.PI / 180

export interface OfficeCanvasProps {
  /** 有会议进行中（点亮会议区） */
  meetingActive?: boolean
  /** 高画质：开阴影 + 更高 dpr（默认关，弱设备友好） */
  highQuality?: boolean
}

export function OfficeCanvas({
  meetingActive = false,
  highQuality = false,
}: OfficeCanvasProps) {
  const visible = usePageVisibility()
  const controlsRef = useRef<ComponentRef<typeof OrbitControls>>(null)

  const focusAgentId = useOfficeStore((s) => s.focusAgentId)
  const seating = useOfficeStore((s) => s.seating)
  const setFocus = useOfficeStore((s) => s.setFocus)
  const p = useOfficePalette()

  const focusPoint = useMemo(() => {
    if (!focusAgentId) return null
    const slot = seating[focusAgentId]
    return slot ? { x: slot.seat.x, z: slot.seat.z } : null
  }, [focusAgentId, seating])

  return (
    <Canvas
      shadows={highQuality}
      dpr={highQuality ? [1, 2] : 1}
      // 切到后台标签页时停止渲染循环，避免持续占用 GPU
      frameloop={visible ? 'always' : 'never'}
      // 机位 ≈ 距离 15、俯仰 52°：18×15 沙盘整体入框（PM 办公室与会议区在背面一侧、
      // 主工位区前置），横向可视约 ±7 单位；聚焦单个 Agent 时推近到 ~5.5。
      camera={{ position: [0, 9.7, 12.2], fov: 55, near: 0.1, far: 120 }}
      gl={{ antialias: true, alpha: false }}
      style={{ width: '100%', height: '100%' }}
      // 点击空白处取消聚焦，回到全景
      onPointerMissed={() => setFocus(null)}
    >
      <color attach="background" args={[p.void]} />
      {/* 远处淡入背景空间，强化「悬浮沙盘」的边界感（取景拉近后雾起点前移） */}
      <fog attach="fog" args={[p.void, 24, 55]} />

      {/* 基础环境光（夜晚偏冷补暗部；白天接近无色且更强，模拟开灯/日光） */}
      <ambientLight intensity={p.ambientIntensity} color={p.ambientColor} />
      {/* 主光：斜上方，提供主要明暗与阴影 */}
      <directionalLight
        position={[8, 15, 7]}
        intensity={p.keyLightIntensity}
        castShadow={highQuality}
        shadow-mapSize={[1024, 1024]}
        shadow-camera-left={-15}
        shadow-camera-right={15}
        shadow-camera-top={15}
        shadow-camera-bottom={-15}
        shadow-camera-far={40}
      />
      {/* 冷色补光：夜晚勾出家具轮廓；白天几乎关闭 */}
      <directionalLight
        position={[-10, 7, -6]}
        intensity={p.fillLightIntensity}
        color="#4c5a8a"
      />

      <OfficeSandbox />

      {DESK_SLOTS.map((slot) => (
        <DeskFurniture key={slot.id} slot={slot} />
      ))}

      <MeetingTable active={meetingActive} />

      <OfficeWalls />

      <OfficeDecor />

      <OfficeAgents />

      <OrbitControls
        ref={controlsRef}
        makeDefault
        // 视点对准内容质心（主工位前置、PM/会议在背面，质心略偏 +Z），
        // 与 CameraRig 的 HOME_TARGET 保持一致，避免初始运镜跳变。
        target={[0, 0.5, 0.4]}
        enablePan
        enableDamping
        dampingFactor={0.08}
        // 水平 ±35°
        minAzimuthAngle={-35 * DEG}
        maxAzimuthAngle={35 * DEG}
        // 俯仰 25°~70°（不会低到让 billboard 精灵「躺平」）
        minPolarAngle={25 * DEG}
        maxPolarAngle={70 * DEG}
        // 5 = 能凑近看清单个 Agent；24 = 能拉远看到整个沙盘
        minDistance={5}
        maxDistance={24}
      />

      <CameraRig controlsRef={controlsRef} focusPoint={focusPoint} />
    </Canvas>
  )
}
