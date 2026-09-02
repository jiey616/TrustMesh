// @react-three/fiber v9 通过 `declare module 'react'` 扩展 JSX.IntrinsicElements，
// 只有在其类型被加载后才会生效。本文件一次性引入 R3F 类型，
// 使 <mesh> / <group> / <planeGeometry> 等 3D 标签在整个项目中可用，
// 无需在每个组件里重复写 `import type {} from '@react-three/fiber'`。
import type {} from '@react-three/fiber'
