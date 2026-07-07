import { cn } from '@/lib/utils'
import logoSvg from '@/assets/logo.svg'

interface TrustMeshLogoProps {
  size?: number
  className?: string
  platformName?: string
}

export function TrustMeshLogo({ size = 24, className, platformName = 'TrustMesh' }: TrustMeshLogoProps) {
  return (
    <img
      src={logoSvg}
      width={size}
      height={size}
      alt={platformName}
      className={cn('shrink-0', className)}
    />
  )
}
