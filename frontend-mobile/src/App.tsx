import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { HashRouter, Navigate, Route, Routes } from 'react-router-dom'
import { LoginPage } from '@/pages/LoginPage'
import { TabLayout } from '@/components/TabLayout'
import { HomePage } from '@/pages/HomePage'
import { TasksPage } from '@/pages/TasksPage'
import { ProjectTasksPage } from '@/pages/ProjectTasksPage'
import { TaskDetailPage } from '@/pages/TaskDetailPage'
import { CreateTaskPage } from '@/pages/CreateTaskPage'
import { PipelinePage } from '@/pages/PipelinePage'
import { InboxPage } from '@/pages/InboxPage'
import { MePage } from '@/pages/MePage'
import { useAuthStore } from '@/stores/authStore'

// 移动端一律 HashRouter：企业内部分发/嵌套 WebView 场景下 history API 不可靠，
// 与桌面端 Electron 的 file:// 分支同一道理。
const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, refetchOnWindowFocus: false, staleTime: 30_000 },
  },
})

function RequireAuth({ children }: { children: ReactNode }) {
  const refreshToken = useAuthStore((s) => s.refreshToken)
  if (!refreshToken) return <Navigate to="/login" replace />
  return <>{children}</>
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <HashRouter>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route
            element={
              <RequireAuth>
                <TabLayout />
              </RequireAuth>
            }
          >
            <Route path="/" element={<HomePage />} />
            <Route path="/tasks" element={<TasksPage />} />
            <Route path="/tasks/new" element={<CreateTaskPage />} />
            <Route path="/tasks/pipeline/:projectId" element={<PipelinePage />} />
            <Route path="/tasks/project/:projectId" element={<ProjectTasksPage />} />
            <Route path="/tasks/detail/:taskId" element={<TaskDetailPage />} />
            <Route path="/inbox" element={<InboxPage />} />
            <Route path="/me" element={<MePage />} />
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </HashRouter>
    </QueryClientProvider>
  )
}
