import type { AuditRef } from '../../../lib/types'
import { useToast, useUndo } from '../../ui'

// Aviso con botón "Deshacer" para el auditId o batchId que devolvió el servidor.
// `after` corre solo si de verdad se deshizo (p. ej. para recargar la lista).
export function useUndoToast() {
  const toast = useToast()
  const undo = useUndo()
  return (message: string, ref: AuditRef, after?: () => void, kind: 'success' | 'error' = 'success') => {
    toast[kind](message, {
      undo: async () => {
        if (await undo(ref)) after?.()
      },
    })
  }
}
