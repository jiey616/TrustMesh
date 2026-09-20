import type { ReactNode } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useUnreadCount } from '@/hooks/useNotifications'
import { useSettingsStore } from '@/stores/settingsStore'

interface TabItem {
  key: string
  path: string
  label: string
  icon: ReactNode
}

// 不引额外图标库（移动端要控体积），四个图标用内联 SVG 画。
const icons = {
  home: (
    <svg viewBox="0 0 24 24" width="25" height="25" fill="none" stroke="currentColor" strokeWidth="1.8">
      <path d="M4 10.5 12 4l8 6.5V20a1 1 0 0 1-1 1h-4v-6H9v6H5a1 1 0 0 1-1-1z" strokeLinejoin="round" />
    </svg>
  ),
  tasks: (
    <svg viewBox="0 0 24 24" width="25" height="25" fill="none" stroke="currentColor" strokeWidth="1.8">
      <path d="M9 6h11M9 12h11M9 18h11" strokeLinecap="round" />
      <path d="m3.5 6 1.4 1.4L7.5 5M3.5 12l1.4 1.4L7.5 11M3.5 18l1.4 1.4L7.5 17" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  ),
  inbox: (
    <svg viewBox="0 0 24 24" width="25" height="25" fill="none" stroke="currentColor" strokeWidth="1.8">
      <path d="M4 13h4l1.5 2.5h5L16 13h4" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M4 13 6.5 5h11L20 13v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1z" strokeLinejoin="round" />
    </svg>
  ),
  me: (
    <svg viewBox="0 0 24 24" width="25" height="25" fill="none" stroke="currentColor" strokeWidth="1.8">
      <circle cx="12" cy="8.5" r="3.6" />
      <path d="M4.5 20c1.4-3.6 4-5.4 7.5-5.4S18.1 16.4 19.5 20" strokeLinecap="round" />
    </svg>
  ),
}

const TABS: TabItem[] = [
  { key: 'home', path: '/', label: '工作台', icon: icons.home },
  { key: 'tasks', path: '/tasks', label: '任务', icon: icons.tasks },
  { key: 'inbox', path: '/inbox', label: '收件箱', icon: icons.inbox },
  { key: 'me', path: '/me', label: '我的', icon: icons.me },
]

export function TabBar() {
  const navigate = useNavigate()
  const { pathname } = useLocation()
  // TabBar 常驻在 TabLayout 里，未读轮询挂这里即可全局生效（间隔由本地设置控制）
  const { data: unread = 0 } = useUnreadCount()
  const showBadge = useSettingsStore((s) => s.showUnreadBadge)

  return (
    <nav
      className="tm-safe-bottom shrink-0 border-t border-[var(--tm-line)] bg-white"
      role="tablist"
      aria-label="主导航"
    >
      <div className="flex">
        {TABS.map((tab) => {
          const active = pathname === tab.path
          return (
            <button
              key={tab.key}
              type="button"
              role="tab"
              aria-selected={active}
              onClick={() => navigate(tab.path)}
              className="relative flex flex-1 flex-col items-center justify-center gap-[4px] py-[10px]"
              style={{ color: active ? 'var(--tm-brand)' : 'var(--tm-text-3)' }}
            >
              <span className="flex h-[25px] items-center">
                {tab.icon}
                {tab.key === 'inbox' && showBadge && unread > 0 ? (
                  <span
                    className="ml-[2px] min-w-[17px] rounded-full px-[4px] text-center text-[10px] leading-[17px] text-white"
                    style={{ background: 'var(--tm-danger)' }}
                  >
                    {unread > 99 ? '99+' : unread}
                  </span>
                ) : null}
              </span>
              <span className="text-[12px] leading-none">{tab.label}</span>
            </button>
          )
        })}
      </div>
    </nav>
  )
}
