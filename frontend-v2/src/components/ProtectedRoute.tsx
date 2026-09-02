import { Navigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import type { ReactNode } from 'react'

interface Props {
  children: ReactNode
}

export function ProtectedRoute({ children }: Props) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)

  if (!isAuthenticated()) {
    return <Navigate to="/login" replace />
  }

  return <>{children}</>
}
