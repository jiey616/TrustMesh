import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Input, Toast } from 'antd-mobile'
import { login } from '@/api/auth'
import { useAuthStore } from '@/stores/authStore'
import { apiBaseLabel } from '@/lib/apiBaseLabel'

/**
 * 登录页：1:1 复刻 frontend-v2 登录页（AuthTechShell 近白主题窄屏形态）。
 * 视觉 token 对齐 web 端 theme/tokens.ts 的 quiet 主题：
 *   signal #7C3AED ｜ surfaceInset #F4F4F5 ｜ line rgba(10,10,10,0.10)
 * 刻意不用 antd v5 / three.js（组件库冲突 + bundle 爆炸），仅复刻观感。
 */

function MailIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <rect x="2" y="4" width="20" height="16" rx="2" />
      <path d="m22 7-10 6L2 7" />
    </svg>
  )
}

function LockIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <rect x="3" y="11" width="18" height="11" rx="2" />
      <path d="M7 11V7a5 5 0 0 1 10 0v4" />
    </svg>
  )
}

function ArrowRightIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M5 12h14" />
      <path d="m12 5 7 7-7 7" />
    </svg>
  )
}

export function LoginPage() {
  const navigate = useNavigate()
  const setTokens = useAuthStore((s) => s.setTokens)
  const setUser = useAuthStore((s) => s.setUser)

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)

  async function submit() {
    if (!email.trim() || !password) {
      Toast.show({ content: '请输入邮箱和密码' })
      return
    }
    setSubmitting(true)
    try {
      const data = await login({ email: email.trim(), password })
      setTokens(data.access_token, data.refresh_token)
      setUser(data.user)
      navigate('/', { replace: true })
    } catch (err) {
      const message = err instanceof Error ? err.message : '登录失败'
      Toast.show({ content: message })
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="tm-safe-top flex min-h-full flex-col justify-center px-6">
      <div className="font-mono text-[11px] tracking-[3px] text-[#7C3AED]">WELCOME BACK</div>
      <h1 className="mt-3 text-[30px] font-semibold leading-tight tracking-[-0.5px] text-[var(--tm-text-1)]">
        登录
      </h1>
      <p className="mt-2 text-[14px] leading-relaxed text-[var(--tm-text-2)]">
        连接你的工作空间，继续未完成的协作
      </p>

      <div className="mt-8 space-y-4">
        <div className="flex h-[48px] items-center rounded-[10px] border border-[rgba(10,10,10,0.10)] bg-[#F4F4F5] px-4 transition-colors focus-within:border-[#7C3AED] focus-within:ring-[3px] focus-within:ring-[rgba(124,58,237,0.10)]">
          <span className="mr-2.5 shrink-0 text-[var(--tm-text-3)]">
            <MailIcon />
          </span>
          <Input
            placeholder="邮箱"
            value={email}
            onChange={setEmail}
            type="email"
            autoComplete="username"
            className="w-full flex-1"
          />
        </div>
        <div className="flex h-[48px] items-center rounded-[10px] border border-[rgba(10,10,10,0.10)] bg-[#F4F4F5] px-4 transition-colors focus-within:border-[#7C3AED] focus-within:ring-[3px] focus-within:ring-[rgba(124,58,237,0.10)]">
          <span className="mr-2.5 shrink-0 text-[var(--tm-text-3)]">
            <LockIcon />
          </span>
          <Input
            placeholder="密码"
            value={password}
            onChange={setPassword}
            type="password"
            autoComplete="current-password"
            className="w-full flex-1"
          />
        </div>
      </div>

      <Button
        block
        size="large"
        loading={submitting}
        onClick={submit}
        style={{
          marginTop: 24,
          height: 48,
          borderRadius: 10,
          border: 'none',
          background: '#7C3AED',
          fontSize: 15,
          fontWeight: 600,
          letterSpacing: 0.5,
          '--background-color': '#7C3AED',
          '--text-color': '#ffffff',
          '--border-color': '#7C3AED',
        } as React.CSSProperties}
      >
        <span className="flex items-center justify-center gap-1.5">
          进入工作空间
          <ArrowRightIcon />
        </span>
      </Button>

      <p className="tm-safe-bottom mt-8 text-center font-mono text-[12px] text-[var(--tm-text-3)]">
        当前服务：{apiBaseLabel()}
      </p>
    </div>
  )
}
