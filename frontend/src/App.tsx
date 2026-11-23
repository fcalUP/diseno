import { lazy, Suspense, useState } from 'react'
import { BrowserRouter, Route, Routes } from 'react-router'
import StudentApp from './student/StudentApp'
import type { AdminUser } from './lib/types'

// El panel va en su propio chunk: los alumnos nunca descargan su código.
const AdminApp = lazy(() => import('./admin/AdminApp'))

function Loading() {
  return (
    <div role="status" className="flex w-full min-h-screen self-stretch items-center justify-center">
      <span className="h-10 w-10 animate-spin rounded-full border-4 border-orange-200 border-t-orange-500" />
      <span className="sr-only">Cargando…</span>
    </div>
  )
}

export default function App() {
  const [admin, setAdmin] = useState<AdminUser | null>(null)
  return (
    <BrowserRouter>
      <Routes>
        <Route
          path="/admin/*"
          element={
            <Suspense fallback={<Loading />}>
              <AdminApp authenticatedAdmin={admin} onAdminSessionChange={setAdmin} />
            </Suspense>
          }
        />
        <Route path="*" element={<StudentApp onAdminSessionChange={setAdmin} />} />
      </Routes>
    </BrowserRouter>
  )
}
