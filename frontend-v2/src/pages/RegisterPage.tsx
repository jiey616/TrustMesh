import { useState } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { Form, Input, Button, Typography, App } from 'antd'
import { MailOutlined, LockOutlined, UserOutlined } from '@ant-design/icons'
import { useMutation } from '@tanstack/react-query'
import { register } from '@/api/auth'
import { useAuthStore } from '@/stores/authStore'
import { ApiRequestError } from '@/types'
import { PlatformName } from '@/components/shared/PlatformName'
import { TrustMeshLogo } from '@/components/shared/TrustMeshLogo'
import { usePlatformStore } from '@/stores/platformStore'
import type { AuthRegisterRequest, AuthSuccessData } from '@/types'
import agentNetworkSvg from '@/assets/agent-network.svg'

const { Text, Title } = Typography
const MIN_PASSWORD_LENGTH = 8

function getRegisterErrorMessage(err: unknown): string {
  if (err instanceof ApiRequestError) {
    if (err.code === 'VALIDATION_ERROR' && typeof err.details?.password === 'string') {
      return `密码${err.details.password}`
    }
    return err.message
  }
  return '注册失败，请稍后重试'
}

function BrandHero() {
  const platformName = usePlatformStore((s) => s.name)
  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', padding: '0 40px', position: 'relative' }}>
      <div style={{ position: 'absolute', inset: 0, background: 'radial-gradient(ellipse at top left, rgba(109,95,245,0.12), transparent 60%), radial-gradient(ellipse at bottom right, rgba(99,102,241,0.1), transparent 60%)', pointerEvents: 'none' }} />
      <TrustMeshLogo size={56} style={{ marginBottom: 16 }} platformName={platformName} />
      <div style={{ marginBottom: 12 }}>
        <PlatformName size="lg" />
      </div>
      <Text style={{ color: 'var(--text-tertiary)', fontSize: 16, marginBottom: 32 }}>多个 AI Agent 汇聚在同一工作空间，协同编排任务、驱动项目交付</Text>
      <img src={agentNetworkSvg} alt="数字员工 Network" style={{ width: '100%', maxWidth: 420 }} />
      <div style={{ marginTop: 32, display: 'flex', alignItems: 'center', gap: 8, fontSize: 13, color: 'var(--text-tertiary)' }}>
        <span style={{ display: 'inline-block', width: 8, height: 8, borderRadius: 'var(--radius-avatar)', background: 'var(--success)', animation: 'pulse 2s infinite' }} />
        多 Agent 协作网络
      </div>
    </div>
  )
}

export function RegisterPage() {
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()
  const setAuth = useAuthStore((s) => s.setAuth)
  const { message } = App.useApp()

  const registerMutation = useMutation({
    mutationFn: (data: AuthRegisterRequest) => register(data),
    onSuccess: (data: AuthSuccessData) => {
      setAuth(data.access_token, data.refresh_token, data.user)
      message.success('注册成功！')
      navigate('/projects')
    },
    onError: (err: unknown) => {
      message.error(getRegisterErrorMessage(err))
      setLoading(false)
    },
  })

  const handleSubmit = (values: AuthRegisterRequest) => {
    if (values.password.length < MIN_PASSWORD_LENGTH) {
      message.error(`密码至少需要 ${MIN_PASSWORD_LENGTH} 位`)
      return
    }
    setLoading(true)
    registerMutation.mutate(values)
  }

  return (
    <div style={{ display: 'flex', width: '100%', maxWidth: 1100, minHeight: '80vh', background: 'var(--surface)', backdropFilter: 'var(--glass-blur)', borderRadius: 'var(--radius-structure)', border: '1px solid var(--line)', boxShadow: '0 8px 32px rgba(0,0,0,0.3), 0 0 40px rgba(109,95,245,0.05)', overflow: 'hidden' }}>
      {/* Left: Brand hero */}
      <div className="auth-brand-hero" style={{ flex: 1, display: 'flex' }}>
        <BrandHero />
      </div>

      {/* Right: Register form */}
      <div style={{ width: 420, padding: '48px 40px', display: 'flex', flexDirection: 'column', justifyContent: 'center', borderLeft: '1px solid var(--line)' }}>
        <div style={{ textAlign: 'center', marginBottom: 24 }}>
          <Title level={3} style={{ color: 'var(--text-primary)', marginBottom: 8 }}>创建账号</Title>
          <Text type="secondary">注册 TrustMesh 开始 AI 驱动的协作</Text>
        </div>
        <Form size="large" onFinish={handleSubmit} layout="vertical" requiredMark={false}>
          <Form.Item name="name" rules={[{ required: true, message: '请输入用户名' }]}>
            <Input prefix={<UserOutlined />} placeholder="用户名" />
          </Form.Item>
          <Form.Item name="email" rules={[{ required: true, message: '请输入邮箱地址' }, { type: 'email', message: '请输入有效的邮箱地址' }]}>
            <Input prefix={<MailOutlined />} placeholder="邮箱" />
          </Form.Item>
          <Form.Item
            name="password"
            rules={[
              { required: true, message: '请输入密码' },
              { min: MIN_PASSWORD_LENGTH, message: `密码至少${MIN_PASSWORD_LENGTH}位` },
            ]}
          >
            <Input.Password prefix={<LockOutlined />} placeholder={`密码（至少 ${MIN_PASSWORD_LENGTH} 位）`} />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" loading={loading} block>
              注册
            </Button>
          </Form.Item>
        </Form>
        <div style={{ textAlign: 'center' }}>
          <Text type="secondary">
            已有账号？<Link to="/login">去登录</Link>
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