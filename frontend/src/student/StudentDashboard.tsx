import type { LevelProgress } from '../lib/level'
import type { PublicConfig, PublicCourse, Student } from '../lib/types'
import CourseSwitcher from './CourseSwitcher'

interface StudentDashboardProps {
  studentData: Student
  progress: LevelProgress
  features: PublicConfig['features']
  fetchLeaderboard: () => void
  openStore: () => void
  handleLogout: () => void
  courses: PublicCourse[]
  currentCourse: PublicCourse
  courseBusy: boolean
  courseError: string
  onSwitchCourse: (courseId: number) => void
  onChooseCourse: () => void
}

export default function StudentDashboard({
  studentData,
  progress,
  features,
  fetchLeaderboard,
  openStore,
  handleLogout,
  courses,
  currentCourse,
  courseBusy,
  courseError,
  onSwitchCourse,
  onChooseCourse,
}: StudentDashboardProps) {
  return (
    <div className="w-full max-w-3xl bg-white rounded-3xl shadow-2xl">
      <nav aria-label="Course navigation" className="flex items-center border-b border-gray-100 px-3 py-2 sm:px-5">
        <CourseSwitcher courses={courses} currentCourse={currentCourse} busy={courseBusy} onSelect={onSwitchCourse} onChooseCourse={onChooseCourse} />
      </nav>
      {courseError && <p role="alert" className="px-6 py-3 text-sm text-red-600">{courseError}</p>}
      <div className="bg-orange-500 text-white text-center p-3">
        <h2 className="text-2xl font-bold">
          Hello, {studentData.name}
        </h2>
      </div>
      <div className="p-8 grid grid-cols-1 md:grid-cols-2 gap-6 items-center">
        <div className="col-span-1 flex justify-center items-center">
          <img
            src={studentData.avatarUrl || studentData.defaultAvatarUrl || undefined}
            onError={(e) => {
              // Si no hay avatar propio para el nivel, se usa el default, una sola vez.
              const img = e.currentTarget
              const fallback = studentData.defaultAvatarUrl
              if (fallback && img.getAttribute('src') !== fallback) img.src = fallback
            }}
            alt="avatar"
            className="w-full h-auto max-w-60 max-h-90"
          />
        </div>

        <div className="col-span-1 flex flex-col justify-center items-center text-center space-y-4">
          <div className="w-full max-w-xs p-3 text-center">
            <div className="text-5xl font-extrabold text-orange-500">
              Level {studentData.level}
            </div>
          </div>

          <div className="w-full max-w-xs p-4 bg-orange-100 rounded-xl shadow">
            <div className="flex justify-between items-center text-md font-semibold text-gray-700">
              <span>Coins</span>
              <span className="font-bold text-yellow-500">
                {studentData.monedas}
              </span>
            </div>
            <div className="mt-2">
              <div className="flex justify-between items-center text-xs font-semibold text-gray-700 mb-1">
                <span>EXP.</span>
                <span>
                  {progress.currentExp}/{progress.requiredExp}
                </span>
              </div>
              <div className="w-full bg-gray-200 rounded-full h-2.5">
                <div
                  className="bg-orange-500 h-2.5 rounded-full"
                  style={{ width: `${progress.percentage}%` }}
                ></div>
              </div>
            </div>
          </div>

          <div className="w-full max-w-xs p-4 bg-orange-100 rounded-xl shadow text-md font-semibold text-gray-700 space-y-2">
            <div className="flex justify-between items-center">
              <span>Activities</span>
              <span className="font-bold">{studentData.tareas}</span>
            </div>
            <div className="flex justify-between items-center">
              <span>Assists</span>
              <span className="font-bold">
                {studentData.asistencias}
              </span>
            </div>
            <div className="flex justify-between items-center">
              <span>Games & Challenges</span>
              <span className="font-bold">
                {studentData.games || 0}
              </span>
            </div>
            <div className="flex justify-between items-center">
              <span>Badges</span>
              <span className="font-bold">
                {Object.keys(studentData.purchases || {}).length}
              </span>
            </div>
            <div className="text-[10px] font-light text-gray-600 mt-1">
              {Object.entries(studentData.purchases || {}).map(
                ([name, qty], idx) => (
                  <div key={idx}>
                    {name} x{qty}
                  </div>
                ),
              )}
            </div>
          </div>
          {/* Comentarios del profesor */}
          <div className="w-full max-w-xs p-4 bg-orange-100 rounded-xl shadow text-md font-semibold text-gray-700 space-y-2">
            <p className="font-bold text-lg">Comments</p>
            <div className="w-full bg-gray-50 p-2 rounded-lg border border-gray-200 overflow-y-auto max-h-24 text-sm text-left font-normal whitespace-pre-wrap">
              {studentData.comments || 'No comments.'}
            </div>
          </div>
        </div>
      </div>
      <div className="px-8 pb-8 flex flex-wrap justify-center w-full gap-4">
        {features.leaderboard && (
          <button
            className="bg-blue-500 text-white px-10 py-3 rounded-full shadow-lg hover:bg-blue-600 transition-all duration-300 font-semibold"
            onClick={fetchLeaderboard}
            disabled={courseBusy}
          >
            Leaderboard
          </button>
        )}

        {features.store && (
          <button
            className="bg-orange-500 text-white px-10 py-3 rounded-full shadow-lg hover:bg-orange-600 transition-all duration-300 font-semibold"
            onClick={openStore}
            disabled={courseBusy}
          >
            Store
          </button>
        )}
        <button
          onClick={handleLogout}
          className="bg-purple-600 text-white px-10 py-3 rounded-full shadow-lg hover:bg-purple-700 transition-all duration-300 font-semibold"
        >
          Log Out
        </button>
      </div>
    </div>
  )
}
