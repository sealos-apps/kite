import { ReactNode } from 'react'
import { useAuth } from '@/contexts/auth-context'
import { Navigate, useLocation } from 'react-router-dom'

interface ProtectedRouteProps {
  children: ReactNode
}

export function ProtectedRoute({ children }: ProtectedRouteProps) {
  const { user, isLoading } = useAuth()
  const location = useLocation()

  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="animate-spin rounded-full h-32 w-32 border-b-2 border-primary"></div>
      </div>
    )
  }

  if (!user) {
    return (
      <Navigate
        to="/login?reason=unauthenticated"
        state={{ from: location }}
        replace
      />
    )
  }

  return <>{children}</>
}
