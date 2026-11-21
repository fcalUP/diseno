import { useCallback } from 'react'
import { api, ApiError, errorMessage } from '../../../lib/api'
import type { ApiErrorBody, AuditRef, BatchUndoResponse, UndoResponse } from '../../../lib/types'
import { useConfirm, useToast } from '../../ui'

// La pareja de useUndo para "Rehacer": mismo manejo del 409 (pregunta antes de forzar) y
// mismo aviso. Resuelve true si se rehízo algo; los errores los muestra ella misma.
export function useRedo(): (ref: AuditRef) => Promise<boolean> {
  const toast = useToast()
  const confirm = useConfirm()

  return useCallback(
    async ({ auditId, batchId }: AuditRef) => {
      if (!batchId && auditId === undefined) return false
      const run = (force: boolean) =>
        batchId ? api.admin.redoBatch(batchId, force) : api.admin.redo(auditId as number, force)

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
              ? `Desde entonces cambiaron estos campos: ${fields.join(', ')}. Si continúas, se vuelve a aplicar el cambio encima.`
              : 'El registro cambió desde entonces. Si continúas, se vuelve a aplicar el cambio encima.',
            confirmLabel: 'Rehacer de todos modos',
            danger: true,
          })
          if (!ok) return false
          res = await run(true)
        }

        if ('done' in res) {
          const failed = res.failed ?? []
          if (failed.length > 0) {
            toast.error(
              `Se rehicieron ${res.done} cambios; ${failed.length} no se pudieron rehacer (${failed[0].error}).`,
            )
            return res.done > 0
          }
          toast.success(res.done === 1 ? 'Se rehízo 1 cambio.' : `Se rehicieron ${res.done} cambios.`)
        } else {
          toast.success(res.summary || 'Cambio rehecho.')
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
