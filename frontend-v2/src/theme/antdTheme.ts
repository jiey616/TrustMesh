import { theme as antdTheme, type ThemeConfig } from 'antd'
import { themes, type ThemeName } from './tokens'

export const ANTD_FONT_FAMILY =
  "Inter, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif"

/**
 * 生成 antd 的 ConfigProvider theme。
 *
 * 与 index.css 的分工：
 *  - 这里管「antd 组件自身」的色板、圆角、控件高度；
 *  - index.css 管「组件之上」的观感层（毛玻璃、发光、光晕），按 [data-theme] 分支。
 */
export function buildAntdTheme(name: ThemeName): ThemeConfig {
  const t = themes[name]
  const isDark = name === 'dark'

  return {
    algorithm: isDark ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
    token: {
      colorPrimary: t.signal,
      colorInfo: t.info,
      colorSuccess: t.success,
      colorWarning: t.warning,
      colorError: t.error,
      colorTextBase: isDark ? '#f4f4f8' : '#0A0A0A',
      colorBgBase: t.canvas,
      colorBgContainer: isDark ? '#12121d' : '#FFFFFF',
      colorBgElevated: isDark ? '#171722' : '#FFFFFF',
      colorBgLayout: t.canvas,
      colorBorder: isDark ? '#232330' : 'rgba(10, 10, 10, 0.14)',
      colorBorderSecondary: isDark ? '#1b1b26' : 'rgba(10, 10, 10, 0.10)',
      colorText: t.textPrimary,
      colorTextSecondary: t.textSecondary,
      colorTextTertiary: t.textTertiary,
      colorTextQuaternary: t.textQuaternary,
      colorSplit: isDark ? '#232330' : 'rgba(10, 10, 10, 0.10)',
      colorPrimaryBg: t.signalSoft,
      colorPrimaryBorder: t.signalBorder,
      // 混合改造：控件统一 4px，卡片/面板走结构直角
      borderRadius: 4,
      borderRadiusLG: 4,
      borderRadiusSM: 4,
      fontFamily: ANTD_FONT_FAMILY,
      fontSize: 14,
      controlHeight: 36,
    },
    components: {
      Layout: {
        headerBg: t.canvasElevated,
        siderBg: t.canvas,
        bodyBg: t.canvas,
      },
      Menu: isDark
        ? {
            darkItemBg: t.canvas,
            darkItemSelectedBg: t.signalSoft,
            darkItemSelectedColor: t.signalHover,
            darkItemColor: t.textTertiary,
            darkItemHoverColor: t.textPrimary,
          }
        : {
            itemBg: t.canvas,
            itemSelectedBg: t.signalSoft,
            itemSelectedColor: t.signal,
            itemColor: t.textTertiary,
            itemHoverColor: t.textPrimary,
          },
      Card: {
        colorBgContainer: isDark ? '#12121d' : '#FFFFFF',
      },
      Table: {
        headerBg: isDark ? '#0f0f18' : '#F4F4F5',
        rowHoverBg: isDark ? '#171722' : '#F4F4F5',
        borderColor: isDark ? '#232330' : 'rgba(10, 10, 10, 0.10)',
      },
      Modal: {
        contentBg: isDark ? '#171722' : '#FFFFFF',
        headerBg: isDark ? '#171722' : '#FFFFFF',
      },
      Drawer: {
        colorBgElevated: isDark ? '#12121d' : '#FFFFFF',
      },
      Dropdown: {
        colorBgElevated: isDark ? '#171722' : '#FFFFFF',
      },
      Select: {
        colorBgElevated: isDark ? '#171722' : '#FFFFFF',
      },
      Button: {
        colorBgContainer: isDark ? '#12121d' : '#FFFFFF',
        colorBorder: isDark ? '#232330' : 'rgba(10, 10, 10, 0.18)',
        primaryShadow: 'none',
      },
      Input: {
        colorBgContainer: isDark ? '#12121d' : '#FFFFFF',
      },
      Tooltip: {
        colorBgSpotlight: isDark ? '#232331' : '#18181B',
      },
    },
  }
}
