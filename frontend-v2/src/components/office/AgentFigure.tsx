import { useRef } from 'react'
import { useFrame } from '@react-three/fiber'
import type { Group, Mesh } from 'three'

export interface AgentFigureProps {
  /** Agent 角色（当前仅用于语义标注，颜色由 color 传入） */
  role: string
  /** 该 Agent 的专属颜色（按 id 稳定分配） */
  color: string
  /** 离线时整体变淡 */
  dimmed: boolean
  /** 走动：摆臂摆腿 */
  walking: boolean
  /** 打字：手臂微抖 */
  typing: boolean
  /** 由 id 派生的稳定相位，避免所有人动作同步 */
  phase: number
}

function approach(current: number, target: number, k: number): number {
  return current + (target - current) * k
}

/**
 * 程序化 Q 版 3D 小人（总高约 1.45）：
 * 圆头 + 短圆身 + 短四肢 + 大眼睛 + 腮红 + 头顶 role 色呆毛。
 * 头部（脸）跟随身体朝向：在工位时面朝桌上显示器，走动时朝移动方向。
 * 动画全部在 useFrame 内直改 Object3D，不触发 re-render。
 */
// eslint-disable-next-line @typescript-eslint/no-unused-vars
export function AgentFigure({ role: _role, color, dimmed, walking, typing, phase }: AgentFigureProps) {
  const legLRef = useRef<Group>(null)
  const legRRef = useRef<Group>(null)
  const armLRef = useRef<Group>(null)
  const armRRef = useRef<Group>(null)
  const bodyRef = useRef<Mesh>(null)

  const tint = color
  const opacity = dimmed ? 0.35 : 1

  useFrame(({ clock }, delta) => {
    const t = clock.elapsedTime
    const k = 1 - Math.exp(-12 * Math.min(delta, 0.1))
    const swing = walking ? Math.sin(t * 10 + phase) * 0.5 : 0
    const jitter = typing && !walking ? Math.sin(t * 14 + phase) * 0.08 : 0

    // 手腿摆动：走路时手脚反相交叉摆；打字时右臂微抖
    if (legLRef.current) legLRef.current.rotation.x = approach(legLRef.current.rotation.x, swing, k)
    if (legRRef.current) legRRef.current.rotation.x = approach(legRRef.current.rotation.x, -swing, k)
    if (armLRef.current) armLRef.current.rotation.x = approach(armLRef.current.rotation.x, -swing * 0.7 + jitter, k)
    if (armRRef.current) armRRef.current.rotation.x = approach(armRRef.current.rotation.x, swing * 0.7 - jitter, k)

    // 呼吸：身体轻微胀缩
    if (bodyRef.current) {
      const breathe = 1 + Math.sin(t * 2 + phase) * 0.025
      bodyRef.current.scale.set(breathe, breathe, breathe)
    }
  })

  return (
    <group>
      {/* 短腿（髋部枢轴，深色） */}
      <group ref={legLRef} position={[-0.17, 0.28, 0]}>
        <mesh position={[0, -0.12, 0]} castShadow={!dimmed}>
          <capsuleGeometry args={[0.13, 0.12, 4, 10]} />
          <meshStandardMaterial
            color="#2b3040"
            roughness={0.55}
            transparent={dimmed}
            opacity={opacity}
          />
        </mesh>
      </group>
      <group ref={legRRef} position={[0.17, 0.28, 0]}>
        <mesh position={[0, -0.12, 0]} castShadow={!dimmed}>
          <capsuleGeometry args={[0.13, 0.12, 4, 10]} />
          <meshStandardMaterial
            color="#2b3040"
            roughness={0.55}
            transparent={dimmed}
            opacity={opacity}
          />
        </mesh>
      </group>

      {/* 圆润短身：role 染色 + 微自发光（契合霓虹多巴胺风格） */}
      <mesh ref={bodyRef} position={[0, 0.58, 0]} castShadow={!dimmed}>
        <capsuleGeometry args={[0.34, 0.36, 6, 16]} />
        <meshStandardMaterial
          color={tint}
          roughness={0.35}
          emissive={tint}
          emissiveIntensity={0.18}
          transparent={dimmed}
          opacity={opacity}
        />
      </mesh>

      {/* 圆手臂（肩部枢轴，与身体同色） */}
      <group ref={armLRef} position={[-0.42, 0.78, 0]}>
        <mesh position={[0, -0.18, 0]} castShadow={!dimmed}>
          <capsuleGeometry args={[0.11, 0.2, 4, 10]} />
          <meshStandardMaterial
            color={tint}
            roughness={0.4}
            transparent={dimmed}
            opacity={opacity}
          />
        </mesh>
      </group>
      <group ref={armRRef} position={[0.42, 0.78, 0]}>
        <mesh position={[0, -0.18, 0]} castShadow={!dimmed}>
          <capsuleGeometry args={[0.11, 0.2, 4, 10]} />
          <meshStandardMaterial
            color={tint}
            roughness={0.4}
            transparent={dimmed}
            opacity={opacity}
          />
        </mesh>
      </group>

      {/* 头部组：跟随身体朝向（脸朝向由父级 figureRef 控制） */}
      <group position={[0, 1.02, 0]}>
        {/* 圆头（暖肤色） */}
        <mesh castShadow={!dimmed}>
          <sphereGeometry args={[0.4, 24, 18]} />
          <meshStandardMaterial
            color="#f6d3b6"
            roughness={0.45}
            transparent={dimmed}
            opacity={opacity}
          />
        </mesh>

        {/* 头顶 role 色呆毛（识别角色又显 Q） */}
        <mesh position={[0, 0.52, 0]}>
          <sphereGeometry args={[0.1, 12, 10]} />
          <meshStandardMaterial
            color={tint}
            roughness={0.4}
            emissive={tint}
            emissiveIntensity={0.25}
            transparent={dimmed}
            opacity={opacity}
          />
        </mesh>

        {/* 大眼睛（脸朝 +Z） */}
        <mesh position={[-0.15, 0.04, 0.33]}>
          <sphereGeometry args={[0.085, 12, 10]} />
          <meshStandardMaterial color="#161a26" roughness={0.25} />
        </mesh>
        <mesh position={[0.15, 0.04, 0.33]}>
          <sphereGeometry args={[0.085, 12, 10]} />
          <meshStandardMaterial color="#161a26" roughness={0.25} />
        </mesh>
        {/* 眼睛高光（更有神） */}
        <mesh position={[-0.11, 0.08, 0.39]}>
          <sphereGeometry args={[0.026, 8, 6]} />
          <meshBasicMaterial color="#ffffff" />
        </mesh>
        <mesh position={[0.19, 0.08, 0.39]}>
          <sphereGeometry args={[0.026, 8, 6]} />
          <meshBasicMaterial color="#ffffff" />
        </mesh>

        {/* 腮红 */}
        <mesh position={[-0.26, -0.08, 0.3]} scale={[1, 0.6, 0.4]}>
          <sphereGeometry args={[0.07, 10, 8]} />
          <meshBasicMaterial color="#f2a09a" transparent opacity={dimmed ? 0.15 : 0.65} />
        </mesh>
        <mesh position={[0.26, -0.08, 0.3]} scale={[1, 0.6, 0.4]}>
          <sphereGeometry args={[0.07, 10, 8]} />
          <meshBasicMaterial color="#f2a09a" transparent opacity={dimmed ? 0.15 : 0.65} />
        </mesh>
      </group>
    </group>
  )
}
