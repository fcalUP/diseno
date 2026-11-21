import { useMemo, useState } from 'react'
import { ImageOff, LoaderCircle, SearchX, TrendingUp, Users } from 'lucide-react'
import { api, errorMessage } from '../../lib/api'
import type { AdminStudent, Avatar } from '../../lib/types'
import {
  Button,
  Card,
  DataTable,
  EmptyState,
  ImageUpload,
  Modal,
  PageHeader,
  Pill,
  SearchInput,
  Select,
  Toggle,
  cx,
  focusRing,
  useAsync,
  type Column,
} from '../ui'
import { ErrorBanner, LinkButton, LoadingBlock } from './students/parts'
import { useAvatarActions } from './students/useAvatarActions'
import { avatarKey, fold, formatNumber, pl, sortCourses } from './students/util'

// Igual que los archivos de la versión anterior: DefaultH para "Hombre", DefaultF para todo lo demás.
const DEFAULTS = [
  { code: 'DefaultH', label: 'Hombre' },
  { code: 'DefaultF', label: 'Mujer' },
]

interface CodeRow {
  code: string
  names: string[]
  courseIds: number[]
  courseNames: string[]
  missing: number
}

interface Slot {
  code: string
  level: number
  who: string
}

// Los avatares van por matrícula: quien está en dos cursos comparte imágenes.
function groupByCode(students: readonly AdminStudent[]): Omit<CodeRow, 'missing'>[] {
  const byCode = new Map<string, Omit<CodeRow, 'missing'>>()
  for (const s of students) {
    let row = byCode.get(s.code)
    if (!row) {
      row = { code: s.code, names: [], courseIds: [], courseNames: [] }
      byCode.set(s.code, row)
    }
    if (!row.names.includes(s.name)) row.names.push(s.name)
    row.courseIds.push(s.courseId)
    row.courseNames.push(s.courseName)
  }
  return [...byCode.values()].sort(
    (a, b) => a.names[0].localeCompare(b.names[0], 'es') || a.code.localeCompare(b.code, 'es'),
  )
}

function SlotButton({ url, label, busy, onClick }: { url?: string; label: string; busy: boolean; onClick: () => void }) {
  const text = url ? `Cambiar avatar del ${label}` : `Subir avatar del ${label} (falta)`
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={busy}
      aria-label={text}
      title={text}
      className={cx(
        'relative inline-flex h-12 w-12 items-center justify-center overflow-hidden rounded-lg align-middle transition disabled:cursor-wait',
        focusRing,
        url
          ? 'border border-gray-200 bg-gray-50 hover:ring-2 hover:ring-orange-300'
          : 'border-2 border-dashed border-red-300 bg-red-50 text-[11px] font-semibold text-red-600 hover:border-red-500',
      )}
    >
      {url ? <img src={url} alt="" loading="lazy" className="h-full w-full object-cover" /> : 'falta'}
      {busy && (
        <span className="absolute inset-0 flex items-center justify-center bg-white/70">
          <LoaderCircle aria-hidden="true" className="h-4 w-4 animate-spin text-orange-500" />
        </span>
      )}
    </button>
  )
}

function SlotDialog({
  slot,
  url,
  onClose,
  onChange,
}: {
  slot: Slot
  url: string
  onClose: () => void
  onChange: (url: string) => Promise<boolean>
}) {
  const [saving, setSaving] = useState(false)
  return (
    <Modal
      open
      onClose={onClose}
      title={`Avatar del nivel ${slot.level}`}
      description={`${slot.who} · ${slot.code}`}
      size="sm"
      footer={
        <Button variant="ghost" onClick={onClose}>
          Cerrar
        </Button>
      }
    >
      <ImageUpload
        label="Imagen"
        value={url}
        disabled={saving}
        previewClassName="h-56 w-full"
        onChange={async (next) => {
          setSaving(true)
          const ok = await onChange(next)
          setSaving(false)
          if (ok) onClose()
        }}
      />
      {saving && (
        <p role="status" className="mt-2 text-sm text-gray-500">
          Guardando…
        </p>
      )}
    </Modal>
  )
}

export default function Avatars() {
  const levelsQ = useAsync(() => api.admin.levels(), [])
  const avatarsQ = useAsync(() => api.admin.avatars(), [])
  const studentsQ = useAsync(() => api.admin.students(), [])
  const coursesQ = useAsync(() => api.admin.courses(), [])
  const actions = useAvatarActions(avatarsQ.reload)

  const [query, setQuery] = useState('')
  const [courseFilter, setCourseFilter] = useState('')
  const [onlyMissing, setOnlyMissing] = useState(false)
  const [slot, setSlot] = useState<Slot | null>(null)
  const [busySlot, setBusySlot] = useState<string | null>(null)

  const levels = useMemo(() => [...(levelsQ.data?.levels ?? [])].sort((a, b) => a.level - b.level), [levelsQ.data])
  const urls = useMemo(
    () => new Map((avatarsQ.data?.avatars ?? []).map((a) => [avatarKey(a.code, a.level), a.url])),
    [avatarsQ.data],
  )
  const courses = useMemo(() => sortCourses(coursesQ.data?.courses ?? []), [coursesQ.data])
  const codeRows = useMemo(
    () =>
      groupByCode(studentsQ.data?.students ?? []).map((r) => ({
        ...r,
        missing: levels.filter((l) => !urls.has(avatarKey(r.code, l.level))).length,
      })),
    [studentsQ.data, levels, urls],
  )

  const visible = useMemo(() => {
    const q = fold(query)
    const cid = Number(courseFilter)
    return codeRows.filter(
      (r) =>
        (!cid || r.courseIds.includes(cid)) &&
        (!onlyMissing || r.missing > 0) &&
        (!q || fold(r.code).includes(q) || r.names.some((n) => fold(n).includes(q))),
    )
  }, [codeRows, query, courseFilter, onlyMissing])

  const totalSlots = codeRows.length * levels.length
  const filled = totalSlots - codeRows.reduce((sum, r) => sum + r.missing, 0)
  const withMissing = codeRows.filter((r) => r.missing > 0).length

  const applySaved = (a: Avatar) =>
    avatarsQ.setData((d) => d && { avatars: [...(d.avatars ?? []).filter((x) => x.code !== a.code || x.level !== a.level), a] })
  const applyRemoved = (code: string, level: number) =>
    avatarsQ.setData((d) => d && { avatars: (d.avatars ?? []).filter((x) => x.code !== code || x.level !== level) })

  // Con url guarda la imagen; con '' la quita (pide confirmación). Devuelve true si cambió algo.
  const changeSlot = async (code: string, level: number, url: string): Promise<boolean> => {
    setBusySlot(avatarKey(code, level))
    try {
      if (url) {
        const saved = await actions.save(code, level, url)
        if (saved) applySaved(saved)
        return saved !== null
      }
      const removed = await actions.remove(code, level)
      if (removed) applyRemoved(code, level)
      return removed
    } finally {
      setBusySlot(null)
    }
  }

  const columns: Column<CodeRow>[] = [
    {
      key: 'who',
      header: 'Alumno',
      cell: (r) => (
        <div className="min-w-[11rem]">
          <p className="font-medium text-gray-800">{r.names.join(' / ')}</p>
          <p className="text-xs text-gray-500">
            <span className="font-mono">{r.code}</span> · {r.courseNames.join(', ')}
          </p>
        </div>
      ),
    },
    {
      key: 'coverage',
      header: 'Cobertura',
      align: 'center',
      cell: (r) =>
        r.missing === 0 ? (
          <Pill tone="green">Completo</Pill>
        ) : (
          <Pill tone="red">
            {pl(r.missing, 'Falta', 'Faltan')} {r.missing}
          </Pill>
        ),
    },
    ...levels.map(
      (l): Column<CodeRow> => ({
        key: `nivel-${l.level}`,
        header: `Nivel ${l.level}`,
        align: 'center',
        cell: (r) => (
          <SlotButton
            url={urls.get(avatarKey(r.code, l.level))}
            label={`nivel ${l.level} de ${r.names[0]} (${r.code})`}
            busy={busySlot === avatarKey(r.code, l.level)}
            onClick={() => setSlot({ code: r.code, level: l.level, who: r.names.join(' / ') })}
          />
        ),
      }),
    ),
  ]

  const loadError = levelsQ.error ?? avatarsQ.error
  const filtersOn = query.trim() !== '' || courseFilter !== '' || onlyMissing

  return (
    <>
      <PageHeader
        title="Avatares"
        subtitle="Una imagen por nivel. Si a un alumno le falta la de su nivel, ve el avatar por defecto."
      />

      {loadError && (
        <ErrorBanner
          className="mb-4"
          message={`No se pudieron cargar los avatares: ${errorMessage(loadError)}`}
          onRetry={() => {
            levelsQ.reload()
            avatarsQ.reload()
          }}
        />
      )}

      <Card className="mb-8">
        <h2 className="text-base font-semibold text-gray-800">Avatares por defecto</h2>
        <p className="mt-1 text-sm text-gray-500">
          DefaultH se usa cuando el sexo del alumno es Hombre y DefaultF en cualquier otro caso. PNG, JPG, GIF o WEBP
          de hasta 20 MB.
        </p>
        {levels.length === 0 || !avatarsQ.data ? (
          levelsQ.loading || avatarsQ.loading ? (
            <LoadingBlock label="Cargando avatares…" />
          ) : (
            !loadError && (
              <EmptyState
                icon={TrendingUp}
                title="No hay niveles"
                description="Configura los niveles antes de subir avatares."
                action={<LinkButton to="/admin/niveles">Ir a Niveles</LinkButton>}
              />
            )
          )
        ) : (
          <div className="mt-5 space-y-6">
            {DEFAULTS.map((d) => (
              <section key={d.code} aria-labelledby={`avatar-defecto-${d.code}`}>
                <h3 id={`avatar-defecto-${d.code}`} className="mb-3 text-sm font-semibold text-gray-700">
                  {d.label} <span className="font-mono text-xs font-normal text-gray-500">{d.code}</span>
                </h3>
                <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
                  {levels.map((l) => {
                    const key = avatarKey(d.code, l.level)
                    return (
                      <ImageUpload
                        key={l.level}
                        label={`Nivel ${l.level} · ${d.label}`}
                        hint=""
                        value={urls.get(key) ?? ''}
                        disabled={busySlot === key || avatarsQ.loading}
                        previewClassName="h-28 w-full"
                        onChange={(url) => void changeSlot(d.code, l.level, url)}
                      />
                    )
                  })}
                </div>
              </section>
            ))}
          </div>
        )}
      </Card>

      <section aria-labelledby="avatar-cobertura">
        <div className="mb-4">
          <h2 id="avatar-cobertura" className="text-lg font-semibold text-gray-800">
            Cobertura por alumno
          </h2>
          {studentsQ.data && levels.length > 0 && (
            <p className="mt-1 text-sm text-gray-500">
              {formatNumber(filled)} de {formatNumber(totalSlots)} espacios con imagen ·{' '}
              {withMissing === 0
                ? 'nadie tiene faltantes.'
                : `${formatNumber(withMissing)} ${pl(withMissing, 'alumno con faltantes', 'alumnos con faltantes')}.`}
            </p>
          )}
        </div>

        {studentsQ.error && (
          <ErrorBanner
            className="mb-4"
            message={`No se pudieron cargar los alumnos: ${errorMessage(studentsQ.error)}`}
            onRetry={studentsQ.reload}
          />
        )}

        <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center">
          <SearchInput
            value={query}
            onChange={setQuery}
            placeholder="Buscar por ID o nombre"
            label="Buscar alumnos"
            className="sm:flex-1"
          />
          <Select
            label="Curso"
            hideLabel
            value={courseFilter}
            onChange={setCourseFilter}
            options={courses.map((c) => ({ value: c.id, label: c.name }))}
            placeholder="Todos los cursos"
            className="sm:w-56"
          />
          <Toggle label="Solo con faltantes" checked={onlyMissing} onChange={setOnlyMissing} />
        </div>

        <DataTable
          rows={visible}
          columns={columns}
          rowKey={(r) => r.code}
          loading={studentsQ.loading || levelsQ.loading || (avatarsQ.loading && !avatarsQ.data)}
          caption="Avatares por alumno y nivel"
          className="relative max-h-[70vh]"
          empty={
            codeRows.length === 0 ? (
              <EmptyState
                icon={Users}
                title="No hay alumnos"
                description="Cuando haya alumnos, aquí verás qué avatares les faltan."
                action={<LinkButton to="/admin/generar">Generar alumnos</LinkButton>}
              />
            ) : onlyMissing && withMissing === 0 ? (
              <EmptyState icon={ImageOff} title="No falta ninguno" description="Todos los alumnos tienen avatar en cada nivel." />
            ) : (
              <EmptyState
                icon={SearchX}
                title="Sin resultados"
                description="Ningún alumno coincide con la búsqueda."
                action={
                  filtersOn && (
                    <Button
                      variant="ghost"
                      onClick={() => {
                        setQuery('')
                        setCourseFilter('')
                        setOnlyMissing(false)
                      }}
                    >
                      Limpiar filtros
                    </Button>
                  )
                }
              />
            )
          }
        />
      </section>

      {slot && (
        <SlotDialog
          slot={slot}
          url={urls.get(avatarKey(slot.code, slot.level)) ?? ''}
          onClose={() => setSlot(null)}
          onChange={(url) => changeSlot(slot.code, slot.level, url)}
        />
      )}
    </>
  )
}
