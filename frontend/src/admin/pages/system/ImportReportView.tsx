import { FileSpreadsheet, Info, TriangleExclamationPoint } from 'lucide-react'
import type { ImportCourseReport, ImportReport } from '../../../lib/types'
import { DataTable, Pill, cx, focusRing, type Column } from '../../ui'
import { formatNumber, plural } from './format'

const n = formatNumber

const courseColumns: Column<ImportCourseReport>[] = [
  {
    key: 'course',
    header: 'Curso',
    cell: (c) => <span className="font-medium text-gray-800">{c.courseName}</span>,
  },
  {
    key: 'sheet',
    header: 'Hoja',
    cell: (c) => (c.sheet ? c.sheet : <Pill tone="yellow">Sin hoja</Pill>),
  },
  { key: 'rows', header: 'Filas', align: 'right', cell: (c) => n(c.rows) },
  { key: 'created', header: 'Nuevos', align: 'right', cell: (c) => n(c.created) },
  { key: 'updated', header: 'Actualizados', align: 'right', cell: (c) => n(c.updated) },
  { key: 'unchanged', header: 'Sin cambios', align: 'right', cell: (c) => n(c.unchanged) },
  { key: 'skipped', header: 'Omitidos', align: 'right', cell: (c) => n(c.skipped) },
]

interface CountsProps {
  title: string
  items: [label: string, value: number][]
}

function Counts({ title, items }: CountsProps) {
  return (
    <div className="rounded-xl border border-gray-100 bg-gray-50 p-4">
      <h4 className="text-sm font-semibold text-gray-700">{title}</h4>
      <dl className="mt-2 space-y-1 text-sm">
        {items.map(([label, value]) => (
          <div key={label} className="flex items-baseline justify-between gap-3">
            <dt className="text-gray-500">{label}</dt>
            <dd className="font-semibold tabular-nums text-gray-800">{n(value)}</dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

// Cuántos avisos se pintan; con cientos la página se vuelve inmanejable.
const MAX_WARNINGS = 200

// Reporte de importación (vista previa o resultado final).
export function ImportReportView({ report }: { report: ImportReport }) {
  // Go manda null en vez de [] cuando una lista viene vacía.
  const courses = report.courses ?? []
  const sheets = report.sheetsFound ?? []
  const totals = courses.reduce(
    (t, c) => ({
      rows: t.rows + c.rows,
      created: t.created + c.created,
      updated: t.updated + c.updated,
      unchanged: t.unchanged + c.unchanged,
      skipped: t.skipped + c.skipped,
    }),
    { rows: 0, created: 0, updated: 0, unchanged: 0, skipped: 0 },
  )
  const media = report.media ?? { avatars: 0, badgeImages: 0, courseImages: 0 }
  const hasMedia = media.avatars + media.badgeImages + media.courseImages > 0
  const warnings = report.warnings ?? []

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-start gap-3">
        <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-green-100 text-green-700">
          <FileSpreadsheet aria-hidden="true" className="h-5 w-5" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="break-words font-semibold text-gray-800">{report.spreadsheetTitle || 'Hoja sin título'}</h3>
            {report.dryRun ? <Pill tone="blue">Vista previa</Pill> : <Pill tone="green">Importado</Pill>}
          </div>
          <p className="mt-0.5 text-sm text-gray-500">
            {sheets.length > 0
              ? `Pestañas encontradas: ${sheets.join(', ')}`
              : 'No se encontraron pestañas en la hoja.'}
          </p>
        </div>
      </div>

      {report.dryRun && (
        <p className="flex items-start gap-2 rounded-xl bg-blue-50 px-4 py-3 text-sm text-blue-800">
          <Info aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />
          Así quedaría la importación. Todavía no se guardó nada.
        </p>
      )}

      <dl className="grid grid-cols-2 gap-3 sm:grid-cols-5">
        {(
          [
            ['Filas leídas', totals.rows, 'text-gray-800'],
            ['Alumnos nuevos', totals.created, 'text-green-700'],
            ['Actualizados', totals.updated, 'text-blue-700'],
            ['Sin cambios', totals.unchanged, 'text-gray-600'],
            ['Omitidos', totals.skipped, totals.skipped > 0 ? 'text-orange-600' : 'text-gray-600'],
          ] as const
        ).map(([label, value, color]) => (
          <div key={label} className="rounded-xl border border-gray-100 bg-white p-3">
            <dt className="text-xs font-medium text-gray-500">{label}</dt>
            <dd className={cx('mt-1 text-xl font-bold tabular-nums', color)}>{n(value)}</dd>
          </div>
        ))}
      </dl>

      <DataTable
        rows={courses}
        columns={courseColumns}
        rowKey={(c) => c.courseId}
        caption="Alumnos por curso"
        className=""
        empty={<p className="px-4 py-6 text-center text-sm text-gray-500">La hoja no trae alumnos de ningún curso.</p>}
      />

      <div className="grid gap-3 md:grid-cols-3">
        <Counts
          title="Insignias"
          items={[
            ['Filas', report.badges.rows],
            ['Nuevas', report.badges.created],
            ['Actualizadas', report.badges.updated],
            ['Sin cambios', report.badges.unchanged],
          ]}
        />
        <Counts
          title="Compras"
          items={[
            ['Filas', report.purchases.rows],
            ['Nuevas', report.purchases.created],
            ['Ya importadas u omitidas', report.purchases.skipped],
            ['Sin alumno o insignia', report.purchases.unmatched],
          ]}
        />
        {hasMedia ? (
          <Counts
            title="Imágenes"
            items={[
              ['Avatares', media.avatars],
              ['Insignias', media.badgeImages],
              ['Cursos', media.courseImages],
            ]}
          />
        ) : (
          <div className="rounded-xl border border-dashed border-gray-200 p-4 text-sm text-gray-500">
            <h4 className="font-semibold text-gray-700">Imágenes</h4>
            <p className="mt-2">{report.dryRun ? 'No hay imágenes por asignar.' : 'No se asignaron imágenes.'}</p>
          </div>
        )}
      </div>

      {warnings.length > 0 ? (
        <details className="group rounded-xl border border-yellow-200 bg-yellow-50">
          <summary
            className={cx(
              'flex cursor-pointer list-none items-center gap-2 rounded-xl px-4 py-3 text-sm font-semibold text-yellow-900 [&::-webkit-details-marker]:hidden',
              focusRing,
            )}
          >
            <TriangleExclamationPoint aria-hidden="true" className="h-4 w-4 shrink-0" />
            {plural(warnings.length, 'advertencia', 'advertencias')}
            {/* El estado abierto/cerrado ya lo anuncia <details>; esto es sólo visual. */}
            <span aria-hidden="true" className="ml-auto text-xs font-medium text-yellow-800 group-open:hidden">
              Ver
            </span>
            <span aria-hidden="true" className="ml-auto hidden text-xs font-medium text-yellow-800 group-open:inline">
              Ocultar
            </span>
          </summary>
          <ul className="max-h-80 list-disc space-y-1 overflow-y-auto px-4 pb-4 pl-9 text-sm text-yellow-900">
            {warnings.slice(0, MAX_WARNINGS).map((w, i) => (
              <li key={i} className="break-words">
                {w}
              </li>
            ))}
            {warnings.length > MAX_WARNINGS && (
              <li className="list-none text-yellow-800">… y {n(warnings.length - MAX_WARNINGS)} más.</li>
            )}
          </ul>
        </details>
      ) : (
        <p className="text-sm text-gray-500">Sin advertencias.</p>
      )}
    </div>
  )
}
