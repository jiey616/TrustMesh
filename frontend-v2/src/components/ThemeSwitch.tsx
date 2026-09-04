import { Tooltip } from 'antd'
import { BulbOutlined, MoonOutlined } from '@ant-design/icons'
import { useTheme } from '../theme/ThemeProvider'

/**
 * 主题切换器。两套皮肤：深色 console（TrustMesh）↔ 近白（Quiet Signal）。
 * 选择持久化在 localStorage，刷新后保持。
 * - iconOnly：仅图标紧凑按钮（用于侧边栏底部与「收起」并排）。
 * - 默认：带边框/背景的完整条目（兼容旧调用）。
 */
export function ThemeSwitch({
  collapsed = false,
  iconOnly = false,
}: {
  collapsed?: boolean
  iconOnly?: boolean
}) {
  const { theme, toggleTheme } = useTheme()
  const isDark = theme === 'dark'
  const label = isDark ? '切换到近白主题' : '切换到深色主题'

  if (iconOnly) {
    return (
      <Tooltip title={label} placement="right">
        <div
          role="button"
          tabIndex={0}
          aria-label={label}
          onClick={toggleTheme}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault()
              toggleTheme()
            }
          }}
          className="tm-iconbtn"
        >
          {isDark ? <MoonOutlined style={{ fontSize: 15 }} /> : <BulbOutlined style={{ fontSize: 15 }} />}
        </div>
      </Tooltip>
    )
  }

  return (
    <Tooltip title={collapsed ? label : ''} placement="right">
      <div
        role="button"
        tabIndex={0}
        aria-label={label}
        onClick={toggleTheme}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            toggleTheme()
          }
        }}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 10,
          padding: '7px 10px',
          margin: '0 8px',
          borderRadius: 'var(--radius-control)',
          cursor: 'pointer',
          fontSize: 13,
          fontWeight: 400,
          color: 'var(--text-tertiary)',
          border: '1px solid var(--line)',
          background: 'var(--surface-sunken)',
          transition: 'color 0.18s ease, border-color 0.18s ease, background 0.18s ease',
          userSelect: 'none',
        }}
        onMouseEnter={(e) => {
          e.currentTarget.style.color = 'var(--text-primary)'
          e.currentTarget.style.borderColor = 'var(--line-strong)'
          e.currentTarget.style.background = 'var(--surface)'
        }}
        onMouseLeave={(e) => {
          e.currentTarget.style.color = 'var(--text-tertiary)'
          e.currentTarget.style.borderColor = 'var(--line)'
          e.currentTarget.style.background = 'var(--surface-sunken)'
        }}
      >
        {isDark ? <MoonOutlined style={{ fontSize: 14 }} /> : <BulbOutlined style={{ fontSize: 14 }} />}
        {!collapsed && (
          <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {isDark ? '深色主题' : '近白主题'}
          </span>
        )}
      </div>
    </Tooltip>
  )
}
