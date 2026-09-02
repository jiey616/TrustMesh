import { apiClient } from './client'

export interface PlatformInfo {
  name: string
}

export async function fetchPlatformInfo(): Promise<PlatformInfo> {
  try {
    const res = await apiClient.get('/api/v1/platform/info').json<{ data: { name: string } }>()
    return { name: res.data?.name ?? 'TrustMesh' }
  } catch {
    return { name: 'TrustMesh' }
  }
}
