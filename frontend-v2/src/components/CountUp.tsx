import { useEffect, useRef, useState } from 'react'

interface Props {
  value: number
  duration?: number
  className?: string
  style?: React.CSSProperties
}

/** 数字递增动效：从 0 平滑递增到目标值（DESIGN.md motion: 800ms ease-out）*/
export function CountUp({ value, duration = 800, className, style }: Props) {
  const [display, setDisplay] = useState(0)
  const startRef = useRef<number | null>(null)
  const rafRef = useRef<number | null>(null)

  useEffect(() => {
    const from = 0
    startRef.current = null

    const step = (timestamp: number) => {
      if (startRef.current === null) startRef.current = timestamp
      const elapsed = timestamp - startRef.current
      const progress = Math.min(elapsed / duration, 1)
      // ease-out cubic
      const eased = 1 - Math.pow(1 - progress, 3)
      setDisplay(Math.round(from + (value - from) * eased))
      if (progress < 1) {
        rafRef.current = requestAnimationFrame(step)
      }
    }

    rafRef.current = requestAnimationFrame(step)
    return () => {
      if (rafRef.current) cancelAnimationFrame(rafRef.current)
    }
  }, [value, duration])

  return <span className={className} style={style}>{display}</span>
}
