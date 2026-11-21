import { useState } from 'react'
import { ImageOff } from 'lucide-react'
import { api, errorMessage } from '../../../lib/api'
import type { AdminBadge } from '../../../lib/types'
import { Button, Drawer, Pill, useConfirm } from '../../ui'
import { MoveButtons } from './MoveButtons'
import { catalogMove, catalogSameList } from './utils'

export interface BadgeOrderDrawerProps {
  /** Todas las insignias en el orden actual. */
  badges: AdminBadge[]
  onClose: () => void
  onSaved: (batchId: string, ids: number[]) => void
}

// Se acomoda la lista completa y se guarda una sola vez: un solo aviso para deshacer.
export function BadgeOrderDrawer({ badges, onClose, onSaved }: BadgeOrderDrawerProps) {
  const confirm = useConfirm()
  const [order, setOrder] = useState(() => badges.map((b) => b.id))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [announce, setAnnounce] = useState('')

  const byId = new Map(badges.map((b) => [b.id, b]))
  const changed = !catalogSameList(
    order,
    badges.map((b) => b.id),
  )

  const move = (from: number, to: number) => {
    const next = catalogMove(order, from, to)
    setOrder(next)
    setAnnounce(`«${byId.get(order[from])?.name ?? ''}» ahora está en la posición ${to + 1} de ${next.length}.`)
  }

  const save = async () => {
    if (!changed || saving) return
    setSaving(true)
    setError('')
    try {
      const { batchId } = await api.admin.reorderBadges(order)
      onSaved(batchId, order)
      onClose()
    } catch (e) {
      setError(errorMessage(e))
      setSaving(false)
    }
  }

  const requestClose = async () => {
    if (saving) return
    if (
      changed &&
      !(await confirm({
        title: '¿Descartar el nuevo orden?',
        message: 'Los movimientos que no guardaste se pierden.',
        confirmLabel: 'Descartar',
        danger: true,
      }))
    ) {
      return
    }
    onClose()
  }

  return (
    <Drawer
      open
      size="md"
      onClose={() => void requestClose()}
      title="Ordenar insignias"
      description="Es el orden de la tienda en todos los cursos. Usa las flechas y guarda al terminar."
      footer={
        <>
          {error && (
            <p role="alert" className="w-full text-sm text-red-600 sm:mr-auto sm:w-auto sm:flex-1 sm:self-center">
              {error}
            </p>
          )}
          <Button variant="ghost" onClick={() => void requestClose()} disabled={saving}>
            Cancelar
          </Button>
          <Button onClick={() => void save()} loading={saving} disabled={!changed}>
            Guardar orden
          </Button>
        </>
      }
    >
      <ol role="list" className="divide-y divide-gray-100 rounded-xl border border-gray-200">
        {order.map((id, i) => {
          const b = byId.get(id)
          if (!b) return null
          return (
            <li key={id} className="flex items-center gap-3 px-3 py-2">
              <span
                aria-hidden="true"
                className="w-6 shrink-0 text-right text-xs font-semibold tabular-nums text-gray-500"
              >
                {i + 1}
              </span>
              {b.imageUrl ? (
                <img
                  src={b.imageUrl}
                  alt=""
                  loading="lazy"
                  className="h-9 w-9 shrink-0 rounded-lg bg-gray-50 object-contain"
                />
              ) : (
                <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-gray-100 text-gray-400">
                  <ImageOff aria-hidden="true" className="h-4 w-4" />
                </span>
              )}
              <span className="min-w-0 flex-1 truncate text-sm text-gray-800">{b.name}</span>
              {!b.active && <Pill>Inactiva</Pill>}
              <MoveButtons name={b.name} index={i} count={order.length} layout="row" onMove={(to) => move(i, to)} />
            </li>
          )
        })}
      </ol>
      <span className="sr-only" aria-live="polite">
        {announce}
      </span>
    </Drawer>
  )
}
