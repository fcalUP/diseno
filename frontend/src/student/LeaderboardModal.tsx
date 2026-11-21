import type { LeaderboardRow } from '../lib/types'

interface LeaderboardModalProps {
  leaderboardData: LeaderboardRow[]
  onClose: () => void
}

// La columna "ID" muestra el nombre, igual que antes: el leaderboard no expone matrículas.
export default function LeaderboardModal({ leaderboardData, onClose }: LeaderboardModalProps) {
  return (
    <div className="fixed inset-0 flex items-center justify-center z-50 bg-black bg-opacity-50">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="leaderboard-title"
        className="bg-white w-full max-w-2xl p-6 rounded-2xl transform transition-transform duration-500 animate-slideIn"
      >
        <div className="flex justify-between items-center mb-4">
          <h3 id="leaderboard-title" className="text-2xl font-bold text-gray-800">
            Leaderboard
          </h3>
          <button
            aria-label="Close"
            onClick={onClose}
            className="text-red-500 text-xl font-bold"
          >
            X
          </button>
        </div>
        <table className="w-full text-left table-auto border-collapse">
          <thead>
            <tr className="text-gray-700 font-semibold border-b">
              <th className="p-2">Ranking</th>
              <th className="p-2">ID</th>
              <th className="p-2">Badges</th>
              <th className="p-2">Level</th>
            </tr>
          </thead>
          <tbody>
            {leaderboardData.map((student, i) => (
              <tr key={i} className="border-b">
                <td className="p-2">{student.rank}</td>
                <td className="p-2">{student.name}</td>
                <td className="p-2">{student.badges}</td>
                <td className="p-2">{student.level}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
