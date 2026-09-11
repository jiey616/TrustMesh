import { useMemo, useRef } from 'react'
import { Canvas, useFrame } from '@react-three/fiber'
import * as THREE from 'three'

/* ------------------------------------------------------------------
 * 登录页 3D 粒子网络球体
 * - 内层：fibonacci 分布的粒子球壳（紫→青渐变）
 * - 中层：三条倾斜粒子轨道环
 * - 外层：稀疏星尘（缓慢反向旋转）
 * - 中心：柔和光晕 sprite
 * 全部用 Points/Sprite，≈2500 顶点，低功耗设备无压力。
 * ------------------------------------------------------------------ */

const INNER_COLORS = [
  new THREE.Color('#6d5ff5'), // 品牌紫
  new THREE.Color('#8b5cf6'),
  new THREE.Color('#22d3ee'), // 科技青
]

function makeSoftDotTexture(): THREE.Texture {
  const size = 64
  const canvas = document.createElement('canvas')
  canvas.width = size
  canvas.height = size
  const ctx = canvas.getContext('2d')!
  const grad = ctx.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2)
  grad.addColorStop(0, 'rgba(255,255,255,1)')
  grad.addColorStop(0.25, 'rgba(255,255,255,0.8)')
  grad.addColorStop(1, 'rgba(255,255,255,0)')
  ctx.fillStyle = grad
  ctx.fillRect(0, 0, size, size)
  const tex = new THREE.CanvasTexture(canvas)
  tex.needsUpdate = true
  return tex
}

/** 球壳上均匀分布的采样点（fibonacci sphere） */
function fibonacciSphere(count: number, radius: number, jitter = 0.06) {
  const positions = new Float32Array(count * 3)
  const colors = new Float32Array(count * 3)
  const golden = Math.PI * (3 - Math.sqrt(5))
  for (let i = 0; i < count; i++) {
    const y = 1 - (i / (count - 1)) * 2
    const r = Math.sqrt(1 - y * y)
    const theta = golden * i
    const jx = (Math.random() - 0.5) * jitter
    const jy = (Math.random() - 0.5) * jitter
    const jz = (Math.random() - 0.5) * jitter
    positions[i * 3] = Math.cos(theta) * r * radius + jx
    positions[i * 3 + 1] = y * radius + jy
    positions[i * 3 + 2] = Math.sin(theta) * r * radius + jz
    // 颜色按纬度渐变：底部紫 → 顶部青
    const t = (y + 1) / 2
    const c = INNER_COLORS[0].clone().lerp(INNER_COLORS[2], t)
    if (t > 0.55) c.lerp(INNER_COLORS[2], (t - 0.55) * 0.8)
    colors[i * 3] = c.r
    colors[i * 3 + 1] = c.g
    colors[i * 3 + 2] = c.b
  }
  return { positions, colors }
}

/** 圆环上均匀采样 */
function torusRing(count: number, radius: number, tilt: number) {
  const positions = new Float32Array(count * 3)
  for (let i = 0; i < count; i++) {
    const a = (i / count) * Math.PI * 2
    const x = Math.cos(a) * radius
    const z = Math.sin(a) * radius
    const y = Math.sin(a * 2) * radius * 0.12
    positions[i * 3] = x
    positions[i * 3 + 1] = y * Math.sin(tilt) + 0 // 简单平面倾斜留给 group 处理
    positions[i * 3 + 2] = z
  }
  return positions
}

function ParticleSphere() {
  const groupRef = useRef<THREE.Group>(null)
  const starRef = useRef<THREE.Points>(null)
  const mouse = useRef({ x: 0, y: 0 })

  const dotTex = useMemo(() => makeSoftDotTexture(), [])

  const sphere = useMemo(() => fibonacciSphere(1500, 1.15), [])
  const starField = useMemo(() => {
    const count = 420
    const positions = new Float32Array(count * 3)
    for (let i = 0; i < count; i++) {
      const r = 2.4 + Math.random() * 1.9
      const theta = Math.random() * Math.PI * 2
      const phi = Math.acos(2 * Math.random() - 1)
      positions[i * 3] = r * Math.sin(phi) * Math.cos(theta)
      positions[i * 3 + 1] = r * Math.sin(phi) * Math.sin(theta) * 0.7
      positions[i * 3 + 2] = r * Math.cos(phi)
    }
    return positions
  }, [])

  const rings = useMemo(() => {
    return [1.65, 2.0, 2.35].map((radius, i) => ({
      radius,
      tilt: [0.35, -0.5, 0.9][i] ?? 0.3,
      speed: [0.9, -0.7, 0.5][i] ?? 0.4,
      positions: torusRing(280, radius, 0),
    }))
  }, [])

  useFrame((state, delta) => {
    // 视差：鼠标位置缓慢逼近
    mouse.current.x += (state.pointer.x * 0.35 - mouse.current.x) * Math.min(delta * 3, 1)
    mouse.current.y += (state.pointer.y * 0.25 - mouse.current.y) * Math.min(delta * 3, 1)

    const g = groupRef.current
    if (g) {
      g.rotation.y += delta * 0.12
      g.rotation.x += delta * 0.02
      g.rotation.x = mouse.current.y * 0.25 + Math.sin(state.clock.elapsedTime * 0.15) * 0.05
      g.rotation.z = mouse.current.x * 0.18
    }
    if (starRef.current) {
      starRef.current.rotation.y -= delta * 0.03
      starRef.current.rotation.x += delta * 0.008
    }
  })

  return (
    <group>
      <group ref={groupRef}>
        {/* 中心光晕 */}
        <sprite scale={[4.4, 4.4, 1]}>
          <spriteMaterial
            map={dotTex}
            color="#6d5ff5"
            transparent
            opacity={0.28}
            depthWrite={false}
            blending={THREE.AdditiveBlending}
          />
        </sprite>

        {/* 内层粒子球壳 */}
        <points>
          <bufferGeometry>
            <bufferAttribute attach="attributes-position" args={[sphere.positions, 3]} />
            <bufferAttribute attach="attributes-color" args={[sphere.colors, 3]} />
          </bufferGeometry>
          <pointsMaterial
            size={0.055}
            map={dotTex}
            vertexColors
            transparent
            opacity={0.95}
            depthWrite={false}
            blending={THREE.AdditiveBlending}
            sizeAttenuation
          />
        </points>

        {/* 轨道环 */}
        {rings.map((ring, i) => (
          <group key={i} rotation={[ring.tilt, 0, ring.tilt * 0.6]}>
            <points>
              <bufferGeometry>
                <bufferAttribute attach="attributes-position" args={[ring.positions, 3]} />
              </bufferGeometry>
              <pointsMaterial
                size={0.03}
                map={dotTex}
                color={i === 2 ? '#22d3ee' : '#8b7ff8'}
                transparent
                opacity={0.5}
                depthWrite={false}
                blending={THREE.AdditiveBlending}
                sizeAttenuation
              />
            </points>
          </group>
        ))}
      </group>

      {/* 外层星尘（不跟随主球旋转，营造空间层次） */}
      <points ref={starRef}>
        <bufferGeometry>
          <bufferAttribute attach="attributes-position" args={[starField, 3]} />
        </bufferGeometry>
        <pointsMaterial
          size={0.028}
          map={dotTex}
          color="#a5b4fc"
          transparent
          opacity={0.5}
          depthWrite={false}
          blending={THREE.AdditiveBlending}
          sizeAttenuation
        />
      </points>
    </group>
  )
}

export function LoginParticles3D() {
  return (
    <Canvas
      dpr={[1, 1.5]}
      camera={{ position: [0, 0, 4.6], fov: 42 }}
      gl={{ antialias: false, alpha: true, powerPreference: 'low-power' }}
      style={{ width: '100%', height: '100%', background: 'transparent' }}
    >
      <ParticleSphere />
    </Canvas>
  )
}
