import { App as AntApp } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter, HashRouter } from 'react-router-dom'
import { AppRouter } from './router'
import { PlatformProvider } from './components/providers/PlatformProvider'
import { ThemeProvider } from './theme/ThemeProvider'
import { isElectronRuntime, syncTlsTrustToMain } from './stores/serverConfigStore'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 30_000,
    },
  },
})

// 主题（深色 console / Quiet Signal 近白）由 ThemeProvider 统一提供，
// 内含 antd ConfigProvider；具体取值见 src/theme/tokens.ts。
// Electron 桌面版以 file:// 加载，BrowserRouter 的 history API 在刷新/深链时不可用，
// 因此在桌面环境使用 HashRouter；浏览器环境保持 BrowserRouter 不变。
const Router = isElectronRuntime() ? HashRouter : BrowserRouter

// 桌面端：把本地持久化的「信任自签名证书」回灌给主进程（重装/配置丢失时仍能生效）
if (isElectronRuntime()) {
  void syncTlsTrustToMain()
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <AntApp>
          <PlatformProvider>
            <Router>
              <AppRouter />
            </Router>
          </PlatformProvider>
        </AntApp>
      </ThemeProvider>
    </QueryClientProvider>
  )
}
