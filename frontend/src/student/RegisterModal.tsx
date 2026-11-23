import { useState } from 'react'
import { CustomSelect } from '../components/CustomSelect'
import { api, errorMessage } from '../lib/api'
import type { PublicConfig } from '../lib/types'

interface RegisterModalProps {
  courses: PublicConfig['courses']
  registerCareer: string
  setRegisterCareer: (career: string) => void
  passwordHint: string
  onClose: () => void
}

// Los campos viven aquí: al cerrar se desmonta y la próxima vez abre vacío, como antes.
export default function RegisterModal({
  courses,
  registerCareer,
  setRegisterCareer,
  passwordHint,
  onClose,
}: RegisterModalProps) {
  const [id, setId] = useState('')
  const [name, setName] = useState('')
  const [sexo, setSexo] = useState('Hombre')
  const [password, setPassword] = useState('')

  const handleRegister = async () => {
    let data
    try {
      data = await api.register({ id, name, sexo, password, career: registerCareer })
    } catch (e) {
      // Sin respuesta del servidor: se avisa y el formulario sigue abierto.
      alert(errorMessage(e))
      return
    }
    alert(data.message || data.error)
    if (data.error === undefined) onClose()
  }

  return (
    <div className="fixed inset-0 bg-black bg-opacity-60 flex justify-center items-center z-50 p-4 transition-opacity duration-300">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="register-title"
        className="bg-white p-8 rounded-2xl shadow-2xl w-full max-w-md space-y-4 transform transition-transform duration-300 scale-95 hover:scale-100"
      >
        <h2 id="register-title" className="text-2xl font-bold text-gray-800">Sign up</h2>
        <input
          className="w-full border p-3 border-gray-300 rounded-lg shadow-sm focus:ring-2 focus:ring-purple-500 transition-all duration-300"
          placeholder="Student ID"
          id="reg-id"
          value={id}
          onChange={(e) => setId(e.target.value)}
        />
        <input
          className="w-full border p-3 border-gray-300 rounded-lg shadow-sm focus:ring-2 focus:ring-purple-500 transition-all duration-300"
          placeholder="Full name"
          id="reg-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <div className="flex items-center justify-around text-gray-700">
          <label className="flex items-center">
            <input
              type="radio"
              name="sexo"
              value="Hombre"
              checked={sexo === 'Hombre'}
              onChange={(e) => setSexo(e.target.value)}
              className="mr-2"
            />{' '}
            Men
          </label>
          <label className="flex items-center">
            <input
              type="radio"
              name="sexo"
              value="Mujer"
              checked={sexo === 'Mujer'}
              onChange={(e) => setSexo(e.target.value)}
              className="mr-2"
            />{' '}
            Women
          </label>
        </div>

        <CustomSelect
          aria-label="Course"
          className="w-full p-3 border border-gray-300 rounded-lg shadow-sm focus:ring-2 focus:ring-purple-500 transition-all duration-300"
          value={registerCareer}
          onChange={(e) => setRegisterCareer(e.target.value)}
        >
          {courses.map((c) => (
            <option key={c.id} value={c.name}>
              {c.name}
            </option>
          ))}
        </CustomSelect>

        <input
          type="password"
          className="w-full border p-3 border-gray-300 rounded-lg shadow-sm focus:ring-2 focus:ring-purple-500 transition-all duration-300"
          placeholder={passwordHint}
          id="reg-pass"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        <button
          className="bg-orange-500 text-white px-4 py-3 rounded-full shadow-lg hover:bg-orange-600 transition-all duration-300 w-full font-semibold"
          onClick={handleRegister}
        >
          Register
        </button>
        <button
          className="text-sm text-gray-500 hover:underline transition-colors duration-300"
          onClick={onClose}
        >
          Cancel
        </button>
      </div>
    </div>
  )
}
