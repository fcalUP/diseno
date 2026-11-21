import { useId, useState, type FormEvent } from 'react'
import { ArrowRightLeft, Calculator, Download, Equal, Trash2, UserCheck, UserX, X } from 'lucide-react'
import { CHUNK_SIZE, api, chunk, errorMessage } from '../../../lib/api'
import { downloadCsv, toCsv } from '../../../lib/csv'
import type { AdminCourse, AdminStudent, BulkStudentsRequest, CounterField } from '../../../lib/types'
import { Button, Modal, NumberField, Select, cx, focusRing, useConfirm, useToast } from '../../ui'
import { useUndoToast } from './useUndoToast'
import { COUNTERS, STUDENT_CSV, alumnos, newBatchId, pl, today } from './util'

type Counters = Record<CounterField, number>
type Action = Omit<BulkStudentsRequest, 'ids' | 'batchId'>

const ZERO: Counters = { tareas: 0, monedas: 0, asistencias: 0, games: 0, exp: 0 }

const badCounter = (values: Counters, keys: readonly CounterField[]) =>
  COUNTERS.find((c) => keys.includes(c.key) && (!Number.isInteger(values[c.key]) || values[c.key] < 0))

export interface BulkBarProps {
  /** Alumnos seleccionados que están a la vista. */
  rows: AdminStudent[]
  courses: AdminCourse[]
  onClear: () => void
  /** Se llama al terminar (bien o mal) para recargar la lista. */
  onDone: () => void
}

// Barra de acciones para los alumnos seleccionados.
export default function BulkBar({ rows, courses, onClear, onDone }: BulkBarProps) {
  const toast = useToast()
  const confirm = useConfirm()
  const done = useUndoToast()
  const [dialog, setDialog] = useState<'delta' | 'set' | 'move' | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const n = rows.length

  // Manda la acción en partes de 50 ids con el mismo batchId, para deshacerla de una vez.
  const run = async (key: string, action: Action, message: (updated: number) => string) => {
    const ids = rows.map((r) => r.id)
    const batchId = newBatchId()
    let updated = 0
    setBusy(key)
    try {
      for (const part of chunk(ids, CHUNK_SIZE)) {
        const res = await api.admin.bulkStudents({ ...action, ids: part, batchId })
        updated += res.updated
      }
      // Sin cambios no hay nada que deshacer.
      if (updated > 0) done(message(updated), { batchId }, onDone)
      else toast.info(message(0))
      setDialog(null)
      onClear()
    } catch (e) {
      if (updated > 0) {
        done(`${errorMessage(e)} Alcanzó a aplicarse a ${alumnos(updated)}.`, { batchId }, onDone, 'error')
      } else {
        toast.error(errorMessage(e))
      }
    } finally {
      setBusy(null)
      onDone()
    }
  }

  const exportSelection = () => downloadCsv(`alumnos-seleccionados-${today()}.csv`, toCsv(rows, STUDENT_CSV))

  const remove = async () => {
    const ok = await confirm({
      title: `¿Eliminar ${alumnos(n)}?`,
      message: 'También se borran sus compras. Puedes deshacerlo desde el aviso o en Actividad.',
      confirmLabel: 'Eliminar',
      danger: true,
    })
    if (ok) void run('delete', { action: 'delete' }, (u) => `${pl(u, 'Se eliminó', 'Se eliminaron')} ${alumnos(u)}.`)
  }

  const locked = busy !== null
  return (
    <section
      aria-label="Acciones para los alumnos seleccionados"
      className="mb-3 flex flex-wrap items-center gap-2 rounded-2xl border border-orange-200 bg-orange-50 px-4 py-3"
    >
      <div className="mr-auto flex flex-wrap items-center gap-2">
        <p className="text-sm font-semibold text-orange-800" aria-live="polite">
          {alumnos(n)} {pl(n, 'seleccionado', 'seleccionados')}
        </p>
        <Button size="sm" variant="ghost" icon={X} disabled={locked} onClick={onClear}>
          Quitar selección
        </Button>
      </div>
      <Button size="sm" variant="ghost" icon={Calculator} disabled={locked} onClick={() => setDialog('delta')}>
        Sumar o restar
      </Button>
      <Button size="sm" variant="ghost" icon={Equal} disabled={locked} onClick={() => setDialog('set')}>
        Fijar valores
      </Button>
      <Button
        size="sm"
        variant="ghost"
        icon={UserCheck}
        disabled={locked}
        loading={busy === 'activate'}
        onClick={() =>
          void run('activate', { action: 'activate' }, (u) => `${pl(u, 'Se activó', 'Se activaron')} ${alumnos(u)}.`)
        }
      >
        Activar
      </Button>
      <Button
        size="sm"
        variant="ghost"
        icon={UserX}
        disabled={locked}
        loading={busy === 'deactivate'}
        onClick={() =>
          void run(
            'deactivate',
            { action: 'deactivate' },
            (u) => `${pl(u, 'Se desactivó', 'Se desactivaron')} ${alumnos(u)}.`,
          )
        }
      >
        Desactivar
      </Button>
      <Button size="sm" variant="ghost" icon={ArrowRightLeft} disabled={locked} onClick={() => setDialog('move')}>
        Mover de curso
      </Button>
      <Button size="sm" variant="ghost" icon={Download} disabled={locked} onClick={exportSelection}>
        Exportar CSV
      </Button>
      <Button size="sm" variant="danger" icon={Trash2} disabled={locked} loading={busy === 'delete'} onClick={remove}>
        Eliminar
      </Button>

      {dialog === 'delta' && (
        <DeltaDialog
          count={n}
          busy={busy === 'delta'}
          onCancel={() => setDialog(null)}
          onSubmit={(delta) =>
            void run('delta', { action: 'delta', delta }, (u) => `Se actualizaron los contadores de ${alumnos(u)}.`)
          }
        />
      )}
      {dialog === 'set' && (
        <SetDialog
          count={n}
          busy={busy === 'set'}
          onCancel={() => setDialog(null)}
          onSubmit={(set) => void run('set', { action: 'set', set }, (u) => `Se fijaron los valores de ${alumnos(u)}.`)}
        />
      )}
      {dialog === 'move' && (
        <MoveDialog
          count={n}
          courses={courses}
          busy={busy === 'move'}
          onCancel={() => setDialog(null)}
          onSubmit={(course) =>
            void run('move', { action: 'move', courseId: course.id }, (u) =>
              u === n
                ? `${pl(u, 'Se movió', 'Se movieron')} ${alumnos(u)} a ${course.name}.`
                : u === 0
                  ? `No se movió ningún alumno: en ${course.name} ya hay alumnos con esas matrículas.`
                  : `Se movieron ${u} de ${alumnos(n)} a ${course.name}; los demás ya tenían esa matrícula ahí.`,
            )
          }
        />
      )}
    </section>
  )
}

interface DialogProps<T> {
  count: number
  busy: boolean
  onCancel: () => void
  onSubmit: (value: T) => void
}

function DialogFooter({ formId, busy, label, onCancel }: { formId: string; busy: boolean; label: string; onCancel: () => void }) {
  return (
    <>
      <Button variant="ghost" onClick={onCancel} disabled={busy}>
        Cancelar
      </Button>
      <Button type="submit" form={formId} loading={busy}>
        {label}
      </Button>
    </>
  )
}

function FormError({ message }: { message: string }) {
  return message ? (
    <p role="alert" className="mt-3 text-sm text-red-600">
      {message}
    </p>
  ) : null
}

function DeltaDialog({ count, busy, onCancel, onSubmit }: DialogProps<Partial<Counters>>) {
  const formId = useId()
  const [mode, setMode] = useState<'add' | 'sub'>('add')
  const [values, setValues] = useState<Counters>(ZERO)
  const [error, setError] = useState('')

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const bad = badCounter(values, COUNTERS.map((c) => c.key))
    if (bad) {
      setError(`${bad.label}: usa un número entero de 0 en adelante.`)
      return
    }
    const delta: Partial<Counters> = {}
    for (const { key } of COUNTERS) {
      if (values[key] > 0) delta[key] = mode === 'add' ? values[key] : -values[key]
    }
    if (Object.keys(delta).length === 0) {
      setError('Escribe al menos una cantidad mayor que 0.')
      return
    }
    onSubmit(delta)
  }

  const option = (value: 'add' | 'sub', label: string) => (
    <label
      className={cx(
        'flex flex-1 cursor-pointer items-center justify-center gap-2 rounded-full px-3 py-1.5 text-sm font-semibold has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-blue-500',
        mode === value ? 'bg-white text-orange-600 shadow-sm' : 'text-gray-600',
      )}
    >
      <input
        type="radio"
        name={`${formId}-mode`}
        className="sr-only"
        checked={mode === value}
        onChange={() => setMode(value)}
      />
      {label}
    </label>
  )

  return (
    <Modal
      open
      onClose={onCancel}
      title="Sumar o restar"
      description={`Se aplica a ${alumnos(count)}.`}
      footer={<DialogFooter formId={formId} busy={busy} label="Aplicar" onCancel={onCancel} />}
    >
      <form id={formId} onSubmit={submit} noValidate>
        <fieldset>
          <legend className="sr-only">Operación</legend>
          <div className="mb-4 flex gap-1 rounded-full bg-gray-100 p-1">
            {option('add', 'Sumar')}
            {option('sub', 'Restar')}
          </div>
        </fieldset>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
          {COUNTERS.map((c, i) => (
            <NumberField
              key={c.key}
              label={c.label}
              value={values[c.key]}
              min={0}
              step={1}
              data-autofocus={i === 0 || undefined}
              onChange={(v) => {
                setValues((prev) => ({ ...prev, [c.key]: v }))
                setError('')
              }}
            />
          ))}
        </div>
        <p className="mt-3 text-xs text-gray-500">
          {mode === 'add' ? 'Las cantidades se suman a lo que ya tiene cada alumno.' : 'Ningún contador baja de 0.'}
        </p>
        <FormError message={error} />
      </form>
    </Modal>
  )
}

function SetDialog({ count, busy, onCancel, onSubmit }: DialogProps<Partial<Counters>>) {
  const formId = useId()
  const [picked, setPicked] = useState<CounterField[]>([])
  const [values, setValues] = useState<Counters>(ZERO)
  const [error, setError] = useState('')

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (picked.length === 0) {
      setError('Marca al menos un contador.')
      return
    }
    const bad = badCounter(values, picked)
    if (bad) {
      setError(`${bad.label}: usa un número entero de 0 en adelante.`)
      return
    }
    const set: Partial<Counters> = {}
    for (const key of picked) set[key] = values[key]
    onSubmit(set)
  }

  return (
    <Modal
      open
      onClose={onCancel}
      title="Fijar valores"
      description={`Los contadores marcados quedan con el mismo valor en ${alumnos(count)}.`}
      footer={<DialogFooter formId={formId} busy={busy} label="Fijar" onCancel={onCancel} />}
    >
      <form id={formId} onSubmit={submit} noValidate className="space-y-3">
        {COUNTERS.map((c, i) => {
          const on = picked.includes(c.key)
          return (
            <div key={c.key} className="flex items-center gap-3">
              <label className="flex w-36 shrink-0 cursor-pointer items-center gap-2 text-sm font-medium text-gray-700">
                <input
                  type="checkbox"
                  checked={on}
                  data-autofocus={i === 0 || undefined}
                  onChange={(e) => {
                    const checked = e.target.checked
                    setPicked((prev) => (checked ? [...prev, c.key] : prev.filter((k) => k !== c.key)))
                    setError('')
                  }}
                  className={cx('h-4 w-4 cursor-pointer accent-orange-500', focusRing)}
                />
                {c.label}
              </label>
              <NumberField
                label={`Nuevo valor de ${c.label}`}
                hideLabel
                value={values[c.key]}
                min={0}
                step={1}
                disabled={!on}
                onChange={(v) => setValues((prev) => ({ ...prev, [c.key]: v }))}
                className="flex-1"
              />
            </div>
          )
        })}
        <FormError message={error} />
      </form>
    </Modal>
  )
}

function MoveDialog({ count, courses, busy, onCancel, onSubmit }: DialogProps<AdminCourse> & { courses: AdminCourse[] }) {
  const formId = useId()
  const [courseId, setCourseId] = useState('')
  const [error, setError] = useState('')

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const course = courses.find((c) => String(c.id) === courseId)
    if (!course) {
      setError('Elige el curso de destino.')
      return
    }
    onSubmit(course)
  }

  return (
    <Modal
      open
      onClose={onCancel}
      title="Mover de curso"
      description={`Mueve ${alumnos(count)} a otro curso, con sus contadores y compras.`}
      footer={<DialogFooter formId={formId} busy={busy} label="Mover" onCancel={onCancel} />}
    >
      <form id={formId} onSubmit={submit} noValidate>
        <Select
          label="Curso de destino"
          value={courseId}
          onChange={(v) => {
            setCourseId(v)
            setError('')
          }}
          options={courses.map((c) => ({ value: c.id, label: c.active ? c.name : `${c.name} (oculto)` }))}
          placeholder="Elige un curso"
          error={error || undefined}
          hint="Si en ese curso ya hay alguien con la misma matrícula, ese alumno se queda donde está."
          data-autofocus
        />
      </form>
    </Modal>
  )
}
