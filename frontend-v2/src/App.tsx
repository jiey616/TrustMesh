import { App as AntApp } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter } from 'react-router-dom'
import { AppRouter } from './router'
import { PlatformProvider } from './components/providers/PlatformProvider'
import { ThemeProvider } from './theme/ThemeProvider'

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
export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <AntApp>
          <PlatformProvider>
            <BrowserRouter>
              <AppRouter />
            </BrowserRouter>
          </PlatformProvider>
        </AntApp>
      </ThemeProvider>
    </QueryClientProvider>
  )
}
