import { useCallback } from 'react'
import { api, ApiError, errorMessage } from '../../lib/api'
import type { ApiErrorBody, AuditRef, BatchUndoResponse, UndoResponse } from '../../lib/types'
import { useConfirm } from './useConfirm'
import { useToast } from './useToast'

// Devuelve una función que deshace un cambio (auditId) o un lote (batchId) y avisa con un
// toast. Si el registro cambió después, pregunta antes de forzar. Resuelve true si se deshizo
// algo; los errores los muestra ella misma, no lanza.
export function useUndo(): (ref: AuditRef) => Promise<boolean> {
  const toast = useToast()
  const confirm = useConfirm()

  return useCallback(
    async ({ auditId, batchId }: AuditRef) => {
      if (!batchId && auditId === undefined) return false
      const run = (force: boolean) =>
        batchId ? api.admin.undoBatch(batchId, force) : api.admin.undo(auditId as number, force)

      try {
        let res: UndoResponse | BatchUndoResponse
        try {
          res = await run(false)
        } catch (e) {
          const body = e instanceof ApiError && e.status === 409 ? (e.data as ApiErrorBody | null) : null
          if (!body?.conflict) throw e
          const fields = body.fields ?? []
          const ok = await confirm({
            title: 'El registro cambió después',
            message: fields.length
              ? `Desde entonces cambiaron estos campos: ${fields.join(', ')}. Si continúas, vuelven al valor anterior.`
              : 'El registro cambió desde entonces. Si continúas, vuelve al valor anterior.',
            confirmLabel: 'Deshacer de todos modos',
            danger: true,
          })
          if (!ok) return false
          res = await run(true)
        }

        if ('done' in res) {
          if (res.failed.length > 0) {
            toast.error(
              `Se deshicieron ${res.done} cambios; ${res.failed.length} no se pudieron deshacer (${res.failed[0].error}).`,
            )
            return res.done > 0
          }
          toast.success(res.done === 1 ? 'Se deshizo 1 cambio.' : `Se deshicieron ${res.done} cambios.`)
        } else {
          toast.success(res.summary || 'Cambio deshecho.')
        }
        return true
      } catch (e) {
        toast.error(errorMessage(e))
        return false
      }
    },
    [toast, confirm],
  )
}
