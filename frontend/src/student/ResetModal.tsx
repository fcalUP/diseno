import type { PublicCourse } from '../lib/types'
import { CustomSelect } from '../components/CustomSelect'

interface ResetModalProps {
  resetId: string
  setResetId: (value: string) => void
  showCodeField: boolean
  resetCode: string
  setResetCode: (value: string) => void
  resetPass: string
  setResetPass: (value: string) => void
  passwordHint: string
  handleSendResetCode: () => void
  handleUpdatePassword: () => void
  onCancel: () => void
  courses: PublicCourse[]
  selectedCareer: string
  setSelectedCareer: (career: string) => void
}

export default function ResetModal({
  resetId,
  setResetId,
  showCodeField,
  resetCode,
  setResetCode,
  resetPass,
  setResetPass,
  passwordHint,
  handleSendResetCode,
  handleUpdatePassword,
  onCancel,
  courses,
  selectedCareer,
  setSelectedCareer,
}: ResetModalProps) {
  return (
    <div className="fixed inset-0 bg-black bg-opacity-60 flex justify-center items-center z-50 p-4 transition-opacity duration-300">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="reset-title"
        className="bg-white p-8 rounded-2xl shadow-2xl w-full max-w-md space-y-4 transform transition-transform duration-300 scale-95 hover:scale-100"
      >
        <h2 id="reset-title" className="text-2xl font-bold text-center text-gray-800">
          Password Recovery
        </h2>
        <CustomSelect
          aria-label="Course"
          className="w-full p-3 border border-gray-300 rounded-lg shadow-sm focus:ring-2 focus:ring-purple-500 transition-all duration-300"
          value={selectedCareer}
          onChange={(e) => setSelectedCareer(e.target.value)}
          disabled={showCodeField}
        >
          {courses.map((course) => <option key={course.id} value={course.name}>{course.name}</option>)}
        </CustomSelect>
        <input
          className="w-full border p-3 border-gray-300 rounded-lg shadow-sm focus:ring-2 focus:ring-purple-500 transition-all duration-300"
          placeholder="Student ID"
          value={resetId}
          onChange={(e) => setResetId(e.target.value)}
        />
        <button
          className="bg-purple-600 text-white px-4 py-3 rounded-full shadow-lg hover:bg-purple-700 transition-all duration-300 w-full font-semibold"
          onClick={handleSendResetCode}
        >
          Send code
        </button>

        {showCodeField && (
          <>
            <input
              className="w-full border p-3 border-gray-300 rounded-lg shadow-sm focus:ring-2 focus:ring-purple-500 transition-all duration-300"
              placeholder="Received code"
              value={resetCode}
              onChange={(e) => setResetCode(e.target.value)}
            />
            <input
              type="password"
              className="w-full border p-3 border-gray-300 rounded-lg shadow-sm focus:ring-2 focus:ring-purple-500 transition-all duration-300"
              placeholder={passwordHint}
              value={resetPass}
              onChange={(e) => setResetPass(e.target.value)}
            />
            <button
              className="bg-orange-500 text-white px-4 py-3 rounded-full shadow-lg hover:bg-orange-600 transition-all duration-300 w-full mt-2 font-semibold"
              onClick={handleUpdatePassword}
            >
              Update password
            </button>
          </>
        )}

        <button
          className="text-sm text-gray-500 hover:underline mt-2 transition-colors duration-300"
          onClick={onCancel}
        >
          Cancel
        </button>
      </div>
    </div>
  )
}
