import { Routes, Route, Navigate, Outlet } from 'react-router-dom'
import { lazy, Suspense } from 'react'
import { MainLayout } from '@/layouts/MainLayout'
import { AuthLayout } from '@/layouts/AuthLayout'
import { ProtectedRoute } from '@/components/ProtectedRoute'
import { PermRoute } from '@/components/PermRoute'
import { LoginPage } from '@/pages/LoginPage'
import { RegisterPage } from '@/pages/RegisterPage'
import { DashboardPage } from '@/pages/DashboardPage'
import { ProjectListPage } from '@/pages/ProjectListPage'
import { ProjectBoardPage } from '@/pages/ProjectBoardPage'
import { WorkflowTemplatesPage } from '@/pages/WorkflowTemplatesPage'
import { AgentListPage } from '@/pages/AgentListPage'
import { AgentDetailPage } from '@/pages/AgentDetailPage'
import { AgentInvitePage } from '@/pages/AgentInvitePage'

import { MeetingListPage } from '@/pages/MeetingListPage'
import { MeetingRoomPage } from '@/pages/MeetingRoomPage'
import { KnowledgePage } from '@/pages/KnowledgePage'
import { InboxPage } from '@/pages/InboxPage'
import { MarketPage } from '@/pages/MarketPage'
import { RoleDetailPage } from '@/pages/RoleDetailPage'
import { ExternalAppsPage } from '@/pages/ExternalAppsPage'
import { ExternalAppFramePage } from '@/pages/ExternalAppFramePage'
import { OrgSettingsPage } from '@/pages/OrgSettingsPage'
import { ProfilePage } from '@/pages/ProfilePage'
import { OpsIncidentsPage } from '@/pages/OpsIncidentsPage'
import { ForbiddenPage } from '@/pages/ForbiddenPage'
import { PlatformOrgsPage } from '@/pages/platform/PlatformOrgsPage'
import { PlatformOrgDetailPage } from '@/pages/platform/PlatformOrgDetailPage'
import { PlatformUsersPage } from '@/pages/platform/PlatformUsersPage'
import { PlatformConfigPage } from '@/pages/platform/PlatformConfigPage'
import { PlatformExternalAppsPage } from '@/pages/platform/PlatformExternalAppsPage'
import { PlatformAuditPage } from '@/pages/platform/PlatformAuditPage'
import { PlatformUsagePage } from '@/pages/platform/PlatformUsagePage'
import { ORG_DOMAIN_PERMS, PERM } from '@/lib/perms'

// 办公室引入 three 生态（体积大），单独成 chunk 按需加载
const OfficePage = lazy(() =>
  import('@/pages/OfficePage').then((m) => ({ default: m.OfficePage })),
)

export function AppRouter() {
  return (
    <Routes>
      {/* Auth pages */}
      <Route element={<AuthLayout />}>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/register" element={<RegisterPage />} />
      </Route>

      {/* Protected pages */}
      <Route
        element={
          <ProtectedRoute>
            <MainLayout />
          </ProtectedRoute>
        }
      >
        {/* 越权统一落地页：平台管理员与业务账号都可访问，故不套业务门禁 */}
        <Route path="/403" element={<ForbiddenPage />} />
        <Route path="/profile" element={<ProfilePage />} />

        {/* 平台管理命名空间：只认平台管理员标记（企业角色一律 403） */}
        <Route
          path="/platform"
          element={
            <PermRoute audience="platform">
              <Outlet />
            </PermRoute>
          }
        >
          <Route index element={<Navigate to="/platform/orgs" replace />} />
          <Route path="orgs" element={<PlatformOrgsPage />} />
          <Route path="orgs/:id" element={<PlatformOrgDetailPage />} />
          <Route path="users" element={<PlatformUsersPage />} />
          <Route path="config" element={<PlatformConfigPage />} />
          <Route path="external-apps" element={<PlatformExternalAppsPage />} />
          <Route path="audit" element={<PlatformAuditPage />} />
          <Route path="usage" element={<PlatformUsagePage />} />
        </Route>

        {/* 业务命名空间：平台管理员被拦（后端对平台管理员全量 403，界面不该诱导点击） */}
        <Route
          element={
            <PermRoute audience="business">
              <Outlet />
            </PermRoute>
          }
        >
          <Route path="/" element={<Navigate to="/dashboard" replace />} />
          <Route path="/dashboard" element={<DashboardPage />} />
          <Route
            path="/office"
            element={
              <Suspense fallback={null}>
                <OfficePage />
              </Suspense>
            }
          />

          <Route path="/projects" element={<ProjectListPage />} />
          <Route path="/projects/:id" element={<ProjectBoardPage />} />
          <Route path="/workflows" element={<WorkflowTemplatesPage />} />
          <Route
            path="/agents"
            element={
              <PermRoute perms={[PERM.AGENT_VIEW]}>
                <AgentListPage />
              </PermRoute>
            }
          />
          <Route
            path="/agents/:id"
            element={
              <PermRoute perms={[PERM.AGENT_VIEW]}>
                <AgentDetailPage />
              </PermRoute>
            }
          />
          {/* 招聘数字员工 = 写操作（后端 POST /agents 走 agent.manage），
              不是 agent.view；member 有 view 但无 manage，不该看到本页。 */}
          <Route
            path="/agent-invite"
            element={
              <PermRoute perms={[PERM.AGENT_MANAGE]}>
                <AgentInvitePage />
              </PermRoute>
            }
          />
          <Route path="/meetings" element={<MeetingListPage />} />
          <Route path="/meetings/:id" element={<MeetingRoomPage />} />
          <Route path="/knowledge" element={<KnowledgePage />} />
          <Route path="/inbox" element={<InboxPage />} />
          <Route
            path="/market"
            element={
              <PermRoute perms={[PERM.MARKET_BROWSE]}>
                <MarketPage />
              </PermRoute>
            }
          />
          <Route
            path="/market/roles/:id"
            element={
              <PermRoute perms={[PERM.MARKET_BROWSE]}>
                <RoleDetailPage />
              </PermRoute>
            }
          />
          <Route path="/external-apps" element={<ExternalAppsPage />} />
          {/* 组织管理：入口在「个人信息」页；组织域任一权限点即可见，页内再按权限点分标签页 */}
          <Route
            path="/organizations"
            element={
              <PermRoute perms={ORG_DOMAIN_PERMS}>
                <OrgSettingsPage />
              </PermRoute>
            }
          />
          <Route
            path="/ops"
            element={
              <PermRoute perms={[PERM.OPS_VIEW]}>
                <OpsIncidentsPage />
              </PermRoute>
            }
          />
          {/* 侧边栏挂载的外部平台外壳页 */}
          <Route path="/app/:id" element={<ExternalAppFramePage />} />
        </Route>
      </Route>

      <Route path="*" element={<Navigate to="/dashboard" replace />} />
    </Routes>
  )
}