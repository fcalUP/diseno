import { useId, useRef, useState } from 'react'
import { FileUp } from 'lucide-react'
import { errorMessage } from '../../../lib/api'
import { Button } from '../../ui'
import { readServiceAccount, type ServiceAccount } from './google'

interface ServiceAccountPickerProps {
  label: string
  /** Recibe el JSON ya validado; si lanza, el error se muestra bajo el botón. */
  onLoad: (sa: ServiceAccount) => void | Promise<void>
  disabled?: boolean
  variant?: 'primary' | 'secondary' | 'ghost'
}

// Botón para elegir el .json de una cuenta de servicio de Google. Valida el archivo antes de
// entregarlo; el contenido sólo vive en memoria.
export function ServiceAccountPicker({ label, onLoad, disabled, variant = 'secondary' }: ServiceAccountPickerProps) {
  const id = useId()
  const inputRef = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const pick = async (file: File | undefined) => {
    if (!file) return
    setError('')
    setBusy(true)
    try {
      await onLoad(await readServiceAccount(file))
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div>
      <Button
        variant={variant}
        size="sm"
        icon={FileUp}
        loading={busy}
        disabled={disabled}
        aria-describedby={error ? `${id}-error` : undefined}
        onClick={() => inputRef.current?.click()}
      >
        {label}
      </Button>
      <input
        ref={inputRef}
        type="file"
        accept=".json,application/json"
        className="sr-only"
        tabIndex={-1}
        aria-hidden="true"
        onChange={(e) => {
          void pick(e.target.files?.[0])
          // Permite volver a elegir el mismo archivo.
          e.target.value = ''
        }}
      />
      {error && (
        <p id={`${id}-error`} role="alert" className="mt-1 text-xs text-red-600">
          {error}
        </p>
      )}
    </div>
  )
}
