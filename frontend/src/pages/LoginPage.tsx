import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useAuthStore } from '@/stores/authStore'
import * as authApi from '@/api/auth'
import { ApiRequestError } from '@/api/client'
import { toast } from 'sonner'
import agentNetworkSvg from '@/assets/agent-network.svg'
import { TrustMeshLogo } from '@/components/shared/TrustMeshLogo'
import { PlatformName } from '@/components/shared/PlatformName'
import { usePlatformStore } from '@/stores/platformStore'

export function LoginPage() {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const setAuth = useAuthStore((s) => s.setAuth)
  const platformName = usePlatformStore((s) => s.name)
  const navigate = useNavigate()

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const res = await authApi.login({ email, password })
      setAuth(res.data.access_token, res.data.refresh_token, res.data.user)
      navigate('/projects')
    } catch (err) {
      const message = err instanceof ApiRequestError ? err.message : '登录失败，请重试'
      toast.error(message)
      setError(message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex min-h-screen">
      {/* Left: Brand hero */}
      <div className="hidden lg:flex lg:w-1/2 relative overflow-hidden bg-[#0a0818] items-center justify-center">
        {/* Background gradient layers - 更丰富的渐变 */}
        <div className="absolute inset-0">
          <div className="absolute top-0 left-0 w-[700px] h-[700px] bg-[radial-gradient(circle,rgba(158,76,255,0.25),transparent_65%)]" />
          <div className="absolute bottom-0 right-0 w-[500px] h-[500px] bg-[radial-gradient(circle,rgba(232,121,249,0.15),transparent_65%)]" />
          <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[600px] h-[600px] bg-[radial-gradient(circle,rgba(192,132,252,0.1),transparent_60%)]" />
        </div>
        {/* Grid overlay */}
        <div className="absolute inset-0 bg-[linear-gradient(rgba(168,85,247,0.04)_1px,transparent_1px),linear-gradient(90deg,rgba(168,85,247,0.04)_1px,transparent_1px)] bg-[size:48px_48px]" />

        <div className="relative z-10 flex flex-col items-center px-12 max-w-lg">
          {/* Logo */}
          <TrustMeshLogo size={56} className="mb-6" platformName={platformName} />
          <PlatformName size="lg" className="mb-3" />
          <p className="text-base text-[#a1a1aa] text-center mb-10 leading-relaxed">
            多个 AI Agent 汇聚在同一工作空间，协同编排任务、驱动项目交付
          </p>

          {/* SVG illustration */}
          <img
            src={agentNetworkSvg}
            alt="Agent Network"
            className="w-full max-w-md"
          />

          {/* Bottom tagline */}
          <div className="mt-10 flex items-center gap-3 text-sm text-[#71717a]">
            <span className="inline-block size-2 rounded-full bg-[#22c55e] animate-pulse" />
            多 Agent 协作网络
          </div>
        </div>
      </div>

      {/* Right: Login form */}
      <div className="flex flex-1 items-center justify-center px-6 py-12 bg-background">
        <div className="w-full max-w-sm">
          {/* Mobile logo */}
          <div className="mb-8 flex flex-col items-center lg:hidden">
            <TrustMeshLogo size={48} className="mb-3" platformName={platformName} />
            <PlatformName size="md" />
          </div>

          <div className="mb-8">
            <h1 className="text-2xl font-bold tracking-tight">欢迎回来</h1>
            <p className="mt-2 text-sm text-muted-foreground">
              登录你的账号以继续
            </p>
          </div>

          <form onSubmit={handleSubmit} className="flex flex-col gap-5">
            <div className="flex flex-col gap-1.5">
              <label className="text-sm font-medium">邮箱</label>
              <Input
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="you@example.com"
                required
                autoFocus
                className="h-11"
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <label className="text-sm font-medium">密码</label>
              <Input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="输入密码"
                required
                className="h-11"
              />
            </div>
            <Button type="submit" className="w-full h-11 mt-1" disabled={loading}>
              {loading ? '登录中...' : '登录'}
            </Button>
          </form>

          <p className="mt-6 text-center text-sm text-muted-foreground">
            还没有账号？{' '}
            <Link to="/register" className="text-primary hover:underline font-medium">
              注册
            </Link>
          </p>
        </div>
      </div>
    </div>
  )
}
