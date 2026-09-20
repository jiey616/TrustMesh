import { useAuthStore } from '@/stores/authStore'
import { isNativeApp } from './native'
import { DEFAULT_NATIVE_API_BASE, resolveApiBase } from './resolveApiBase'

/** 展示当前 API 基址（同源时显示"同源"，避免暴露内部路径细节）。 */
export function apiBaseLabel(): string {
  const base = resolveApiBase({
    override: useAuthStore.getState().apiBaseOverride,
    envBase: import.meta.env.VITE_API_BASE_URL,
    nativeDefault: isNativeApp() ? DEFAULT_NATIVE_API_BASE : null,
  })
  return base.startsWith('/') ? '同源' : base.replace(/\/+$/, '')
}
