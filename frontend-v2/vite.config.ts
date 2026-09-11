import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

export default defineConfig({
  // Web 部署在根路径：必须用绝对 base，否则二级路由（/agents/:id 等）刷新时
  // 相对资源 ./assets/* 会解析成 /agents/assets/* 被 nginx fallback 到 index.html → MIME 错误白屏。
  // 桌面版（Electron file:// loadFile）需要相对路径，由 desktop:pack 单独传 --base=./ 覆盖。
  base: '/',
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  cacheDir: '.vite-cache',
  optimizeDeps: {
    include: ['react-markdown', 'remark-gfm', 'dayjs/plugin/relativeTime'],
  },
  build: {
    outDir: 'dist-v2',
    emptyOutDir: false,
  },
  server: {
    port: 5174,
    proxy: {
      '/api/v1': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
