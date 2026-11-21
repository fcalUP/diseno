import { useRef } from 'react'
import { ChevronDown, ChevronUp } from 'lucide-react'
import { IconButton, cx } from '../../ui'

export interface MoveButtonsProps {
  /** Nombre del elemento para los lectores de pantalla ("Subir «TAR»"). */
  name: string
  index: number
  count: number
  onMove: (to: number) => void
  /** Ignora los clics mientras se guarda el movimiento anterior. */
  busy?: boolean
  layout?: 'column' | 'row'
}

// Botones Subir / Bajar. En los extremos quedan con aria-disabled en vez de disabled para
// que el foco no se pierda al llegar arriba o abajo.
export function MoveButtons({ name, index, count, onMove, busy = false, layout = 'column' }: MoveButtonsProps) {
  const wrap = useRef<HTMLDivElement>(null)
  const upOff = busy || index === 0
  const downOff = busy || index === count - 1

  const move = (dir: 'up' | 'down') => {
    if (dir === 'up' ? upOff : downOff) return
    onMove(dir === 'up' ? index - 1 : index + 1)
    // Al reordenar, React mueve el nodo y el navegador le quita el foco; se lo regresamos.
    requestAnimationFrame(() => wrap.current?.querySelector<HTMLElement>(`[data-dir="${dir}"]`)?.focus())
  }

  return (
    <div ref={wrap} className={cx('flex shrink-0', layout === 'column' ? 'flex-col' : 'flex-row gap-1')}>
      <IconButton
        icon={ChevronUp}
        size="sm"
        label={`Subir «${name}»`}
        data-dir="up"
        aria-disabled={upOff || undefined}
        className={cx(upOff && 'cursor-not-allowed opacity-40')}
        onClick={() => move('up')}
      />
      <IconButton
        icon={ChevronDown}
        size="sm"
        label={`Bajar «${name}»`}
        data-dir="down"
        aria-disabled={downOff || undefined}
        className={cx(downOff && 'cursor-not-allowed opacity-40')}
        onClick={() => move('down')}
      />
    </div>
  )
}
