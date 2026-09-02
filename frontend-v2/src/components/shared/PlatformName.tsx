import { Typography } from 'antd'
import { usePlatformStore } from '@/stores/platformStore'

interface PlatformNameProps {
  size?: 'sm' | 'md' | 'lg'
  className?: string
  style?: React.CSSProperties
}

const sizeMap: Record<string, { fontSize: number; fontWeight: number }> = {
  sm: { fontSize: 18, fontWeight: 600 },
  md: { fontSize: 24, fontWeight: 700 },
  lg: { fontSize: 36, fontWeight: 700 },
}

export function PlatformName({ size = 'md', className, style }: PlatformNameProps) {
  const platformName = usePlatformStore((s) => s.name)
  const displayName = platformName || 'TrustMesh'
  const sz = sizeMap[size]
  return (
    <Typography.Text
      className={className}
      style={{
        fontSize: sz.fontSize,
        fontWeight: sz.fontWeight,
        color: '#fff',
        letterSpacing: '-0.5px',
        ...style,
      }}
    >
      {displayName}
    </Typography.Text>
  )
}
