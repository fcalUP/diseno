import { useState, type ReactNode } from 'react'
import { ArrowUpDown, Award, Coins, EyeOff, ImageOff, Pencil, Plus } from 'lucide-react'
import { api, errorMessage } from '../../lib/api'
import type { AdminBadge, AdminCourse, AuditRef } from '../../lib/types'
import {
  Button,
  Card,
  EmptyState,
  PageHeader,
  Pill,
  SearchInput,
  Select,
  Toggle,
  useAsync,
  useToast,
  useUndo,
} from '../ui'
import { BadgeDrawer } from './catalog/BadgeDrawer'
import { BadgeOrderDrawer } from './catalog/BadgeOrderDrawer'
import { LoadError, LoadingCard } from './catalog/PageState'
import { StockStepper } from './catalog/StockStepper'
import { catalogCount, catalogNormalize, catalogNumber, catalogWith } from './catalog/utils'

type Status = 'all' | 'active' | 'inactive' | 'out'

const STATUS_OPTIONS = [
  { value: 'all', label: 'Todas' },
  { value: 'active', label: 'Activas' },
  { value: 'inactive', label: 'Inactivas' },
  { value: 'out', label: 'Agotadas (stock 0)' },
]

const badgeOrder = (a: AdminBadge, b: AdminBadge) =>
  a.sortOrder - b.sortOrder || a.name.localeCompare(b.name, 'es') || a.id - b.id
const courseOrder = (a: AdminCourse, b: AdminCourse) => a.sortOrder - b.sortOrder || a.id - b.id

interface BadgeCardProps {
  badge: AdminBadge
  courseNames: Map<number, string>
  onActive: (badge: AdminBadge, active: boolean) => void
  onStock: (badge: AdminBadge, stock: number) => Promise<void>
  onEdit: (badge: AdminBadge) => void
}

function BadgeCard({ badge: b, courseNames, onActive, onStock, onEdit }: BadgeCardProps) {
  // Si los cursos no cargaron todavía, se muestra el id en vez de decir que se ve en todos.
  const courseName = (id: number) => courseNames.get(id) ?? `Curso #${id}`
  return (
    <Card padded={false} className="flex h-full flex-col overflow-hidden">
      <div className="relative flex h-36 items-center justify-center bg-gray-50">
        {b.imageUrl ? (
          <img src={b.imageUrl} alt="" loading="lazy" className="h-28 w-28 object-contain" />
        ) : (
          <ImageOff aria-hidden="true" className="h-8 w-8 text-gray-300" />
        )}
        <div className="absolute inset-x-3 top-3 flex justify-between gap-2">
          <span>{!b.active && <Pill>Inactiva</Pill>}</span>
          <span>{b.stock === 0 && <Pill tone="red">Agotada</Pill>}</span>
        </div>
      </div>
      <div className="flex flex-1 flex-col gap-3 p-4">
        <div className="min-w-0">
          <h2 className="break-words font-semibold text-gray-800">{b.name}</h2>
          <p className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-gray-600">
            <span className="inline-flex items-center gap-1">
              <Coins aria-hidden="true" className="h-4 w-4 text-yellow-500" />
              {catalogCount(b.cost, 'moneda', 'monedas')}
            </span>
            <span>{catalogCount(b.purchasedCount, 'vendida', 'vendidas')}</span>
          </p>
        </div>
        <div className="flex items-center justify-between gap-2">
          <span className="text-sm font-medium text-gray-700">Stock</span>
          <StockStepper name={b.name} stock={b.stock} onCommit={(stock) => onStock(b, stock)} />
        </div>
        <div className="text-xs">
          {b.hiddenCourseIds.length === 0 ? (
            <p className="text-gray-500">Visible en todos los cursos</p>
          ) : (
            <div className="flex flex-wrap items-center gap-1">
              <span className="text-gray-500">Oculta en:</span>
              {b.hiddenCourseIds.map((id) => (
                <Pill key={id} title={courseName(id)}>
                  <EyeOff aria-hidden="true" className="h-3 w-3" />
                  <span className="max-w-[10rem] truncate">{courseName(id)}</span>
                </Pill>
              ))}
            </div>
          )}
        </div>
        <div className="mt-auto flex items-center justify-between gap-2 border-t border-gray-100 pt-3">
          <div className="flex items-center gap-2">
            <Toggle checked={b.active} onChange={(v) => onActive(b, v)} label={`«${b.name}» activa`} hideLabel />
            {/* El lector de pantalla ya oye el nombre del interruptor; el texto sólo ayuda al clic. */}
            <span
              aria-hidden="true"
              className="cursor-pointer select-none text-sm text-gray-600"
              onClick={() => onActive(b, !b.active)}
            >
              Activa
            </span>
          </div>
          <Button variant="ghost" size="sm" icon={Pencil} aria-label={`Editar «${b.name}»`} onClick={() => onEdit(b)}>
            Editar
          </Button>
        </div>
      </div>
    </Card>
  )
}

export default function Badges() {
  const badges = useAsync(() => api.admin.badges().then((r) => r.badges), [])
  const courses = useAsync(() => api.admin.courses().then((r) => r.courses), [])
  const toast = useToast()
  const undo = useUndo()
  // undefined = cajón cerrado, null = insignia nueva.
  const [editing, setEditing] = useState<AdminBadge | null | undefined>(undefined)
  const [ordering, setOrdering] = useState(false)
  const [search, setSearch] = useState('')
  const [courseId, setCourseId] = useState('')
  const [status, setStatus] = useState<Status>('all')
  const [toggling, setToggling] = useState<ReadonlySet<number>>(new Set())

  const all = (badges.data ?? []).toSorted(badgeOrder)
  const courseList = courses.data?.toSorted(courseOrder)
  const courseNames = new Map((courseList ?? []).map((c) => [c.id, c.name]))
  const reload = badges.reload

  const term = catalogNormalize(search)
  const filterCourse = courseId ? Number(courseId) : null
  const shown = all.filter(
    (b) =>
      (status === 'all' ||
        (status === 'active' && b.active) ||
        (status === 'inactive' && !b.active) ||
        (status === 'out' && b.stock === 0)) &&
      (filterCourse === null || !b.hiddenCourseIds.includes(filterCourse)) &&
      (!term || catalogNormalize(b.name).includes(term)),
  )
  const filtering = Boolean(term || filterCourse !== null || status !== 'all')

  const undoThenReload = async (ref: AuditRef) => {
    if (await undo(ref)) reload()
  }

  const upsert = (badge: AdminBadge) =>
    badges.setData((prev = []) =>
      prev.some((b) => b.id === badge.id) ? prev.map((b) => (b.id === badge.id ? badge : b)) : [...prev, badge],
    )

  const setActive = async (badge: AdminBadge, active: boolean) => {
    if (toggling.has(badge.id)) return
    upsert({ ...badge, active })
    setToggling((s) => catalogWith(s, badge.id, true))
    try {
      const r = await api.admin.updateBadge(badge.id, { active })
      upsert(r.badge)
      toast.success(
        active ? `«${r.badge.name}» vuelve a la tienda.` : `«${r.badge.name}» desactivada: ya no aparece en la tienda.`,
        { undo: () => undoThenReload({ auditId: r.auditId }) },
      )
    } catch (e) {
      upsert(badge)
      toast.error(errorMessage(e))
    } finally {
      setToggling((s) => catalogWith(s, badge.id, false))
    }
  }

  const setStock = async (badge: AdminBadge, stock: number) => {
    try {
      const r = await api.admin.updateBadge(badge.id, { stock })
      upsert(r.badge)
      toast.success(`Stock de «${r.badge.name}»: ${catalogNumber(badge.stock)} → ${catalogNumber(r.badge.stock)}.`, {
        undo: () => undoThenReload({ auditId: r.auditId }),
      })
    } catch (e) {
      toast.error(errorMessage(e))
    }
  }

  const clearFilters = () => {
    setSearch('')
    setCourseId('')
    setStatus('all')
  }

  let content: ReactNode
  if (!badges.data) {
    content = badges.error ? (
      <LoadError message={errorMessage(badges.error)} onRetry={reload} />
    ) : (
      <LoadingCard label="Cargando insignias…" />
    )
  } else if (all.length === 0) {
    content = (
      <Card padded={false}>
        <EmptyState
          icon={Award}
          title="Todavía no hay insignias"
          description="Crea la primera para abrir la tienda, o impórtalas desde Google Sheets en Importar."
          action={
            <Button icon={Plus} onClick={() => setEditing(null)}>
              Nueva insignia
            </Button>
          }
        />
      </Card>
    )
  } else {
    content = (
      <>
        {badges.error && <LoadError className="mb-4" message={errorMessage(badges.error)} onRetry={reload} />}
        {courses.error && (
          <LoadError
            className="mb-4"
            message={`No se pudieron cargar los cursos: ${errorMessage(courses.error)}`}
            onRetry={courses.reload}
          />
        )}
        <div className="mb-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)]">
          <SearchInput
            value={search}
            onChange={setSearch}
            label="Buscar insignia"
            placeholder="Buscar por nombre…"
            className="sm:col-span-2 lg:col-span-1"
          />
          <Select
            label="Curso"
            hideLabel
            value={courseId}
            onChange={setCourseId}
            placeholder="Todos los cursos"
            options={(courseList ?? []).map((c) => ({ value: c.id, label: `Visibles en ${c.name}` }))}
          />
          <Select
            label="Estado"
            hideLabel
            value={status}
            onChange={(v) => setStatus(v as Status)}
            options={STATUS_OPTIONS}
          />
        </div>
        <p className="mb-3 text-sm text-gray-500" aria-live="polite">
          {filtering
            ? `${catalogCount(shown.length, 'insignia', 'insignias')} de ${catalogNumber(all.length)}`
            : catalogCount(all.length, 'insignia', 'insignias')}
          {filterCourse !== null &&
            courseNames.has(filterCourse) &&
            ` · las que se ven en ${courseNames.get(filterCourse)}`}
        </p>
        {shown.length === 0 ? (
          <Card padded={false}>
            <EmptyState
              title="Ninguna insignia coincide"
              description="Prueba con otra búsqueda o quita los filtros."
              action={
                <Button variant="ghost" onClick={clearFilters}>
                  Quitar filtros
                </Button>
              }
            />
          </Card>
        ) : (
          <ul role="list" className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
            {shown.map((b) => (
              <li key={b.id}>
                <BadgeCard
                  badge={b}
                  courseNames={courseNames}
                  onActive={(badge, active) => void setActive(badge, active)}
                  onStock={setStock}
                  onEdit={setEditing}
                />
              </li>
            ))}
          </ul>
        )}
      </>
    )
  }

  return (
    <>
      <PageHeader
        title="Insignias"
        subtitle="Lo que los alumnos compran con sus monedas en la tienda."
        actions={
          <>
            <Button variant="secondary" icon={ArrowUpDown} onClick={() => setOrdering(true)} disabled={all.length < 2}>
              Ordenar
            </Button>
            <Button icon={Plus} onClick={() => setEditing(null)}>
              Nueva insignia
            </Button>
          </>
        }
      />
      {content}
      {editing !== undefined && (
        <BadgeDrawer
          badge={editing}
          badges={all}
          courses={courseList}
          coursesError={courses.error ? errorMessage(courses.error) : ''}
          onRetryCourses={courses.reload}
          onClose={() => setEditing(undefined)}
          onSaved={upsert}
          onDeleted={(id) => badges.setData((prev = []) => prev.filter((b) => b.id !== id))}
          onUndone={reload}
        />
      )}
      {ordering && (
        <BadgeOrderDrawer
          badges={all}
          onClose={() => setOrdering(false)}
          onSaved={(batchId, ids) => {
            const pos = new Map(ids.map((id, i) => [id, i + 1]))
            badges.setData((prev = []) => prev.map((b) => ({ ...b, sortOrder: pos.get(b.id) ?? b.sortOrder })))
            toast.success('Orden de las insignias guardado.', { undo: () => undoThenReload({ batchId }) })
          }}
        />
      )}
    </>
  )
}
