import { useState } from 'react'
import { Download, LoaderCircle, Redo2, ShoppingCart, Undo2 } from 'lucide-react'
import { api, errorMessage } from '../../lib/api'
import { downloadCsv, toCsv, type CsvColumn } from '../../lib/csv'
import type { AdminPurchase, AuditRef } from '../../lib/types'
import {
  Button,
  DataTable,
  EmptyState,
  PageHeader,
  Pill,
  SearchInput,
  Select,
  useAsync,
  useConfirm,
  useToast,
  useUndo,
  type Column,
} from '../ui'
import { LoadError } from './catalog/PageState'
import { useCatalogDebounce } from './catalog/useCatalogDebounce'
import {
  catalogCount,
  catalogDateParts,
  catalogDateTime,
  catalogNumber,
  catalogStamp,
  catalogWith,
} from './catalog/utils'

type Status = 'all' | 'false' | 'true'

const STATUS_OPTIONS = [
  { value: 'all', label: 'Todas las compras' },
  { value: 'false', label: 'Vigentes' },
  { value: 'true', label: 'Deshechas' },
]

const SOURCES = {
  app: { label: 'Tienda', full: 'Tienda del portal', tone: 'orange' },
  sheets: { label: 'Sheets', full: 'Importada de Google Sheets', tone: 'green' },
  admin: { label: 'Panel', full: 'Hecha desde el panel', tone: 'blue' },
} as const

// Sin límite el servidor manda las 500 compras más recientes; acepta hasta 5000.
const PAGE = 500
const MAX_LIMIT = 5000

const CSV_COLUMNS: CsvColumn<AdminPurchase>[] = [
  { header: 'Fecha', value: (p) => catalogStamp(p.createdAt) },
  { header: 'Matrícula', value: (p) => p.studentCode },
  { header: 'Alumno', value: (p) => p.studentName },
  { header: 'Curso', value: (p) => p.courseName },
  { header: 'Insignia', value: (p) => p.badgeName },
  { header: 'Cantidad', value: (p) => p.quantity },
  { header: 'Costo unitario', value: (p) => p.unitCost },
  { header: 'Costo total', value: (p) => p.totalCost },
  { header: 'Monedas antes', value: (p) => p.coinsBefore },
  { header: 'Monedas después', value: (p) => p.coinsAfter },
  { header: 'Origen', value: (p) => SOURCES[p.source]?.full ?? p.source },
  { header: 'Estado', value: (p) => (p.revertedAt ? 'Deshecha' : 'Vigente') },
  { header: 'Deshecha el', value: (p) => catalogStamp(p.revertedAt) },
]

const byName = (a: { name: string }, b: { name: string }) => a.name.localeCompare(b.name, 'es')

export default function Purchases() {
  const toast = useToast()
  const confirm = useConfirm()
  const undo = useUndo()
  const courses = useAsync(() => api.admin.courses().then((r) => r.courses), [])
  const badges = useAsync(() => api.admin.badges().then((r) => r.badges), [])
  const [courseId, setCourseId] = useState('')
  const [badgeId, setBadgeId] = useState('')
  const [status, setStatus] = useState<Status>('all')
  const [search, setSearch] = useState('')
  const [limit, setLimit] = useState(PAGE)
  const [busy, setBusy] = useState<ReadonlySet<number>>(new Set())
  const q = useCatalogDebounce(search.trim(), 300)

  const purchases = useAsync(
    () =>
      api.admin
        .purchases({
          courseId: courseId ? Number(courseId) : undefined,
          badgeId: badgeId ? Number(badgeId) : undefined,
          q: q || undefined,
          reverted: status,
          limit,
        })
        .then((r) => r.purchases),
    [courseId, badgeId, status, q, limit],
  )

  const rows = purchases.data ?? []
  const current = rows.filter((p) => !p.revertedAt)
  const totalQty = current.reduce((sum, p) => sum + p.quantity, 0)
  const totalCoins = current.reduce((sum, p) => sum + p.totalCost, 0)
  const undoneCount = rows.length - current.length
  const filtering = Boolean(courseId || badgeId || q || status !== 'all')
  const truncated = rows.length >= limit

  const replace = (p: AdminPurchase) => purchases.setData((prev = []) => prev.map((x) => (x.id === p.id ? p : x)))

  const undoThenReload = async (ref: AuditRef) => {
    if (await undo(ref)) purchases.reload()
  }

  const run = async (
    p: AdminPurchase,
    call: () => Promise<{ purchase: AdminPurchase; auditId: number }>,
    message: (updated: AdminPurchase) => string,
  ) => {
    setBusy((s) => catalogWith(s, p.id, true))
    try {
      const r = await call()
      replace(r.purchase)
      toast.success(message(r.purchase), { undo: () => undoThenReload({ auditId: r.auditId }) })
    } catch (e) {
      toast.error(errorMessage(e))
    } finally {
      setBusy((s) => catalogWith(s, p.id, false))
    }
  }

  const revert = async (p: AdminPurchase) => {
    const coins = catalogCount(p.totalCost, 'moneda', 'monedas')
    const ok = await confirm({
      title: '¿Deshacer esta compra?',
      message:
        p.badgeId !== null
          ? `Se le devuelven ${coins} a ${p.studentName} y vuelven ${catalogCount(p.quantity, 'unidad', 'unidades')} al stock de «${p.badgeName}». Puedes rehacerla después.`
          : `Se le devuelven ${coins} a ${p.studentName}. La insignia ya no existe, así que no se repone stock. Puedes rehacerla después.`,
      confirmLabel: 'Deshacer compra',
    })
    if (!ok) return
    await run(
      p,
      () => api.admin.revertPurchase(p.id),
      (u) => `Compra deshecha: se devolvieron ${coins} a ${u.studentName}.`,
    )
  }

  const restore = (p: AdminPurchase) =>
    run(
      p,
      () => api.admin.restorePurchase(p.id),
      (u) => `Compra rehecha: se cobraron ${catalogCount(u.totalCost, 'moneda', 'monedas')} a ${u.studentName}.`,
    )

  const exportCsv = () => {
    const today = catalogStamp(new Date().toISOString()).slice(0, 10)
    downloadCsv(`compras-${today}.csv`, toCsv(rows, CSV_COLUMNS))
    toast.info(`Se descargaron ${catalogCount(rows.length, 'compra', 'compras')}.`)
  }

  const clearFilters = () => {
    setCourseId('')
    setBadgeId('')
    setStatus('all')
    setSearch('')
  }

  const columns: Column<AdminPurchase>[] = [
    {
      key: 'date',
      header: 'Fecha',
      className: 'whitespace-nowrap',
      cell: (p) => {
        const [day, time] = catalogDateParts(p.createdAt)
        return (
          <time dateTime={p.createdAt} className="block">
            {day}
            <span className="block text-xs text-gray-500">{time}</span>
          </time>
        )
      },
    },
    {
      key: 'student',
      header: 'Alumno',
      cell: (p) => (
        <div className="min-w-[9rem]">
          <p className="font-medium text-gray-800">{p.studentName}</p>
          <p className="text-xs text-gray-500">{p.studentCode}</p>
        </div>
      ),
    },
    {
      key: 'course',
      header: 'Curso',
      cell: (p) => (
        <span className="block max-w-[10rem] truncate" title={p.courseName}>
          {p.courseName}
        </span>
      ),
    },
    {
      key: 'badge',
      header: 'Insignia',
      cell: (p) => (
        <div className="min-w-[8rem]">
          <span className="text-gray-800">{p.badgeName}</span>
          {p.badgeId === null && (
            <Pill className="ml-2" title="La insignia se eliminó; la compra conserva el nombre.">
              Eliminada
            </Pill>
          )}
        </div>
      ),
    },
    { key: 'qty', header: 'Cantidad', align: 'right', cell: (p) => catalogNumber(p.quantity) },
    {
      key: 'cost',
      header: 'Costo',
      align: 'right',
      className: 'whitespace-nowrap',
      cell: (p) => (
        <>
          <span className="font-medium text-gray-800">{catalogNumber(p.totalCost)}</span>
          {p.quantity > 1 && (
            <span className="block text-xs text-gray-500">
              {catalogNumber(p.quantity)} × {catalogNumber(p.unitCost)}
            </span>
          )}
        </>
      ),
    },
    {
      key: 'coins',
      header: 'Monedas',
      align: 'right',
      className: 'whitespace-nowrap tabular-nums',
      // El texto sr-only va dentro de un span relative: si no, se posiciona fuera de la tabla
      // con scroll y le da scroll horizontal a toda la página en el celular.
      cell: (p) =>
        p.coinsBefore !== null && p.coinsAfter !== null ? (
          <span className="relative">
            <span aria-hidden="true">
              {catalogNumber(p.coinsBefore)} → {catalogNumber(p.coinsAfter)}
            </span>
            <span className="sr-only">
              de {catalogNumber(p.coinsBefore)} a {catalogNumber(p.coinsAfter)}
            </span>
          </span>
        ) : (
          <span className="relative text-gray-500" title="La compra no guardó el saldo (p. ej. importada de Sheets).">
            —<span className="sr-only">sin dato</span>
          </span>
        ),
    },
    {
      key: 'source',
      header: 'Origen',
      cell: (p) => {
        const s = SOURCES[p.source]
        return s ? (
          <Pill tone={s.tone} title={s.full}>
            {s.label}
          </Pill>
        ) : (
          <Pill>{p.source}</Pill>
        )
      },
    },
    {
      key: 'status',
      header: 'Estado',
      cell: (p) =>
        p.revertedAt ? (
          <Pill title={`Deshecha el ${catalogDateTime(p.revertedAt)}`}>Deshecha</Pill>
        ) : (
          <Pill tone="green">Vigente</Pill>
        ),
    },
    {
      key: 'actions',
      header: <span className="sr-only">Acciones</span>,
      align: 'right',
      // Fija a la derecha desde sm: la tabla es ancha y el botón no debe quedar fuera de vista.
      className: 'sm:sticky sm:right-0 sm:bg-white sm:shadow-[-8px_0_8px_-8px_rgba(0,0,0,0.12)]',
      headerClassName: 'sm:right-0',
      cell: (p) => {
        const working = busy.has(p.id)
        const undone = p.revertedAt !== null
        // Sin disabled mientras trabaja, para que el foco no se pierda; sólo se ignoran los clics.
        return (
          <Button
            variant="ghost"
            size="sm"
            icon={working ? LoaderCircle : undone ? Redo2 : Undo2}
            aria-busy={working || undefined}
            aria-disabled={working || undefined}
            aria-label={`${undone ? 'Rehacer' : 'Deshacer'} compra de ${p.studentName}: ${p.badgeName}`}
            className={working ? 'cursor-wait [&>svg]:animate-spin' : undefined}
            onClick={() => {
              if (working) return
              void (undone ? restore(p) : revert(p))
            }}
          >
            {undone ? 'Rehacer compra' : 'Deshacer compra'}
          </Button>
        )
      },
    },
  ]

  const courseOptions = (courses.data ?? []).toSorted(byName).map((c) => ({ value: c.id, label: c.name }))
  const badgeOptions = (badges.data ?? []).toSorted(byName).map((b) => ({ value: b.id, label: b.name }))

  return (
    <>
      <PageHeader
        title="Compras"
        subtitle="Historial de la tienda. Deshacer una compra le devuelve las monedas al alumno y repone el stock."
        actions={
          <Button variant="ghost" icon={Download} onClick={exportCsv} disabled={rows.length === 0}>
            Exportar CSV
          </Button>
        }
      />

      <div className="mb-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <SearchInput
          value={search}
          onChange={setSearch}
          label="Buscar por alumno o insignia"
          placeholder="Buscar alumno…"
        />
        <Select
          label="Curso"
          hideLabel
          value={courseId}
          onChange={setCourseId}
          placeholder="Todos los cursos"
          options={courseOptions}
        />
        <Select
          label="Insignia"
          hideLabel
          value={badgeId}
          onChange={setBadgeId}
          placeholder="Todas las insignias"
          options={badgeOptions}
        />
        <Select
          label="Estado"
          hideLabel
          value={status}
          onChange={(v) => setStatus(v as Status)}
          options={STATUS_OPTIONS}
        />
      </div>

      {purchases.error && (
        <LoadError className="mb-4" message={errorMessage(purchases.error)} onRetry={purchases.reload} />
      )}

      <DataTable
        caption="Compras"
        rows={rows}
        columns={columns}
        rowKey={(p) => p.id}
        loading={purchases.loading}
        className="max-h-[65vh] rounded-b-none"
        empty={
          purchases.error ? (
            <EmptyState title="No se pudo cargar la lista" description="Usa «Reintentar» arriba." />
          ) : (
            <EmptyState
              icon={ShoppingCart}
              title={filtering ? 'Ninguna compra coincide' : 'Todavía no hay compras'}
              description={
                filtering
                  ? 'Prueba con otros filtros.'
                  : 'Aquí aparecen las compras de la tienda y las que se importen de Google Sheets.'
              }
              action={
                filtering ? (
                  <Button variant="ghost" onClick={clearFilters}>
                    Quitar filtros
                  </Button>
                ) : undefined
              }
            />
          )
        }
      />
      <div
        className="flex flex-col gap-2 rounded-b-2xl border border-t-0 border-gray-100 bg-gray-50 px-4 py-3 text-sm text-gray-700 shadow-sm sm:flex-row sm:flex-wrap sm:items-center sm:justify-between"
        aria-live="polite"
      >
        <p>
          <span className="font-semibold">Totales</span>
          {undoneCount > 0 ? ' (sin las deshechas)' : ''}: {catalogCount(current.length, 'compra', 'compras')} ·{' '}
          {catalogCount(totalQty, 'insignia', 'insignias')} · {catalogCount(totalCoins, 'moneda', 'monedas')}
          {undoneCount > 0 && (
            <span className="text-gray-500"> · {catalogCount(undoneCount, 'deshecha', 'deshechas')}</span>
          )}
        </p>
        {truncated && (
          <p className="flex flex-wrap items-center gap-2 text-gray-500">
            Se muestran las {catalogNumber(rows.length)} más recientes.
            {limit < MAX_LIMIT && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setLimit((l) => Math.min(l + PAGE, MAX_LIMIT))}
                disabled={purchases.loading}
              >
                Cargar {catalogNumber(Math.min(PAGE, MAX_LIMIT - limit))} más
              </Button>
            )}
          </p>
        )}
      </div>
    </>
  )
}
