import { useState, type ReactNode } from 'react'
import { cx, focusRing } from '../../ui'
import { plural } from './format'

type Obj = Record<string, unknown>

const isObj = (v: unknown): v is Obj => typeof v === 'object' && v !== null && !Array.isArray(v)
const json = (v: unknown) => JSON.stringify(v) ?? ''

function Value({ value }: { value: unknown }) {
  if (value === undefined) return <span className="text-gray-400">—</span>
  if (value === null) return <span className="italic text-gray-400">nulo</span>
  if (typeof value === 'boolean') return <>{value ? 'sí' : 'no'}</>
  if (typeof value === 'string') {
    return value === '' ? <span className="italic text-gray-400">(vacío)</span> : <>{value}</>
  }
  if (typeof value === 'number') return <>{String(value)}</>
  return <Pretty value={value} />
}

function Pretty({ value }: { value: unknown }) {
  return (
    <pre className="max-h-80 overflow-auto whitespace-pre-wrap break-all font-mono text-xs">
      {JSON.stringify(value, null, 2)}
    </pre>
  )
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="min-w-0 rounded-xl border border-gray-100 bg-white p-3">
      <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-gray-500">{title}</p>
      {children}
    </div>
  )
}

interface AuditDiffProps {
  before: unknown
  after: unknown
  /** Texto para lectores de pantalla, p. ej. "Cambios del movimiento 12". */
  caption: string
}

// Antes y después de una entrada de la bitácora. Con objetos muestra campo por campo y
// resalta los que cambiaron; con cualquier otra cosa (listas, niveles...) muestra el JSON.
export function AuditDiff({ before, after, caption }: AuditDiffProps) {
  const [showAll, setShowAll] = useState(false)

  if (before == null && after == null) {
    return <p className="text-sm text-gray-500">Este movimiento no guardó detalles.</p>
  }

  const objects = (before == null || isObj(before)) && (after == null || isObj(after))
  if (!objects) {
    return (
      <div className="grid gap-3 md:grid-cols-2">
        <Section title="Antes">{before == null ? <Value value={undefined} /> : <Pretty value={before} />}</Section>
        <Section title="Después">{after == null ? <Value value={undefined} /> : <Pretty value={after} />}</Section>
      </div>
    )
  }

  const b = before as Obj | null
  const a = after as Obj | null
  const both = b !== null && a !== null
  const keys = [...new Set([...Object.keys(b ?? {}), ...Object.keys(a ?? {})])]
  // Con un solo lado (alta o baja) no hay nada que resaltar: se muestran todos los campos.
  const rows = keys.map((key) => ({ key, changed: both && json(b[key]) !== json(a[key]) }))
  const changedCount = rows.filter((r) => r.changed).length
  const hidden = rows.length - changedCount
  const visible = showAll || changedCount === 0 ? rows : rows.filter((r) => r.changed)

  return (
    <div>
      <div className="overflow-x-auto rounded-xl border border-gray-100 bg-white">
        <table className="w-full min-w-[20rem] text-left text-sm">
          <caption className="sr-only">{caption}</caption>
          <thead className="bg-gray-50 text-xs font-semibold uppercase tracking-wide text-gray-500">
            <tr>
              <th scope="col" className="px-3 py-2">
                Campo
              </th>
              <th scope="col" className="px-3 py-2">
                Antes
              </th>
              <th scope="col" className="px-3 py-2">
                Después
              </th>
            </tr>
          </thead>
          <tbody>
            {visible.map(({ key, changed }) => (
              <tr key={key} className={cx('border-t border-gray-100 align-top', changed && 'bg-yellow-50')}>
                <th scope="row" className="whitespace-nowrap px-3 py-2 font-mono text-xs font-medium text-gray-700">
                  {changed && (
                    <span
                      aria-hidden="true"
                      className="mr-1.5 inline-block h-1.5 w-1.5 rounded-full bg-yellow-500 align-middle"
                    />
                  )}
                  {key}
                  {changed && <span className="sr-only"> (cambió)</span>}
                </th>
                <td
                  className={cx('max-w-[18rem] whitespace-pre-wrap break-words px-3 py-2', changed && 'text-red-700')}
                >
                  <Value value={b?.[key]} />
                </td>
                <td
                  className={cx(
                    'max-w-[18rem] whitespace-pre-wrap break-words px-3 py-2',
                    changed && 'font-medium text-green-700',
                  )}
                >
                  <Value value={a?.[key]} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {both && hidden > 0 && changedCount > 0 && (
        <button
          type="button"
          onClick={() => setShowAll((v) => !v)}
          aria-expanded={showAll}
          className={cx(
            'mt-2 rounded-full px-2 py-1 text-xs font-medium text-purple-600 hover:bg-purple-50',
            focusRing,
          )}
        >
          {showAll ? 'Ocultar campos sin cambios' : `Mostrar ${plural(hidden, 'campo', 'campos')} sin cambios`}
        </button>
      )}
    </div>
  )
}
