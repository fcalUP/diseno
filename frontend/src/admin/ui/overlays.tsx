import { useEffect, useId, useRef, type KeyboardEvent, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { X } from 'lucide-react'
import { Button, IconButton } from './Button'
import { cx } from './styles'

const FOCUSABLE =
  'a[href],button:not([disabled]),input:not([disabled]):not([type="hidden"]),select:not([disabled]),textarea:not([disabled]),[tabindex]'

interface FrameProps {
  open: boolean
  onClose: () => void
  titleId: string
  descriptionId?: string
  side: 'center' | 'right'
  className: string
  children: ReactNode
}

// Base de Modal y Drawer: portal, fondo oscuro, Esc, foco atrapado y scroll bloqueado.
// Para enfocar un control al abrir, ponle el atributo data-autofocus.
function DialogFrame({ open, onClose, titleId, descriptionId, side, className, children }: FrameProps) {
  const panelRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const panel = panelRef.current
    const before = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const overflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    if (panel && !panel.contains(document.activeElement)) {
      const target = panel.querySelector<HTMLElement>('[data-autofocus]') ?? panel
      target.focus()
    }
    return () => {
      document.body.style.overflow = overflow
      if (before && !panel?.contains(before)) before.focus()
    }
  }, [open])

  if (!open) return null

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const panel = panelRef.current
    // Ignora teclas que vienen de otro diálogo abierto encima (también va por portal).
    if (!panel || !panel.contains(e.target as Node)) return
    if (e.key === 'Escape') {
      e.stopPropagation()
      onClose()
      return
    }
    if (e.key !== 'Tab') return
    const items = [...panel.querySelectorAll<HTMLElement>(FOCUSABLE)].filter(
      (el) => el.tabIndex >= 0 && el.getClientRects().length > 0,
    )
    if (items.length === 0) {
      e.preventDefault()
      return
    }
    const first = items[0]
    const last = items[items.length - 1]
    const active = document.activeElement
    if (e.shiftKey && (active === first || active === panel)) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && active === last) {
      e.preventDefault()
      first.focus()
    }
  }

  return createPortal(
    <div
      className={cx(
        'fixed inset-0 z-50 flex bg-black/50',
        side === 'center' ? 'items-center justify-center p-4' : 'justify-end',
      )}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={descriptionId}
        tabIndex={-1}
        onKeyDown={onKeyDown}
        className={cx('bg-white shadow-2xl outline-none', className)}
      >
        {children}
      </div>
    </div>,
    document.body,
  )
}

interface PanelProps {
  open: boolean
  onClose: () => void
  title: ReactNode
  description?: ReactNode
  children?: ReactNode
  /** Botones de abajo (Cancelar, Guardar...). */
  footer?: ReactNode
}

function PanelHeader({ id, title, description, onClose }: Pick<PanelProps, 'title' | 'description' | 'onClose'> & { id: string }) {
  return (
    <div className="flex items-start justify-between gap-4 border-b border-gray-100 px-6 py-4">
      <div className="min-w-0">
        <h2 id={`${id}-title`} className="text-lg font-semibold text-gray-800">
          {title}
        </h2>
        {description && (
          <p id={`${id}-desc`} className="mt-1 text-sm text-gray-500">
            {description}
          </p>
        )}
      </div>
      <IconButton icon={X} label="Cerrar" size="sm" onClick={onClose} />
    </div>
  )
}

const modalSizes = { sm: 'max-w-sm', md: 'max-w-lg', lg: 'max-w-2xl', xl: 'max-w-4xl' }

export interface ModalProps extends PanelProps {
  size?: keyof typeof modalSizes
}

export function Modal({ open, onClose, title, description, children, footer, size = 'md' }: ModalProps) {
  const id = useId()
  return (
    <DialogFrame
      open={open}
      onClose={onClose}
      titleId={`${id}-title`}
      descriptionId={description ? `${id}-desc` : undefined}
      side="center"
      className={cx('flex max-h-[90vh] w-full flex-col rounded-2xl', modalSizes[size])}
    >
      <PanelHeader id={id} title={title} description={description} onClose={onClose} />
      {children && <div className="flex-1 overflow-y-auto px-6 py-4">{children}</div>}
      {footer && <div className="flex flex-wrap justify-end gap-2 border-t border-gray-100 px-6 py-4">{footer}</div>}
    </DialogFrame>
  )
}

const drawerSizes = { md: 'max-w-md', lg: 'max-w-xl', xl: 'max-w-3xl' }

export interface DrawerProps extends PanelProps {
  size?: keyof typeof drawerSizes
}

// Panel lateral derecho a toda la altura.
export function Drawer({ open, onClose, title, description, children, footer, size = 'lg' }: DrawerProps) {
  const id = useId()
  return (
    <DialogFrame
      open={open}
      onClose={onClose}
      titleId={`${id}-title`}
      descriptionId={description ? `${id}-desc` : undefined}
      side="right"
      className={cx('flex h-full w-full flex-col', drawerSizes[size])}
    >
      <PanelHeader id={id} title={title} description={description} onClose={onClose} />
      <div className="flex-1 overflow-y-auto px-6 py-5">{children}</div>
      {footer && <div className="flex flex-wrap justify-end gap-2 border-t border-gray-100 px-6 py-4">{footer}</div>}
    </DialogFrame>
  )
}

export interface ConfirmDialogProps {
  open: boolean
  title: string
  message?: ReactNode
  confirmLabel?: string
  cancelLabel?: string
  /** Acción destructiva: botón rojo y el foco empieza en Cancelar. */
  danger?: boolean
  busy?: boolean
  onConfirm: () => void
  onCancel: () => void
}

export function ConfirmDialog({
  open,
  title,
  message,
  confirmLabel = 'Confirmar',
  cancelLabel = 'Cancelar',
  danger = false,
  busy = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  return (
    <Modal
      open={open}
      onClose={onCancel}
      title={title}
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onCancel} disabled={busy} data-autofocus={danger || undefined}>
            {cancelLabel}
          </Button>
          <Button
            variant={danger ? 'danger' : 'primary'}
            onClick={onConfirm}
            loading={busy}
            data-autofocus={danger ? undefined : true}
          >
            {confirmLabel}
          </Button>
        </>
      }
    >
      {message && <div className="text-sm text-gray-600">{message}</div>}
    </Modal>
  )
}
