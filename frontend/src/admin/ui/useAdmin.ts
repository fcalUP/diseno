import { createContext, useContext } from 'react'
import type { AdminUser } from '../../lib/types'

export interface AdminSession {
  admin: AdminUser
  logout: () => Promise<void>
  /** Vuelve a pedir /api/admin/me (p. ej. después de cambiar tu propio usuario). */
  refresh: () => void
}

export const AdminContext = createContext<AdminSession | null>(null)

export function useAdmin(): AdminSession {
  const session = useContext(AdminContext)
  if (!session) throw new Error('useAdmin debe usarse dentro del panel con sesión iniciada')
  return session
}
