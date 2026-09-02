/**
 * GradientText — animated flowing gradient text
 * Inspired by Aceternity UI / Magic UI
 */
import { motion } from 'framer-motion'
import type { ReactNode } from 'react'

interface GradientTextProps {
  children: ReactNode
  /** Gradient stops. Default: purple → blue → cyan */
  colors?: string[]
  /** Animation duration in seconds */
  speed?: number
  style?: React.CSSProperties
  className?: string
}

export function GradientText({
  children,
  colors = ['#8b7ff8', '#818cf8', '#60a5fa', '#22d3ee'],
  speed = 4,
  style,
  className,
}: GradientTextProps) {
  const gradient = `linear-gradient(120deg, ${colors.join(', ')})`

  return (
    <motion.span
      className={className}
      style={{
        background: gradient,
        backgroundSize: '200% 200%',
        WebkitBackgroundClip: 'text',
        backgroundClip: 'text',
        color: 'transparent',
        display: 'inline',
        ...style,
      }}
      animate={{
        backgroundPosition: ['0% 50%', '100% 50%', '0% 50%'],
      }}
      transition={{
        duration: speed,
        repeat: Infinity,
        ease: 'linear',
      }}
    >
      {children}
    </motion.span>
  )
}
