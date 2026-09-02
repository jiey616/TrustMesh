import { useState } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { Form, Input, Button, Typography, App } from 'antd'
import { MailOutlined, LockOutlined } from '@ant-design/icons'
import { useMutation } from '@tanstack/react-query'
import { login } from '@/api/auth'
import { useAuthStore } from '@/stores/authStore'
import { ApiRequestError } from '@/types'
import { PlatformName } from '@/components/shared/PlatformName'
import { TrustMeshLogo } from '@/components/shared/TrustMeshLogo'
import { usePlatformStore } from '@/stores/platformStore'
import type { AuthLoginRequest, AuthSuccessData } from '@/types'
import agentNetworkSvg from '@/assets/agent-network.svg'

const { Text, Title } = Typography

function BrandHero() {
  const platformName = usePlatformStore((s) => s.name)
  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', padding: '0 40px', position: 'relative' }}>
      <div style={{ position: 'absolute', inset: 0, background: 'radial-gradient(ellipse at top left, rgba(109,95,245,0.12), transparent 60%), radial-gradient(ellipse at bottom right, rgba(99,102,241,0.1), transparent 60%)', pointerEvents: 'none' }} />
      <TrustMeshLogo size={56} style={{ marginBottom: 16 }} platformName={platformName} />
      <div style={{ marginBottom: 12 }}>
        <PlatformName size="lg" />
      </div>
      <Text style={{ color: 'rgba(255,255,255,0.45)', fontSize: 16, marginBottom: 32 }}>多个 AI Agent 汇聚在同一工作空间，协同编排任务、驱动项目交付</Text>
      <img src={agentNetworkSvg} alt="数字员工 Network" style={{ width: '100%', maxWidth: 420 }} />
      <div style={{ marginTop: 32, display: 'flex', alignItems: 'center', gap: 8, fontSize: 13, color: 'rgba(255,255,255,0.5)' }}>
        <span style={{ display: 'inline-block', width: 8, height: 8, borderRadius: '50%', background: '#22c55e', animation: 'pulse 2s infinite' }} />
        多 Agent 协作网络
      </div>
    </div>
  )
}

export function LoginPage() {
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()
  const setAuth = useAuthStore((s) => s.setAuth)
  const { message } = App.useApp()

  const loginMutation = useMutation({
    mutationFn: (data: AuthLoginRequest) => login(data),
    onSuccess: (data: AuthSuccessData) => {
      setAuth(data.access_token, data.refresh_token, data.user)
      message.success(`欢迎回来，${data.user.name}`)
      navigate('/projects')
    },
    onError: (err: unknown) => {
      const msg = err instanceof ApiRequestError ? err.message : '登录失败，请检查邮箱和密码'
      message.error(msg)
      setLoading(false)
    },
  })

  const handleSubmit = (values: AuthLoginRequest) => {
    setLoading(true)
    loginMutation.mutate(values)
  }

  return (
    <div style={{ display: 'flex', width: '100%', maxWidth: 1100, minHeight: '80vh', background: 'rgba(255,255,255,0.03)', backdropFilter: 'blur(40px)', borderRadius: 24, border: '1px solid rgba(255,255,255,0.08)', boxShadow: '0 8px 32px rgba(0,0,0,0.3), 0 0 40px rgba(109,95,245,0.05)', overflow: 'hidden' }}>
      {/* Left: Brand hero */}
      <div className="auth-brand-hero" style={{ flex: 1, display: 'flex' }}>
        <BrandHero />
      </div>

      {/* Right: Login form */}
      <div style={{ width: 420, padding: '48px 40px', display: 'flex', flexDirection: 'column', justifyContent: 'center', borderLeft: '1px solid rgba(255,255,255,0.06)' }}>
        <div style={{ textAlign: 'center', marginBottom: 24 }}>
          <Title level={3} style={{ color: '#fff', marginBottom: 8 }}>欢迎回来</Title>
          <Text type="secondary">登录你的账号以继续</Text>
        </div>
        <Form size="large" onFinish={handleSubmit} layout="vertical" requiredMark={false}>
          <Form.Item name="email" rules={[{ required: true, message: '请输入邮箱地址' }, { type: 'email', message: '请输入有效的邮箱地址' }]}>
            <Input prefix={<MailOutlined />} placeholder="邮箱" />
          </Form.Item>
          <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password prefix={<LockOutlined />} placeholder="密码" />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" loading={loading} block>
              登录
            </Button>
          </Form.Item>
        </Form>
        <div style={{ textAlign: 'center' }}>
          <Text type="secondary">
            还没有账号？<Link to="/register">立即注册</Link>
          </Text>
        </div>
      </div>

      <style>{`
        @media (max-width: 900px) {
          .auth-brand-hero { display: none !important; }
        }
      `}</style>
    </div>
  )
}