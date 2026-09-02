import { useMemo } from 'react'
import { useOfficePalette } from '@/stores/officeStore'
import type { OfficePalette } from './officeLayout'

// ─── 装饰：家具 + 绿植 ───
// 参考实景办公室：沿后墙一排置物柜（台面摆小物件），房间角落与边缘
// 分布盆栽绿植。全部程序化几何体，无外部素材。
// 位置手工避开过道与工位，不会被走动的小人穿过。
// 颜色随主题：夜晚 = 深色柜体 + 微发光绿植；白天 = 浅木色 + 日光绿。

/** 一盆绿植：陶盆 + 三球叶簇 */
function Plant({ x, z, p, scale = 1 }: { x: number; z: number; p: OfficePalette; scale?: number }) {
  return (
    <group position={[x, 0, z]} scale={scale}>
      {/* 花盆 */}
      <mesh position={[0, 0.16, 0]} castShadow>
        <cylinderGeometry args={[0.2, 0.16, 0.32, 12]} />
        <meshStandardMaterial color={p.pot} roughness={0.8} />
      </mesh>
      {/* 盆沿 */}
      <mesh position={[0, 0.33, 0]} castShadow>
        <cylinderGeometry args={[0.22, 0.22, 0.05, 12]} />
        <meshStandardMaterial color={p.potRim} roughness={0.7} />
      </mesh>
      {/* 叶簇（错位三球，微自发光的绿） */}
      <mesh position={[0, 0.62, 0]} castShadow>
        <sphereGeometry args={[0.3, 12, 10]} />
        <meshStandardMaterial
          color={p.leaf1}
          roughness={0.75}
          emissive={p.leafEmissive}
          emissiveIntensity={p.leafGlow}
        />
      </mesh>
      <mesh position={[0.18, 0.85, 0.06]} castShadow>
        <sphereGeometry args={[0.22, 10, 8]} />
        <meshStandardMaterial
          color={p.leaf2}
          roughness={0.75}
          emissive={p.leafEmissive}
          emissiveIntensity={p.leafGlow}
        />
      </mesh>
      <mesh position={[-0.16, 0.88, -0.05]} castShadow>
        <sphereGeometry args={[0.18, 10, 8]} />
        <meshStandardMaterial
          color={p.leaf3}
          roughness={0.75}
          emissive={p.leafEmissive}
          emissiveIntensity={p.leafGlow}
        />
      </mesh>
    </group>
  )
}

/** 柜台台面小物件：书立/杯子/收纳盒的抽象体 */
function CounterItems({ baseY, p }: { baseY: number; p: OfficePalette }) {
  const items = useMemo(() => {
    // 固定伪随机，避免每次渲染重排
    const arr: { x: number; w: number; h: number; d: number }[] = []
    let seed = 42
    const rand = () => {
      seed = (seed * 16807) % 2147483647
      return seed / 2147483647
    }
    for (let i = 0; i < 10; i++) {
      arr.push({
        x: -5.6 + i * 1.25 + rand() * 0.3,
        w: 0.18 + rand() * 0.22,
        h: 0.16 + rand() * 0.3,
        d: 0.18 + rand() * 0.2,
      })
    }
    return arr
  }, [])

  const shades = [p.counterLine, p.counter, p.counterTop, p.potRim, p.wallTop]

  return (
    <group>
      {items.map((it, i) => (
        <mesh key={i} position={[it.x, baseY + it.h / 2, 0]} castShadow>
          <boxGeometry args={[it.w, it.h, it.d]} />
          <meshStandardMaterial color={shades[i % shades.length]} roughness={0.55} metalness={0.25} />
        </mesh>
      ))}
    </group>
  )
}

/** 沿后墙的置物矮柜 */
function BackCounter({ p }: { p: OfficePalette }) {
  const w = 13
  const d = 0.65
  const h = 0.85
  return (
    <group position={[0, 0, -6.9]}>
      {/* 柜体 */}
      <mesh position={[0, h / 2, 0]} castShadow receiveShadow>
        <boxGeometry args={[w, h, d]} />
        <meshStandardMaterial color={p.counter} roughness={0.6} metalness={0.15} />
      </mesh>
      {/* 台面（略突出） */}
      <mesh position={[0, h + 0.025, 0]}>
        <boxGeometry args={[w + 0.1, 0.05, d + 0.08]} />
        <meshStandardMaterial color={p.counterTop} roughness={0.45} metalness={0.3} />
      </mesh>
      {/* 柜门分隔线 */}
      {[-4.3, -2.15, 0, 2.15, 4.3].map((x) => (
        <mesh key={x} position={[x, h / 2, d / 2 + 0.005]}>
          <planeGeometry args={[0.03, h * 0.7]} />
          <meshBasicMaterial color={p.counterLine} />
        </mesh>
      ))}
      <CounterItems baseY={h + 0.05} p={p} />
    </group>
  )
}

/** 左右墙边的矮边柜（对称各一个） */
function SideCounter({ x, z, len, p }: { x: number; z: number; len: number; p: OfficePalette }) {
  const d = 0.55
  const h = 0.7
  return (
    <group position={[x, 0, z]} rotation={[0, Math.PI / 2, 0]}>
      <mesh position={[0, h / 2, 0]} castShadow receiveShadow>
        <boxGeometry args={[len, h, d]} />
        <meshStandardMaterial color={p.counter} roughness={0.6} metalness={0.15} />
      </mesh>
      <mesh position={[0, h + 0.025, 0]}>
        <boxGeometry args={[len + 0.08, 0.05, d + 0.08]} />
        <meshStandardMaterial color={p.counterTop} roughness={0.45} metalness={0.3} />
      </mesh>
    </group>
  )
}

export function OfficeDecor() {
  const p = useOfficePalette()
  return (
    <group>
      <BackCounter p={p} />
      <SideCounter x={-8.35} z={-0.2} len={3.6} p={p} />
      <SideCounter x={8.35} z={-0.2} len={3.6} p={p} />

      {/* 绿植：四角 + 背柜两端 + 前侧，避开所有工位与过道 */}
      <Plant x={-7.9} z={-6.6} p={p} />
      <Plant x={8} z={-6.7} p={p} scale={1.15} />
      <Plant x={-8.2} z={4.6} p={p} scale={1.1} />
      <Plant x={8.2} z={4.8} p={p} />
      <Plant x={-7.9} z={6.6} p={p} scale={0.9} />
      <Plant x={7.6} z={6.7} p={p} />
    </group>
  )
}
