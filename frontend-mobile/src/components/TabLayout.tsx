import { useEffect } from 'react'
import { Outlet } from 'react-router-dom'
import { TabBar } from './TabBar'
import { useWorkspaceBootstrap } from '@/hooks/useWorkspaceBootstrap'
import { useRealtimeEvents } from '@/hooks/useRealtimeEvents'
import { primeLocalNotifications } from '@/lib/native'

export function TabLayout() {
  useWorkspaceBootstrap()
  // 登录后全局订阅一次 SSE：事件 → 失效 React Query → 各页面实时刷新
  useRealtimeEvents()
  // P3.2：原生壳内初始化本地通知（权限 + 渠道 + 点击跳转）；Web 上 no-op
  useEffect(() => {
    void primeLocalNotifications()
  }, [])
  return (
    <div className="flex h-full flex-col">
      <div className="min-h-0 flex-1 overflow-y-auto">
        <Outlet />
      </div>
      <TabBar />
    </div>
  )
}
