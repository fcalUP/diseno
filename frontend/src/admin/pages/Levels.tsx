import { useEffect, useId, useState, type ReactNode } from 'react'
import { Link } from 'react-router'
import { Images, Info, Plus, Trash2 } from 'lucide-react'
import { api, errorMessage } from '../../lib/api'
import { calcLevel } from '../../lib/level'
import type { Level } from '../../lib/types'
import { Button, Card, IconButton, NumberField, PageHeader, cx, focusRing, useAsync, useToast, useUndo } from '../ui'
import { LoadError, LoadingCard } from './catalog/PageState'
import { CATALOG_INT_MAX, catalogNumber } from './catalog/utils'

// Mismos límites que el servidor (spec §4.3).
const MAX_LEVELS = 20

// Mensaje de error por fila ('' si está bien): nivel 0 en 0 y EXP siempre creciente.
function rowErrors(levels: Level[]): string[] {
  return levels.map((l, i) => {
    if (!Number.isInteger(l.minExp) || l.minExp < 0) return 'Usa un número entero de 0 en adelante.'
    if (l.minExp > CATALOG_INT_MAX) return 'Es demasiado grande.'
    if (i === 0 && l.minExp !== 0) return 'El nivel 0 siempre empieza en 0.'
    if (i > 0 && l.minExp <= levels[i - 1].minExp) {
      return `Debe ser mayor que la del nivel ${i - 1} (${catalogNumber(levels[i - 1].minExp)}).`
    }
    return ''
  })
}

function sameLevels(a: Level[], b: Level[]): boolean {
  return a.length === b.length && a.every((l, i) => l.level === b[i].level && l.minExp === b[i].minExp)
}

// Cuántos alumnos caen en cada nivel con estos umbrales.
function countByLevel(exps: number[], levels: Level[]): Map<number, number> {
  const out = new Map<number, number>()
  for (const exp of exps) {
    const n = calcLevel(exp, levels)
    out.set(n, (out.get(n) ?? 0) + 1)
  }
  return out
}

export default function Levels() {
  const levels = useAsync(() => api.admin.levels().then((r) => r.levels), [])
  const students = useAsync(() => api.admin.students({ active: 'true' }).then((r) => r.students), [])
  const toast = useToast()
  const undo = useUndo()
  const uid = useId()
  // null = sin cambios: se muestra lo guardado.
  const [draft, setDraft] = useState<Level[] | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')

  const saved = (levels.data ?? []).toSorted((a, b) => a.level - b.level)
  const rows = draft ?? saved
  const dirty = draft !== null && !sameLevels(draft, saved)
  const errors = rowErrors(rows)
  const invalid = errors.some(Boolean)

  // Avisa antes de cerrar la pestaña con cambios sin guardar.
  useEffect(() => {
    if (!dirty) return
    const onBeforeUnload = (e: BeforeUnloadEvent) => e.preventDefault()
    window.addEventListener('beforeunload', onBeforeUnload)
    return () => window.removeEventListener('beforeunload', onBeforeUnload)
  }, [dirty])

  const exps = (students.data ?? []).map((s) => s.exp)
  const counts = countByLevel(exps, rows)
  const before = dirty ? countByLevel(exps, saved) : counts
  const maxCount = Math.max(1, ...counts.values())

  const setMinExp = (index: number, minExp: number) =>
    setDraft(rows.map((l, i) => (i === index ? { ...l, minExp } : l)))

  const addLevel = () => {
    if (rows.length >= MAX_LEVELS) return
    const last = rows[rows.length - 1]
    const prev = rows[rows.length - 2]
    // Propone el mismo salto que entre los dos últimos niveles.
    const step = last && prev && last.minExp > prev.minExp ? last.minExp - prev.minExp : 10
    const minExp = Math.min(CATALOG_INT_MAX, (last?.minExp ?? 0) + step)
    setDraft([...rows, { level: rows.length, minExp: rows.length === 0 ? 0 : minExp }])
    const newId = `${uid}-exp-${rows.length}`
    requestAnimationFrame(() => document.getElementById(newId)?.focus())
  }

  const removeLast = () => {
    if (rows.length <= 1) return
    setDraft(rows.slice(0, -1))
    // El botón de quitar pasa a la fila anterior; si ya no hay, el foco va a "Agregar nivel".
    requestAnimationFrame(() =>
      (document.getElementById(`${uid}-remove`) ?? document.getElementById(`${uid}-add`))?.focus(),
    )
  }

  const discard = () => {
    setDraft(null)
    setSaveError('')
  }

  const save = async () => {
    if (!dirty || invalid || saving) return
    setSaving(true)
    setSaveError('')
    try {
      const r = await api.admin.saveLevels(rows)
      levels.setData(() => r.levels)
      setDraft(null)
      toast.success('Niveles guardados.', {
        undo: async () => {
          if (await undo({ auditId: r.auditId })) levels.reload()
        },
      })
    } catch (e) {
      setSaveError(errorMessage(e))
    } finally {
      setSaving(false)
    }
  }

  let preview: ReactNode = null
  if (students.error) {
    preview = (
      <LoadError
        className="mt-4"
        message={`No se pudo cargar la vista previa de alumnos: ${errorMessage(students.error)}`}
        onRetry={students.reload}
      />
    )
  }

  const studentCell = (level: number) => {
    if (!students.data) {
      return <span className="text-gray-500">{students.error ? '—' : '…'}</span>
    }
    const n = counts.get(level) ?? 0
    const old = before.get(level) ?? 0
    return (
      <div className="flex items-center justify-end gap-3 sm:justify-start">
        <span className="hidden h-2 w-24 overflow-hidden rounded-full bg-gray-100 sm:block lg:w-40" aria-hidden="true">
          <span className="block h-2 rounded-full bg-purple-500" style={{ width: `${(n / maxCount) * 100}%` }} />
        </span>
        <span className="tabular-nums">
          <span className="font-semibold text-gray-800">{catalogNumber(n)}</span>
          {dirty && old !== n && <span className="ml-1 text-xs text-gray-500">(antes {catalogNumber(old)})</span>}
        </span>
      </div>
    )
  }

  let content: ReactNode
  if (!levels.data) {
    content = levels.error ? (
      <LoadError message={errorMessage(levels.error)} onRetry={levels.reload} />
    ) : (
      <LoadingCard label="Cargando niveles…" />
    )
  } else {
    content = (
      <Card padded={false}>
        {levels.error && <LoadError className="m-4" message={errorMessage(levels.error)} onRetry={levels.reload} />}
        <div className="overflow-x-auto">
          {/* relative: así las etiquetas sr-only quedan dentro del scroll y no ensanchan la página. */}
          <table className="relative w-full text-sm">
            <caption className="sr-only">Niveles y EXP mínima de cada uno</caption>
            <thead>
              <tr className="border-b border-gray-200 bg-gray-50 text-xs font-semibold uppercase tracking-wide text-gray-500">
                <th scope="col" className="w-14 px-2 py-3 text-left sm:w-16 sm:px-4">
                  Nivel
                </th>
                <th scope="col" className="px-2 py-3 sm:px-4 text-left">
                  EXP mínima
                </th>
                <th scope="col" className="px-2 py-3 sm:px-4 text-right sm:text-left">
                  Alumnos activos
                </th>
                <th scope="col" className="w-12 px-2 py-3">
                  <span className="sr-only">Quitar</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map((l, i) => {
                const isLast = i === rows.length - 1
                return (
                  <tr key={l.level} className="border-b border-gray-100 align-top last:border-0">
                    <th scope="row" className="px-2 py-3 sm:px-4 text-left">
                      <span className="inline-flex h-9 min-w-9 items-center justify-center rounded-full bg-orange-100 px-2 font-bold text-orange-700">
                        {l.level}
                      </span>
                    </th>
                    <td className="px-2 py-3 sm:px-4">
                      {i === 0 ? (
                        <p className="py-2 text-gray-700">
                          <span className="font-semibold tabular-nums">0</span>{' '}
                          <span className="text-xs text-gray-500">(siempre empieza en 0)</span>
                        </p>
                      ) : (
                        <NumberField
                          id={`${uid}-exp-${i}`}
                          label={`EXP mínima del nivel ${l.level}`}
                          hideLabel
                          value={l.minExp}
                          onChange={(v) => setMinExp(i, v)}
                          error={errors[i] || undefined}
                          min={0}
                          step={1}
                          inputMode="numeric"
                          className="max-w-[16rem]"
                        />
                      )}
                    </td>
                    <td className="px-2 py-3 sm:px-4 pt-5">{studentCell(l.level)}</td>
                    <td className="px-2 py-3">
                      {isLast && i > 0 && (
                        <IconButton
                          id={`${uid}-remove`}
                          icon={Trash2}
                          variant="danger"
                          label={`Quitar el nivel ${l.level}`}
                          onClick={removeLast}
                        />
                      )}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
        {errors[0] && (
          <p role="alert" className="px-4 pb-2 text-sm text-red-600">
            {errors[0]}
          </p>
        )}
        <div className="flex flex-col gap-3 border-t border-gray-100 p-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex flex-col gap-1">
            <Button
              id={`${uid}-add`}
              variant="ghost"
              icon={Plus}
              onClick={addLevel}
              disabled={rows.length >= MAX_LEVELS}
              className="self-start"
            >
              Agregar nivel
            </Button>
            {rows.length >= MAX_LEVELS && (
              <p className="text-xs text-gray-500">Puede haber {MAX_LEVELS} niveles como máximo.</p>
            )}
          </div>
          <div className="flex flex-col items-stretch gap-2 sm:items-end">
            {saveError && (
              <p role="alert" className="text-sm text-red-600">
                {saveError}
              </p>
            )}
            {dirty && invalid && !saveError && (
              <p className="text-sm text-red-600">Corrige los valores marcados para poder guardar.</p>
            )}
            <div className="flex flex-wrap justify-end gap-2">
              {dirty && (
                <Button variant="ghost" onClick={discard} disabled={saving}>
                  Descartar cambios
                </Button>
              )}
              <Button onClick={() => void save()} loading={saving} disabled={!dirty || invalid}>
                Guardar niveles
              </Button>
            </div>
          </div>
        </div>
      </Card>
    )
  }

  return (
    <>
      <PageHeader title="Niveles" subtitle="La EXP mínima que necesita un alumno para llegar a cada nivel." />
      <div className="mb-6 flex gap-3 rounded-2xl border border-blue-100 bg-blue-50 p-4 text-sm text-blue-900">
        <Info aria-hidden="true" className="mt-0.5 h-5 w-5 shrink-0 text-blue-600" />
        <div className="space-y-2">
          <p>
            <strong className="font-semibold">Cada nivel tiene su propio avatar.</strong> Si agregas un nivel, sube sus
            imágenes en{' '}
            <Link
              to="/admin/avatares"
              className={cx('inline-flex items-center gap-1 font-semibold text-blue-700 underline', focusRing)}
            >
              <Images aria-hidden="true" className="h-4 w-4" />
              Avatares
            </Link>{' '}
            (las predeterminadas y las de cada alumno con avatar propio).
          </p>
          <p>
            El nivel se calcula con la EXP de cada alumno: al guardar, todos quedan en su nuevo nivel al instante y
            quien suba verá la animación la próxima vez que entre.
          </p>
        </div>
      </div>
      {content}
      {preview}
    </>
  )
}
