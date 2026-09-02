/**
 * GlowCard — futuristic glass card with 3D tilt and neon borderder glow
 * Inspired by Aceternity UI's card + Farstar UI's intensity system
 */
import { motion, useMotionValue, useSpring, useTransform } from 'framer-motion'
import { useCallback, useRef, type ReactNode } from 'react'

type Intensity = 'subtle' | 'medium' | 'full'

interface GlowCardProps {
  children: ReactNode
  className?: string
  intensity?: Intensity
  glowColor?: string
  onClick?: () => void
}

const CFG: Record<Intensity, { tilt: number; glow: number }> = {
  subtle: { tilt: 3, glow: 0.25 },
  medium: { tilt: 6, glow: 0.5 },
  full: { tilt: 10, glow: 0.8 },
}

export function GlowCard({
  children,
  className = '',
  intensity = 'medium',
  glowColor = '#6d5ff5',
  onClick,
}: GlowCardProps) {
  const ref = useRef<HTMLDivElement>(null)
  const rawX = useMotionValue(0.5)
  const rawY = useMotionValue(0.5)

  const x = useSpring(rawX, { stiffness: 300, damping: 30 })
  const y = useSpring(rawY, { stiffness: 300, damping: 30 })

  const { tilt } = CFG[intensity]

  const rotateX = useTransform(y, [0, 1], [tilt, -tilt])
  const rotateY = useTransform(x, [0, 1], [-tilt, tilt])

  const handleMove = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (!ref.current) return
      const r = ref.current.getBoundingClientRect()
      rawX.set((e.clientX - r.left) / r.width)
      rawY.set((e.clientY - r.top) / r.height)
    },
    [rawX, rawY],
  )

  const handleLeave = useCallback(() => {
    rawX.set(0.5)
    rawY.set(0.5)
  }, [rawX, rawY])

  return (
    <div ref={ref} style={{ perspective: 1000 }} className={className}>
      <motion.div
        onClick={onClick}
        onMouseMove={handleMove}
        onMouseLeave={handleLeave}
        style={{
          rotateX,
          rotateY,
          background: 'rgba(255, 255, 255, 0.03)',
          border: '1px solid rgba(255, 255, 255, 0.07)',
          backdropFilter: 'blur(24px)',
          WebkitBackdropFilter: 'blur(24px)',
          borderRadius: 16,
          cursor: onClick ? 'pointer' : 'default',
          position: 'relative',
          overflow: 'hidden',
        }}
        whileHover={{
          borderColor: `${glowColor}44`,
          boxShadow: `0 8px 32px rgba(0,0,0,0.4), 0 0 20px ${glowColor}22`,
        }}
        transition={{ type: 'spring', stiffness: 300, damping: 30 }}
      >
        {/* Glass shine spotlight — uses raw motion values for position */}
        <motion.div
          style={{
            position: 'absolute',
            inset: 0,
            borderRadius: 16,
            background: 'radial-gradient(circle at 50% 50%, rgba(255,255,255,0.06) 0%, transparent 50%)',
            pointerEvents: 'none',
            zIndex: 1,
            left: useTransform(x, [0, 1], ['-50%', '50%']),
            top: useTransform(y, [0, 1], ['-50%', '50%']),
          }}
        />
        <div style={{ position: 'relative', zIndex: 2 }}>{children}</div>
      </motion.div>
    </div>
  )
}
