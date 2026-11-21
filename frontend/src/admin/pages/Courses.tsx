import { useState, type ReactNode } from 'react'
import { BookOpen, EyeOff, ImageOff, Pencil, Plus, Users } from 'lucide-react'
import { api, errorMessage } from '../../lib/api'
import type { AdminCourse, AuditRef } from '../../lib/types'
import { Button, Card, EmptyState, PageHeader, Toggle, useAsync, useConfirm, useToast, useUndo } from '../ui'
import { CourseDrawer } from './catalog/CourseDrawer'
import { MoveButtons } from './catalog/MoveButtons'
import { LoadError, LoadingCard } from './catalog/PageState'
import { catalogCount, catalogMove, catalogWith } from './catalog/utils'

const byOrder = (a: AdminCourse, b: AdminCourse) => a.sortOrder - b.sortOrder || a.id - b.id

function CourseLogo({ url }: { url: string }) {
  return url ? (
    <img
      src={url}
      alt=""
      className="h-12 w-12 shrink-0 rounded-xl border border-gray-100 bg-gray-50 object-contain p-1"
    />
  ) : (
    <span className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-gray-100 text-gray-400">
      <ImageOff aria-hidden="true" className="h-5 w-5" />
    </span>
  )
}

export default function Courses() {
  const courses = useAsync(() => api.admin.courses().then((r) => r.courses), [])
  const badges = useAsync(() => api.admin.badges().then((r) => r.badges), [])
  const toast = useToast()
  const confirm = useConfirm()
  const undo = useUndo()
  // undefined = cajón cerrado, null = curso nuevo.
  const [editing, setEditing] = useState<AdminCourse | null | undefined>(undefined)
  const [toggling, setToggling] = useState<ReadonlySet<number>>(new Set())
  const [reordering, setReordering] = useState(false)
  const [announce, setAnnounce] = useState('')

  const list = (courses.data ?? []).toSorted(byOrder)
  const reload = courses.reload

  const undoThenReload = async (ref: AuditRef) => {
    if (await undo(ref)) reload()
  }

  const upsert = (course: AdminCourse) =>
    courses.setData((prev = []) =>
      prev.some((c) => c.id === course.id) ? prev.map((c) => (c.id === course.id ? course : c)) : [...prev, course],
    )

  const setActive = async (course: AdminCourse, active: boolean) => {
    if (toggling.has(course.id)) return
    if (!active && list.filter((c) => c.active).length === 1) {
      const ok = await confirm({
        title: '¿Ocultar el último curso visible?',
        message:
          'El selector del login se quedará vacío: nadie podrá entrar ni registrarse hasta que vuelvas a mostrar un curso.',
        confirmLabel: 'Ocultar de todos modos',
        danger: true,
      })
      if (!ok) return
    }
    // Se ve el cambio al instante; si falla, se regresa.
    upsert({ ...course, active })
    setToggling((s) => catalogWith(s, course.id, true))
    try {
      const r = await api.admin.updateCourse(course.id, { active })
      upsert(r.course)
      toast.success(
        active ? `«${r.course.name}» ya aparece en el login.` : `«${r.course.name}» ya no aparece en el login.`,
        { undo: () => undoThenReload({ auditId: r.auditId }) },
      )
    } catch (e) {
      upsert(course)
      toast.error(errorMessage(e))
    } finally {
      setToggling((s) => catalogWith(s, course.id, false))
    }
  }

  const move = async (from: number, to: number) => {
    if (reordering) return
    const next = catalogMove(list, from, to).map((c, i) => ({ ...c, sortOrder: i + 1 }))
    courses.setData(() => next)
    setAnnounce(`«${list[from].name}» ahora está en la posición ${to + 1} de ${next.length}.`)
    setReordering(true)
    try {
      const { batchId } = await api.admin.reorderCourses(next.map((c) => c.id))
      toast.success('Orden de los cursos guardado.', { undo: () => undoThenReload({ batchId }) })
    } catch (e) {
      toast.error(errorMessage(e))
      reload()
    } finally {
      setReordering(false)
    }
  }

  let content: ReactNode
  if (!courses.data) {
    content = courses.error ? (
      <LoadError message={errorMessage(courses.error)} onRetry={reload} />
    ) : (
      <LoadingCard label="Cargando cursos…" />
    )
  } else if (list.length === 0) {
    content = (
      <Card padded={false}>
        <EmptyState
          icon={BookOpen}
          title="Todavía no hay cursos"
          description="Crea el primero para que aparezca en el selector del login."
          action={
            <Button icon={Plus} onClick={() => setEditing(null)}>
              Nuevo curso
            </Button>
          }
        />
      </Card>
    )
  } else {
    content = (
      <>
        {courses.error && <LoadError className="mb-4" message={errorMessage(courses.error)} onRetry={reload} />}
        <ol role="list" aria-label="Cursos en el orden del login" className="space-y-3">
          {list.map((c, i) => (
            <li key={c.id}>
              <Card className="flex flex-col gap-4 sm:flex-row sm:items-center">
                <div className="flex min-w-0 flex-1 items-center gap-3">
                  <MoveButtons
                    name={c.name}
                    index={i}
                    count={list.length}
                    busy={reordering}
                    onMove={(to) => void move(i, to)}
                  />
                  <CourseLogo url={c.logoUrl} />
                  <div className="min-w-0">
                    <h2 className="break-words font-semibold text-gray-800">{c.name}</h2>
                    <p className="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-500">
                      <span>Hoja: {c.sheetName}</span>
                      <span className="inline-flex items-center gap-1">
                        <Users aria-hidden="true" className="h-3.5 w-3.5" />
                        {catalogCount(c.studentCount, 'alumno', 'alumnos')}
                      </span>
                      {c.hiddenBadgeIds.length > 0 && (
                        <span className="inline-flex items-center gap-1">
                          <EyeOff aria-hidden="true" className="h-3.5 w-3.5" />
                          {catalogCount(c.hiddenBadgeIds.length, 'insignia oculta', 'insignias ocultas')}
                        </span>
                      )}
                    </p>
                  </div>
                </div>
                <div className="flex items-center justify-between gap-4 border-t border-gray-100 pt-3 sm:justify-end sm:border-0 sm:pt-0">
                  <div className="flex items-center gap-2">
                    <Toggle
                      checked={c.active}
                      onChange={(v) => void setActive(c, v)}
                      label={`«${c.name}» visible en el login`}
                      hideLabel
                    />
                    {/* El lector de pantalla ya oye el nombre del interruptor; el texto sólo ayuda al clic. */}
                    <span
                      aria-hidden="true"
                      className="cursor-pointer select-none text-sm text-gray-600"
                      onClick={() => void setActive(c, !c.active)}
                    >
                      Visible en el login
                    </span>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    icon={Pencil}
                    aria-label={`Editar «${c.name}»`}
                    onClick={() => setEditing(c)}
                  >
                    Editar
                  </Button>
                </div>
              </Card>
            </li>
          ))}
        </ol>
        <span className="sr-only" aria-live="polite">
          {announce}
        </span>
      </>
    )
  }

  return (
    <>
      <PageHeader
        title="Cursos"
        subtitle="Los cursos visibles aparecen en el selector del login y del registro, en este orden."
        actions={
          <Button icon={Plus} onClick={() => setEditing(null)}>
            Nuevo curso
          </Button>
        }
      />
      {content}
      {editing !== undefined && (
        <CourseDrawer
          course={editing}
          courses={list}
          badges={badges.data}
          badgesError={badges.error ? errorMessage(badges.error) : ''}
          onRetryBadges={badges.reload}
          onClose={() => setEditing(undefined)}
          onSaved={upsert}
          onDeleted={(id) => courses.setData((prev = []) => prev.filter((c) => c.id !== id))}
          onUndone={reload}
        />
      )}
    </>
  )
}
