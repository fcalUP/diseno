import type { SecretStatus } from '../../../lib/types'
import { Pill } from '../../ui'

const SOURCES: Record<SecretStatus['source'], string> = {
  db: 'guardada en Ajustes',
  env: 'variable de entorno',
  none: '',
}

// Estado de un secreto sin mostrar nunca su valor: "Configurada ✓ (variable de entorno)".
export function SecretBadge({ status, word = 'Configurada' }: { status: SecretStatus; word?: string }) {
  if (!status.configured) return <Pill tone="yellow">Sin configurar</Pill>
  return (
    <Pill tone="green">
      {word} ✓{SOURCES[status.source] ? ` (${SOURCES[status.source]})` : ''}
    </Pill>
  )
}
