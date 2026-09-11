import { Typography } from 'antd'

const { Text, Title } = Typography

interface AuthTechShellProps {
  /** 表单上方小字标签，如 WELCOME BACK */
  kicker?: string
  /** 卡片标题，如「登录」 */
  title: React.ReactNode
  subtitle?: React.ReactNode
  /** 左侧品牌区内容（AuthBrandHero） */
  brand: React.ReactNode
  /** 表单主体 */
  form: React.ReactNode
  /** 表单底部（注册/登录切换链接） */
  foot?: React.ReactNode
}

/**
 * 登录/注册页公共科技感外壳：
 * 深空背景（网格 + 光斑 + 噪点）→ 玻璃拟态卡片 → 左品牌右表单。
 */
export function AuthTechShell({ kicker, title, subtitle, brand, form, foot }: AuthTechShellProps) {
  return (
    <div className="tm-login-shell">
      {/* 全局背景层 */}
      <div className="tm-login-bg">
        <div className="tm-login-bg-grid" />
        <div className="tm-login-bg-orb tm-login-bg-orb--1" />
        <div className="tm-login-bg-orb tm-login-bg-orb--2" />
        <div className="tm-login-bg-noise" />
      </div>

      <div className="tm-login-card">
        <div className="tm-login-card-border" />

        {/* 左侧品牌区 */}
        <div className="tm-login-left">{brand}</div>

        {/* 右侧表单区 */}
        <div className="tm-login-right">
          <div className="tm-login-form-inner">
            {kicker && <div className="tm-login-form-kicker">{kicker}</div>}
            {title && (
              <Title level={2} className="tm-login-form-title">
                {title}
              </Title>
            )}
            {subtitle && <Text className="tm-login-form-sub">{subtitle}</Text>}

            {form}

            {foot}
          </div>

          {/* 底部装饰条 */}
          <div className="tm-login-right-glow" />
        </div>
      </div>
    </div>
  )
}
