import type { PublicConfig } from '../lib/types'

interface LoginScreenProps {
  courses: PublicConfig['courses']
  features: PublicConfig['features']
  studentId: string
  setStudentId: (value: string) => void
  password: string
  setPassword: (value: string) => void
  handleLogin: () => void
  loading: boolean
  error: string
  setShowRegister: (show: boolean) => void
  setShowReset: (show: boolean) => void
  setShowAdminLoginModal: (show: boolean) => void
}

export default function LoginScreen({
  courses,
  features,
  studentId,
  setStudentId,
  password,
  setPassword,
  handleLogin,
  loading,
  error,
  setShowRegister,
  setShowReset,
  setShowAdminLoginModal,
}: LoginScreenProps) {
  // Conserva las imágenes del portal; el curso se detecta después de autenticar.
  const course = courses[0]
  const sideImage = course?.sideImageUrl ?? ''
  const centerLogo = course?.logoUrl ?? ''

  return (
    <>
      <div className="flex flex-col md:flex-row w-screen h-screen">
        <div
          className="hidden md:block md:w-1/2 bg-cover bg-center"
          style={{ backgroundImage: sideImage ? `url('${sideImage}')` : undefined }}
        ></div>
        <div className="w-full md:w-1/2 flex items-center justify-center bg-white p-8">
          <form
            className="w-full max-w-md px-6 py-8 text-center"
            onSubmit={(e) => {
              e.preventDefault()
              handleLogin()
            }}
            aria-busy={loading}
          >
            <img
              src={centerLogo || undefined}
              alt="Logo"
              className="mx-auto w-32 h-32 mb-4"
            />
            <h2 className="text-3xl font-bold mb-4 text-gray-800">
              Student Portal
            </h2>

            <input
              className="w-full p-3 mb-2 border border-gray-300 rounded-lg shadow-sm focus:ring-2 focus:ring-purple-500 transition-all duration-300"
              placeholder="Student ID"
              aria-label="Student ID"
              autoComplete="username"
              autoCapitalize="none"
              spellCheck={false}
              required
              disabled={loading}
              value={studentId}
              onChange={(e) => setStudentId(e.target.value)}
            />
            <input
              type="password"
              className="w-full p-3 mb-4 border border-gray-300 rounded-lg shadow-sm focus:ring-2 focus:ring-purple-500 transition-all duration-300"
              placeholder="Password"
              aria-label="Password"
              autoComplete="current-password"
              required
              disabled={loading}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
            <button
              type="submit"
              className="bg-orange-500 text-white px-4 py-3 rounded-full shadow-lg hover:bg-orange-600 transition-all duration-300 w-full font-semibold"
              disabled={loading}
            >
              {loading ? 'Logging in…' : 'Log in'}
            </button>
            {error && <p role="alert" className="mt-3 text-sm text-red-600">{error}</p>}

            <div className="text-sm mt-6">
              {features.registration && (
                <button
                  type="button"
                  onClick={() => setShowRegister(true)}
                  className="text-purple-600 hover:underline mr-4 transition-colors duration-300"
                >
                  Sign up
                </button>
              )}
              {features.passwordReset && (
                <button
                  type="button"
                  onClick={() => setShowReset(true)}
                  className="text-red-500 hover:underline transition-colors duration-300"
                >
                  Forgot your password?
                </button>
              )}
              <button
                type="button"
                onClick={() => setShowAdminLoginModal(true)}
                className="text-blue-500 hover:underline ml-4 transition-colors duration-300"
              >
                Administrator
              </button>
            </div>
          </form>
        </div>
      </div>
    </>
  )
}
