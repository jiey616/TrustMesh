import { Typography } from 'antd'
import { PlatformName } from '@/components/shared/PlatformName'
import { TrustMeshLogo } from '@/components/shared/TrustMeshLogo'
import { usePlatformStore } from '@/stores/platformStore'
import { LoginParticles3D } from '@/components/login/LoginParticles3D'

const { Text, Title } = Typography

interface AuthBrandHeroProps {
  title: React.ReactNode
  subtitle?: React.ReactNode
  /** 3D 粒子球容器最大宽度（注册页表单较长时用更小值） */
  canvasMaxWidth?: number
}

/** 登录/注册页左侧科技感品牌区：渐变标题 + 3D 粒子网络球体 */
export function AuthBrandHero({ title, subtitle, canvasMaxWidth = 520 }: AuthBrandHeroProps) {
  const platformName = usePlatformStore((s) => s.name)
  return (
    <div className="tm-login-hero">
      {/* 背景网格 + 光晕 */}
      <div className="tm-login-hero-grid" />
      <div className="tm-login-hero-glow tm-login-hero-glow--a" />
      <div className="tm-login-hero-glow tm-login-hero-glow--b" />
      {/* 扫描线 */}
      <div className="tm-login-hero-scanline" />

      <div className="tm-login-hero-content">
        <div className="tm-login-logo-row">
          <TrustMeshLogo size={44} platformName={platformName} />
          <PlatformName size="lg" />
        </div>

        <Title level={1} className="tm-login-hero-title">
          {title}
        </Title>

        {subtitle && <Text className="tm-login-hero-sub">{subtitle}</Text>}

        {/* 3D 粒子网络球体 + 脉冲环 */}
        <div className="tm-login-network-wrap">
          <div className="tm-login-network-ring tm-login-network-ring--1" />
          <div className="tm-login-network-ring tm-login-network-ring--2" />
          <div className="tm-login-network-ring tm-login-network-ring--3" />
          <div className="tm-login-3d-canvas" style={{ maxWidth: canvasMaxWidth }}>
            <LoginParticles3D />
          </div>
        </div>

        {/* 底部状态点 */}
        <div className="tm-login-hero-status">
          <span className="tm-login-pulse-dot" />
          <span>多 Agent 实时协作网络</span>
          <span className="tm-login-hero-status-sep" />
          <span>Task Orchestration</span>
          <span className="tm-login-hero-status-sep" />
          <span>ClawSynapse</span>
        </div>
      </div>
    </div>
  )
}
