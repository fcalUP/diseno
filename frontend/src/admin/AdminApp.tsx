import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { Navigate, Route, Routes } from 'react-router'
import { RefreshCw } from 'lucide-react'
import { ADMIN_SESSION_EXPIRED, ApiError, api, errorMessage } from '../lib/api'
import type { AdminUser } from '../lib/types'
import AdminLayout from './AdminLayout'
import AdminLogin from './AdminLogin'
import Activity from './pages/Activity'
import Admins from './pages/Admins'
import Avatars from './pages/Avatars'
import Badges from './pages/Badges'
import Courses from './pages/Courses'
import Dashboard from './pages/Dashboard'
import GenerateStudents from './pages/GenerateStudents'
import Import from './pages/Import'
import Levels from './pages/Levels'
import Purchases from './pages/Purchases'
import Settings from './pages/Settings'
import Students from './pages/Students'
import { AdminContext, Button, Card, ConfirmProvider, Spinner, Toaster, type AdminSession } from './ui'

type Session =
  | { status: 'loading' }
  | { status: 'out' }
  | { status: 'error'; message: string }
  | { status: 'in'; admin: AdminUser }

function FullScreen({ children }: { children: ReactNode }) {
  return <div className="flex w-full min-h-screen self-stretch items-center justify-center p-4">{children}</div>
}

export default function AdminApp() {
  const [session, setSession] = useState<Session>({ status: 'loading' })
  const [check, setCheck] = useState(0)

  // Pregunta al servidor si ya hay sesión de administrador (y otra vez con refresh).
  useEffect(() => {
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
  }, [check])

  // Si cualquier llamada del panel recibe 401, la sesión expiró: de vuelta al login.
  useEffect(() => {
    const expired = () => setSession({ status: 'out' })
    window.addEventListener(ADMIN_SESSION_EXPIRED, expired)
    return () => window.removeEventListener(ADMIN_SESSION_EXPIRED, expired)
  }, [])

  const refresh = useCallback(() => setCheck((n) => n + 1), [])

  const logout = useCallback(async () => {
    try {
      await api.admin.logout()
    } catch {
      // Aunque falle la petición, se sale del panel en este navegador.
    }
    setSession({ status: 'out' })
  }, [])

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
    content = <AdminLogin onLoggedIn={(user) => setSession({ status: 'in', admin: user })} />
  } else {
    content = (
      <AdminContext value={ctx}>
        <Routes>
          <Route element={<AdminLayout />}>
            <Route index element={<Dashboard />} />
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
