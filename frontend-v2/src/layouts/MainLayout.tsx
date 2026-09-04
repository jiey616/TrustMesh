import { useMemo } from 'react'
import { Outlet, useNavigate, useLocation } from 'react-router-dom'
import { Layout, Menu, Avatar, Dropdown, Badge, Tooltip, Tag } from 'antd'
import type { MenuProps } from 'antd'
import {
  DashboardOutlined,
  ProjectOutlined,
  ApartmentOutlined,
  RobotOutlined,
  TeamOutlined,
  BookOutlined,
  InboxOutlined,
  ShopOutlined,
  LogoutOutlined,
  UserOutlined,
  BellOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  DownOutlined,
  AppstoreOutlined,
  HomeOutlined,
} from '@ant-design/icons'
import { useState } from 'react'
import { useAuthStore } from '@/stores/authStore'
import { useUnreadCount } from '@/hooks/useNotifications'
import { useRealtimeEvents } from '@/hooks/useRealtimeEvents'
import { useProjects } from '@/hooks/useProjects'
import { useExternalApps } from '@/hooks/useExternalApps'
import { FloatingOrbs } from '@/components/FloatingOrbs'
import { GradientText } from '@/components/GradientText'
import { ThemeSwitch } from '@/components/ThemeSwitch'
import { useTheme } from '@/theme/ThemeProvider'
import { motion } from 'framer-motion'
import { hasPlacement, type ProjectWorkStatus } from '@/types'

const { Sider, Content } = Layout

const workStatusMap: Record<ProjectWorkStatus, { color: string; label: string }> = {
  empty: { color: 'default', label: '空' },
  idle: { color: 'default', label: '空闲' },
  queued: { color: 'processing', label: '排队' },
  running: { color: 'success', label: '执行中' },
  attention: { color: 'warning', label: '需关注' },
  archived: { color: 'default', label: '归档' },
}

const staticMenuItems: MenuProps['items'] = [
  { key: '/dashboard', icon: <DashboardOutlined />, label: '仪表盘' },
  { key: '/office', icon: <HomeOutlined />, label: 'AI 办公室' },
  { key: '/projects', icon: <ProjectOutlined />, label: '项目' },
  { key: '/workflows', icon: <ApartmentOutlined />, label: '工作流' },
  { key: '/agents', icon: <RobotOutlined />, label: '数字员工' },
  { key: '/meetings', icon: <TeamOutlined />, label: '会议' },
  { key: '/knowledge', icon: <BookOutlined />, label: '知识库' },
  { key: '/inbox', icon: <InboxOutlined />, label: '收件箱' },
  { key: '/market', icon: <ShopOutlined />, label: '市场' },
  { key: '/external-apps', icon: <AppstoreOutlined />, label: '外部应用' },
]

export function MainLayout() {
  const [collapsed, setCollapsed] = useState(false)
  const navigate = useNavigate()
  const location = useLocation()
  const { user, logout } = useAuthStore()
  const { data: unreadCount } = useUnreadCount()
  const { data: projects } = useProjects()
  const { data: externalApps } = useExternalApps()
  const { theme } = useTheme()
  useRealtimeEvents()

  // 声明了 sidebar 挂载点的启用中外部平台，追加在主菜单末尾。
  // 后端已按可见性过滤（公共 + 自己创建的），这里只做挂载点筛选。
  const mountedApps = useMemo(
    () =>
      (externalApps ?? []).filter(
        (a) => a.status === 'enabled' && hasPlacement(a.placement, 'sidebar'),
      ),
    [externalApps],
  )

  const menuItems = useMemo<MenuProps['items']>(() => {
    if (mountedApps.length === 0) return staticMenuItems
    return [
      ...(staticMenuItems ?? []),
      { type: 'divider' },
      ...mountedApps.map((a) => ({
        key: `/app/${a.id}`,
        icon: a.icon_url ? (
          <img
            src={a.icon_url}
            alt=""
            style={{ width: 14, height: 14, objectFit: 'contain' }}
          />
        ) : (
          <AppstoreOutlined />
        ),
        label: a.name,
      })),
    ]
  }, [mountedApps])

  const pathSeg = location.pathname.split('/')[1] || 'dashboard'
  const selectedKey =
    pathSeg === 'app'
      ? `/app/${location.pathname.split('/')[2] ?? ''}`
      : '/' + (pathSeg === 'agent-invite' ? 'agents' : pathSeg === 'office' ? 'office' : pathSeg)

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  const userMenuItems = [
    { key: 'profile', icon: <UserOutlined />, label: '个人信息' },
    { type: 'divider' as const },
    { key: 'logout', icon: <LogoutOutlined />, label: '退出登录', danger: true },
  ]

  // 最近活跃项目（非归档，按最近任务活动排序）
  const recentProjects = (Array.isArray(projects) ? projects : [])
    .filter((p) => p.status !== 'archived')
    .sort((a, b) =>
      (b.task_summary?.latest_task_at ?? b.created_at).localeCompare(
        a.task_summary?.latest_task_at ?? a.created_at,
      ),
    )
    .slice(0, 5)

  // 底部入口行（展开显示文字，折叠只显示图标并居中）
  const rowStyle: React.CSSProperties = {
    display: 'flex',
    alignItems: 'center',
    gap: 10,
    padding: collapsed ? '9px 0' : '9px 12px',
    margin: '0 8px',
    borderRadius: 'var(--radius-control)',
    cursor: 'pointer',
    fontSize: 13,
    color: 'var(--text-secondary)',
    justifyContent: collapsed ? 'center' : 'flex-start',
    whiteSpace: 'nowrap',
    transition: 'background 0.15s',
  }

  const isProjectDetail = pathSeg === 'projects' && location.pathname.split('/').length > 2

  return (
    <Layout style={{ minHeight: '100vh', background: 'transparent', position: 'relative' }}>
      <FloatingOrbs />

      <Sider
        trigger={null}
        collapsible
        collapsed={collapsed}
        breakpoint="lg"
        width={220}
        style={{
          height: '100vh',
          position: 'fixed',
          left: 0,
          top: 0,
          bottom: 0,
          zIndex: 100,
          background: 'var(--canvas-elevated)',
          backdropFilter: 'var(--glass-blur)',
          WebkitBackdropFilter: 'var(--glass-blur)',
          borderRight: '1px solid var(--line)',
        }}
      >
        <style>{`
          .tm-sidebar .ant-menu-inline .ant-menu-item {
            height: 36px;
            line-height: 36px;
            margin: 2px 8px !important;
            width: calc(100% - 16px);
            border-radius: 8px;
          }
          .tm-sidebar .ant-menu-inline { gap: 0; }
        `}</style>
        <div className="tm-sidebar" style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
          {/* 顶部 Logo */}
          <div
            style={{
              height: 56,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              borderBottom: '1px solid var(--line)',
              flexShrink: 0,
            }}
          >
            {collapsed ? (
              <span style={{ fontSize: 18, fontWeight: 700 }}>
                <GradientText speed={5}>TM</GradientText>
              </span>
            ) : (
              <span style={{ fontSize: 16, fontWeight: 700, letterSpacing: '-0.3px' }}>
                <GradientText speed={5}>TrustMesh</GradientText>
              </span>
            )}
          </div>

          {/* 中部：主导航 + 最近项目快捷入口 */}
          <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', overflowX: 'hidden' }}>
            <Menu
              theme={theme === 'dark' ? 'dark' : 'light'}
              mode="inline"
              selectedKeys={[selectedKey]}
              items={menuItems}
              onClick={({ key }) => navigate(key)}
              style={{ background: 'transparent', borderInlineEnd: 'none', marginTop: 8 }}
            />

            {!collapsed && recentProjects.length > 0 && (
              <div style={{ margin: '10px 8px 6px' }}>
                <div
                  style={{
                    fontSize: 14,
                    fontWeight: 400,
                    color: 'var(--text-secondary)',
                    padding: '0 10px',
                    marginBottom: 2,
                  }}
                >
                  最近项目
                </div>
                {recentProjects.map((p) => {
                  const active = location.pathname === `/projects/${p.id}`
                  const status = p.task_summary?.work_status || 'idle'
                  const statusInfo = workStatusMap[status]
                  return (
                    <div
                      key={p.id}
                      onClick={() => navigate(`/projects/${p.id}`)}
                      title={p.name}
                      style={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: 8,
                        padding: '5px 10px',
                        borderRadius: 'var(--radius-control)',
                        cursor: 'pointer',
                        fontSize: 14,
                        fontWeight: 400,
                        color: active ? 'var(--signal-hover)' : 'var(--text-secondary)',
                        background: active ? 'rgba(109,95,245,0.12)' : 'transparent',
                        overflow: 'hidden',
                      }}
                    >
                      <ProjectOutlined style={{ fontSize: 14, color: 'var(--text-quaternary)', flexShrink: 0 }} />
                      <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {p.name}
                      </span>
                      <Tag
                        color={statusInfo?.color}
                        style={{
                          marginInlineEnd: 0,
                          fontSize: 10,
                          lineHeight: '16px',
                          padding: status === 'running' ? '0 4px 0 14px' : '0 4px',
                          borderRadius: 'var(--radius-control)',
                          flexShrink: 0,
                          position: 'relative',
                          overflow: 'hidden',
                        }}
                        className={status === 'running' ? 'running-tag' : ''}
                      >
                        {statusInfo?.label}
                      </Tag>
                    </div>
                  )
                })}
              </div>
            )}
          </div>

          {/* 底部固定区：消息通知 / 个人信息 / 收起侧边栏 */}
          <div
            style={{
              flexShrink: 0,
              borderTop: '1px solid var(--line)',
              padding: '8px 0 10px',
              display: 'flex',
              flexDirection: 'column',
              gap: 2,
              background: 'var(--surface-inset)',
            }}
          >
            <Tooltip title={collapsed ? '消息通知' : ''} placement="right">
              <div onClick={() => navigate('/inbox')} style={rowStyle}>
                <Badge count={unreadCount ?? 0} size="small" offset={collapsed ? [2, -2] : [4, 0]}>
                  <BellOutlined style={{ fontSize: 15 }} />
                </Badge>
                {!collapsed && <span style={{ flex: 1 }}>消息通知</span>}
              </div>
            </Tooltip>

            <Dropdown
              menu={{
                items: userMenuItems,
                onClick: ({ key }) => {
                  if (key === 'logout') handleLogout()
                },
              }}
              trigger={['click']}
            >
              <Tooltip title={collapsed ? (user?.name || '个人信息') : ''} placement="right">
                <div style={rowStyle}>
                  <Avatar
                    size={20}
                    icon={<UserOutlined />}
                    style={{ background: 'linear-gradient(135deg, var(--signal), var(--signal))', flexShrink: 0 }}
                  />
                  {!collapsed && (
                    <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {user?.name || '用户'}
                    </span>
                  )}
                  {!collapsed && <DownOutlined style={{ fontSize: 10, color: 'var(--text-quaternary)' }} />}
                </div>
              </Tooltip>
            </Dropdown>

            <Tooltip title={collapsed ? '收起侧边栏' : ''} placement="right">
              <div onClick={() => setCollapsed(!collapsed)} style={rowStyle}>
                {collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
                {!collapsed && <span style={{ flex: 1 }}>收起侧边栏</span>}
              </div>
            </Tooltip>

            <ThemeSwitch collapsed={collapsed} />
          </div>
        </div>
      </Sider>

      <Layout style={{ marginLeft: collapsed ? 80 : 220, transition: 'margin-left 0.3s ease', height: '100vh', overflow: 'hidden' }}>
        <Content style={{ flex: 1, minHeight: 0, overflow: 'auto', margin: '16px 20px 0', position: 'relative', zIndex: 1 }}>
          <motion.div
            initial={{ opacity: 0, y: 8 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.35, ease: 'easeOut' }}
            key={isProjectDetail ? location.pathname.split('/')[2] : location.pathname}
            style={{ height: '100%' }}
          >
            <Outlet />
          </motion.div>
        </Content>
      </Layout>
    </Layout>
  )
}
