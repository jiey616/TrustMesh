import { defineConfig } from 'vitest/config'
import path from 'path'

/**
 * frontend-v2 单测配置（T1.5 测试护栏）。
 *
 * 与 vite.config.ts 保持一致的 `@` 别名；不做任何浏览器渲染依赖
 * （不安装 @testing-library/*），测的是「多租户隔离不变量」而非像素：
 *   - apiClient 请求头是否按当前租户带上 X-Org-Id / Authorization
 *   - 401 → refresh → 重试链路
 *   - 登出是否清空租户上下文
 *   - 跨租户缓存失效原语（queryClient.clear / removeQueries）与源码级契约
 *
 * VITE_API_BASE_URL 固定为绝对地址（含 /api/v1）：ky 内部会构造 Request，
 * Node 的 undici 只接受绝对 URL，相对 prefixUrl 会在单测环境下直接抛错。
 * 取值与真实部署形态一致（容器里由 VITE_API_BASE_URL 注入同源 /api/v1）。
 */
export default defineConfig({
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.{ts,tsx}'],
    env: {
      VITE_API_BASE_URL: 'http://localhost:8080/api/v1',
    },
  },
})
