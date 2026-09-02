import { ConfigProvider, App as AntApp, theme as antdTheme } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter } from 'react-router-dom'
import { AppRouter } from './router'
import { PlatformProvider } from './components/providers/PlatformProvider'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 30_000,
    },
  },
})

// TrustMesh DESIGN.md — Linear-inspired dark console
// canvas #0a0a12 · surface ladder #12121d-232331 · violet #6d5ff5 · hairline #232330
const theme = {
  algorithm: antdTheme.darkAlgorithm,
  token: {
    colorPrimary: '#6d5ff5',
    colorInfo: '#6d5ff5',
    colorSuccess: '#27a644',
    colorWarning: '#f59e0b',
    colorError: '#ef4444',
    colorTextBase: '#f4f4f8',
    colorBgBase: '#0a0a12',
    colorBgContainer: '#12121d',
    colorBgElevated: '#171722',
    colorBgLayout: '#0a0a12',
    colorBorder: '#232330',
    colorBorderSecondary: '#1b1b26',
    colorText: '#f4f4f8',
    colorTextSecondary: '#c8ccd8',
    colorTextTertiary: '#8b8f9e',
    colorTextQuaternary: '#5f6372',
    colorSplit: '#232330',
    colorPrimaryBg: 'rgba(109,95,245,0.14)',
    colorPrimaryBorder: 'rgba(109,95,245,0.35)',
    borderRadius: 8,
    borderRadiusLG: 12,
    fontFamily:
      "Inter, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif",
    fontSize: 14,
    controlHeight: 36,
  },
  components: {
    Layout: {
      headerBg: '#12121d',
      siderBg: '#0a0a12',
      bodyBg: '#0a0a12',
    },
    Menu: {
      darkItemBg: '#0a0a12',
      darkItemSelectedBg: 'rgba(109,95,245,0.14)',
      darkItemSelectedColor: '#8b7ff8',
      darkItemColor: '#8b8f9e',
      darkItemHoverColor: '#f4f4f8',
    },
    Card: {
      colorBgContainer: '#12121d',
    },
    Table: {
      headerBg: '#0f0f18',
      rowHoverBg: '#171722',
      borderColor: '#232330',
    },
    Modal: {
      contentBg: '#171722',
      headerBg: '#171722',
    },
    Dropdown: {
      colorBgElevated: '#171722',
    },
    Select: {
      colorBgElevated: '#171722',
    },
    Button: {
      colorBgContainer: '#12121d',
      colorBorder: '#232330',
    },
    Input: {
      colorBgContainer: '#12121d',
    },
    Tooltip: {
      colorBgSpotlight: '#232331',
    },
  },
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ConfigProvider theme={theme} locale={zhCN}>
        <AntApp>
          <PlatformProvider>
            <BrowserRouter>
              <AppRouter />
            </BrowserRouter>
          </PlatformProvider>
        </AntApp>
      </ConfigProvider>
    </QueryClientProvider>
  )
}
