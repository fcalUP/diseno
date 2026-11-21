import { useState } from 'react'
import { Link } from 'react-router'
import { ArrowRight, Award, BookOpen, Coins, RefreshCw, ShoppingCart, Undo2, Users } from 'lucide-react'
import { api, errorMessage } from '../../lib/api'
import type { AdminStats, AuditEntry } from '../../lib/types'
import {
  Button,
  Card,
  DataTable,
  EmptyState,
  PageHeader,
  Pill,
  Stat,
  cx,
  focusRing,
  useAsync,
  useUndo,
  type Column,
} from '../ui'
import { ErrorBanner, LinkButton, LoadingBlock } from './students/parts'
import { formatDateTime, formatNumber, pl, timeAgo } from './students/util'

type CourseRow = AdminStats['perCourse'][number]

const COURSE_COLUMNS: Column<CourseRow>[] = [
  {
    key: 'name',
    header: 'Curso',
    cell: (c) => (
      <Link
        to={`/admin/alumnos?curso=${c.courseId}`}
        className={cx('rounded font-medium text-orange-600 hover:underline', focusRing)}
      >
        {c.courseName}
      </Link>
    ),
  },
  {
    key: 'active',
    header: 'En el login',
    cell: (c) => (c.active ? <Pill tone="green">Visible</Pill> : <Pill tone="gray">Oculto</Pill>),
  },
  { key: 'students', header: 'Alumnos', align: 'right', className: 'tabular-nums', cell: (c) => formatNumber(c.students) },
  {
    key: 'avgLevel',
    header: 'Nivel promedio',
    align: 'right',
    className: 'tabular-nums',
    cell: (c) => c.avgLevel.toLocaleString('es-MX', { maximumFractionDigits: 1 }),
  },
  { key: 'totalExp', header: 'EXP total', align: 'right', className: 'tabular-nums', cell: (c) => formatNumber(c.totalExp) },
]

function actor(e: AuditEntry): string {
  if (e.actorType === 'system') return 'Sistema'
  if (e.actorType === 'student') return `Alumno ${e.actorLabel}`
  return e.actorLabel || 'Admin'
}

export default function Dashboard() {
  const undo = useUndo()
  // La hora de carga sirve para los "hace 5 minutos" sin leer el reloj al pintar.
  const statsQ = useAsync(async () => {
    const stats = await api.admin.stats()
    // Go manda null en vez de [] cuando una lista sale vacía; así no truena al pintar.
    return {
      stats: {
        ...stats,
        perCourse: stats.perCourse ?? [],
        recentActivity: stats.recentActivity ?? [],
        badges: { ...stats.badges, lowStock: stats.badges.lowStock ?? [] },
      },
      loadedAt: Date.now(),
    }
  }, [])
  const [undoing, setUndoing] = useState<number | null>(null)

  const undoEntry = async (id: number) => {
    setUndoing(id)
    const ok = await undo({ auditId: id })
    setUndoing(null)
    if (ok) statsQ.reload()
  }

  const data = statsQ.data
  return (
    <>
      <PageHeader
        title="Resumen"
        subtitle="Cómo va el portal de un vistazo."
        actions={
          <Button variant="ghost" icon={RefreshCw} onClick={statsQ.reload} loading={statsQ.loading && !!data}>
            Actualizar
          </Button>
        }
      />

      {statsQ.error && (
        <ErrorBanner
          className="mb-6"
          message={
            data
              ? `No se pudo actualizar el resumen: ${errorMessage(statsQ.error)}`
              : `No se pudo cargar el resumen: ${errorMessage(statsQ.error)}`
          }
          onRetry={statsQ.reload}
        />
      )}

      {!data ? (
        statsQ.loading && <LoadingBlock label="Cargando resumen…" />
      ) : (
        <Overview
          stats={data.stats}
          loadedAt={data.loadedAt}
          undoing={undoing}
          onUndo={(id) => void undoEntry(id)}
        />
      )}
    </>
  )
}

function Overview({
  stats,
  loadedAt,
  undoing,
  onUndo,
}: {
  stats: AdminStats
  loadedAt: number
  undoing: number | null
  onUndo: (id: number) => void
}) {
  const { students, courses, badges, purchases } = stats
  return (
    <div className="space-y-6">
      {students.total === 0 && (
        <div className="rounded-2xl border border-orange-200 bg-orange-50 p-5">
          <h2 className="font-semibold text-orange-900">Aún no hay alumnos</h2>
          <p className="mt-1 text-sm text-orange-800">
            Impórtalos desde Google Sheets o genéralos a partir de una lista.
          </p>
          <div className="mt-3 flex flex-wrap gap-2">
            <LinkButton to="/admin/importar" variant="primary">
              Importar desde Sheets
            </LinkButton>
            <LinkButton to="/admin/generar">Generar alumnos</LinkButton>
          </div>
        </div>
      )}

      {/* 3 + 2 tarjetas que llenan cada fila; las de abajo son más anchas. */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-6 [&>div>*]:h-full">
        <div className="lg:col-span-2">
          <Stat
            label="Alumnos activos"
            value={formatNumber(students.active)}
            hint={`de ${formatNumber(students.total)} en total`}
            icon={Users}
            tone="orange"
          />
        </div>
        <div className="lg:col-span-2">
          <Stat
            label="Cursos en el login"
            value={formatNumber(courses.active)}
            hint={`de ${formatNumber(courses.total)} ${pl(courses.total, 'curso', 'cursos')}`}
            icon={BookOpen}
            tone="purple"
          />
        </div>
        <div className="lg:col-span-2">
          <Stat
            label="Insignias activas"
            value={formatNumber(badges.active)}
            hint={
              badges.lowStock.length > 0
                ? `${formatNumber(badges.lowStock.length)} con inventario bajo`
                : `de ${formatNumber(badges.total)} en total`
            }
            icon={Award}
            tone="blue"
          />
        </div>
        <div className="lg:col-span-3">
          <Stat
            label="Monedas en circulación"
            value={formatNumber(stats.coinsInCirculation)}
            hint="Suma de las monedas de los alumnos"
            icon={Coins}
            tone="green"
          />
        </div>
        <div className="sm:col-span-2 lg:col-span-3">
          <Stat
            label="Compras en 7 días"
            value={formatNumber(purchases.last7Days)}
            hint={`${formatNumber(purchases.total)} en total`}
            icon={ShoppingCart}
            tone="gray"
          />
        </div>
      </div>

      <div className="grid gap-6 xl:grid-cols-3">
        <section aria-labelledby="resumen-cursos" className="min-w-0 xl:col-span-2">
          <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
            <h2 id="resumen-cursos" className="text-base font-semibold text-gray-800">
              Por curso
            </h2>
            <LinkButton to="/admin/cursos" size="sm" variant="link">
              Cursos
              <ArrowRight aria-hidden="true" className="h-4 w-4" />
            </LinkButton>
          </div>
          <DataTable
            rows={stats.perCourse}
            columns={COURSE_COLUMNS}
            rowKey={(c) => c.courseId}
            caption="Alumnos, nivel promedio y EXP por curso"
            className="relative"
            empty={<EmptyState icon={BookOpen} title="Sin cursos" description="Crea el primero en la sección Cursos." />}
          />
        </section>

        <Card>
          <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
            <h2 className="text-base font-semibold text-gray-800">Inventario bajo</h2>
            <LinkButton to="/admin/insignias" size="sm" variant="link">
              Insignias
              <ArrowRight aria-hidden="true" className="h-4 w-4" />
            </LinkButton>
          </div>
          {badges.lowStock.length === 0 ? (
            <p className="text-sm text-gray-500">Todas las insignias activas tienen inventario.</p>
          ) : (
            <ul className="space-y-3">
              {badges.lowStock.map((b) => (
                <li key={b.id} className="flex items-center gap-3">
                  {b.imageUrl ? (
                    <img src={b.imageUrl} alt="" loading="lazy" className="h-10 w-10 shrink-0 rounded-lg bg-gray-50 object-contain" />
                  ) : (
                    <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-gray-100">
                      <Award aria-hidden="true" className="h-5 w-5 text-gray-400" />
                    </span>
                  )}
                  <span className="min-w-0 flex-1 truncate text-sm font-medium text-gray-800">{b.name}</span>
                  {b.stock === 0 ? (
                    <Pill tone="red">Agotada</Pill>
                  ) : (
                    <Pill tone="orange">
                      {pl(b.stock, 'Queda', 'Quedan')} {formatNumber(b.stock)}
                    </Pill>
                  )}
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>

      <Card>
        <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-base font-semibold text-gray-800">Actividad reciente</h2>
          <LinkButton to="/admin/actividad" size="sm" variant="link">
            Ver toda
            <ArrowRight aria-hidden="true" className="h-4 w-4" />
          </LinkButton>
        </div>
        {stats.recentActivity.length === 0 ? (
          <p className="text-sm text-gray-500">Todavía no hay movimientos.</p>
        ) : (
          <ul className="divide-y divide-gray-100">
            {stats.recentActivity.map((e) => (
              <li key={e.id} className="flex flex-wrap items-center gap-x-3 gap-y-2 py-3">
                <div className="min-w-0 flex-1">
                  <p className={cx('text-sm', e.undoneAt ? 'text-gray-400 line-through' : 'text-gray-800')}>{e.summary}</p>
                  <p className="mt-0.5 text-xs text-gray-500">
                    {actor(e)} ·{' '}
                    <time dateTime={e.at} title={formatDateTime(e.at)}>
                      {timeAgo(e.at, loadedAt)}
                    </time>
                  </p>
                </div>
                {e.undoneAt ? (
                  <Pill tone="gray">Deshecho</Pill>
                ) : (
                  e.undoable && (
                    <Button
                      size="sm"
                      variant="ghost"
                      icon={Undo2}
                      loading={undoing === e.id}
                      disabled={undoing !== null}
                      onClick={() => onUndo(e.id)}
                      aria-label={`Deshacer: ${e.summary}`}
                    >
                      Deshacer
                    </Button>
                  )
                )}
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  )
}
