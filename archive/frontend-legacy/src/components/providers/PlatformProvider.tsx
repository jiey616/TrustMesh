import { useEffect } from 'react'
import { usePlatformStore } from '@/stores/platformStore'

export function PlatformProvider({ children }: { children: React.ReactNode }) {
  const { loaded, name, fetch } = usePlatformStore()

  useEffect(() => {
    fetch()
  }, [fetch])

  // Set document.title once loaded (or use fallback)
  useEffect(() => {
    document.title = loaded ? name : 'TrustMesh'
  }, [loaded, name])

  return <>{children}</>
}
