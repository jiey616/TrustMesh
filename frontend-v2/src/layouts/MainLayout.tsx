import { useEffect, useMemo } from 'react'
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
  ShopOutlined,
  LogoutOutlined,
  UserOutlined,
  BellOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  DownOutlined,
  AppstoreOutlined,
  HomeOutlined,
  CrownOutlined,
  SwapOutlined,
  CheckOutlined,
  BankOutlined,
  AlertOutlined,
  ApiOutlined,
} from '@ant-design/icons'
import { useState } from 'react'
import { useAuthStore } from '@/stores/authStore'
import { resolveWorkspaceTarget } from '@/lib/workspaceMemory'
import { isElectronRuntime } from '@/stores/serverConfigStore'
import { useUnreadCount } from '@/hooks/useNotifications'
import { useRealtimeEvents } from '@/hooks/useRealtimeEvents'
import { useProjects } from '@/hooks/useProjects'
import { useExternalApps } from '@/hooks/useExternalApps'
import { useOrganizations } from '@/hooks/useOrgs'
import { useQueryClient } from '@tanstack/react-query'
import { FloatingOrbs } from '@/components/FloatingOrbs'
import { AssistantFab } from '@/components/assistant/AssistantFab'
import { GradientText } from '@/components/GradientText'
import { ThemeSwitch } from '@/components/ThemeSwitch'
import { useTheme } from '@/theme/useTheme'
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
  { key: '/market', icon: <ShopOutlined />, label: '市场' },
  { key: '/external-apps', icon: <AppstoreOutlined />, label: '外部应用' },
  { key: '/ops', icon: <AlertOutlined />, label: '运维工单' },
]

export function MainLayout() {
  const [collapsed, setCollapsed] = useState(false)
  const navigate = useNavigate()
  const location = useLocation()
  const { data: unreadCount } = useUnreadCount()
  const { data: projects } = useProjects()
  const { data: externalApps } = useExternalApps()
  const { data: orgs } = useOrganizations()
  const {
    user,
    logout,
    activeOrgId,
    setActiveOrg,
    personalOrgId,
    setPersonalOrgId,
    workspaceMemory,
    rememberWorkspace,
  } = useAuthStore()
  const qc = useQueryClient()
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
    // 清掉 react-query 缓存：QueryClient 是模块级单例，跨账号存活，
    // 不清会导致换账号登录后命中上一个账号的 orgs/项目等缓存数据。
    qc.clear()
    navigate('/login')
  }

  // 防御：持久化的 activeOrgId 已不在我的租户列表（被移出/数据回退）→ 回落个人空间
  useEffect(() => {
    if (activeOrgId && orgs && !orgs.some((o) => o.id === activeOrgId)) {
      setActiveOrg(null)
      qc.removeQueries()
    }
  }, [activeOrgId, orgs, qc, setActiveOrg])

  const personalOrg = useMemo(() => (orgs ?? []).find((o) => o.kind === 'personal'), [orgs])
  const enterpriseOrgs = useMemo(() => (orgs ?? []).filter((o) => o.kind === 'enterprise'), [orgs])

  // 统一工作区校准：orgs 就绪后**一次**决定 (a) 个人租户 id 水合 (b) 依「记忆」恢复企业空间。
  //
  // 时序保证（照设计 §3.3(c)）：两个运行时 id 在**同一 tick 内**先写个人、再写企业，
  // 且只有紧随其后的 qc.removeQueries() 会触发重取（setActiveOrg/setPersonalOrgId 不改变
  // 任何 queryKey）→ 重取时读到的一定是**最终（已校验）**的运行时态，不存在
  // 「先发个人头、后发企业头」的中间窗口。
  //
  // 记忆经 resolveWorkspaceTarget 双守门（userId 匹配 + org 必须 ∈ 本账号 orgs）；
  // 任一不满足 → 返回 null → activeOrgId 回落 null（个人空间）。绝不写入未校验值。
  useEffect(() => {
    if (!user || !orgs) return
    const nextPersonal = personalOrg?.id ?? null
    const nextActive = resolveWorkspaceTarget(workspaceMemory, user.id, orgs)
    const personalChanged = nextPersonal !== personalOrgId
    const activeChanged = nextActive !== activeOrgId
    if (!personalChanged && !activeChanged) return
    if (personalChanged) setPersonalOrgId(nextPersonal)
    if (activeChanged) setActiveOrg(nextActive)
    qc.removeQueries()
  }, [user, orgs, personalOrg, workspaceMemory, activeOrgId, personalOrgId, qc, setActiveOrg, setPersonalOrgId])
  const roleLabel = (r: string) => (r === 'owner' ? 'Owner' : r === 'admin' ? 'Admin' : '成员')

  // 当前生效工作区：activeOrgId 为空 = 个人空间。侧边栏用户区与切换菜单都需要它。
  const activeOrg = useMemo(
    () => (orgs ?? []).find((o) => o.id === activeOrgId),
    [orgs, activeOrgId],
  )
  const activeWorkspaceName = activeOrg?.name || personalOrg?.name || '个人空间'

  const orgMenuItems: MenuProps['items'] = useMemo(
    () => [
      {
        key: '__personal__',
        icon: <UserOutlined />,
        label: (
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
            <span>{personalOrg?.name || '个人空间'}</span>
            {!activeOrgId && (
              <CheckOutlined style={{ fontSize: 11, color: 'var(--signal)' }} />
            )}
          </span>
        ),
      },
      { type: 'divider' as const },
      ...enterpriseOrgs.map((o) => ({
        key: o.id,
        icon: <CrownOutlined style={{ color: o.my_role === 'owner' ? 'var(--signal)' : undefined }} />,
        label: (
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
            <span>{o.name}</span>
            <Tag style={{ marginInlineEnd: 0, fontSize: 10, lineHeight: '16px' }}>{roleLabel(o.my_role)}</Tag>
            {activeOrgId === o.id && (
              <CheckOutlined style={{ fontSize: 11, color: 'var(--signal)' }} />
            )}
          </span>
        ),
      })),
    ],
    [personalOrg, enterpriseOrgs, activeOrgId],
  )

  const handleOrgSwitch = (key: string) => {
    if (key === '__personal__') {
      if (!activeOrgId) return
      setActiveOrg(null)
      // 记住「上次选中的空间」= 个人（persist 在 set 时同步落盘，先于 reload）
      rememberWorkspace('personal')
      // 全局刷新：整页重载，所有数据按新租户上下文重新加载
      window.location.reload()
      return
    }
    // 企业分支：key 已通过 orgs 校验（∈ 本账号租户），故可安全记住
    if (!orgs?.some((o) => o.id === key) || key === activeOrgId) return
    setActiveOrg(key)
    rememberWorkspace('enterprise', key)
    window.location.reload()
  }

  const userMenuItems = [
    { key: 'orgs', icon: <SwapOutlined />, label: '切换工作区', children: orgMenuItems },
    { key: 'org-manage', icon: <CrownOutlined />, label: '企业管理' },
    { type: 'divider' as const },
    { key: 'profile', icon: <UserOutlined />, label: '个人信息' },
    // 服务器地址配置仅桌面端可用（Web/容器部署固定走同源 /api/v1/）
    ...(isElectronRuntime()
      ? ([{ key: 'server', icon: <ApiOutlined />, label: '服务器设置' }] as const)
      : []),
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
          .tm-iconbtn {
            display: flex;
            align-items: center;
            justify-content: center;
            width: 32px;
            height: 32px;
            border-radius: var(--radius-control);
            cursor: pointer;
            color: var(--text-secondary);
            transition: background 0.15s ease, color 0.15s ease;
            user-select: none;
          }
          .tm-iconbtn:hover { background: var(--surface); color: var(--text-primary); }
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

          {/* 底部固定区：消息通知 / 个人信息 / 收起+主题（图标并排） */}
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
                  else if (key === 'org-manage') navigate('/organizations')
                  else if (key === 'profile') navigate('/profile')
                  else if (key === 'server') navigate('/profile#server')
                  // 其余 key 为工作区 id（含 __personal__）
                  else handleOrgSwitch(key)
                },
              }}
              trigger={['click']}
            >
              <Tooltip title={collapsed ? `当前工作区：${activeWorkspaceName}` : ''} placement="right">
                <div style={rowStyle}>
                  {activeOrgId ? (
                    <BankOutlined style={{ fontSize: 15, flexShrink: 0 }} />
                  ) : (
                    <Avatar
                      size={20}
                      icon={<UserOutlined />}
                      style={{ background: 'linear-gradient(135deg, var(--signal), var(--signal))', flexShrink: 0 }}
                    />
                  )}
                  {!collapsed && (
                    <span style={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', gap: 1 }}>
                      <span
                        style={{
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          whiteSpace: 'nowrap',
                          fontSize: 13,
                          color: 'var(--text-primary)',
                          fontWeight: 500,
                          lineHeight: '16px',
                        }}
                      >
                        {activeWorkspaceName}
                      </span>
                      <span
                        style={{
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          whiteSpace: 'nowrap',
                          fontSize: 11,
                          color: 'var(--text-quaternary)',
                          lineHeight: '13px',
                        }}
                      >
                        {user?.name || '用户'}
                      </span>
                    </span>
                  )}
                  {!collapsed && <DownOutlined style={{ fontSize: 10, color: 'var(--text-quaternary)' }} />}
                </div>
              </Tooltip>
            </Dropdown>

            {/* 收起侧边栏 + 主题切换：同一行，仅图标 */}
            <div
              style={{
                display: 'flex',
                flexDirection: collapsed ? 'column' : 'row',
                alignItems: 'center',
                justifyContent: collapsed ? 'center' : 'flex-start',
                gap: 6,
                padding: collapsed ? '6px 0' : '6px 12px',
                margin: '0 8px',
              }}
            >
              <Tooltip title="收起侧边栏" placement="right">
                <div
                  role="button"
                  tabIndex={0}
                  aria-label="收起侧边栏"
                  onClick={() => setCollapsed(!collapsed)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault()
                      setCollapsed(!collapsed)
                    }
                  }}
                  className="tm-iconbtn"
                >
                  {collapsed ? <MenuUnfoldOutlined style={{ fontSize: 15 }} /> : <MenuFoldOutlined style={{ fontSize: 15 }} />}
                </div>
              </Tooltip>
              <ThemeSwitch collapsed={collapsed} iconOnly />
            </div>
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
        {/* AI 助手悬浮入口（Ctrl+K） */}
        <AssistantFab />
      </Layout>
    </Layout>
  )
}
