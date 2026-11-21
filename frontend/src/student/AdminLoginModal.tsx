import { useState } from 'react'
import { api, errorMessage } from '../lib/api'
import type { AdminLoginResponse } from '../lib/types'

interface AdminLoginModalProps {
  show: boolean
  onClose: () => void
  onAdminLoginSuccess: (career: string) => Promise<void>
  selectedCareer: string
}

// Siempre montado, como antes: usuario y contraseña se conservan entre aperturas.
export default function AdminLoginModal({
  show,
  onClose,
  onAdminLoginSuccess,
  selectedCareer,
}: AdminLoginModalProps) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')

  const handleAdminLogin = async () => {
    let data: AdminLoginResponse
    try {
      data = await api.admin.login({ username, password })
    } catch (e) {
      alert(errorMessage(e))
      return
    }
    alert(data.message)
    void onAdminLoginSuccess(selectedCareer)
    onClose()
  }

  if (!show) return null

  return (
    <div className="fixed inset-0 bg-black bg-opacity-60 flex justify-center items-center z-50 p-4 transition-opacity duration-300">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="admin-login-title"
        className="bg-white p-8 rounded-2xl shadow-2xl w-full max-w-md space-y-4 transform transition-transform duration-300 scale-95 hover:scale-100"
      >
        <h2 id="admin-login-title" className="text-2xl font-bold text-center text-gray-800">
          Administrator Access
        </h2>
        <input
          className="w-full border p-3 border-gray-300 rounded-lg shadow-sm focus:ring-2 focus:ring-blue-500 transition-all duration-300"
          placeholder="Username"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
        />
        <input
          type="password"
          className="w-full border p-3 border-gray-300 rounded-lg shadow-sm focus:ring-2 focus:ring-blue-500 transition-all duration-300"
          placeholder="Password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        <button
          className="bg-blue-600 text-white px-4 py-3 rounded-full shadow-lg hover:bg-blue-700 transition-all duration-300 w-full font-semibold"
          onClick={handleAdminLogin}
        >
          Log in (Admin)
        </button>
        <button
          className="text-sm text-gray-500 hover:underline mt-2 transition-colors duration-300"
          onClick={onClose}
        >
          Cancel
        </button>
      </div>
    </div>
  )
}
