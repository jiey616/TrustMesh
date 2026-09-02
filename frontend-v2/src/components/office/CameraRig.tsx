import { useEffect, useRef, type ComponentRef, type RefObject } from 'react'
import { useFrame } from '@react-three/fiber'
import { Vector3 } from 'three'
import type { OrbitControls } from '@react-three/drei'

// ─── 相机运镜 ───
// 单击 Agent 时把轨道中心平滑推到它身上并拉近，点空白处回到全景。
// 所有插值都在 useFrame 里直接改 camera / controls，不触发 React re-render。

/** 全景视角（与 OfficeCanvas 的初始机位、OrbitControls.target 保持一致）
 *  target 对准内容质心 (0, 0.5, 0.4)，相机距离 ≈ 15 */
const HOME_TARGET = new Vector3(0, 0.5, 0.4)
const HOME_DISTANCE = 15.0
/** 聚焦单个 Agent 时的观察距离 */
const FOCUS_DISTANCE = 5.5

export interface CameraRigProps {
  controlsRef: RefObject<ComponentRef<typeof OrbitControls> | null>
  /** 聚焦目标（地面坐标）；null = 回到全景 */
  focusPoint: { x: number; z: number } | null
}

export function CameraRig({ controlsRef, focusPoint }: CameraRigProps) {
  const desiredTarget = useRef(new Vector3())
  const desiredDistance = useRef(HOME_DISTANCE)
  const dir = useRef(new Vector3())

  useEffect(() => {
    if (focusPoint) {
      // 抬到人物胸口高度，别对着脚底
      desiredTarget.current.set(focusPoint.x, 1.0, focusPoint.z)
      desiredDistance.current = FOCUS_DISTANCE
    } else {
      desiredTarget.current.copy(HOME_TARGET)
      desiredDistance.current = HOME_DISTANCE
    }
  }, [focusPoint])

  useFrame(({ camera }, delta) => {
    const controls = controlsRef.current
    if (!controls) return

    // 帧率无关的指数逼近；delta 设上限，避免切回前台时相机瞬移
    const k = 1 - Math.exp(-6 * Math.min(delta, 0.1))
    controls.target.lerp(desiredTarget.current, k)

    dir.current.copy(camera.position).sub(controls.target)
    const distance = dir.current.length()
    if (distance < 1e-4) return
    dir.current.divideScalar(distance)
    const next = distance + (desiredDistance.current - distance) * k
    camera.position.copy(controls.target).addScaledVector(dir.current, next)

    controls.update()
  })

  return null
}
