import { useState } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { Form, Input, Button, Typography, App } from 'antd'
import { MailOutlined, LockOutlined, ArrowRightOutlined } from '@ant-design/icons'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { login } from '@/api/auth'
import { useAuthStore } from '@/stores/authStore'
import { ApiRequestError } from '@/types'
import type { AuthLoginRequest, AuthSuccessData } from '@/types'
import { AuthBrandHero } from '@/components/login/AuthBrandHero'
import { AuthTechShell } from '@/components/login/AuthTechShell'
import { AUTH_TECH_CSS } from '@/pages/authTechCss'

const { Text } = Typography

export function LoginPage() {
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()
  const setAuth = useAuthStore((s) => s.setAuth)
  const queryClient = useQueryClient()
  const { message } = App.useApp()

  const loginMutation = useMutation({
    mutationFn: (data: AuthLoginRequest) => login(data),
    onSuccess: (data: AuthSuccessData) => {
      // 换账号登录防御：清掉上一个账号残留在模块级 QueryClient 里的缓存
      //（orgs/项目/通知等），避免设置页展示旧账号数据。
      queryClient.clear()
      setAuth(data.access_token, data.refresh_token, data.user)
      message.success(`欢迎回来，${data.user.name}`)
      // 登录后统一进首页（不回退出前页面）；replace 把 /login 顶出历史栈
      navigate('/dashboard', { replace: true })
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

  const form = (
    <Form size="large" onFinish={handleSubmit} layout="vertical" requiredMark={false} className="tm-login-form">
      <Form.Item
        name="email"
        rules={[
          { required: true, message: '请输入邮箱地址' },
          { type: 'email', message: '请输入有效的邮箱地址' },
        ]}
      >
        <Input prefix={<MailOutlined />} placeholder="邮箱" autoComplete="email" className="tm-login-input" />
      </Form.Item>

      <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
        <Input.Password
          prefix={<LockOutlined />}
          placeholder="密码"
          autoComplete="current-password"
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
          icon={<ArrowRightOutlined />}
        >
          进入工作空间
        </Button>
      </Form.Item>
    </Form>
  )

  const foot = (
    <div className="tm-login-form-foot">
      <Text type="secondary" style={{ fontSize: 13 }}>
        还没有账号？
      </Text>
      <Link to="/register" className="tm-login-link">
        立即注册
      </Link>
    </div>
  )

  return (
    <>
      <AuthTechShell
        kicker="WELCOME BACK"
        title="登录"
        subtitle="连接你的工作空间，继续未完成的协作"
        brand={
          <AuthBrandHero
            title={
              <>
                让 AI Agent
                <br />
                成为你的团队
              </>
            }
            subtitle={
              <>
                多个数字员工汇聚在同一工作空间，
                <br />
                自主拆解任务、协同执行、驱动项目交付。
              </>
            }
          />
        }
        form={form}
        foot={foot}
      />
      <style>{AUTH_TECH_CSS}</style>
    </>
  )
}
