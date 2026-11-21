import { useEffect, useState } from 'react'
import { NavLink, Outlet, useLocation } from 'react-router'
import {
  Activity,
  Award,
  BookOpen,
  CircleUser,
  ExternalLink,
  FileSpreadsheet,
  Images,
  LayoutDashboard,
  LogOut,
  Menu,
  Settings,
  ShieldCheck,
  ShoppingCart,
  TrendingUp,
  Trophy,
  UserPlus,
  Users,
  X,
  type LucideIcon,
} from 'lucide-react'
import { Button, IconButton, cx, focusRing, useAdmin } from './ui'

interface NavItem {
  to: string
  label: string
  icon: LucideIcon
  end?: boolean
}

// Mismo orden que docs/spec.md §6.3.
const NAV: NavItem[] = [
  { to: '/admin', label: 'Resumen', icon: LayoutDashboard, end: true },
  { to: '/admin/alumnos', label: 'Alumnos', icon: Users },
  { to: '/admin/generar', label: 'Generar alumnos', icon: UserPlus },
  { to: '/admin/avatares', label: 'Avatares', icon: Images },
  { to: '/admin/cursos', label: 'Cursos', icon: BookOpen },
  { to: '/admin/insignias', label: 'Insignias', icon: Award },
  { to: '/admin/compras', label: 'Compras', icon: ShoppingCart },
  { to: '/admin/niveles', label: 'Niveles', icon: TrendingUp },
  { to: '/admin/importar', label: 'Importar', icon: FileSpreadsheet },
  { to: '/admin/actividad', label: 'Actividad', icon: Activity },
  { to: '/admin/ajustes', label: 'Ajustes', icon: Settings },
  { to: '/admin/administradores', label: 'Administradores', icon: ShieldCheck },
]

function titleFor(pathname: string): string {
  const path = pathname.replace(/\/+$/, '')
  const item = NAV.find((n) => (n.end ? path === n.to : path === n.to || path.startsWith(`${n.to}/`)))
  return item?.label ?? 'Panel'
}

export default function AdminLayout() {
  const { admin, logout } = useAdmin()
  const { pathname } = useLocation()
  const [menuOpen, setMenuOpen] = useState(false)
  const [leaving, setLeaving] = useState(false)
  const title = titleFor(pathname)

  useEffect(() => {
    const previous = document.title
    document.title = `${title} · Panel de administración`
    return () => {
      document.title = previous
    }
  }, [title])

  // En celular: al abrir el menú el foco entra a la navegación; Esc lo cierra y regresa al botón.
  useEffect(() => {
    if (!menuOpen) return
    const link =
      document.querySelector<HTMLElement>('#admin-sidebar a[aria-current="page"]') ??
      document.querySelector<HTMLElement>('#admin-sidebar a')
    link?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      setMenuOpen(false)
      document.getElementById('admin-menu-button')?.focus()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [menuOpen])

  return (
    <div className="flex w-full min-h-screen self-stretch bg-gray-50 text-gray-800">
      {menuOpen && (
        <div className="fixed inset-0 z-30 bg-black/40 lg:hidden" aria-hidden="true" onClick={() => setMenuOpen(false)} />
      )}

      <aside
        id="admin-sidebar"
        className={cx(
          'fixed inset-y-0 left-0 z-40 flex w-64 flex-col border-r border-gray-200 bg-white transition-transform duration-200 lg:visible lg:translate-x-0',
          menuOpen ? 'visible translate-x-0 shadow-2xl' : 'invisible -translate-x-full',
        )}
      >
        <div className="flex h-16 shrink-0 items-center justify-between gap-2 border-b border-gray-100 px-5">
          <div className="flex items-center gap-3">
            <span className="flex h-9 w-9 items-center justify-center rounded-xl bg-orange-500 text-white shadow-sm">
              <Trophy aria-hidden="true" className="h-5 w-5" />
            </span>
            <div className="leading-tight">
              <p className="font-bold text-gray-800">Gamification</p>
              <p className="text-xs text-gray-500">Administración</p>
            </div>
          </div>
          <IconButton icon={X} label="Cerrar menú" size="sm" className="lg:hidden" onClick={() => setMenuOpen(false)} />
        </div>

        <nav aria-label="Secciones del panel" className="flex-1 overflow-y-auto p-3">
          <ul className="space-y-1">
            {NAV.map(({ to, label, icon: Icon, end }) => (
              <li key={to}>
                <NavLink
                  to={to}
                  end={end}
                  onClick={() => setMenuOpen(false)}
                  className={({ isActive }) =>
                    cx(
                      'flex items-center gap-3 rounded-xl px-3 py-2 text-sm font-medium transition-colors',
                      focusRing,
                      isActive ? 'bg-orange-50 text-orange-600' : 'text-gray-600 hover:bg-gray-100 hover:text-gray-900',
                    )
                  }
                >
                  <Icon aria-hidden="true" className="h-[18px] w-[18px] shrink-0" />
                  {label}
                </NavLink>
              </li>
            ))}
          </ul>
        </nav>

        {/* En pantallas chicas el usuario no cabe en la barra superior. */}
        <p className="flex items-center gap-2 border-t border-gray-100 px-5 py-4 text-sm text-gray-600 md:hidden">
          <CircleUser aria-hidden="true" className="h-5 w-5 text-gray-400" />
          {admin.username}
        </p>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col lg:pl-64">
        <header className="sticky top-0 z-20 flex h-16 items-center gap-2 border-b border-gray-200 bg-white/90 px-4 backdrop-blur sm:gap-3 sm:px-6">
          <IconButton
            id="admin-menu-button"
            icon={Menu}
            label="Abrir menú"
            className="lg:hidden"
            aria-expanded={menuOpen}
            aria-controls="admin-sidebar"
            onClick={() => setMenuOpen(true)}
          />
          <p className="min-w-0 flex-1 truncate text-lg font-semibold text-gray-800">{title}</p>
          <span className="hidden items-center gap-2 text-sm text-gray-600 md:flex">
            <CircleUser aria-hidden="true" className="h-5 w-5 text-gray-400" />
            {admin.username}
          </span>
          <a
            href="/"
            target="_blank"
            rel="noreferrer"
            aria-label="Ver portal (se abre en otra pestaña)"
            className={cx(
              'inline-flex items-center gap-1.5 rounded-full px-3 py-1.5 text-sm font-medium text-purple-600 hover:bg-purple-50',
              focusRing,
            )}
          >
            <ExternalLink aria-hidden="true" className="h-4 w-4" />
            <span className="hidden sm:inline">Ver portal</span>
          </a>
          <Button
            variant="ghost"
            size="sm"
            icon={LogOut}
            loading={leaving}
            aria-label="Cerrar sesión"
            onClick={() => {
              setLeaving(true)
              void logout()
            }}
          >
            <span className="hidden sm:inline">Cerrar sesión</span>
          </Button>
        </header>

        <main className="mx-auto w-full max-w-screen-2xl flex-1 p-4 sm:p-6 lg:p-8">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
