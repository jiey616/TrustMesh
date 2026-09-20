import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// 移动端独立工程：不复用 frontend-v2 的任何组件或依赖，只消费同一套后端 API。
//
// 开发期 /api 走 Vite 代理而不是直连，原因有两条：
//   1. 生产是**自签证书**（裸 IP），浏览器对 XHR/fetch 的证书错误**没有"继续访问"入口**
//      （只有顶层导航才有 interstitial）⇒ 直连必然 net::ERR_CERT_AUTHORITY_INVALID；
//   2. 代理在 Node 侧发起请求，`secure: false` 可跳过证书校验 ⇒ 本机开发零摩擦。
// 生产部署时把构建产物放到与 API **同源**的 nginx 下（/api 反代），则默认走相对地址，
// 无需任何环境变量。
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5175,
    proxy: {
      '/api': {
        target: 'https://175.27.135.91',
        changeOrigin: true,
        secure: false,
      },
    },
  },
  build: {
    outDir: 'dist-mobile',
    emptyOutDir: true,
  },
  test: {
    environment: 'jsdom',
    globals: true,
    include: ['src/**/*.test.{ts,tsx}'],
  },
})
