import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { ConfigProvider } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import { themes, toCssVars, type ThemeName, type ThemeTokens } from './tokens'
import { buildAntdTheme } from './antdTheme'

const STORAGE_KEY = 'trustmesh-theme'

interface ThemeContextValue {
  theme: ThemeName
  tokens: ThemeTokens
  setTheme: (t: ThemeName) => void
  toggleTheme: () => void
}

const ThemeContext = createContext<ThemeContextValue | null>(null)

function readStoredTheme(): ThemeName {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'dark' || v === 'quiet') return v
  } catch {
    /* localStorage 不可用（隐私模式 / 跨源 iframe）时静默降级到默认主题 */
  }
  return 'dark'
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<ThemeName>(readStoredTheme)

  const setTheme = useCallback((next: ThemeName) => {
    setThemeState(next)
    try {
      localStorage.setItem(STORAGE_KEY, next)
    } catch {
      /* 忽略写入失败，主题仍在内存中生效 */
    }
  }, [])

  const toggleTheme = useCallback(() => {
    setThemeState((prev) => {
      const next: ThemeName = prev === 'dark' ? 'quiet' : 'dark'
      try {
        localStorage.setItem(STORAGE_KEY, next)
      } catch {
        /* 同上 */
      }
      return next
    })
  }, [])

  // 注入 CSS 变量 + data-theme，供内联样式与全局 CSS 消费
  useEffect(() => {
    const root = document.documentElement
    const vars = toCssVars(themes[theme])
    for (const [name, value] of Object.entries(vars)) {
      root.style.setProperty(name, value)
    }
    root.dataset.theme = theme
    root.style.colorScheme = theme === 'dark' ? 'dark' : 'light'
  }, [theme])

  const antdConfig = useMemo(() => buildAntdTheme(theme), [theme])

  const value = useMemo<ThemeContextValue>(
    () => ({ theme, tokens: themes[theme], setTheme, toggleTheme }),
    [theme, setTheme, toggleTheme],
  )

  return (
    <ThemeContext.Provider value={value}>
      <ConfigProvider theme={antdConfig} locale={zhCN}>
        {children}
      </ConfigProvider>
    </ThemeContext.Provider>
  )
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext)
  if (!ctx) throw new Error('useTheme 必须在 ThemeProvider 内使用')
  return ctx
}
