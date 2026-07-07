import { apiBaseUrl } from '@/lib/apiBase'

export interface PlatformInfo {
  name: string
}

export async function fetchPlatformInfo(): Promise<PlatformInfo> {
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), 5000)

  try {
    const res = await fetch(`${apiBaseUrl}/platform/info`, {
      signal: controller.signal,
    })
    if (!res.ok) {
      throw new Error(`HTTP ${res.status}`)
    }
    const body = await res.json()
    // API wraps data: { data: { name: "..." } }
    return { name: body?.data?.name ?? 'TrustMesh' }
  } catch {
    // fallback on network error / timeout
    return { name: 'TrustMesh' }
  } finally {
    clearTimeout(timeout)
  }
}
