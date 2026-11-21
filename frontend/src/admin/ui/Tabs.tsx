import type { KeyboardEvent, ReactNode } from 'react'
import { cx, focusRing } from './styles'

export interface TabItem<V extends string | number> {
  value: V
  label: ReactNode
  count?: number
}

export interface TabsProps<V extends string | number> {
  items: TabItem<V>[]
  value: V
  onChange: (value: V) => void
  /** Nombre del grupo para lectores de pantalla. */
  label: string
  className?: string
}

const NAV_KEYS = ['ArrowLeft', 'ArrowRight', 'Home', 'End']

// Pestañas; se recorren con las flechas como pide el patrón de ARIA.
export function Tabs<V extends string | number>({ items, value, onChange, label, className }: TabsProps<V>) {
  const current = items.findIndex((t) => t.value === value)

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (!NAV_KEYS.includes(e.key) || items.length === 0) return
    e.preventDefault()
    const n = items.length
    const next =
      e.key === 'Home' ? 0 : e.key === 'End' ? n - 1 : (current + (e.key === 'ArrowRight' ? 1 : -1) + n) % n
    onChange(items[next].value)
    e.currentTarget.querySelectorAll<HTMLButtonElement>('[role="tab"]')[next]?.focus()
  }

  return (
    <div
      role="tablist"
      aria-label={label}
      onKeyDown={onKeyDown}
      className={cx('flex gap-1 overflow-x-auto border-b border-gray-200', className)}
    >
      {items.map((t, i) => {
        const active = i === current
        return (
          <button
            key={t.value}
            type="button"
            role="tab"
            aria-selected={active}
            tabIndex={active || (current === -1 && i === 0) ? 0 : -1}
            onClick={() => onChange(t.value)}
            className={cx(
              '-mb-px inline-flex shrink-0 items-center gap-2 whitespace-nowrap rounded-t-lg border-b-2 px-3 py-2 text-sm font-medium transition-colors',
              focusRing,
              active
                ? 'border-orange-500 text-orange-600'
                : 'border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700',
            )}
          >
            {t.label}
            {t.count !== undefined && (
              <span
                className={cx(
                  'rounded-full px-2 py-0.5 text-xs',
                  active ? 'bg-orange-100 text-orange-700' : 'bg-gray-100 text-gray-600',
                )}
              >
                {t.count}
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}
