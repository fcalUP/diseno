import { useMemo, useState } from 'react'
import { useSearchParams } from 'react-router'
import { BookOpen, Download, Eraser, Undo2, UserPlus } from 'lucide-react'
import { CHUNK_SIZE, api, errorMessage } from '../../lib/api'
import { downloadCsv, toCsv, type CsvColumn } from '../../lib/csv'
import type { AdminStudent, GenerateRow } from '../../lib/types'
import {
  Button,
  Card,
  DataTable,
  EmptyState,
  PageHeader,
  Pill,
  Select,
  TextArea,
  cx,
  focusRing,
  useAsync,
  useConfirm,
  useUndo,
  type Column,
} from '../ui'
import { parsePaste, validateRows, type PreviewRow } from './students/parse'
import { CopyButton, ErrorBanner, LinkButton, LoadingBlock, ProgressBar } from './students/parts'
import { useUndoToast } from './students/useUndoToast'
import { alumnos, fileSlug, formatNumber, newBatchId, pl, sortCourses, today } from './students/util'

// El WAF corta cuerpos de 8 KB: cada parte lleva máximo 50 filas y deja margen para lo demás.
const MAX_CHUNK_BYTES = 7000

interface Created {
  student: AdminStudent
  password: string
}

interface RunResult {
  batchId: string
  courseName: string
  created: Created[]
  skipped: { code: string; reason: string }[]
  error: string | null
  /** Filas que no se alcanzaron a mandar por el error. */
  pending: number
  /** Filas que se quedaron en la caja para corregirlas. */
  kept: number
  undone: boolean
}

// Parte las filas en grupos de hasta 50 que además quepan en el límite de bytes.
function packRows(rows: readonly GenerateRow[]): GenerateRow[][] {
  const enc = new TextEncoder()
  const out: GenerateRow[][] = []
  let cur: GenerateRow[] = []
  let size = 0
  for (const row of rows) {
    const n = enc.encode(JSON.stringify(row)).length + 1
    if (cur.length > 0 && (cur.length >= CHUNK_SIZE || size + n > MAX_CHUNK_BYTES)) {
      out.push(cur)
      cur = []
      size = 0
    }
    cur.push(row)
    size += n
  }
  if (cur.length > 0) out.push(cur)
  return out
}

// Solo viajan los campos que traen algo.
function toRow(r: PreviewRow): GenerateRow {
  const row: GenerateRow = { code: r.code, name: r.name }
  if (r.sexo) row.sexo = r.sexo
  if (r.email) row.email = r.email
  if (r.password) row.password = r.password
  return row
}

// Al aparecer el resultado el foco va a su título (y la vista sube hasta él).
const focusOnMount = (el: HTMLElement | null) => el?.focus()

function Missing() {
  return <span className="text-xs font-medium text-red-600">Falta</span>
}

const Dash = () => <span className="text-gray-400">—</span>

const PREVIEW_COLUMNS: Column<PreviewRow>[] = [
  { key: 'line', header: 'Línea', align: 'right', className: 'tabular-nums text-gray-400', cell: (r) => r.line },
  {
    key: 'code',
    header: 'ID',
    cell: (r) => (r.code ? <span className="font-mono text-xs">{r.code}</span> : <Missing />),
  },
  { key: 'name', header: 'Nombre', className: 'min-w-[10rem]', cell: (r) => r.name || <Missing /> },
  { key: 'sexo', header: 'Sexo', cell: (r) => r.sexo || <Dash /> },
  { key: 'email', header: 'Correo', cell: (r) => r.email || <Dash /> },
  {
    key: 'password',
    header: 'Contraseña',
    cell: (r) =>
      r.password ? (
        <span className="font-mono">{r.password}</span>
      ) : (
        <span className="text-xs italic text-gray-400">se genera</span>
      ),
  },
  {
    key: 'status',
    header: 'Estado',
    className: 'min-w-[12rem]',
    cell: (r) =>
      r.errors.length === 0 ? (
        <Pill tone="green">Lista</Pill>
      ) : (
        <ul className="space-y-0.5 text-xs text-red-600">
          {r.errors.map((e) => (
            <li key={e}>{e}</li>
          ))}
        </ul>
      ),
  },
]

const CREATED_COLUMNS: Column<Created>[] = [
  { key: 'code', header: 'ID', cell: (c) => <span className="font-mono text-xs">{c.student.code}</span> },
  { key: 'name', header: 'Nombre', cell: (c) => c.student.name },
  { key: 'email', header: 'Correo', cell: (c) => c.student.email || <Dash /> },
  {
    key: 'password',
    header: 'Contraseña',
    cell: (c) => <code className="select-all font-mono font-bold tracking-widest text-gray-900">{c.password}</code>,
  },
]

const CREATED_CSV: CsvColumn<Created>[] = [
  { header: 'Curso', value: (c) => c.student.courseName },
  { header: 'ID', value: (c) => c.student.code },
  { header: 'Nombre', value: (c) => c.student.name },
  { header: 'Sexo', value: (c) => c.student.sexo },
  { header: 'Correo', value: (c) => c.student.email },
  { header: 'Contraseña', value: (c) => c.password },
]

const PLACEHOLDER = '0243123\tAna López García\tMujer\n0243124\tLuis Pérez Soto\tHombre\tluis@up.edu.mx'

export default function GenerateStudents() {
  const [params] = useSearchParams()
  const confirm = useConfirm()
  const undo = useUndo()
  const done = useUndoToast()
  const coursesQ = useAsync(() => api.admin.courses(), [])
  // Solo para avisar antes de mandar si una contraseña escrita no cumple la regla.
  const settingsQ = useAsync(() => api.admin.settings(), [])
  const [courseId, setCourseId] = useState(() => {
    const n = Number(params.get('curso'))
    return Number.isInteger(n) && n > 0 ? n : 0
  })
  // Matrículas que ya existen en el curso, para marcar repetidos en la vista previa.
  const existingQ = useAsync(
    () => (courseId ? api.admin.students({ courseId }) : Promise.resolve(null)),
    [courseId],
  )
  const [text, setText] = useState('')
  const [headerOverride, setHeaderOverride] = useState<boolean | null>(null)
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null)
  const [result, setResult] = useState<RunResult | null>(null)
  const [undoing, setUndoing] = useState(false)

  const courses = useMemo(() => sortCourses(coursesQ.data?.courses ?? []), [coursesQ.data])
  const course = courses.find((c) => c.id === courseId)
  const parsed = useMemo(() => parsePaste(text, headerOverride), [text, headerOverride])
  const existing = useMemo(() => new Set((existingQ.data?.students ?? []).map((s) => s.code)), [existingQ.data])
  const pattern = settingsQ.data?.settings.passwordPattern
  const rows = useMemo(() => validateRows(parsed.rows, existing, pattern), [parsed.rows, existing, pattern])
  // Las filas con errores van primero para que no haya que buscarlas.
  const previewRows = useMemo(
    () => [...rows].sort((a, b) => Number(b.errors.length > 0) - Number(a.errors.length > 0)),
    [rows],
  )
  const ready = rows.filter((r) => r.errors.length === 0)
  const withErrors = rows.length - ready.length
  const running = progress !== null

  const markUndone = (batchId: string) => {
    setResult((r) => (r && r.batchId === batchId ? { ...r, undone: true } : r))
    existingQ.reload()
  }

  const create = async () => {
    if (!course || ready.length === 0 || running) return
    const payload = ready.map(toRow)
    const batchId = newBatchId()
    const created: Created[] = []
    const skipped: RunResult['skipped'] = []
    let sent = 0
    let error: string | null = null
    setResult(null)
    setProgress({ done: 0, total: payload.length })
    for (const part of packRows(payload)) {
      try {
        const res = await api.admin.generateStudents({ courseId: course.id, rows: part, batchId })
        // Go manda null en vez de [] cuando una lista sale vacía.
        created.push(...(res.created ?? []))
        skipped.push(...(res.skipped ?? []))
        sent += part.length
        setProgress({ done: sent, total: payload.length })
      } catch (e) {
        error = errorMessage(e)
        break
      }
    }
    let kept = 0
    if (!error) {
      // En la caja se quedan solo las filas que no se crearon, para corregirlas.
      // Si hubo error se deja todo: al reintentar, lo ya creado sale como repetido.
      const skippedCodes = new Set(skipped.map((s) => s.code))
      const keep = new Set(rows.filter((r) => r.errors.length > 0 || skippedCodes.has(r.code)).map((r) => r.line))
      const lines = text.split(/\r?\n/)
      const headerIndex = parsed.header ? lines.findIndex((l) => l.trim() !== '') : -1
      const left = lines.filter((_, i) => keep.has(i + 1))
      kept = left.length
      setText(left.length > 0 ? (headerIndex >= 0 ? [lines[headerIndex], ...left] : left).join('\n') : '')
    }
    setProgress(null)
    setResult({
      batchId,
      courseName: course.name,
      created,
      skipped,
      error,
      pending: payload.length - sent,
      kept,
      undone: false,
    })
    existingQ.reload()
    if (created.length > 0) {
      done(
        `${pl(created.length, 'Se creó', 'Se crearon')} ${alumnos(created.length)} en ${course.name}.`,
        { batchId },
        () => markUndone(batchId),
      )
    }
  }

  const undoAll = async () => {
    if (!result) return
    const { batchId } = result
    const ok = await confirm({
      title: `¿Deshacer la creación de ${alumnos(result.created.length)}?`,
      message: 'Se eliminan los alumnos que se crearon en este lote.',
      confirmLabel: 'Deshacer todo',
      danger: true,
    })
    if (!ok) return
    setUndoing(true)
    if (await undo({ batchId })) markUndone(batchId)
    setUndoing(false)
  }

  const downloadResult = () => {
    if (!result) return
    downloadCsv(`alumnos-nuevos-${fileSlug(result.courseName)}-${today()}.csv`, toCsv(result.created, CREATED_CSV))
  }

  const copyAllText = result
    ? [
        'ID\tNombre\tCorreo\tContraseña',
        ...result.created.map((c) => [c.student.code, c.student.name, c.student.email, c.password].join('\t')),
      ].join('\n')
    : ''

  const noCourses = coursesQ.data !== undefined && courses.length === 0

  return (
    <>
      <PageHeader
        title="Generar alumnos"
        subtitle="Pega una lista y crea a muchos alumnos a la vez, cada uno con su contraseña."
      />

      {coursesQ.error && (
        <ErrorBanner
          className="mb-4"
          message={`No se pudieron cargar los cursos: ${errorMessage(coursesQ.error)}`}
          onRetry={coursesQ.reload}
        />
      )}

      {result && (
        <Card key={result.batchId} className="mb-6">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0">
              <h2 ref={focusOnMount} tabIndex={-1} className="text-lg font-semibold text-gray-800 outline-none">
                Resultado
              </h2>
              <p className="mt-1 text-sm text-gray-600">
                {result.created.length > 0
                  ? `${pl(result.created.length, 'Se creó', 'Se crearon')} ${alumnos(result.created.length)} en ${result.courseName}.`
                  : 'No se creó ningún alumno.'}
                {result.skipped.length > 0 &&
                  ` ${pl(result.skipped.length, 'Se omitió', 'Se omitieron')} ${formatNumber(result.skipped.length)}.`}
              </p>
              {result.kept > 0 && (
                <p className="mt-1 text-sm text-gray-500">
                  En la lista {pl(result.kept, 'quedó la fila', 'quedaron las filas')} que no se {pl(result.kept, 'creó', 'crearon')},
                  para que la{pl(result.kept, '', 's')} corrijas.
                </p>
              )}
            </div>
            {result.undone && <Pill tone="gray">Deshecho</Pill>}
          </div>

          {result.error && (
            <ErrorBanner
              className="mt-3"
              message={`Se detuvo por un error: ${result.error} ${pl(result.pending, 'Quedó', 'Quedaron')} ${formatNumber(result.pending)} ${pl(result.pending, 'fila', 'filas')} sin enviar; vuelve a crear para intentarlo de nuevo.`}
            />
          )}

          {result.created.length > 0 && (
            <>
              {!result.undone && (
                <p className="mt-3 rounded-xl bg-orange-50 px-4 py-2 text-sm text-orange-800">
                  Guarda las contraseñas ahora: no se vuelven a mostrar.
                </p>
              )}
              <div className="mt-3 flex flex-wrap gap-2">
                <CopyButton text={copyAllText} label="Copiar todo" />
                <Button size="sm" variant="ghost" icon={Download} onClick={downloadResult}>
                  Descargar CSV
                </Button>
                <Button
                  size="sm"
                  variant="danger"
                  icon={Undo2}
                  onClick={undoAll}
                  loading={undoing}
                  disabled={result.undone}
                >
                  Deshacer todo
                </Button>
              </div>
              <DataTable
                rows={result.created}
                columns={CREATED_COLUMNS}
                rowKey={(c) => c.student.id}
                caption="Alumnos creados con su contraseña"
                className="relative mt-3 max-h-[50vh]"
              />
            </>
          )}

          {result.skipped.length > 0 && (
            <div className="mt-4">
              <h3 className="text-sm font-semibold text-gray-800">Omitidos</h3>
              <ul className="mt-2 space-y-1 text-sm text-gray-600">
                {result.skipped.map((s, i) => (
                  <li key={`${s.code}-${i}`}>
                    <span className="font-mono text-xs text-gray-800">{s.code || '(sin ID)'}</span>: {s.reason}
                  </li>
                ))}
              </ul>
            </div>
          )}
        </Card>
      )}

      {noCourses ? (
        <Card>
          <EmptyState
            icon={BookOpen}
            title="Primero crea un curso"
            description="Los alumnos se generan dentro de un curso."
            action={<LinkButton to="/admin/cursos">Ir a Cursos</LinkButton>}
          />
        </Card>
      ) : coursesQ.loading && !coursesQ.data ? (
        <LoadingBlock label="Cargando cursos…" />
      ) : (
        <>
          <Card className="mb-6">
            <h2 className="text-base font-semibold text-gray-800">1. Curso y lista</h2>
            <div className="mt-4 grid gap-6 lg:grid-cols-3">
              <div className="space-y-4">
                <Select
                  label="Curso"
                  value={courseId || ''}
                  onChange={(v) => setCourseId(Number(v))}
                  options={courses.map((c) => ({ value: c.id, label: c.active ? c.name : `${c.name} (oculto)` }))}
                  placeholder="Elige un curso"
                  disabled={running}
                />
                <div className="space-y-2 rounded-xl bg-gray-50 p-4 text-sm text-gray-600">
                  <p>
                    Una fila por alumno, en este orden: <strong>ID, Nombre, Sexo</strong> y, si quieres, correo y
                    contraseña.
                  </p>
                  <p>Sirve copiar y pegar desde Sheets o Excel, o separar con punto y coma o coma.</p>
                  <p>Sexo: Hombre o Mujer (también H o M). Si lo dejas vacío se usa Hombre.</p>
                  <p>Sin contraseña se genera una. Con encabezado (ID, Nombre, Correo…) las columnas van en cualquier orden.</p>
                </div>
              </div>
              <div className="lg:col-span-2">
                <TextArea
                  label="Lista de alumnos"
                  rows={10}
                  value={text}
                  onChange={(e) => setText(e.target.value)}
                  placeholder={PLACEHOLDER}
                  spellCheck={false}
                  disabled={running}
                  className="[&_textarea]:font-mono [&_textarea]:text-xs"
                />
                <div className="mt-2 flex flex-wrap items-center justify-between gap-3">
                  <label className="inline-flex cursor-pointer items-center gap-2 text-sm text-gray-700">
                    <input
                      type="checkbox"
                      checked={parsed.header}
                      disabled={!text.trim() || running}
                      onChange={(e) => setHeaderOverride(e.target.checked)}
                      className={cx('h-4 w-4 cursor-pointer accent-orange-500', focusRing)}
                    />
                    La primera fila es encabezado
                    {parsed.headerDetected && <span className="text-gray-500">(detectado)</span>}
                  </label>
                  <Button
                    variant="ghost"
                    size="sm"
                    icon={Eraser}
                    disabled={!text || running}
                    onClick={() => {
                      setText('')
                      setHeaderOverride(null)
                    }}
                  >
                    Limpiar
                  </Button>
                </div>
              </div>
            </div>
          </Card>

          {text.trim() !== '' && (
            <Card>
              <div className="flex flex-wrap items-center justify-between gap-3">
                <h2 className="text-base font-semibold text-gray-800">2. Revisa y crea</h2>
                <div className="flex flex-wrap gap-2" aria-live="polite">
                  <Pill>
                    {formatNumber(rows.length)} {pl(rows.length, 'fila', 'filas')}
                  </Pill>
                  <Pill tone="green">
                    {formatNumber(ready.length)} {pl(ready.length, 'lista', 'listas')}
                  </Pill>
                  {withErrors > 0 && (
                    <Pill tone="red">
                      {formatNumber(withErrors)} con errores
                    </Pill>
                  )}
                </div>
              </div>

              {parsed.error && <ErrorBanner className="mt-4" message={parsed.error} />}
              {existingQ.error && (
                <ErrorBanner
                  className="mt-4"
                  message="No se pudo revisar qué matrículas ya existen en el curso. Si hay repetidas, el servidor las omite."
                  onRetry={existingQ.reload}
                />
              )}

              {!parsed.error && (
                <DataTable
                  rows={previewRows}
                  columns={PREVIEW_COLUMNS}
                  rowKey={(r) => r.line}
                  rowClassName={(r) => (r.errors.length > 0 ? 'bg-red-50/60' : undefined)}
                  caption="Vista previa de los alumnos a crear"
                  className="relative mt-4 max-h-[50vh]"
                />
              )}

              <div className="mt-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <p className="text-sm text-gray-600">
                  {!course
                    ? 'Elige un curso para continuar.'
                    : existingQ.loading
                      ? 'Revisando las matrículas del curso…'
                      : withErrors > 0
                        ? 'Las filas con errores no se crean. Corrígelas en la lista si las necesitas.'
                        : `Todo listo para ${course.name}.`}
                </p>
                <Button
                  icon={UserPlus}
                  onClick={create}
                  loading={running}
                  disabled={!course || ready.length === 0 || existingQ.loading}
                >
                  Crear {alumnos(ready.length)}
                </Button>
              </div>

              {progress && (
                <div className="mt-4">
                  <ProgressBar value={progress.done} max={progress.total} label="Creando alumnos" />
                </div>
              )}
            </Card>
          )}
        </>
      )}
    </>
  )
}
