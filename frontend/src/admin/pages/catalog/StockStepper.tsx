import { useEffect, useRef, useState } from 'react'
import { LoaderCircle, Minus, Plus } from 'lucide-react'
import { IconButton, cx } from '../../ui'
import { CATALOG_INT_MAX, catalogNumber } from './utils'

export interface StockStepperProps {
  name: string
  stock: number
  /** Guarda el stock nuevo. Debe avisar ella misma si falla (no lanza). */
  onCommit: (stock: number) => Promise<void>
}

// Espera tras el último clic antes de guardar: varios clics seguidos = un solo cambio.
const WAIT_MS = 700

interface Pending {
  timer: ReturnType<typeof setTimeout> | null
  target: number | null
  commit: (stock: number) => Promise<void>
}

// Botones +/- del stock de una insignia.
export function StockStepper({ name, stock, onCommit }: StockStepperProps) {
  const [shown, setShown] = useState<number | null>(null)
  const [saving, setSaving] = useState(false)
  const box = useRef<Pending>({ timer: null, target: null, commit: onCommit })

  useEffect(() => {
    box.current.commit = onCommit
  })

  // Si la tarjeta desaparece con un cambio pendiente, se guarda de todos modos.
  useEffect(() => {
    const b = box.current
    return () => {
      if (b.timer !== null && b.target !== null) {
        clearTimeout(b.timer)
        void b.commit(b.target)
      }
    }
  }, [])

  const value = shown ?? stock

  const flush = async () => {
    const b = box.current
    const target = b.target
    b.timer = null
    b.target = null
    if (target === null) return
    if (target === stock) {
      setShown(null)
      return
    }
    setSaving(true)
    try {
      await b.commit(target)
    } finally {
      setSaving(false)
      setShown(null)
    }
  }

  const bump = (delta: number) => {
    if (saving) return
    const b = box.current
    const next = Math.min(CATALOG_INT_MAX, Math.max(0, (b.target ?? stock) + delta))
    b.target = next
    setShown(next)
    if (b.timer !== null) clearTimeout(b.timer)
    b.timer = setTimeout(() => void flush(), WAIT_MS)
  }

  const minusOff = saving || value <= 0
  const plusOff = saving || value >= CATALOG_INT_MAX

  return (
    <div role="group" aria-label={`Stock de «${name}»`} className="inline-flex items-center gap-1">
      <IconButton
        icon={Minus}
        size="sm"
        label={`Restar 1 al stock de «${name}»`}
        aria-disabled={minusOff || undefined}
        className={cx('border border-gray-200', minusOff && 'cursor-not-allowed opacity-40')}
        onClick={() => {
          if (!minusOff) bump(-1)
        }}
      />
      <output
        aria-live="polite"
        className={cx(
          'min-w-[3ch] text-center text-base font-semibold tabular-nums',
          value === 0 ? 'text-red-600' : 'text-gray-800',
        )}
      >
        {catalogNumber(value)}
      </output>
      <IconButton
        icon={Plus}
        size="sm"
        label={`Sumar 1 al stock de «${name}»`}
        aria-disabled={plusOff || undefined}
        className={cx('border border-gray-200', plusOff && 'cursor-not-allowed opacity-40')}
        onClick={() => {
          if (!plusOff) bump(1)
        }}
      />
      <span className="flex h-4 w-4 items-center justify-center">
        {saving && <LoaderCircle aria-hidden="true" className="h-4 w-4 animate-spin text-orange-500" />}
        {shown !== null && !saving && (
          <span title="Sin guardar todavía" className="h-2 w-2 rounded-full bg-orange-400" aria-hidden="true" />
        )}
      </span>
      <span className="sr-only" aria-live="polite">
        {saving ? 'Guardando stock…' : ''}
      </span>
    </div>
  )
}
