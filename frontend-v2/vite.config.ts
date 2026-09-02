import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

export default defineConfig({
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
