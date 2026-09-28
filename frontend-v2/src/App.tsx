import { App as AntApp } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter, HashRouter } from 'react-router-dom'
import { AppRouter } from './router'
import { PlatformProvider } from './components/providers/PlatformProvider'
import { ThemeProvider } from './theme/ThemeProvider'
import { isElectronRuntime, syncTlsTrustToMain } from './stores/serverConfigStore'
import { ApiRequestError } from './types'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // 🔴 429 不在这里重试：ky 已经把 429 列入可重试状态码、并会读 `Retry-After`
      // 退避（见 api/client.ts 的 ky 实例），React Query 再重试一次等于把压力翻倍
      // （一次页面操作实测能放大到 6 次请求）。429 的恢复交给「等下一轮轮询」，
      // 期间 UI 保留上次数据而不是报错 —— 与限流中间件的读路径放宽配套。
      retry: (failureCount, error) => {
        if (error instanceof ApiRequestError && error.status === 429) return false
        return failureCount < 1
      },
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
