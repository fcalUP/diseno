import { useId, type ReactNode } from 'react'
import { ImageOff } from 'lucide-react'
import { Button, cx, focusRing } from '../../ui'

export interface CheckItem {
  id: number
  label: string
  /** Miniatura (insignias); '' muestra un hueco gris. */
  imageUrl?: string
  /** Texto chico bajo el nombre ("Inactiva", "se oculta por…"). */
  note?: ReactNode
  disabled?: boolean
}

export interface CheckListProps {
  legend: string
  hint?: ReactNode
  items: CheckItem[]
  checked: ReadonlySet<number>
  onChange: (next: Set<number>) => void
  allLabel: string
  noneLabel: string
  /** Resumen que se lee al cambiar ("8 de 10 visibles"). */
  summary: (checked: number, total: number) => string
  /** Lo que se muestra cuando no hay elementos. */
  empty: ReactNode
  withImages?: boolean
  /** Botones extra junto a "todas / ninguna". */
  extra?: ReactNode
}

// Lista de casillas con atajos para marcar o desmarcar todo.
export function CheckList({
  legend,
  hint,
  items,
  checked,
  onChange,
  allLabel,
  noneLabel,
  summary,
  empty,
  withImages = false,
  extra,
}: CheckListProps) {
  const hintId = useId()

  const setAll = (on: boolean) => {
    const next = new Set(checked)
    for (const it of items) {
      if (it.disabled) continue
      if (on) next.add(it.id)
      else next.delete(it.id)
    }
    onChange(next)
  }

  const toggle = (id: number) => {
    const next = new Set(checked)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    onChange(next)
  }

  const count = items.filter((it) => checked.has(it.id)).length

  return (
    <fieldset aria-describedby={hint ? hintId : undefined} className="min-w-0">
      <legend className="text-sm font-medium text-gray-700">{legend}</legend>
      {hint && (
        <p id={hintId} className="mt-1 text-xs text-gray-500">
          {hint}
        </p>
      )}
      {items.length === 0 ? (
        <div className="mt-2 rounded-xl border border-dashed border-gray-300 p-4 text-center text-sm text-gray-500">
          {empty}
        </div>
      ) : (
        <>
          <div className="mt-2 flex flex-wrap items-center justify-between gap-2">
            <p className="text-xs font-medium text-gray-600" aria-live="polite">
              {summary(count, items.length)}
            </p>
            <div className="flex flex-wrap gap-1">
              {extra}
              <Button variant="ghost" size="sm" onClick={() => setAll(true)}>
                {allLabel}
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setAll(false)}>
                {noneLabel}
              </Button>
            </div>
          </div>
          <ul
            role="list"
            className="mt-1 max-h-72 divide-y divide-gray-100 overflow-y-auto rounded-xl border border-gray-200"
          >
            {items.map((it) => (
              <li key={it.id}>
                <label
                  className={cx(
                    'flex items-center gap-3 px-3 py-2 text-sm',
                    it.disabled ? 'cursor-not-allowed bg-gray-50' : 'cursor-pointer hover:bg-gray-50',
                  )}
                >
                  <input
                    type="checkbox"
                    checked={checked.has(it.id)}
                    disabled={it.disabled}
                    onChange={() => toggle(it.id)}
                    className={cx(
                      'h-4 w-4 shrink-0 cursor-pointer accent-orange-500 disabled:cursor-not-allowed',
                      focusRing,
                    )}
                  />
                  {withImages &&
                    (it.imageUrl ? (
                      <img
                        src={it.imageUrl}
                        alt=""
                        loading="lazy"
                        className="h-8 w-8 shrink-0 rounded-lg bg-gray-50 object-contain"
                      />
                    ) : (
                      <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-gray-100 text-gray-400">
                        <ImageOff aria-hidden="true" className="h-4 w-4" />
                      </span>
                    ))}
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-gray-800">{it.label}</span>
                    {it.note && <span className="block text-xs text-gray-500">{it.note}</span>}
                  </span>
                </label>
              </li>
            ))}
          </ul>
        </>
      )}
    </fieldset>
  )
}
