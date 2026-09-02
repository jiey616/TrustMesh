import { cn } from '@/lib/utils'
import { usePlatformStore } from '@/stores/platformStore'

interface PlatformNameProps {
  /** Text size variant: sm, md, lg */
  size?: 'sm' | 'md' | 'lg'
  className?: string
}

const lineSizes: Record<string, string> = {
  sm: 'text-base',
  md: 'text-lg',
  lg: 'text-2xl',
}

export function PlatformName({ size = 'md', className }: PlatformNameProps) {
  const platformName = usePlatformStore((s) => s.name)
  const displayName = platformName || 'TrustMesh'

  return (
    <span className={cn('inline-flex flex-col items-center leading-tight', className)}>
      <span className={cn('font-bold tracking-wide', lineSizes[size])}>{displayName}</span>
    </span>
  )
}
