import { ArrowLeft, ArrowRight, LoaderCircle, LogOut } from 'lucide-react'
import { useState } from 'react'
import type { PublicCourse } from '../lib/types'

interface CourseSelectorProps {
  courses: PublicCourse[]
  studentName: string
  busyCourseId: number | null
  error: string
  onSelect: (courseId: number) => void
  onLogout: () => void
  onBack?: () => void
}

export function CourseLogo({ course, className = 'h-12 w-12' }: { course: PublicCourse; className?: string }) {
  const [failedUrl, setFailedUrl] = useState('')
  return course.logoUrl && course.logoUrl !== failedUrl ? (
    <img
      src={course.logoUrl}
      alt=""
      className={`${className} shrink-0 rounded-xl bg-white object-contain p-1`}
      onError={() => setFailedUrl(course.logoUrl)}
    />
  ) : (
    <span aria-hidden="true" className={`${className} flex shrink-0 items-center justify-center rounded-xl bg-orange-100 font-bold text-orange-600`}>
      {course.name.trim().slice(0, 1)}
    </span>
  )
}

export default function CourseSelector({ courses, studentName, busyCourseId, error, onSelect, onLogout, onBack }: CourseSelectorProps) {
  return (
    <main className="w-full max-w-3xl rounded-3xl bg-white p-6 shadow-2xl sm:p-10" aria-busy={busyCourseId !== null}>
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="text-sm font-semibold text-orange-500">Hello, {studentName}</p>
          <h1 className="mt-1 text-2xl font-bold text-gray-800 sm:text-3xl">Choose your course</h1>
          <p className="mt-2 text-sm text-gray-500">Your progress, coins and badges are saved in each course.</p>
        </div>
        <button type="button" onClick={onLogout} disabled={busyCourseId !== null} className="inline-flex items-center gap-2 rounded-full px-3 py-2 text-sm font-semibold text-purple-600 hover:bg-purple-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-purple-500 disabled:opacity-50">
          <LogOut aria-hidden="true" className="h-4 w-4" /> Log Out
        </button>
      </div>
      {error && <p role="alert" className="mt-5 rounded-xl bg-red-50 p-3 text-sm text-red-600">{error}</p>}
      {onBack && <button type="button" onClick={onBack} disabled={busyCourseId !== null} className="mt-5 inline-flex items-center gap-2 rounded-full text-sm font-semibold text-purple-600 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-purple-500 disabled:opacity-50"><ArrowLeft aria-hidden="true" className="h-4 w-4" /> Back to current course</button>}
      <div className="mt-8 grid gap-4 sm:grid-cols-2">
        {courses.map((course) => (
          <button
            key={course.id}
            type="button"
            onClick={() => onSelect(course.id)}
            disabled={busyCourseId !== null}
            className="group flex min-h-32 items-center gap-4 rounded-2xl border border-gray-200 p-5 text-left shadow-sm transition-colors hover:border-orange-300 hover:bg-orange-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-orange-500 disabled:cursor-wait disabled:opacity-60"
          >
            <CourseLogo course={course} className="h-16 w-16" />
            <span className="min-w-0 flex-1 break-words text-base font-semibold text-gray-800">{course.name}</span>
            {busyCourseId === course.id ? <LoaderCircle aria-label="Loading course" className="h-5 w-5 shrink-0 animate-spin text-orange-500" /> : <ArrowRight aria-hidden="true" className="h-5 w-5 shrink-0 text-orange-500" />}
          </button>
        ))}
      </div>
    </main>
  )
}
