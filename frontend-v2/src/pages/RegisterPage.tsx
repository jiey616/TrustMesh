import { useState } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { Form, Input, Button, Typography, App } from 'antd'
import { MailOutlined, LockOutlined, UserOutlined, RocketOutlined } from '@ant-design/icons'
import { useMutation } from '@tanstack/react-query'
import { register } from '@/api/auth'
import { useAuthStore } from '@/stores/authStore'
import { ApiRequestError } from '@/types'
import type { AuthRegisterRequest, AuthSuccessData } from '@/types'
import { AuthBrandHero } from '@/components/login/AuthBrandHero'
import { AuthTechShell } from '@/components/login/AuthTechShell'
import { AUTH_TECH_CSS } from '@/pages/authTechCss'

const { Text } = Typography
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
      // 与 LoginPage 同语义：注册后统一进首页；replace 顶掉 /register
      navigate('/dashboard', { replace: true })
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

  const form = (
    <Form size="large" onFinish={handleSubmit} layout="vertical" requiredMark={false} className="tm-login-form">
      <Form.Item name="name" rules={[{ required: true, message: '请输入用户名' }]}>
        <Input prefix={<UserOutlined />} placeholder="用户名" autoComplete="nickname" className="tm-login-input" />
      </Form.Item>

      <Form.Item
        name="email"
        rules={[
          { required: true, message: '请输入邮箱地址' },
          { type: 'email', message: '请输入有效的邮箱地址' },
        ]}
      >
        <Input prefix={<MailOutlined />} placeholder="邮箱" autoComplete="email" className="tm-login-input" />
      </Form.Item>

      <Form.Item
        name="password"
        rules={[
          { required: true, message: '请输入密码' },
          { min: MIN_PASSWORD_LENGTH, message: `密码至少${MIN_PASSWORD_LENGTH}位` },
        ]}
      >
        <Input.Password
          prefix={<LockOutlined />}
          placeholder={`密码（至少 ${MIN_PASSWORD_LENGTH} 位）`}
          autoComplete="new-password"
          className="tm-login-input"
        />
      </Form.Item>

      <Form.Item style={{ marginBottom: 12 }}>
        <Button
          type="primary"
          htmlType="submit"
          loading={loading}
          block
          className="tm-login-submit"
          iconPosition="end"
          icon={<RocketOutlined />}
        >
          创建账号并开始
        </Button>
      </Form.Item>
    </Form>
  )

  const foot = (
    <div className="tm-login-form-foot">
      <Text type="secondary" style={{ fontSize: 13 }}>
        已有账号？
      </Text>
      <Link to="/login" className="tm-login-link">
        去登录
      </Link>
    </div>
  )

  return (
    <>
      <AuthTechShell
        kicker="CREATE ACCOUNT"
        title="创建账号"
        subtitle="加入 TrustMesh，开启 AI 驱动的团队协作"
        brand={
          <AuthBrandHero
            title={
              <>
                组建你的
                <br />
                AI 数字员工团队
              </>
            }
            subtitle={
              <>
                一个工作空间管理多个 AI Agent，
                <br />
                从规划到交付全程自主协同。
              </>
            }
            canvasMaxWidth={430}
          />
        }
        form={form}
        foot={foot}
      />
      <style>{AUTH_TECH_CSS}</style>
    </>
  )
}
