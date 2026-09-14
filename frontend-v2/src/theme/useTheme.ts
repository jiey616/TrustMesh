import { createContext, useContext } from 'react'
import { type ThemeName, type ThemeTokens } from './tokens'

export interface ThemeContextValue {
  theme: ThemeName
  tokens: ThemeTokens
  setTheme: (t: ThemeName) => void
  toggleTheme: () => void
}

export const ThemeContext = createContext<ThemeContextValue | null>(null)

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext)
  if (!ctx) throw new Error('useTheme 必须在 ThemeProvider 内使用')
  return ctx
}
