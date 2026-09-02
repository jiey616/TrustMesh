import logoSvg from '@/assets/logo.svg'

interface TrustMeshLogoProps {
  size?: number
  className?: string
  style?: React.CSSProperties
  platformName?: string
}

export function TrustMeshLogo({ size = 24, className, style, platformName = 'TrustMesh' }: TrustMeshLogoProps) {
  return <img src={logoSvg} width={size} height={size} alt={platformName} className={className} style={style} />
}
