import type { PublicConfig } from '../lib/types'

interface LoginScreenProps {
  courses: PublicConfig['courses']
  features: PublicConfig['features']
  studentId: string
  setStudentId: (value: string) => void
  password: string
  setPassword: (value: string) => void
  handleLogin: () => void
  setShowRegister: (show: boolean) => void
  setShowReset: (show: boolean) => void
  selectedCareer: string
  setSelectedCareer: (career: string) => void
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
  setShowRegister,
  setShowReset,
  selectedCareer,
  setSelectedCareer,
  setShowAdminLoginModal,
}: LoginScreenProps) {
  // Imagen lateral y logo del curso elegido; antes iban fijos en el código.
  const course = courses.find((c) => c.name === selectedCareer)
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
          <div className="w-full max-w-md px-6 py-8 text-center">
            <img
              src={centerLogo || undefined}
              alt="Logo"
              className="mx-auto w-32 h-32 mb-4"
            />
            <h2 className="text-3xl font-bold mb-4 text-gray-800">
              {selectedCareer}
            </h2>

            <select
              aria-label="Course"
              className="w-full p-3 mb-4 border border-gray-300 rounded-lg shadow-sm focus:ring-2 focus:ring-purple-500 transition-all duration-300"
              value={selectedCareer}
              onChange={(e) => setSelectedCareer(e.target.value)}
            >
              {courses.map((c) => (
                <option key={c.id} value={c.name}>
                  {c.name}
                </option>
              ))}
            </select>

            <input
              className="w-full p-3 mb-2 border border-gray-300 rounded-lg shadow-sm focus:ring-2 focus:ring-purple-500 transition-all duration-300"
              placeholder="Student ID"
              value={studentId}
              onChange={(e) => setStudentId(e.target.value)}
            />
            <input
              type="password"
              className="w-full p-3 mb-4 border border-gray-300 rounded-lg shadow-sm focus:ring-2 focus:ring-purple-500 transition-all duration-300"
              placeholder="Password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
            <button
              className="bg-orange-500 text-white px-4 py-3 rounded-full shadow-lg hover:bg-orange-600 transition-all duration-300 w-full font-semibold"
              onClick={handleLogin}
            >
              Log in
            </button>

            <div className="text-sm mt-6">
              {features.registration && (
                <button
                  onClick={() => setShowRegister(true)}
                  className="text-purple-600 hover:underline mr-4 transition-colors duration-300"
                >
                  Sign up
                </button>
              )}
              {features.passwordReset && (
                <button
                  onClick={() => setShowReset(true)}
                  className="text-red-500 hover:underline transition-colors duration-300"
                >
                  Forgot your password?
                </button>
              )}
              <button
                onClick={() => setShowAdminLoginModal(true)}
                className="text-blue-500 hover:underline ml-4 transition-colors duration-300"
              >
                Administrator
              </button>
            </div>
          </div>
        </div>
      </div>
    </>
  )
}
