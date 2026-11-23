import { lazy, useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { Navigate, Route, Routes } from 'react-router'
import { RefreshCw } from 'lucide-react'
import { ADMIN_SESSION_EXPIRED, ApiError, api, errorMessage } from '../lib/api'
import type { AdminUser } from '../lib/types'
import AdminLayout from './AdminLayout'
import { AdminContext, Button, Card, ConfirmProvider, Spinner, Toaster, type AdminSession } from './ui'

const Students = lazy(() => import('./pages/Students'))
const GenerateStudents = lazy(() => import('./pages/GenerateStudents'))
const Avatars = lazy(() => import('./pages/Avatars'))
const Courses = lazy(() => import('./pages/Courses'))
const Badges = lazy(() => import('./pages/Badges'))
const Purchases = lazy(() => import('./pages/Purchases'))
const Levels = lazy(() => import('./pages/Levels'))
const Import = lazy(() => import('./pages/Import'))
const Activity = lazy(() => import('./pages/Activity'))
const Settings = lazy(() => import('./pages/Settings'))
const Admins = lazy(() => import('./pages/Admins'))

type Session =
  | { status: 'loading' }
  | { status: 'out' }
  | { status: 'error'; message: string }
  | { status: 'in'; admin: AdminUser }

function FullScreen({ children }: { children: ReactNode }) {
  return <div className="flex w-full min-h-screen self-stretch items-center justify-center p-4">{children}</div>
}

export default function AdminApp({ authenticatedAdmin, onAdminSessionChange }: {
  authenticatedAdmin: AdminUser | null
  onAdminSessionChange: (admin: AdminUser | null) => void
}) {
  const [session, setSession] = useState<Session>(() => authenticatedAdmin ? { status: 'in', admin: authenticatedAdmin } : { status: 'loading' })
  const [check, setCheck] = useState(0)

  // Pregunta al servidor si ya hay sesión de administrador (y otra vez con refresh).
  useEffect(() => {
    // El login o /api/session ya revalidó la cuenta en el servidor.
    if (authenticatedAdmin && check === 0) return
    let alive = true
    api.admin.me().then(
      ({ admin }) => {
        if (alive) setSession({ status: 'in', admin })
      },
      (e: unknown) => {
        if (!alive) return
        setSession(
          e instanceof ApiError && e.status === 401
            ? { status: 'out' }
            : { status: 'error', message: errorMessage(e) },
        )
      },
    )
    return () => {
      alive = false
    }
  }, [authenticatedAdmin, check])

  // Si cualquier llamada del panel recibe 401, la sesión expiró: de vuelta al login.
  useEffect(() => {
    const expired = () => {
      onAdminSessionChange(null)
      setSession({ status: 'out' })
    }
    window.addEventListener(ADMIN_SESSION_EXPIRED, expired)
    return () => window.removeEventListener(ADMIN_SESSION_EXPIRED, expired)
  }, [onAdminSessionChange])

  // index.html dice lang="en" por el portal de alumnos; el panel está en español.
  useEffect(() => {
    const html = document.documentElement
    const previous = html.lang
    html.lang = 'es-MX'
    return () => {
      html.lang = previous
    }
  }, [])

  const refresh = useCallback(() => setCheck((n) => n + 1), [])

  const logout = useCallback(async () => {
    await api.admin.logout()
    onAdminSessionChange(null)
    setSession({ status: 'out' })
  }, [onAdminSessionChange])

  const admin = session.status === 'in' ? session.admin : null
  const ctx = useMemo<AdminSession | null>(() => (admin ? { admin, logout, refresh } : null), [admin, logout, refresh])

  let content: ReactNode
  if (session.status === 'loading') {
    content = (
      <FullScreen>
        <Spinner size="lg" label="Cargando panel…" showLabel />
      </FullScreen>
    )
  } else if (session.status === 'error') {
    content = (
      <FullScreen>
        <Card className="w-full max-w-md text-center">
          <h1 className="text-lg font-semibold text-gray-800">No se pudo abrir el panel</h1>
          <p className="mt-2 text-sm text-gray-500">{session.message}</p>
          <Button
            className="mt-5"
            icon={RefreshCw}
            onClick={() => {
              setSession({ status: 'loading' })
              refresh()
            }}
          >
            Reintentar
          </Button>
        </Card>
      </FullScreen>
    )
  } else if (!ctx) {
    content = <Navigate to="/" replace />
  } else {
    content = (
      <AdminContext value={ctx}>
        <Routes>
          <Route element={<AdminLayout />}>
            <Route index element={<Navigate to="/admin/alumnos" replace />} />
            <Route path="alumnos" element={<Students />} />
            <Route path="generar" element={<GenerateStudents />} />
            <Route path="avatares" element={<Avatars />} />
            <Route path="cursos" element={<Courses />} />
            <Route path="insignias" element={<Badges />} />
            <Route path="compras" element={<Purchases />} />
            <Route path="niveles" element={<Levels />} />
            <Route path="importar" element={<Import />} />
            <Route path="actividad" element={<Activity />} />
            <Route path="ajustes" element={<Settings />} />
            <Route path="administradores" element={<Admins />} />
            <Route path="*" element={<Navigate to="/admin" replace />} />
          </Route>
        </Routes>
      </AdminContext>
    )
  }

  return (
    <Toaster>
      <ConfirmProvider>{content}</ConfirmProvider>
    </Toaster>
  )
}
