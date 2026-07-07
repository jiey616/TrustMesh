import { useEffect, useState } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MainLayout } from '@/components/layout/MainLayout'
import { LoginPage } from '@/pages/LoginPage'
import { RegisterPage } from '@/pages/RegisterPage'
import { ProjectListPage } from '@/pages/ProjectListPage'
import { ProjectBoardPage } from '@/pages/ProjectBoardPage'
import { DashboardPage } from '@/pages/DashboardPage'
import { AgentDetailPage } from '@/pages/AgentDetailPage'
import { AgentInvitePage } from '@/pages/AgentInvitePage'
import { InboxPage } from '@/pages/InboxPage'
import { KnowledgePage } from '@/pages/KnowledgePage'
import { MarketPage } from '@/pages/MarketPage'
import { RoleDetailPage } from '@/pages/RoleDetailPage'
import { useAuthStore } from '@/stores/authStore'
import { Toaster } from '@/components/ui/sonner'
import { RealtimeProvider } from '@/realtime/provider'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { refresh as refreshApi } from '@/api/auth'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 10_000,
      refetchOnWindowFocus: true,
    },
  },
})

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  if (!isAuthenticated()) return <Navigate to="/login" replace />
  return <>{children}</>
}

function GuestRoute({ children }: { children: React.ReactNode }) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  if (isAuthenticated()) return <Navigate to="/dashboard" replace />
  return <>{children}</>
}

function useTokenBootstrap() {
  const [ready, setReady] = useState(false)
  const { refreshToken, accessToken, setTokens, logout } = useAuthStore()

  useEffect(() => {
    if (refreshToken && !accessToken) {
      refreshApi(refreshToken)
        .then((res) => {
          setTokens(res.data.access_token, res.data.refresh_token)
        })
        .catch(() => {
          logout()
        })
        .finally(() => setReady(true))
    } else {
      setReady(true)
    }
    // Only run on mount
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return ready
}

export default function App() {
  const ready = useTokenBootstrap()

  if (!ready) return null

  return (
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <RealtimeProvider>
          <BrowserRouter>
            <Routes>
              {/* Guest Routes */}
              <Route path="/login" element={<GuestRoute><LoginPage /></GuestRoute>} />
              <Route path="/register" element={<GuestRoute><RegisterPage /></GuestRoute>} />

              {/* Protected Routes */}
              <Route
                element={
                  <ProtectedRoute>
                    <MainLayout />
                  </ProtectedRoute>
                }
              >
                <Route path="/dashboard" element={<DashboardPage />} />
                <Route path="/inbox" element={<InboxPage />} />
                <Route path="/projects" element={<ProjectListPage />} />
                <Route path="/projects/:projectId" element={<ProjectBoardPage />} />
                <Route path="/agent-invite" element={<AgentInvitePage />} />
                <Route path="/agents/:id" element={<AgentDetailPage />} />
                <Route path="/knowledge" element={<KnowledgePage />} />
                <Route path="/market" element={<MarketPage />} />
                <Route path="/market/roles/:id" element={<RoleDetailPage />} />
              </Route>

              {/* Redirect */}
              <Route path="*" element={<Navigate to="/dashboard" replace />} />
            </Routes>
          </BrowserRouter>
        </RealtimeProvider>
        <Toaster position="top-center" />
      </QueryClientProvider>
    </ErrorBoundary>
  )
}
