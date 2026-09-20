import type { CapacitorConfig } from '@capacitor/cli'

/**
 * TrustMesh 移动端（frontend-mobile 独立子项目）Capacitor 配置。
 *
 * 关键约定（沿用旧方案验证过的结论，见备份 docs/mobile-android-app-plan-2026-09-19.md）：
 * 1. webDir 用 `dist-mobile`（本子项目自己的构建产物目录，与桌面端 dist-v2 无关）。
 * 2. `androidScheme` 保持默认 `https`（WebView 源为 https://localhost），应用内路由必须走 HashRouter。
 * 3. 生产自签证书不在这里配，由 android-config/network_security_config.xml 预置，
 *    经 android-config/apply.mjs 套用（含 AndroidManifest 属性注入）。
 * 4. 不配 server.url：壳内加载本地资产，API 走可配的服务端地址。
 * 5. 安全区靠 CSS：Capacitor 8 SystemBars 插件注入 --safe-area-inset-*（见 styles/index.css）。
 *    🔴 别加 adjustMarginsForEdgeToEdge —— 8.5.2 里不存在该选项（静默无效，真机验证过）。
 */
const config: CapacitorConfig = {
  appId: 'com.trustmesh.mobile',
  appName: 'TrustMesh',
  webDir: 'dist-mobile',
  android: {
    allowMixedContent: false,
  },
}

export default config
