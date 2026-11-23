import { useEffect, useId, useRef, useState, type KeyboardEvent } from 'react'
import { Check, ChevronsUpDown, LayoutGrid, LoaderCircle } from 'lucide-react'
import type { PublicCourse } from '../lib/types'
import { CourseLogo } from './CourseSelector'

interface CourseSwitcherProps {
  courses: PublicCourse[]
  currentCourse: PublicCourse
  busy: boolean
  onSelect: (courseId: number) => void
  onChooseCourse: () => void
}

export default function CourseSwitcher({ courses, currentCourse, busy, onSelect, onChooseCourse }: CourseSwitcherProps) {
  const [open, setOpen] = useState(false)
  const root = useRef<HTMLDivElement>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const menu = useRef<HTMLDivElement>(null)
  const id = useId()
  const multiple = courses.length > 1

  useEffect(() => {
    if (!open) return
    menu.current?.querySelector<HTMLElement>('[aria-checked="true"]')?.focus()
    const outside = (e: PointerEvent) => {
      if (!root.current?.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('pointerdown', outside)
    return () => document.removeEventListener('pointerdown', outside)
  }, [open])

  const close = () => {
    setOpen(false)
    trigger.current?.focus()
  }

  const keydown = (e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.preventDefault()
      close()
    } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      if (!open) {
        if (multiple && !busy) setOpen(true)
        return
      }
      const options = [...(menu.current?.querySelectorAll<HTMLButtonElement>('[role^="menuitem"]') ?? [])]
      const index = options.indexOf(document.activeElement as HTMLButtonElement)
      options[(index + (e.key === 'ArrowDown' ? 1 : -1) + options.length) % options.length]?.focus()
    } else if (e.key === 'Tab') setOpen(false)
  }

  return (
    <div ref={root} className="relative min-w-0" onKeyDown={keydown}>
      <button
        ref={trigger}
        type="button"
        aria-label={multiple ? `Switch course: ${currentCourse.name}` : `Course: ${currentCourse.name}`}
        aria-haspopup={multiple ? 'menu' : undefined}
        aria-expanded={multiple ? open : undefined}
        aria-controls={open ? id : undefined}
        disabled={busy || !multiple}
        onClick={() => setOpen((v) => !v)}
        className="flex max-w-full items-center gap-3 rounded-xl px-3 py-2 text-left text-sm font-semibold text-gray-800 transition-colors enabled:hover:bg-orange-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-orange-500 disabled:cursor-default"
      >
        <CourseLogo course={currentCourse} className="h-9 w-9" />
        <span className="min-w-0 truncate">{currentCourse.name}</span>
        {busy ? <LoaderCircle aria-label="Loading course" className="h-4 w-4 shrink-0 animate-spin text-orange-500" /> : multiple && <ChevronsUpDown aria-hidden="true" className="h-4 w-4 shrink-0 text-gray-500" />}
      </button>
      {open && (
        <div ref={menu} id={id} role="menu" aria-label="Your courses" className="absolute left-0 top-full z-40 mt-2 w-80 max-w-[calc(100vw-3rem)] overflow-hidden rounded-xl border border-gray-200 bg-white p-2 shadow-xl">
          <p className="px-3 py-2 text-xs font-semibold uppercase tracking-wide text-gray-400">Your courses</p>
          <div className="max-h-72 overflow-y-auto">
            {courses.map((course) => (
              <button
                key={course.id}
                type="button"
                role="menuitemradio"
                aria-checked={course.id === currentCourse.id}
                onClick={() => {
                  close()
                  if (course.id !== currentCourse.id) onSelect(course.id)
                }}
                className="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-sm text-gray-700 hover:bg-orange-50 focus:bg-orange-50 focus:outline-none"
              >
                <CourseLogo course={course} className="h-8 w-8" />
                <span className="min-w-0 flex-1 break-words">{course.name}</span>
                {course.id === currentCourse.id && <Check aria-hidden="true" className="h-4 w-4 shrink-0 text-orange-500" />}
              </button>
            ))}
          </div>
          <div className="mt-2 border-t border-gray-100 pt-2">
            <button type="button" role="menuitem" onClick={() => { close(); onChooseCourse() }} className="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-sm text-purple-600 hover:bg-purple-50 focus:bg-purple-50 focus:outline-none">
              <LayoutGrid aria-hidden="true" className="h-4 w-4" /> View all my courses
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
