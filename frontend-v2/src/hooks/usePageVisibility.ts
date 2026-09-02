import { useEffect, useState } from 'react'

/**
 * 页面是否可见（切到后台标签页时为 false）。
 * 办公室用它暂停 3D 渲染循环，避免后台标签页持续占用 GPU。
 */
export function usePageVisibility(): boolean {
  const [visible, setVisible] = useState(
    () => typeof document === 'undefined' || !document.hidden,
  )

  useEffect(() => {
    const onChange = () => setVisible(!document.hidden)
    document.addEventListener('visibilitychange', onChange)
    return () => document.removeEventListener('visibilitychange', onChange)
  }, [])

  return visible
}
