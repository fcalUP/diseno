import { useState } from 'react'
import { Link } from 'react-router'
import { ArrowLeft, Eye, EyeOff, LoaderCircle, ShieldCheck } from 'lucide-react'
import { ApiError, api, errorMessage } from '../lib/api'
import type { AdminUser } from '../lib/types'
import { TextField, cx, focusRing, inputClass } from './ui'

interface AdminLoginProps {
  onLoggedIn: (admin: AdminUser) => void
}

// Misma línea visual que el login de alumnos: tarjeta blanca, naranja y morado, esquinas redondas.
export default function AdminLogin({ onLoggedIn }: AdminLoginProps) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const submit = async () => {
    if (!username.trim() || !password) {
      setError('Escribe tu usuario y tu contraseña.')
      return
    }
    setBusy(true)
    setError('')
    try {
      const res = await api.admin.login({ username: username.trim(), password })
      onLoggedIn(res.admin)
    } catch (e) {
      setError(e instanceof ApiError && e.status === 401 ? 'Usuario o contraseña incorrectos.' : errorMessage(e))
      setBusy(false)
    }
  }

  return (
    <div className="flex w-full min-h-screen self-stretch items-center justify-center p-4">
      <div className="w-full max-w-md">
        <div className="overflow-hidden rounded-3xl bg-white shadow-2xl">
          <div className="bg-orange-500 px-8 py-7 text-center text-white">
            <span className="mx-auto mb-3 flex h-14 w-14 items-center justify-center rounded-2xl bg-white/20">
              <ShieldCheck aria-hidden="true" className="h-7 w-7" />
            </span>
            <h1 className="text-2xl font-bold">Panel de administración</h1>
            <p className="mt-1 text-sm text-orange-50">Cursos, alumnos, insignias y más</p>
          </div>

          <form
            className="space-y-4 px-8 py-8"
            noValidate
            onSubmit={(e) => {
              e.preventDefault()
              void submit()
            }}
          >
            <TextField
              label="Usuario"
              name="username"
              autoComplete="username"
              autoFocus
              autoCapitalize="none"
              spellCheck={false}
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              required
            />

            <div>
              <label htmlFor="admin-password" className="mb-1 block text-sm font-medium text-gray-700">
                Contraseña
              </label>
              <div className="relative">
                <input
                  id="admin-password"
                  name="password"
                  type={showPassword ? 'text' : 'password'}
                  autoComplete="current-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                  className={cx(inputClass, 'pr-11')}
                />
                <button
                  type="button"
                  onClick={() => setShowPassword((v) => !v)}
                  aria-label={showPassword ? 'Ocultar contraseña' : 'Mostrar contraseña'}
                  aria-pressed={showPassword}
                  className={cx(
                    'absolute right-2 top-1/2 -translate-y-1/2 rounded-full p-1.5 text-gray-400 hover:text-gray-600',
                    focusRing,
                  )}
                >
                  {showPassword ? (
                    <EyeOff aria-hidden="true" className="h-4 w-4" />
                  ) : (
                    <Eye aria-hidden="true" className="h-4 w-4" />
                  )}
                </button>
              </div>
            </div>

            {error && (
              <p role="alert" className="rounded-xl bg-red-50 px-4 py-3 text-sm text-red-700">
                {error}
              </p>
            )}

            <button
              type="submit"
              disabled={busy}
              className={cx(
                'flex w-full items-center justify-center gap-2 rounded-full bg-orange-500 px-4 py-3 font-semibold text-white shadow-lg transition-all duration-300 hover:bg-orange-600 disabled:cursor-not-allowed disabled:opacity-70',
                focusRing,
              )}
            >
              {busy && <LoaderCircle aria-hidden="true" className="h-5 w-5 animate-spin" />}
              {busy ? 'Entrando…' : 'Entrar'}
            </button>
          </form>
        </div>

        <div className="mt-6 text-center">
          <Link
            to="/"
            className={cx(
              'inline-flex items-center gap-2 rounded-full px-3 py-1.5 text-sm font-medium text-purple-600 transition-colors duration-300 hover:underline',
              focusRing,
            )}
          >
            <ArrowLeft aria-hidden="true" className="h-4 w-4" />
            Volver al portal
          </Link>
        </div>
      </div>
    </div>
  )
}
