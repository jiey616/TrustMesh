import { useEffect } from 'react'
import { usePlatformStore } from '@/stores/platformStore'

export function PlatformProvider({ children }: { children: React.ReactNode }) {
  const loaded = usePlatformStore((s) => s.loaded)
  const name = usePlatformStore((s) => s.name)
  const fetchName = usePlatformStore((s) => s.fetch)

  useEffect(() => {
    fetchName()
  }, [fetchName])

  useEffect(() => {
    document.title = loaded ? name : 'TrustMesh'
  }, [loaded, name])

  return <>{children}</>
}
