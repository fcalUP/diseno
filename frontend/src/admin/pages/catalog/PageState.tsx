import { CircleAlert, RefreshCw } from 'lucide-react'
import { Button, Card, Spinner, cx } from '../../ui'

export function LoadingCard({ label = 'Cargando…' }: { label?: string }) {
  return (
    <Card className="flex justify-center py-12">
      <Spinner label={label} showLabel />
    </Card>
  )
}

export interface LoadErrorProps {
  message: string
  onRetry: () => void
  className?: string
}

// Aviso de que no se pudo cargar algo, con botón para reintentar.
export function LoadError({ message, onRetry, className }: LoadErrorProps) {
  return (
    <div
      role="alert"
      className={cx(
        'flex flex-col gap-3 rounded-2xl border border-red-100 bg-red-50 p-4 text-sm text-red-700 sm:flex-row sm:items-center',
        className,
      )}
    >
      <CircleAlert aria-hidden="true" className="h-5 w-5 shrink-0" />
      <p className="min-w-0 flex-1">{message}</p>
      <Button variant="ghost" size="sm" icon={RefreshCw} onClick={onRetry} className="self-start sm:self-auto">
        Reintentar
      </Button>
    </div>
  )
}
