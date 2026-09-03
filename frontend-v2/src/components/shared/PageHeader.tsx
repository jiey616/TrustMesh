import type { ReactNode } from 'react'
import { Typography } from 'antd'

const { Title, Text } = Typography

/**
 * 统一的页面头部（DESIGN.md：card-title/body，ink 标题 + ink-subtle 副标题 + 右侧操作区）
 */
export function PageHeader({
  title,
  icon,
  subtitle,
  actions,
  marginBottom = 20,
}: {
  title: string
  icon?: ReactNode
  subtitle?: string
  actions?: ReactNode
  marginBottom?: number
}) {
  return (
    <div
      style={{
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        gap: 12,
        marginBottom,
        flexWrap: 'wrap',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'baseline', gap: 10, minWidth: 0 }}>
        <Title level={4} style={{ margin: 0, color: 'var(--text-primary)' }}>
          {icon && (
            <span style={{ marginRight: 8, color: 'var(--signal)' }}>{icon}</span>
          )}
          {title}
        </Title>
        {subtitle && (
          <Text type="secondary" style={{ fontSize: 13 }}>{subtitle}</Text>
        )}
      </div>
      {actions && (
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexShrink: 0 }}>
          {actions}
        </div>
      )}
    </div>
  )
}
