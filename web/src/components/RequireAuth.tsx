import type { ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router-dom'
import { useAuthStore } from '@/stores/auth'
import { Spinner } from './ui/Spinner'

/**
 * Client-side gate. It is a UX affordance only: every protected endpoint enforces auth on the
 * server, so hiding a route is never the security boundary (NFR-FESEC-05).
 */
export function RequireAuth({ children }: { children: ReactNode }) {
  const status = useAuthStore((state) => state.status)
  const location = useLocation()

  if (status === 'unknown') {
    return (
      <div className="flex h-[60dvh] items-center justify-center text-mist-500">
        <Spinner size={22} />
      </div>
    )
  }

  if (status === 'guest') {
    const redirect = encodeURIComponent(`${location.pathname}${location.search}`)
    return <Navigate to={`/login?redirect=${redirect}`} replace />
  }

  return <>{children}</>
}
