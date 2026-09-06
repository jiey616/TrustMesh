import { Routes, Route, Navigate } from 'react-router-dom'
import { lazy, Suspense } from 'react'
import { MainLayout } from '@/layouts/MainLayout'
import { AuthLayout } from '@/layouts/AuthLayout'
import { ProtectedRoute } from '@/components/ProtectedRoute'
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
        <Route path="/agents" element={<AgentListPage />} />
        <Route path="/agents/:id" element={<AgentDetailPage />} />
        <Route path="/agent-invite" element={<AgentInvitePage />} />
        <Route path="/meetings" element={<MeetingListPage />} />
        <Route path="/meetings/:id" element={<MeetingRoomPage />} />
        <Route path="/knowledge" element={<KnowledgePage />} />
        <Route path="/inbox" element={<InboxPage />} />
        <Route path="/market" element={<MarketPage />} />
        <Route path="/market/roles/:id" element={<RoleDetailPage />} />
        <Route path="/external-apps" element={<ExternalAppsPage />} />
        <Route path="/organizations" element={<OrgSettingsPage />} />
        <Route path="/profile" element={<ProfilePage />} />
        <Route path="/ops" element={<OpsIncidentsPage />} />
        {/* 侧边栏挂载的外部平台外壳页 */}
        <Route path="/app/:id" element={<ExternalAppFramePage />} />
      </Route>

      <Route path="*" element={<Navigate to="/dashboard" replace />} />
    </Routes>
  )
}
