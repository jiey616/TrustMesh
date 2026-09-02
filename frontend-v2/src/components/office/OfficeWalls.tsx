import { SANDBOX } from './officeLayout'
import { useOfficePalette } from '@/stores/officeStore'

// ─── 外围墙体 ───
// 房间外壳：后墙 + 左右墙全高，前侧（z 正方向、面向默认相机）完全开放，
// 既给出「房间」的围合感，又不遮挡任何视线。
// 颜色随主题：夜晚 = 深蓝灰 + 霓虹；白天 = 白墙 + 日光。

const WALL_H = 2.6
const T = 0.16 // 墙厚

/** 半宽/半深，墙体贴着沙盘边缘内侧放置 */
const HW = SANDBOX.width / 2
const HD = SANDBOX.depth / 2

/** 竖长发光「窗」板（贴在内墙面上） */
function GlowWindow({
  position,
  color,
  rotationY = 0,
  w = 0.7,
  h = 0.9,
}: {
  position: [number, number, number]
  color: string
  rotationY?: number
  w?: number
  h?: number
}) {
  return (
    <mesh position={position} rotation={[0, rotationY, 0]}>
      <planeGeometry args={[w, h]} />
      <meshBasicMaterial color={color} transparent opacity={0.35} />
    </mesh>
  )
}

export function OfficeWalls() {
  const p = useOfficePalette()

  return (
    <group>
      {/* 后墙（z 负方向，远端） */}
      <group position={[0, 0, -HD + T / 2]}>
        <mesh position={[0, WALL_H / 2, 0]} castShadow receiveShadow>
          <boxGeometry args={[SANDBOX.width, WALL_H, T]} />
          <meshStandardMaterial color={p.wall} roughness={0.65} metalness={0.2} />
        </mesh>
        {/* 顶沿发光 */}
        <mesh position={[0, WALL_H + 0.03, 0]}>
          <boxGeometry args={[SANDBOX.width, 0.05, T + 0.04]} />
          <meshBasicMaterial color={p.wallTop} />
        </mesh>
        {/* 内墙面窗户 */}
        <GlowWindow position={[-4.5, 1.45, T / 2 + 0.01]} color={p.windowA} />
        <GlowWindow position={[4.5, 1.45, T / 2 + 0.01]} color={p.windowB} />
      </group>

      {/* 左墙 */}
      <group position={[-HW + T / 2, 0, 0]}>
        <mesh position={[0, WALL_H / 2, 0]} castShadow receiveShadow>
          <boxGeometry args={[T, WALL_H, SANDBOX.depth]} />
          <meshStandardMaterial color={p.wall} roughness={0.65} metalness={0.2} />
        </mesh>
        <mesh position={[0, WALL_H + 0.03, 0]}>
          <boxGeometry args={[T + 0.04, 0.05, SANDBOX.depth]} />
          <meshBasicMaterial color={p.wallTop} />
        </mesh>
        <GlowWindow position={[T / 2 + 0.01, 1.45, -2]} color={p.windowA} rotationY={Math.PI / 2} />
        <GlowWindow position={[T / 2 + 0.01, 1.45, 3]} color={p.windowB} rotationY={Math.PI / 2} />
      </group>

      {/* 右墙 */}
      <group position={[HW - T / 2, 0, 0]}>
        <mesh position={[0, WALL_H / 2, 0]} castShadow receiveShadow>
          <boxGeometry args={[T, WALL_H, SANDBOX.depth]} />
          <meshStandardMaterial color={p.wall} roughness={0.65} metalness={0.2} />
        </mesh>
        <mesh position={[0, WALL_H + 0.03, 0]}>
          <boxGeometry args={[T + 0.04, 0.05, SANDBOX.depth]} />
          <meshBasicMaterial color={p.wallTop} />
        </mesh>
        <GlowWindow position={[-T / 2 - 0.01, 1.45, -2]} color={p.windowB} rotationY={-Math.PI / 2} />
        <GlowWindow position={[-T / 2 - 0.01, 1.45, 3]} color={p.windowA} rotationY={-Math.PI / 2} />
      </group>

      {/* 前侧开放：只在地面留一条低矮发光门槛线，提示「这里是入口方向」 */}
      <mesh position={[0, 0.03, HD - 0.08]} rotation={[-Math.PI / 2, 0, 0]}>
        <planeGeometry args={[SANDBOX.width, 0.12]} />
        <meshBasicMaterial color={p.wallTop} transparent opacity={0.4} />
      </mesh>
    </group>
  )
}
