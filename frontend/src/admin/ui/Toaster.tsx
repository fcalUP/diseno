import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { CircleCheck, CircleX, Info, X } from 'lucide-react'
import { errorMessage } from '../../lib/api'
import { cx, focusRing } from './styles'
import { ToastContext, type ToastApi, type ToastKind, type ToastOptions } from './useToast'

interface ToastItem {
  id: number
  kind: ToastKind
  message: string
  undo?: () => Promise<unknown>
}

const kinds = {
  success: { Icon: CircleCheck, color: 'text-green-600' },
  error: { Icon: CircleX, color: 'text-red-600' },
  info: { Icon: Info, color: 'text-blue-600' },
}

const MAX_VISIBLE = 5
let lastId = 0

// Provee useToast() y pinta los avisos arriba al centro. Abajo a la derecha tapaban el pie de
// los cajones ("Guardar") y los botones de cada fila, y un clic ahí caía en "Deshacer".
export function Toaster({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([])

  const dismiss = useCallback((id: number) => setItems((list) => list.filter((t) => t.id !== id)), [])

  const toast = useMemo<ToastApi>(() => {
    const push = (kind: ToastKind) => (message: string, options?: ToastOptions) => {
      lastId += 1
      const item: ToastItem = { id: lastId, kind, message, undo: options?.undo }
      setItems((list) => [...list.slice(-(MAX_VISIBLE - 1)), item])
    }
    return { success: push('success'), error: push('error'), info: push('info') }
  }, [])

  const runUndo = useCallback(
    async (item: ToastItem) => {
      try {
        await item.undo?.()
      } catch (e) {
        toast.error(errorMessage(e))
      } finally {
        dismiss(item.id)
      }
    },
    [toast, dismiss],
  )

  return (
    <ToastContext value={toast}>
      {children}
      <div
        aria-live="polite"
        className="pointer-events-none fixed inset-x-0 top-4 z-[60] mx-auto flex w-[calc(100vw-2rem)] max-w-sm flex-col gap-2"
      >
        {items.map((t) => (
          <ToastView key={t.id} item={t} onDismiss={dismiss} onUndo={runUndo} />
        ))}
      </div>
    </ToastContext>
  )
}

interface ToastViewProps {
  item: ToastItem
  onDismiss: (id: number) => void
  onUndo: (item: ToastItem) => Promise<void>
}

function ToastView({ item, onDismiss, onUndo }: ToastViewProps) {
  // Con el mouse o el foco encima no se cierra solo.
  const [paused, setPaused] = useState(false)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (paused || busy) return
    const timer = setTimeout(() => onDismiss(item.id), item.undo ? 10_000 : 6_000)
    return () => clearTimeout(timer)
  }, [paused, busy, item, onDismiss])

  const { Icon, color } = kinds[item.kind]
  return (
    <div
      role={item.kind === 'error' ? 'alert' : 'status'}
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      onFocus={() => setPaused(true)}
      onBlur={() => setPaused(false)}
      className="pointer-events-auto flex items-start gap-3 rounded-2xl border border-gray-100 bg-white p-4 shadow-lg"
    >
      <Icon aria-hidden="true" className={cx('mt-0.5 h-5 w-5 shrink-0', color)} />
      <p className="min-w-0 flex-1 text-sm text-gray-800">{item.message}</p>
      {item.undo && (
        <button
          type="button"
          disabled={busy}
          onClick={() => {
            setBusy(true)
            void onUndo(item)
          }}
          className={cx(
            'shrink-0 rounded-full px-2 py-0.5 text-sm font-semibold text-orange-600 hover:bg-orange-50 disabled:opacity-60',
            focusRing,
          )}
        >
          {busy ? 'Deshaciendo…' : 'Deshacer'}
        </button>
      )}
      <button
        type="button"
        onClick={() => onDismiss(item.id)}
        aria-label="Cerrar aviso"
        className={cx('shrink-0 rounded-full p-0.5 text-gray-400 hover:text-gray-600', focusRing)}
      >
        <X aria-hidden="true" className="h-4 w-4" />
      </button>
    </div>
  )
}
