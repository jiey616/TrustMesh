import { motion } from 'framer-motion'

type Variant = 'purple' | 'blue' | 'cyan' | 'green' | 'amber' | 'rose'

interface NeonBadgeProps {
  label: string
  variant?: Variant
  pulse?: boolean
  size?: 'sm' | 'md'
}

const COLORS: Record<Variant, { bg: string; border: string; text: string; glow: string }> = {
  purple: { bg: 'rgba(109,95,245,0.15)', border: 'rgba(109,95,245,0.4)', text: 'var(--signal-hover)', glow: '#a855f' },
  blue: { bg: 'rgba(59,130,246,0.15)', border: 'rgba(59,130,246,0.4)', text: 'var(--info)', glow: 'var(--info)' },
  cyan: { bg: 'rgba(34,211,238,0.15)', border: 'rgba(34,211,238,0.4)', text: 'var(--cyan)', glow: 'var(--cyan)' },
  green: { bg: 'rgba(16,185,129,0.15)', border: 'rgba(16,185,129,0.4)', text: 'var(--success)', glow: 'var(--success)' },
  amber: { bg: 'rgba(245,158,11,0.15)', border: 'rgba(245,158,11,0.4)', text: 'var(--warning)', glow: 'var(--warning)' },
  rose: { bg: 'rgba(244,63,94,0.15)', border: 'rgba(244,63,94,0.4)', text: 'var(--error)', glow: 'var(--error)' },
}

export function NeonBadge({ label, variant = 'purple', pulse = false, size = 'md' }: NeonBadgeProps) {
  const c = COLORS[variant]
  const pad = size === 'sm' ? '4px 10px' : '6px 14px'
  const fontSize = size === 'sm' ? 12 : 13

  return (
    <motion.span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 6,
        padding: pad,
        borderRadius: 'var(--radius-structure)',
        background: c.bg,
        border: `1px solid ${c.border}`,
        color: c.text,
        fontSize,
        fontWeight: 500,
        lineHeight: 1,
      }}
      whileHover={{
        boxShadow: `0 0 12px ${c.glow}44`,
        borderColor: c.glow,
      }}
    >
      {pulse && (
        <motion.span
          style={{
            width: 7,
            height: 7,
            borderRadius: 'var(--radius-avatar)',
            background: c.glow,
            boxShadow: `0 0 6px ${c.glow}`,
            display: 'inline-block',
          }}
          animate={{ opacity: [1, 0.5, 1] }}
          transition={{ duration: 2, repeat: Infinity, ease: 'easeInOut' }}
        />
      )}
      {label}
    </motion.span>
  )
}