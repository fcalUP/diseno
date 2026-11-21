import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react'
import { Link } from 'react-router'
import { CircleX, Copy, ExternalLink, Eye, Images, LoaderCircle, Pencil, Play, RefreshCw, Undo2, X } from 'lucide-react'
import { ApiError, api, errorMessage } from '../../lib/api'
import type { AuditEntry, ImportJob, ImportReport, ImportRunRequest, ImportSource, ImportStatus } from '../../lib/types'
import {
  Button,
  Card,
  IconButton,
  PageHeader,
  Pill,
  Spinner,
  TextField,
  cx,
  focusRing,
  useAsync,
  useConfirm,
  useToast,
  useUndo,
} from '../ui'
import { formatDateTime, formatRelative, plural } from './system/format'
import { parseSpreadsheetId, sheetUrl, spreadsheetIdError, type ServiceAccount } from './system/google'
import { ImportReportView } from './system/ImportReportView'
import { LoadError, PageLoading } from './system/PageState'
import { SecretBadge } from './system/SecretBadge'
import { ServiceAccountPicker } from './system/ServiceAccountPicker'
import { useNow } from './system/useNow'

const POLL_MS = 1500

// Importación en curso. Vive fuera del componente para que, si vas a otra sección del panel
// y regresas, se siga viendo el avance.
let activeJob: { id: string; since: number } | null = null

type Include = ImportRunRequest['include']
type Mode = ImportRunRequest['mode']

const INCLUDE: [keyof Include, string, string][] = [
  ['students', 'Alumnos', 'Una fila por alumno en la pestaña de cada curso.'],
  ['badges', 'Insignias', 'Nombre, costo y existencias.'],
  ['purchases', 'Compras', 'Historial de compras; las que ya se importaron no se duplican.'],
  ['media', 'Imágenes heredadas', 'Avatares e imágenes de insignias y cursos de la versión anterior.'],
]

const MODES: [Mode, string, string][] = [
  ['upsert', 'Actualizar existentes', 'Los datos de la hoja reemplazan a los del portal y se agrega lo que falte.'],
  ['create_only', 'Solo agregar nuevos', 'No toca lo que ya existe; sólo agrega lo que falte.'],
]

const linkClass = cx(
  'inline-flex items-center gap-1.5 rounded-full text-sm font-medium text-purple-600 hover:underline',
  focusRing,
)

interface SpreadsheetEditorProps {
  value: string
  onSaved: (spreadsheetId: string, auditId: number) => void
}

// ID de la hoja con edición en línea; guardar lo cambia en Ajustes.
function SpreadsheetEditor({ value, onSaved }: SpreadsheetEditorProps) {
  const [editing, setEditing] = useState(false)
  const [text, setText] = useState('')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  // Al cerrar la edición el foco regresa al botón "Cambiar".
  const [refocus, setRefocus] = useState(false)

  const close = () => {
    setEditing(false)
    setError('')
    setRefocus(true)
  }

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    const id = parseSpreadsheetId(text)
    const invalid = spreadsheetIdError(id)
    if (invalid) {
      setError(invalid)
      return
    }
    if (id === value) {
      close()
      return
    }
    setSaving(true)
    try {
      const res = await api.admin.updateSettings({ spreadsheetId: id })
      onSaved(res.settings.spreadsheetId, res.auditId)
      close()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setSaving(false)
    }
  }

  if (editing) {
    return (
      <form noValidate onSubmit={submit} className="flex flex-col gap-2 sm:flex-row sm:items-start">
        <TextField
          label="ID o enlace de la hoja de Google"
          hideLabel
          autoFocus
          autoComplete="off"
          spellCheck={false}
          autoCapitalize="none"
          placeholder="https://docs.google.com/spreadsheets/d/…"
          value={text}
          onChange={(e) => {
            setText(e.target.value)
            setError('')
          }}
          onKeyDown={(e) => {
            if (e.key === 'Escape' && !saving) {
              e.stopPropagation()
              close()
            }
          }}
          error={error || undefined}
          hint="Pega el enlace de la hoja o sólo su ID."
          className="min-w-0 flex-1"
        />
        <div className="flex gap-2">
          <Button type="submit" loading={saving}>
            Guardar
          </Button>
          <Button variant="ghost" onClick={close} disabled={saving}>
            Cancelar
          </Button>
        </div>
      </form>
    )
  }

  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
      <code className="min-w-0 break-all rounded-lg bg-gray-100 px-2 py-1 font-mono text-xs text-gray-800">
        {value || 'Sin hoja configurada'}
      </code>
      <Button
        size="sm"
        variant="ghost"
        icon={Pencil}
        autoFocus={refocus}
        aria-label="Cambiar el ID de la hoja"
        onClick={() => {
          setText(value)
          setEditing(true)
        }}
      >
        Cambiar
      </Button>
      {value && (
        <a href={sheetUrl(value)} target="_blank" rel="noreferrer" className={linkClass}>
          <ExternalLink aria-hidden="true" className="h-4 w-4" />
          Abrir la hoja<span className="sr-only"> (se abre en otra pestaña)</span>
        </a>
      )}
    </div>
  )
}

interface LastImportProps {
  entry: AuditEntry
  now: number
  undone: boolean
  onUndo: (batchId: string) => void
  busy: boolean
}

function LastImport({ entry, now, undone, onUndo, busy }: LastImportProps) {
  const batchId = entry.batchId
  const isUndone = undone || Boolean(entry.undoneAt)
  return (
    <div className="flex flex-col gap-2 border-t border-gray-100 pt-4 sm:flex-row sm:items-center">
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium text-gray-700">
          Última importación{' '}
          {isUndone && (
            <Pill tone="gray" className="ml-1">
              Deshecha
            </Pill>
          )}
        </p>
        <p className="mt-0.5 break-words text-sm text-gray-600">{entry.summary}</p>
        <p className="mt-0.5 text-xs text-gray-500">
          <time dateTime={entry.at}>{formatRelative(entry.at, now)}</time> · {formatDateTime(entry.at)}
          {entry.actorLabel && ` · ${entry.actorLabel}`}
        </p>
      </div>
      {/* La entrada resumen puede no ser deshacible por sí misma; lo que se deshace es su lote. */}
      {batchId && !isUndone && (
        <Button variant="ghost" size="sm" icon={Undo2} loading={busy} onClick={() => onUndo(batchId)}>
          Deshacer importación
        </Button>
      )}
    </div>
  )
}

interface LegacyMediaCardProps {
  media: ImportStatus['legacyMedia'] | undefined
  onImported: () => void
}

function LegacyMediaCard({ media, onImported }: LegacyMediaCardProps) {
  const toast = useToast()
  const confirm = useConfirm()
  const undo = useUndo()
  const [busy, setBusy] = useState(false)
  const [warnings, setWarnings] = useState<string[]>([])

  const run = async () => {
    const ok = await confirm({
      title: '¿Importar imágenes heredadas?',
      message:
        'Se asignan las imágenes de la versión anterior (avatares, insignias y cursos) a los registros que coincidan. Puedes deshacerlo desde el aviso o desde Actividad.',
      confirmLabel: 'Importar imágenes',
    })
    if (!ok) return
    setBusy(true)
    try {
      const res = await api.admin.importMedia()
      setWarnings(res.warnings ?? [])
      const { avatars, badgeImages, courseImages } = res.media ?? { avatars: 0, badgeImages: 0, courseImages: 0 }
      const message = `Imágenes asignadas: ${plural(avatars, 'avatar', 'avatares')}, ${plural(badgeImages, 'insignia', 'insignias')} y ${plural(courseImages, 'curso', 'cursos')}.`
      const batchId = res.batchId
      // Sin batchId no cambió nada, así que no hay qué deshacer.
      if (batchId) {
        toast.success(message, {
          undo: async () => {
            if (await undo({ batchId })) onImported()
          },
        })
      } else {
        toast.success(message)
      }
      onImported()
    } catch (e) {
      toast.error(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card className="flex flex-col">
      <div className="flex items-start gap-3">
        <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-purple-100 text-purple-600">
          <Images aria-hidden="true" className="h-5 w-5" />
        </span>
        <div className="min-w-0">
          <h2 className="text-lg font-semibold text-gray-800">Imágenes heredadas</h2>
          <p className="mt-0.5 text-sm text-gray-500">Avatares e imágenes de insignias de la versión anterior.</p>
        </div>
      </div>
      <div className="mt-4 flex-1 text-sm text-gray-700">
        {!media ? (
          <p className="text-gray-500">Sin información por ahora.</p>
        ) : media.available ? (
          <p>
            Se encontraron {plural(media.avatars, 'avatar', 'avatares')} y {plural(media.badges, 'imagen', 'imágenes')}{' '}
            de insignias para asignar.
          </p>
        ) : (
          <p className="text-gray-500">No se encontraron imágenes de la versión anterior en el almacenamiento.</p>
        )}
      </div>
      <div className="mt-4">
        <Button
          variant="secondary"
          icon={Images}
          loading={busy}
          disabled={!media?.available}
          onClick={() => void run()}
        >
          Importar imágenes heredadas
        </Button>
      </div>
      {warnings.length > 0 && (
        <details className="mt-4 rounded-xl border border-yellow-200 bg-yellow-50">
          <summary className={cx('cursor-pointer rounded-xl px-3 py-2 text-sm font-medium text-yellow-900', focusRing)}>
            {plural(warnings.length, 'advertencia', 'advertencias')}
          </summary>
          <ul className="max-h-60 list-disc space-y-1 overflow-y-auto px-3 pb-3 pl-8 text-sm text-yellow-900">
            {warnings.map((w, i) => (
              <li key={i} className="break-words">
                {w}
              </li>
            ))}
          </ul>
        </details>
      )}
    </Card>
  )
}

function Problem({ children }: { children: ReactNode }) {
  return (
    <p role="alert" className="flex items-start gap-2 rounded-xl bg-red-50 px-4 py-3 text-sm text-red-700">
      <CircleX aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />
      <span className="min-w-0 break-words">{children}</span>
    </p>
  )
}

interface PreviewState {
  loading: boolean
  report?: ImportReport
  error?: string
}

export default function Import() {
  const toast = useToast()
  const confirm = useConfirm()
  const undo = useUndo()
  const now = useNow()
  const status = useAsync(() => api.admin.importStatus(), [])
  const reloadStatus = status.reload
  const info = status.data
  // Cuenta las vistas previas pedidas: sólo la última puede pintar su resultado.
  const previewSeq = useRef(0)

  // Credenciales de un solo uso: sólo en memoria, se olvidan al salir de la página.
  const [oneOff, setOneOff] = useState<ServiceAccount | null>(null)
  const [include, setInclude] = useState<Include>({ students: true, badges: true, purchases: true, media: true })
  const [mode, setMode] = useState<Mode>('upsert')
  const [view, setView] = useState<'preview' | 'job' | null>(() => (activeJob ? 'job' : null))
  const [preview, setPreview] = useState<PreviewState>({ loading: false })
  const [jobId, setJobId] = useState<string | null>(() => activeJob?.id ?? null)
  const [job, setJob] = useState<ImportJob | null>(null)
  const [pollError, setPollError] = useState('')
  // Trabajo que dejó de responder por fallas de red; se puede volver a consultar.
  const [stalled, setStalled] = useState<string | null>(null)
  const [elapsed, setElapsed] = useState(0)
  const [starting, setStarting] = useState(false)
  const [undoing, setUndoing] = useState(false)
  // Lotes que ya se deshicieron en esta visita, para esconder su botón.
  const [undone, setUndone] = useState<ReadonlySet<string>>(new Set())
  const [announce, setAnnounce] = useState('')

  const running = jobId !== null
  const mediaAvailable = info?.legacyMedia.available ?? false
  const effective: Include = { ...include, media: include.media && mediaAvailable }
  const nothingSelected = !Object.values(effective).some(Boolean)
  const credsMissing = info ? !info.credentials.configured && !oneOff : false
  const serviceEmail = oneOff?.clientEmail ?? info?.credentials.clientEmail
  const source: ImportSource = oneOff ? { credentialsJson: oneOff.json } : {}

  const markUndone = (batchId: string) => setUndone((s) => new Set(s).add(batchId))

  // Consulta el trabajo cada 1.5 s hasta que termina.
  useEffect(() => {
    if (!jobId) return
    let alive = true
    let timer: ReturnType<typeof setTimeout> | undefined
    let failures = 0
    const since = activeJob?.id === jobId ? activeJob.since : Date.now()

    // finished: el trabajo terminó o ya no existe; si no, se puede volver a consultar.
    const stop = (finished: boolean) => {
      if (finished) activeJob = null
      setJobId(null)
      reloadStatus()
    }

    const poll = async () => {
      try {
        const j = await api.admin.importJob(jobId)
        if (!alive) return
        failures = 0
        setJob(j)
        setPollError('')
        setElapsed(Math.round((Date.now() - since) / 1000))
        if (j.status === 'running') {
          timer = setTimeout(poll, POLL_MS)
          return
        }
        stop(true)
        const batchId = j.report?.batchId
        if (j.status === 'done') {
          setAnnounce('Importación terminada.')
          toast.success(
            'Importación terminada.',
            batchId
              ? {
                  undo: async () => {
                    if (await undo({ batchId })) {
                      markUndone(batchId)
                      reloadStatus()
                    }
                  },
                }
              : undefined,
          )
        } else {
          setAnnounce('La importación falló.')
          toast.error(`La importación falló: ${j.error || 'error desconocido'}.`)
        }
      } catch (e) {
        if (!alive) return
        failures += 1
        const gone = e instanceof ApiError && e.status === 404
        if (gone || failures >= 3) {
          stop(gone)
          if (!gone) setStalled(jobId)
          setPollError(
            gone
              ? 'El servidor ya no tiene ese trabajo (quizá se reinició). Revisa la última importación en la tarjeta de arriba.'
              : `No se pudo consultar el avance: ${errorMessage(e)}`,
          )
          return
        }
        timer = setTimeout(poll, POLL_MS)
      }
    }

    timer = setTimeout(poll, 0)
    return () => {
      alive = false
      clearTimeout(timer)
    }
  }, [jobId, reloadStatus, toast, undo])

  // Descarta la vista previa (cambió la hoja o las credenciales); una respuesta en camino se ignora.
  const clearPreview = () => {
    previewSeq.current += 1
    setPreview({ loading: false })
    setView((v) => (v === 'preview' ? null : v))
  }

  const onSpreadsheetSaved = (spreadsheetId: string, auditId: number) => {
    status.setData((s) => (s ? { ...s, spreadsheetId } : s))
    clearPreview()
    toast.success('Se guardó el ID de la hoja.', {
      undo: async () => {
        if (await undo({ auditId })) {
          reloadStatus()
          clearPreview()
        }
      },
    })
  }

  const runPreview = async () => {
    const seq = ++previewSeq.current
    setView('preview')
    setPreview({ loading: true })
    setAnnounce('')
    try {
      const report = await api.admin.importPreview(source)
      if (seq !== previewSeq.current) return
      setPreview({ loading: false, report })
      setAnnounce(
        `Vista previa lista: ${plural(
          (report.courses ?? []).reduce((t, c) => t + c.rows, 0),
          'fila',
          'filas',
        )} en ${plural((report.courses ?? []).length, 'curso', 'cursos')}.`,
      )
    } catch (e) {
      if (seq === previewSeq.current) setPreview({ loading: false, error: errorMessage(e) })
    }
  }

  const resumePolling = () => {
    if (!stalled) return
    setPollError('')
    setStalled(null)
    setJobId(stalled)
  }

  const startImport = async () => {
    const what = INCLUDE.filter(([key]) => effective[key]).map(([, label]) => label.toLowerCase())
    const ok = await confirm({
      title: '¿Importar desde Google Sheets?',
      message: (
        <div className="space-y-2">
          <p>
            Se importan: {what.join(', ')}. Modo: {MODES.find(([m]) => m === mode)?.[1].toLowerCase()}.
          </p>
          {!preview.report && <p className="font-medium text-orange-700">Todavía no revisas la vista previa.</p>}
          <p>Al terminar puedes deshacer toda la importación de una sola vez.</p>
        </div>
      ),
      confirmLabel: 'Importar',
    })
    if (!ok) return
    setStarting(true)
    try {
      const res = await api.admin.importRun({ ...source, include: effective, mode })
      activeJob = { id: res.jobId, since: Date.now() }
      setJob(null)
      setPollError('')
      setStalled(null)
      setElapsed(0)
      setAnnounce('Importación en curso.')
      setView('job')
      setJobId(res.jobId)
    } catch (e) {
      toast.error(errorMessage(e))
    } finally {
      setStarting(false)
    }
  }

  const undoImport = async (batchId: string) => {
    const ok = await confirm({
      title: '¿Deshacer toda la importación?',
      message: 'Se revierten todos sus cambios: lo que se creó se elimina y lo que se actualizó vuelve a como estaba.',
      confirmLabel: 'Deshacer importación',
      danger: true,
    })
    if (!ok) return
    setUndoing(true)
    if (await undo({ batchId })) {
      markUndone(batchId)
      reloadStatus()
    }
    setUndoing(false)
  }

  const copyEmail = async (email: string) => {
    try {
      await navigator.clipboard.writeText(email)
      toast.info('Correo copiado.')
    } catch {
      toast.error('No se pudo copiar. Selecciónalo y cópialo a mano.')
    }
  }

  const finishedBatch = job?.report?.batchId ?? null

  return (
    <>
      <PageHeader
        title="Importar desde Google Sheets"
        subtitle="Trae alumnos, insignias y compras de la hoja que se usaba antes. Revisa la vista previa antes de importar."
      />
      <p className="sr-only" aria-live="polite">
        {announce}
      </p>

      <div className="grid gap-6 xl:grid-cols-3">
        <Card className="xl:col-span-2">
          <h2 className="text-lg font-semibold text-gray-800">Origen de los datos</h2>
          {!info ? (
            <div className="mt-4">
              {status.error ? (
                <LoadError
                  title="No se pudo leer el estado de la importación"
                  message={errorMessage(status.error)}
                  onRetry={reloadStatus}
                  retrying={status.loading}
                />
              ) : (
                <PageLoading />
              )}
            </div>
          ) : (
            <div className="mt-4 space-y-5">
              <div>
                <h3 className="mb-2 text-sm font-semibold text-gray-700">Hoja de Google</h3>
                <SpreadsheetEditor value={info.spreadsheetId} onSaved={onSpreadsheetSaved} />
              </div>

              <div>
                <div className="mb-2 flex flex-wrap items-center gap-2">
                  <h3 className="text-sm font-semibold text-gray-700">Credenciales de Google</h3>
                  <SecretBadge status={info.credentials} word="Configuradas" />
                </div>
                {serviceEmail ? (
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="min-w-0 break-all font-mono text-xs text-gray-800">{serviceEmail}</span>
                    <IconButton
                      icon={Copy}
                      label="Copiar el correo de la cuenta de servicio"
                      size="sm"
                      onClick={() => void copyEmail(serviceEmail)}
                    />
                  </div>
                ) : null}
                <p className="mt-1 text-sm text-gray-600">
                  Comparte la hoja de Google con {serviceEmail ? 'ese correo' : 'el correo de la cuenta de servicio'}{' '}
                  (como lector) para que se pueda leer.
                </p>

                <div className="mt-3 rounded-xl border border-dashed border-gray-200 p-4">
                  {oneOff ? (
                    <div className="flex flex-wrap items-center gap-2">
                      <Pill tone="purple">JSON sólo para esta visita</Pill>
                      <span className="min-w-0 break-all font-mono text-xs text-gray-700">{oneOff.clientEmail}</span>
                      <Button
                        size="sm"
                        variant="ghost"
                        icon={X}
                        onClick={() => {
                          setOneOff(null)
                          clearPreview()
                        }}
                      >
                        Quitar
                      </Button>
                    </div>
                  ) : (
                    <div className="space-y-2">
                      <p className="text-sm text-gray-600">
                        ¿Prefieres no guardar las credenciales? Usa el JSON de la cuenta de servicio sólo aquí; no se
                        guarda y se olvida al salir de esta página.
                      </p>
                      <ServiceAccountPicker
                        label="Usar un JSON sólo esta vez"
                        variant="ghost"
                        onLoad={(sa) => {
                          setOneOff(sa)
                          clearPreview()
                        }}
                      />
                    </div>
                  )}
                  <p className="mt-2 text-xs text-gray-500">
                    Para guardarlas de forma permanente, súbelas en{' '}
                    <Link to="/admin/ajustes#google" className="font-medium text-purple-600 hover:underline">
                      Ajustes
                    </Link>
                    .
                  </p>
                </div>
              </div>

              {info.lastImport && (
                <LastImport
                  entry={info.lastImport}
                  now={now}
                  undone={Boolean(info.lastImport.batchId && undone.has(info.lastImport.batchId))}
                  busy={undoing}
                  onUndo={(batchId) => void undoImport(batchId)}
                />
              )}
            </div>
          )}
        </Card>

        <LegacyMediaCard media={info?.legacyMedia} onImported={reloadStatus} />
      </div>

      <Card className="mt-6">
        <h2 className="text-lg font-semibold text-gray-800">Importar</h2>
        <div className="mt-4 grid gap-6 md:grid-cols-2">
          <fieldset>
            <legend className="text-sm font-semibold text-gray-700">Qué importar</legend>
            <div className="mt-2 space-y-2">
              {INCLUDE.map(([key, label, hint]) => {
                const disabled = running || (key === 'media' && !mediaAvailable)
                return (
                  <label
                    key={key}
                    className={cx(
                      'flex items-start gap-3 rounded-xl border border-gray-100 p-3',
                      disabled ? 'cursor-not-allowed opacity-60' : 'cursor-pointer hover:bg-gray-50',
                    )}
                  >
                    <input
                      type="checkbox"
                      checked={effective[key]}
                      disabled={disabled}
                      onChange={(e) => setInclude((s) => ({ ...s, [key]: e.target.checked }))}
                      className="mt-0.5 h-4 w-4 shrink-0 accent-orange-500"
                    />
                    <span className="min-w-0">
                      <span className="block text-sm font-medium text-gray-800">{label}</span>
                      <span className="block text-xs text-gray-500">
                        {key === 'media' && !mediaAvailable ? 'No hay imágenes heredadas disponibles.' : hint}
                      </span>
                    </span>
                  </label>
                )
              })}
            </div>
          </fieldset>

          <fieldset>
            <legend className="text-sm font-semibold text-gray-700">Si un registro ya existe</legend>
            <div className="mt-2 space-y-2">
              {MODES.map(([value, label, hint]) => (
                <label
                  key={value}
                  className={cx(
                    'flex items-start gap-3 rounded-xl border p-3',
                    mode === value ? 'border-orange-300 bg-orange-50' : 'border-gray-100',
                    running ? 'cursor-not-allowed opacity-60' : 'cursor-pointer',
                  )}
                >
                  <input
                    type="radio"
                    name="import-mode"
                    value={value}
                    checked={mode === value}
                    disabled={running}
                    onChange={() => setMode(value)}
                    className="mt-0.5 h-4 w-4 shrink-0 accent-orange-500"
                  />
                  <span className="min-w-0">
                    <span className="block text-sm font-medium text-gray-800">{label}</span>
                    <span className="block text-xs text-gray-500">{hint}</span>
                  </span>
                </label>
              ))}
            </div>
          </fieldset>
        </div>

        <div className="mt-6 flex flex-col gap-3 border-t border-gray-100 pt-4 sm:flex-row sm:items-center">
          <p className="min-w-0 flex-1 text-xs text-gray-500">
            {credsMissing
              ? 'Faltan las credenciales de Google: súbelas en Ajustes o usa un JSON sólo esta vez.'
              : nothingSelected
                ? 'Elige al menos una cosa para importar.'
                : 'La vista previa sólo lee la hoja; no cambia nada.'}
          </p>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="secondary"
              icon={Eye}
              loading={preview.loading}
              disabled={running || credsMissing}
              onClick={() => void runPreview()}
            >
              Vista previa
            </Button>
            <Button
              icon={Play}
              loading={starting}
              disabled={running || credsMissing || nothingSelected || preview.loading}
              onClick={() => void startImport()}
            >
              Importar
            </Button>
          </div>
        </div>
      </Card>

      {view === 'preview' && (
        <section aria-labelledby="import-preview-title" className="mt-6">
          <Card>
            <h2 id="import-preview-title" className="mb-4 text-lg font-semibold text-gray-800">
              Vista previa
            </h2>
            {preview.loading ? (
              <div className="flex justify-center py-10">
                <Spinner label="Leyendo la hoja de Google…" showLabel />
              </div>
            ) : preview.error ? (
              <Problem>{preview.error}</Problem>
            ) : preview.report ? (
              <ImportReportView report={preview.report} />
            ) : null}
          </Card>
        </section>
      )}

      {view === 'job' && (
        <section aria-labelledby="import-job-title" className="mt-6">
          <Card>
            <div className="mb-4 flex flex-col items-start gap-3 sm:flex-row sm:items-center">
              <h2 id="import-job-title" className="min-w-0 text-lg font-semibold text-gray-800 sm:flex-1">
                {running
                  ? 'Importando…'
                  : job?.status === 'error'
                    ? 'La importación falló'
                    : job?.status === 'done'
                      ? 'Resultado de la importación'
                      : 'Importación'}
              </h2>
              {!running &&
                finishedBatch &&
                job?.status === 'done' &&
                (undone.has(finishedBatch) ? (
                  <Pill tone="gray">Importación deshecha</Pill>
                ) : (
                  <Button
                    variant="danger"
                    size="sm"
                    icon={Undo2}
                    loading={undoing}
                    onClick={() => void undoImport(finishedBatch)}
                  >
                    Deshacer importación
                  </Button>
                ))}
            </div>

            {running ? (
              <div>
                <p role="status" className="flex items-center gap-2 text-sm font-medium text-gray-700">
                  <LoaderCircle aria-hidden="true" className="h-4 w-4 animate-spin text-orange-500" />
                  Leyendo la hoja y guardando los cambios…
                </p>
                {/* El trabajo no reporta porcentaje: barra indeterminada, sólo visual. */}
                <div aria-hidden="true" className="mt-3 h-2 overflow-hidden rounded-full bg-orange-100">
                  <div className="h-full w-full animate-pulse rounded-full bg-orange-400" />
                </div>
                <p className="mt-2 text-xs text-gray-500">
                  Lleva {elapsed} s. Puedes ir a otra sección del panel; al volver verás el avance.
                </p>
              </div>
            ) : (
              <div className="space-y-4">
                {pollError && <Problem>{pollError}</Problem>}
                {stalled && (
                  <Button variant="ghost" icon={RefreshCw} onClick={resumePolling}>
                    Volver a consultar
                  </Button>
                )}
                {job?.status === 'error' && <Problem>{job.error || 'Error desconocido.'}</Problem>}
                {job?.report && <ImportReportView report={job.report} />}
                {job?.status === 'error' && finishedBatch && !undone.has(finishedBatch) && (
                  <Button variant="ghost" icon={Undo2} loading={undoing} onClick={() => void undoImport(finishedBatch)}>
                    Deshacer lo que alcanzó a importarse
                  </Button>
                )}
              </div>
            )}
          </Card>
        </section>
      )}
    </>
  )
}
