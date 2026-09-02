import { create } from 'zustand'
import { fetchPlatformInfo } from '@/api/platform'

interface PlatformState {
  name: string
  loaded: boolean
  fetch: () => Promise<void>
}

export const usePlatformStore = create<PlatformState>()((set, get) => ({
  name: 'TrustMesh',
  loaded: false,
  fetch: async () => {
    if (get().loaded) return
    const info = await fetchPlatformInfo()
    set({ name: info.name, loaded: true })
  },
}))
