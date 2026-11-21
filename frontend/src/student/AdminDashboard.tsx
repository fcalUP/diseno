import type { Dispatch, SetStateAction } from 'react'
import { Link } from 'react-router'
import { api, errorMessage } from '../lib/api'
import { calcLevel } from '../lib/level'
import type { AdminStudent, CounterField, Level } from '../lib/types'

interface AdminDashboardProps {
  onClose: () => void
  studentsData: AdminStudent[]
  setStudentsData: Dispatch<SetStateAction<AdminStudent[]>>
  selectedCareer: string
  levels: Level[]
  onRefreshStudents: (career: string) => Promise<void>
}

// Panel rápido de la versión anterior. Las filas se editan sobre la lista del padre, así que
// el componente no tiene hooks y al recargar se descartan los cambios sin guardar, como antes.
export default function AdminDashboard({
  onClose,
  studentsData,
  setStudentsData,
  selectedCareer,
  levels,
  onRefreshStudents,
}: AdminDashboardProps) {
  const handleFieldChange = (index: number, field: CounterField, value: number) => {
    setStudentsData((prev) => prev.map((s, i) => (i === index ? { ...s, [field]: value } : s)))
  }

  const handleUpdateClick = async (student: AdminStudent) => {
    const { id, code, tareas, monedas, asistencias, games, exp } = student
    try {
      await api.admin.updateStudentData({ id, tareas, monedas, asistencias, games, exp })
    } catch (e) {
      alert(errorMessage(e))
      return
    }
    alert(`Data for ${code} updated successfully.`)
    await onRefreshStudents(selectedCareer)
  }

  return (
    <div className="fixed inset-0 bg-black bg-opacity-60 flex items-center justify-center z-50 p-4 transition-opacity duration-300">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="admin-panel-title"
        className="bg-white p-4 sm:p-8 rounded-2xl shadow-2xl w-full max-w-5xl mx-auto transform transition-transform duration-300 scale-95 hover:scale-100 h-5/6 flex flex-col"
      >
        <div className="flex justify-between items-center mb-4">
          <div className="flex flex-wrap items-baseline gap-x-3">
            <h3 id="admin-panel-title" className="text-xl sm:text-2xl font-bold text-gray-800">
              Administrator Panel - {selectedCareer}
            </h3>
            <Link to="/admin" className="text-sm text-blue-600 hover:underline">
              Panel completo →
            </Link>
          </div>
          <button
            aria-label="Close"
            onClick={onClose}
            className="text-red-500 text-xl font-bold"
          >
            X
          </button>
        </div>
        {/* En celulares la tabla pasa a tarjetas (estilos admin-table en index.css) */}
        <div className="flex-grow overflow-y-auto admin-table-container">
          <table className="w-full text-left table-auto border-collapse admin-table">
            <thead>
              <tr className="bg-gray-200 text-gray-700 font-semibold sticky top-0">
                <th className="p-2 border text-sm">ID</th>
                <th className="p-2 border text-sm">Name</th>
                <th className="p-2 border text-sm w-[100px]">
                  Tasks
                </th>
                <th className="p-2 border text-sm w-[100px]">
                  Coins
                </th>
                <th className="p-2 border text-sm w-[100px]">
                  Attendance
                </th>
                <th className="p-2 border text-sm w-[100px]">Games</th>
                <th className="p-2 border text-sm w-[80px]">EXP.</th>
                <th className="p-2 border text-sm w-[80px]">
                  Level
                </th>
                <th className="p-2 border text-sm w-[120px]">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody>
              {studentsData.map((student, index) => (
                <tr
                  key={student.id}
                  className="border-b hover:bg-gray-50"
                >
                  <td className="p-2 border text-sm" data-label="ID">
                    {student.code}
                  </td>
                  <td className="p-2 border text-sm" data-label="Name">
                    {student.name}
                  </td>
                  <td className="p-2 border" data-label="Tasks">
                    <div className="flex items-center justify-end md:justify-center">
                      <button
                        aria-label="Decrease Tasks"
                        onClick={() =>
                          handleFieldChange(
                            index,
                            'tareas',
                            student.tareas - 1,
                          )
                        }
                        className="bg-red-400 text-white w-6 h-6 rounded-full flex items-center justify-center text-sm"
                      >
                        -
                      </button>
                      <span className="w-10 sm:w-12 text-center mx-1 border-b py-0.5 text-sm">
                        {student.tareas}
                      </span>
                      <button
                        aria-label="Increase Tasks"
                        onClick={() =>
                          handleFieldChange(
                            index,
                            'tareas',
                            student.tareas + 1,
                          )
                        }
                        className="bg-green-400 text-white w-6 h-6 rounded-full flex items-center justify-center text-sm"
                      >
                        +
                      </button>
                    </div>
                  </td>
                  <td className="p-2 border" data-label="Coins">
                    <div className="flex items-center justify-end md:justify-center">
                      <button
                        aria-label="Decrease Coins"
                        onClick={() =>
                          handleFieldChange(
                            index,
                            'monedas',
                            student.monedas - 1,
                          )
                        }
                        className="bg-red-400 text-white w-6 h-6 rounded-full flex items-center justify-center text-sm"
                      >
                        -
                      </button>
                      <span className="w-10 sm:w-12 text-center mx-1 border-b py-0.5 text-sm">
                        {student.monedas}
                      </span>
                      <button
                        aria-label="Increase Coins"
                        onClick={() =>
                          handleFieldChange(
                            index,
                            'monedas',
                            student.monedas + 1,
                          )
                        }
                        className="bg-green-400 text-white w-6 h-6 rounded-full flex items-center justify-center text-sm"
                      >
                        +
                      </button>
                    </div>
                  </td>
                  <td className="p-2 border" data-label="Attendance">
                    <div className="flex items-center justify-end md:justify-center">
                      <button
                        aria-label="Decrease Attendance"
                        onClick={() =>
                          handleFieldChange(
                            index,
                            'asistencias',
                            student.asistencias - 1,
                          )
                        }
                        className="bg-red-400 text-white w-6 h-6 rounded-full flex items-center justify-center text-sm"
                      >
                        -
                      </button>
                      <span className="w-10 sm:w-12 text-center mx-1 border-b py-0.5 text-sm">
                        {student.asistencias}
                      </span>
                      <button
                        aria-label="Increase Attendance"
                        onClick={() =>
                          handleFieldChange(
                            index,
                            'asistencias',
                            student.asistencias + 1,
                          )
                        }
                        className="bg-green-400 text-white w-6 h-6 rounded-full flex items-center justify-center text-sm"
                      >
                        +
                      </button>
                    </div>
                  </td>
                  <td
                    className="p-2 border text-center"
                    data-label="Games"
                  >
                    <div className="flex items-center justify-end md:justify-center">
                      <button
                        aria-label="Decrease Games"
                        onClick={() =>
                          handleFieldChange(
                            index,
                            'games',
                            student.games - 1,
                          )
                        }
                        className="bg-red-400 text-white w-6 h-6 rounded-full flex items-center justify-center text-sm"
                      >
                        -
                      </button>
                      <span className="w-full text-center py-0.5 block text-sm">
                        {student.games}
                      </span>
                      <button
                        aria-label="Increase Games"
                        onClick={() =>
                          handleFieldChange(
                            index,
                            'games',
                            student.games + 1,
                          )
                        }
                        className="bg-green-400 text-white w-6 h-6 rounded-full flex items-center justify-center text-sm"
                      >
                        +
                      </button>
                    </div>
                  </td>
                  <td
                    className="p-2 border text-center"
                    data-label="EXP."
                  >
                    <span className="w-full text-center py-0.5 block text-sm">
                      {student.exp}
                    </span>
                  </td>
                  <td
                    className="p-2 border text-center"
                    data-label="Level"
                  >
                    {calcLevel(student.exp, levels)}
                  </td>
                  <td
                    className="p-2 border text-center"
                    data-label="Actions"
                  >
                    <button
                      className="bg-blue-500 text-white px-3 py-1 rounded-full text-xs hover:bg-blue-600"
                      onClick={() => handleUpdateClick(student)}
                    >
                      Update
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
