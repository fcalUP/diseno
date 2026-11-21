import { CircleX, RefreshCw } from 'lucide-react'
import { Button, Card, Spinner } from '../../ui'

export function PageLoading({ label = 'Cargando…' }: { label?: string }) {
  return (
    <Card className="flex justify-center py-12">
      <Spinner label={label} showLabel />
    </Card>
  )
}

interface LoadErrorProps {
  title?: string
  message: string
  onRetry?: () => void
  retrying?: boolean
}

// Tarjeta de error con "Reintentar" para cuando no se pudo cargar una sección.
export function LoadError({ title = 'No se pudo cargar la información', message, onRetry, retrying }: LoadErrorProps) {
  return (
    <Card role="alert" className="flex flex-col items-center gap-3 py-10 text-center">
      <CircleX aria-hidden="true" className="h-8 w-8 text-red-500" />
      <div>
        <p className="font-semibold text-gray-800">{title}</p>
        <p className="mt-1 text-sm text-gray-500">{message}</p>
      </div>
      {onRetry && (
        <Button variant="ghost" icon={RefreshCw} onClick={onRetry} loading={retrying}>
          Reintentar
        </Button>
      )}
    </Card>
  )
}
