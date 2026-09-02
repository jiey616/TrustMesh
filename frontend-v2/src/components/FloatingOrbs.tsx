/**
 * FloatingOrbs — ambient floating light orbs background
 * Inspired by Neon Glass UI's OrbBackground
 */
import { motion } from 'framer-motion'
import type { CSSProperties } from 'react'

interface Orb {
  size: number
  color: string
  x: string
  y: string
  delay: number
  duration: number
  blur: number
  opacity: number
}

const DEFAULT_ORBS: Orb[] = [
  { size: 400, color: 'rgba(109,95,245,0.18)', x: '5%', y: '10%', delay: 0, duration: 12, blur: 80, opacity: 0.5 },
  { size: 350, color: 'rgba(59,130,246,0.15)', x: '80%', y: '15%', delay: 2, duration: 14, blur: 70, opacity: 0.4 },
  { size: 300, color: 'rgba(34,211,238,0.12)', x: '50%', y: '60%', delay: 4, duration: 16, blur: 90, opacity: 0.35 },
  { size: 250, color: 'rgba(244,63,94,0.1)', x: '90%', y: '75%', delay: 1, duration: 13, blur: 75, opacity: 0.3 },
  { size: 350, color: 'rgba(245,158,11,0.08)', x: '15%', y: '80%', delay: 3, duration: 15, blur: 85, opacity: 0.25 },
]

interface FloatingOrbsProps {
  orbs?: Orb[]
}

export function FloatingOrbs({ orbs = DEFAULT_ORBS }: FloatingOrbsProps) {
  const containerStyle: CSSProperties = {
    position: 'fixed',
    inset: 0,
    overflow: 'hidden',
    pointerEvents: 'none',
    zIndex: 0,
  }

  return (
    <div style={containerStyle}>
      {orbs.map((orb, i) => (
        <motion.div
          key={i}
          style={{
            position: 'absolute',
            left: orb.x,
            top: orb.y,
            width: orb.size,
            height: orb.size,
            borderRadius: '50%',
            background: `radial-gradient(circle, ${orb.color}, transparent 70%)`,
            filter: `blur(${orb.blur}px)`,
            opacity: orb.opacity,
          }}
          animate={{
            x: [0, 30, -20, 0],
            y: [0, -25, 15, 0],
            scale: [1, 1.08, 0.95, 1],
          }}
          transition={{
            duration: orb.duration,
            delay: orb.delay,
            repeat: Infinity,
            ease: 'easeInOut',
          }}
        />
      ))}
    </div>
  )
}
