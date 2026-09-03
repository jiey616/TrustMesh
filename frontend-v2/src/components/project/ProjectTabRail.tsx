import { useState, type ReactNode } from 'react'
import { Tooltip } from 'antd'
import { MenuFoldOutlined, MenuUnfoldOutlined } from '@ant-design/icons'

export interface ProjectTabItem {
  key: string
  label: string
  icon: ReactNode
}

interface ProjectTabRailProps {
  tabs: ProjectTabItem[]
  activeKey: string
  onChange: (key: string) => void
}

/**
 * 项目详情页的竖向 tab 条，贴在内容区右侧。
 *
 * 两种形态：
 * - 折叠态（默认）：48px 窄条，只显示图标，常驻在内容区预留的留白里，不遮挡内容。
 * - 展开态：184px 浮层，图标 + 文字，向左覆盖在内容之上（不挤压布局）。
 *
 * 展开时会盖一层透明遮罩，点击任意空白处收起 —— 保证同一时刻只有浮层可交互，
 * 避免"点内容时先收起再触发点击"的那种别扭手感。
 */
export function ProjectTabRail({ tabs, activeKey, onChange }: ProjectTabRailProps) {
  const [expanded, setExpanded] = useState(false)
  const [hovered, setHovered] = useState<string | null>(null)

  const rowBase: React.CSSProperties = {
    display: 'flex',
    alignItems: 'center',
    gap: 8,
    height: 36,
    padding: 0,
    border: 'none',
    borderRadius: 'var(--radius-control)',
    cursor: 'pointer',
    fontFamily: 'inherit',
    fontSize: 13,
    transition: 'background 0.15s ease, color 0.15s ease',
    background: 'transparent',
    color: 'var(--text-secondary)',
  }

  const renderRow = (key: string, label: string, icon: ReactNode, active: boolean) => (
    <button
      type="button"
      onClick={() => {
        onChange(key)
        setExpanded(false)
      }}
      onMouseEnter={() => setHovered(key)}
      onMouseLeave={() => setHovered((h) => (h === key ? null : h))}
      style={{
        ...rowBase,
        width: expanded ? '100%' : 36,
        justifyContent: expanded ? 'flex-start' : 'center',
        paddingLeft: expanded ? 4 : 0,
        background: active
          ? 'rgba(109,95,245,0.2)'
          : hovered === key
            ? 'rgba(255,255,255,0.06)'
            : 'transparent',
        color: active ? '#a78bfa' : 'rgba(255,255,255,0.6)',
        fontWeight: active ? 600 : 400,
      }}
    >
      <span
        style={{
          width: 28,
          display: 'inline-flex',
          justifyContent: 'center',
          alignItems: 'center',
          flexShrink: 0,
          fontSize: 14,
        }}
      >
        {icon}
      </span>
      {expanded && (
        <span
          style={{
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            textAlign: 'left',
          }}
        >
          {label}
        </span>
      )}
    </button>
  )

  return (
    <>
      {expanded && (
        <div
          onClick={() => setExpanded(false)}
          style={{ position: 'absolute', inset: 0, zIndex: 25 }}
        />
      )}

      <div
        style={{
          position: 'absolute',
          right: 16,
          top: 12,
          zIndex: 30,
          width: expanded ? 184 : 48,
          maxHeight: 'calc(100% - 24px)',
          overflowY: 'auto',
          display: 'flex',
          flexDirection: 'column',
          gap: 2,
          padding: 6,
          background: 'var(--canvas-elevated)',
          border: '1px solid var(--line)',
          borderRadius: 'var(--radius-structure)',
          backdropFilter: 'var(--glass-blur)',
          boxShadow: expanded ? 'var(--shadow-float)' : 'var(--shadow-card)',
          transition: 'width 0.18s ease',
        }}
      >
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          onMouseEnter={() => setHovered('__toggle')}
          onMouseLeave={() => setHovered((h) => (h === '__toggle' ? null : h))}
          style={{
            ...rowBase,
            width: expanded ? '100%' : 36,
            justifyContent: expanded ? 'flex-start' : 'center',
            paddingLeft: expanded ? 4 : 0,
            color: 'var(--text-tertiary)',
            background: hovered === '__toggle' ? 'rgba(255,255,255,0.06)' : 'transparent',
          }}
          title={expanded ? '收起' : '展开'}
        >
          <span
            style={{
              width: 28,
              display: 'inline-flex',
              justifyContent: 'center',
              alignItems: 'center',
              flexShrink: 0,
              fontSize: 13,
            }}
          >
            {expanded ? <MenuFoldOutlined /> : <MenuUnfoldOutlined />}
          </span>
          {expanded && <span style={{ fontSize: 12 }}>收起</span>}
        </button>

        <div style={{ height: 1, background: 'var(--surface-raised)', margin: '3px 0' }} />

        {tabs.map((t) => {
          const active = t.key === activeKey
          return (
            <div key={t.key}>
              {expanded ? (
                renderRow(t.key, t.label, t.icon, active)
              ) : (
                <Tooltip title={t.label} placement="left">
                  {renderRow(t.key, t.label, t.icon, active)}
                </Tooltip>
              )}
            </div>
          )
        })}
      </div>
    </>
  )
}
