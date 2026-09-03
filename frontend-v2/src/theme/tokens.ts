/**
 * TrustMesh 设计令牌 — 双主题（深色 console / Quiet Signal 近白）
 *
 * 治理规则（见 docs/frontend-v2-visual-hybrid-plan.md）：
 *  1. 组件内联样式一律引用 var(--x)，禁止再写死色值。
 *  2. 紫色只作「信号」，不作大面积气氛场。
 *  3. 圆角只有四档：结构 0 / 控件 4 / 头像 50 / 胶囊 999。
 *  4. 毛玻璃、发光、流光只在深色主题启用，近白主题全部关闭。
 */

export type ThemeName = 'dark' | 'quiet'

/** 一套主题的完整取值。CSS 变量名 = 键的 kebab-case（见 toCssVars）。 */
export interface ThemeTokens {
  // ---- 画布与表面 ----
  canvas: string
  canvasElevated: string
  surface: string
  surfaceSunken: string
  surfaceRaised: string
  /** 凹陷区：深色下压黑、近白下压灰。用于代码块、内联输入、内嵌容器 */
  surfaceInset: string

  // ---- 线条 ----
  line: string
  lineStrong: string

  // ---- 文字 ----
  textPrimary: string
  textSecondary: string
  textTertiary: string
  textQuaternary: string
  textInverse: string

  // ---- 信号与状态 ----
  signal: string
  signalSoft: string
  signalBorder: string
  signalHover: string
  success: string
  warning: string
  error: string
  info: string
  cyan: string

  // ---- 圆角 ----
  radiusStructure: string
  radiusControl: string
  radiusAvatar: string
  radiusPill: string

  // ---- 效果开关（近白主题全部归零）----
  glassBlur: string
  canvasGlow: string
  shadowCard: string
  shadowFloat: string
  gradientCta: string
}

/** 深色：TrustMesh console 画布 + 混合改造后的收敛层级 */
export const darkTokens: ThemeTokens = {
  canvas: '#0a0a12',
  canvasElevated: '#12121d',
  surface: 'rgba(255, 255, 255, 0.04)',
  surfaceSunken: 'rgba(255, 255, 255, 0.02)',
  surfaceRaised: 'rgba(255, 255, 255, 0.06)',
  surfaceInset: 'rgba(0, 0, 0, 0.25)',

  line: 'rgba(255, 255, 255, 0.07)',
  lineStrong: 'rgba(255, 255, 255, 0.12)',

  textPrimary: '#f4f4f8',
  textSecondary: 'rgba(255, 255, 255, 0.65)',
  textTertiary: 'rgba(255, 255, 255, 0.48)',
  textQuaternary: 'rgba(255, 255, 255, 0.34)',
  textInverse: '#0a0a12',

  signal: '#6d5ff5',
  signalSoft: 'rgba(109, 95, 245, 0.14)',
  signalBorder: 'rgba(109, 95, 245, 0.35)',
  signalHover: '#8b7ff8',
  success: '#22c55e',
  warning: '#f59e0b',
  error: '#ef4444',
  info: '#3b82f6',
  cyan: '#22d3ee',

  radiusStructure: '0px',
  radiusControl: '4px',
  radiusAvatar: '50%',
  radiusPill: '999px',

  glassBlur: 'blur(24px) saturate(150%)',
  canvasGlow:
    'radial-gradient(ellipse 60% 50% at 10% -15%, rgba(109, 95, 245, 0.16), transparent 55%),' +
    'radial-gradient(ellipse 50% 40% at 85% 0%, rgba(34, 211, 238, 0.06), transparent 55%)',
  shadowCard: '0 1px 2px rgba(0, 0, 0, 0.2), 0 4px 16px rgba(0, 0, 0, 0.25)',
  shadowFloat: '0 18px 48px rgba(0, 0, 0, 0.45)',
  gradientCta: 'linear-gradient(135deg, #6d5ff5 0%, #8b7ff8 100%)',
}

/** 近白：Quiet Signal — 结构直角、无玻璃、无发光、紫色仅作信号 */
export const quietTokens: ThemeTokens = {
  canvas: '#FAFAFA',
  canvasElevated: '#FFFFFF',
  surface: '#FFFFFF',
  surfaceSunken: '#F4F4F5',
  surfaceRaised: '#FFFFFF',
  surfaceInset: '#F4F4F5',

  line: 'rgba(10, 10, 10, 0.10)',
  lineStrong: 'rgba(10, 10, 10, 0.18)',

  textPrimary: '#0A0A0A',
  textSecondary: 'rgba(10, 10, 10, 0.72)',
  textTertiary: 'rgba(10, 10, 10, 0.56)',
  textQuaternary: 'rgba(10, 10, 10, 0.40)',
  textInverse: '#FFFFFF',

  signal: '#7C3AED',
  signalSoft: 'rgba(124, 58, 237, 0.10)',
  signalBorder: 'rgba(124, 58, 237, 0.30)',
  signalHover: '#6D28D9',
  success: '#15803D',
  warning: '#B45309',
  error: '#DC2626',
  info: '#1D4ED8',
  cyan: '#0E7490',

  radiusStructure: '0px',
  radiusControl: '4px',
  radiusAvatar: '50%',
  radiusPill: '999px',

  glassBlur: 'none',
  canvasGlow: 'none',
  shadowCard: '0 1px 2px rgba(10, 10, 10, 0.04)',
  shadowFloat: '0 12px 32px rgba(10, 10, 10, 0.12)',
  gradientCta: '#7C3AED',
}

export const themes: Record<ThemeName, ThemeTokens> = {
  dark: darkTokens,
  quiet: quietTokens,
}

const KEBAB_OVERRIDE: Partial<Record<keyof ThemeTokens, string>> = {
  // 效果类统一走 --fx-* 前缀，语义更清楚
}

/** camelCase → --kebab-case */
function toCssVarName(key: string): string {
  if (KEBAB_OVERRIDE[key as keyof ThemeTokens]) {
    return `--${KEBAB_OVERRIDE[key as keyof ThemeTokens]}`
  }
  return `--${key.replace(/([A-Z])/g, '-$1').toLowerCase()}`
}

/** 把一套 token 摊平成 CSS 变量声明，用于注入 :root */
export function toCssVars(tokens: ThemeTokens): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [k, v] of Object.entries(tokens)) {
    out[toCssVarName(k)] = v
  }
  return out
}
