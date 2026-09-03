import type { AgentRole, AgentStatus } from '@/types'

/**
 * 符合 TrustMesh UI 规范（frontend-v2/DESIGN.md）的程序化数字员工头像：
 * - 深色 console、无饱和霓虹，violet + 语义色作为唯一 accent
 * - 基于 id/name 的确定性渐变（同一数字员工始终同一头像）
 * - 角色色：pm=amber→rose、developer=blue→cyan、reviewer=green、custom=violet（品牌色）
 * - 右上角状态点（online 带柔和光晕，busy/offline 无光晕）
 */

const roleGradients: Record<AgentRole, { from: string; to: string }> = {
  pm: { from: 'var(--warning)', to: 'var(--error)' },
  developer: { from: 'var(--info)', to: 'var(--cyan)' },
  reviewer: { from: 'var(--success)', to: 'var(--success)' },
  custom: { from: 'var(--signal)', to: 'var(--signal-hover)' },
}

const statusColors: Record<AgentStatus, string> = {
  online: 'var(--success)',
  busy: 'var(--info)',
  offline: 'var(--text-quaternary)',
}

/** 确定性字符串哈希（32 位） */
function hashString(s: string): number {
  let h = 0
  for (let i = 0; i < s.length; i++) {
    h = (h * 31 + s.charCodeAt(i)) >>> 0
  }
  return h
}

interface AgentAvatarProps {
  name: string
  role: AgentRole
  /** 用于确定性渐变角度；不传则用 name 哈希 */
  seed?: string
  size?: number
  status?: AgentStatus
  /** 形状：circle（默认，pill）/ rounded */
  shape?: 'circle' | 'rounded'
}

export function AgentAvatar({
  name,
  role,
  seed,
  size = 40,
  status,
  shape = 'circle',
}: AgentAvatarProps) {
  const g = roleGradients[role] ?? roleGradients.custom
  const h = hashString(seed || name)
  const angle = h % 360
  const letter = (name || '?').slice(0, 1).toUpperCase()
  const dot = Math.max(4, Math.round(size * 0.2))

  return (
    <div
      style={{
        width: size,
        height: size,
        borderRadius: shape === 'circle' ? '50%' : size * 0.28,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: `linear-gradient(${angle}deg, ${g.from} 0%, ${g.to} 100%)`,
        color: 'var(--text-primary)',
        fontSize: size * 0.42,
        fontWeight: 600,
        letterSpacing: '-0.3px',
        fontFamily: 'Inter, -apple-system, sans-serif',
        flexShrink: 0,
        position: 'relative',
        boxShadow: 'inset 0 0 0 1px rgba(255,255,255,0.12)',
        userSelect: 'none',
      }}
      title={name}
    >
      {letter}
      {status && (
        <span
          style={{
            position: 'absolute',
            right: -Math.round(dot * 0.15),
            bottom: -Math.round(dot * 0.15),
            width: dot,
            height: dot,
            borderRadius: 'var(--radius-avatar)',
            background: statusColors[status] ?? statusColors.offline,
            border: `2px solid #0a0a12`,
            boxShadow: status === 'online' ? `0 0 ${Math.max(4, size * 0.12)}px ${statusColors.online}` : 'none',
          }}
        />
      )}
    </div>
  )
}
