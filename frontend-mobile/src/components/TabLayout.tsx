import { Outlet } from 'react-router-dom'
import { TabBar } from './TabBar'
import { useWorkspaceBootstrap } from '@/hooks/useWorkspaceBootstrap'
import { useRealtimeEvents } from '@/hooks/useRealtimeEvents'

export function TabLayout() {
  useWorkspaceBootstrap()
  // 登录后全局订阅一次 SSE：事件 → 失效 React Query → 各页面实时刷新
  useRealtimeEvents()
  return (
    <div className="flex h-full flex-col">
      <div className="min-h-0 flex-1 overflow-y-auto">
        <Outlet />
      </div>
      <TabBar />
    </div>
  )
}
