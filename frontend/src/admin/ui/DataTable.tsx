import type { ReactNode } from 'react'
import { EmptyState, Spinner } from './display'
import { cx } from './styles'

export interface Column<T> {
  key: string
  header: ReactNode
  cell: (row: T) => ReactNode
  align?: 'left' | 'center' | 'right'
  /** Clases de cada celda <td>. */
  className?: string
  headerClassName?: string
}

export interface DataTableProps<T, K extends string | number> {
  rows: readonly T[]
  columns: Column<T>[]
  rowKey: (row: T) => K
  /** Con `selected` y `onSelectionChange` aparece la columna de casillas. */
  selected?: ReadonlySet<K>
  onSelectionChange?: (next: Set<K>) => void
  onRowClick?: (row: T) => void
  /** Nombre de la fila para lectores de pantalla ("Seleccionar Ana López"). */
  rowLabel?: (row: T) => string
  rowClassName?: (row: T) => string | undefined
  loading?: boolean
  /** Lo que se muestra sin filas; por defecto "Sin resultados". */
  empty?: ReactNode
  caption?: string
  /** Clases del contenedor con scroll; por defecto limita la altura para fijar el encabezado. */
  className?: string
}

const alignClass = { left: 'text-left', center: 'text-center', right: 'text-right' }

// Un clic dentro de un control de la fila no abre la fila.
const INTERACTIVE = 'a,button,input,select,textarea,label,[role="switch"],[data-row-ignore]'

export function DataTable<T, K extends string | number>({
  rows,
  columns,
  rowKey,
  selected,
  onSelectionChange,
  onRowClick,
  rowLabel,
  rowClassName,
  loading = false,
  empty,
  caption,
  className = 'max-h-[70vh]',
}: DataTableProps<T, K>) {
  const keys = rows.map(rowKey)
  const selection = selected && onSelectionChange ? { set: selected, change: onSelectionChange } : null
  const selectedCount = selection ? keys.filter((k) => selection.set.has(k)).length : 0
  const allSelected = keys.length > 0 && selectedCount === keys.length

  const toggleAll = () => {
    if (!selection) return
    const next = new Set(selection.set)
    for (const k of keys) {
      if (allSelected) next.delete(k)
      else next.add(k)
    }
    selection.change(next)
  }

  const toggleRow = (k: K) => {
    if (!selection) return
    const next = new Set(selection.set)
    if (next.has(k)) next.delete(k)
    else next.add(k)
    selection.change(next)
  }

  const th =
    'sticky top-0 z-10 whitespace-nowrap border-b border-gray-200 bg-gray-50 px-4 py-3 text-xs font-semibold uppercase tracking-wide text-gray-500'

  return (
    <div
      className={cx('overflow-auto rounded-2xl border border-gray-100 bg-white shadow-sm', className)}
      aria-busy={loading || undefined}
    >
      <table className="w-full border-separate border-spacing-0 text-sm">
        {caption && <caption className="sr-only">{caption}</caption>}
        <thead>
          <tr>
            {selection && (
              <th scope="col" className={cx(th, 'w-10')}>
                <input
                  type="checkbox"
                  aria-label="Seleccionar todas las filas"
                  checked={allSelected}
                  ref={(el) => {
                    if (el) el.indeterminate = selectedCount > 0 && !allSelected
                  }}
                  onChange={toggleAll}
                  disabled={keys.length === 0}
                  className="h-4 w-4 cursor-pointer accent-orange-500"
                />
              </th>
            )}
            {columns.map((c) => (
              <th key={c.key} scope="col" className={cx(th, alignClass[c.align ?? 'left'], c.headerClassName)}>
                {c.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className={cx(loading && rows.length > 0 && 'opacity-60')}>
          {rows.length === 0 ? (
            <tr>
              <td colSpan={columns.length + (selection ? 1 : 0)} className="p-0">
                {loading ? (
                  <div className="flex justify-center py-12">
                    <Spinner />
                  </div>
                ) : (
                  (empty ?? <EmptyState title="Sin resultados" description="No hay registros que mostrar." />)
                )}
              </td>
            </tr>
          ) : (
            rows.map((row, i) => {
              const k = keys[i]
              const isSelected = selection ? selection.set.has(k) : false
              return (
                <tr
                  key={k}
                  tabIndex={onRowClick ? 0 : undefined}
                  onClick={
                    onRowClick
                      ? (e) => {
                          if (!(e.target as HTMLElement).closest(INTERACTIVE)) onRowClick(row)
                        }
                      : undefined
                  }
                  onKeyDown={
                    onRowClick
                      ? (e) => {
                          if (e.target === e.currentTarget && (e.key === 'Enter' || e.key === ' ')) {
                            e.preventDefault()
                            onRowClick(row)
                          }
                        }
                      : undefined
                  }
                  className={cx(
                    'transition-colors',
                    onRowClick &&
                      'cursor-pointer focus-visible:outline focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-blue-500',
                    isSelected ? 'bg-orange-50' : 'hover:bg-gray-50',
                    rowClassName?.(row),
                  )}
                >
                  {selection && (
                    <td className="border-b border-gray-100 px-4 py-3">
                      <input
                        type="checkbox"
                        checked={isSelected}
                        onChange={() => toggleRow(k)}
                        aria-label={rowLabel ? `Seleccionar ${rowLabel(row)}` : 'Seleccionar fila'}
                        className="h-4 w-4 cursor-pointer accent-orange-500"
                      />
                    </td>
                  )}
                  {columns.map((c) => (
                    <td
                      key={c.key}
                      className={cx(
                        'border-b border-gray-100 px-4 py-3 text-gray-700',
                        alignClass[c.align ?? 'left'],
                        c.className,
                      )}
                    >
                      {c.cell(row)}
                    </td>
                  ))}
                </tr>
              )
            })
          )}
        </tbody>
      </table>
    </div>
  )
}
