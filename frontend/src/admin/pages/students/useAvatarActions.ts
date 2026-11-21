import { api, errorMessage } from '../../../lib/api'
import type { Avatar } from '../../../lib/types'
import { useConfirm, useToast } from '../../ui'
import { useUndoToast } from './useUndoToast'

const DEFAULT_CODES = ['DefaultH', 'DefaultF']

// Guardar y quitar el avatar de un nivel, con confirmación al quitar y "Deshacer".
// `afterUndo` recarga lo que muestre la pantalla cuando se deshace el cambio.
export function useAvatarActions(afterUndo: () => void) {
  const toast = useToast()
  const confirm = useConfirm()
  const done = useUndoToast()

  const save = async (code: string, level: number, url: string): Promise<Avatar | null> => {
    try {
      const res = await api.admin.setAvatar({ code, level, url })
      done(`Avatar del nivel ${level} guardado (${code}).`, { auditId: res.auditId }, afterUndo)
      return res.avatar
    } catch (e) {
      toast.error(errorMessage(e))
      return null
    }
  }

  const remove = async (code: string, level: number): Promise<boolean> => {
    const ok = await confirm({
      title: `¿Quitar el avatar del nivel ${level}?`,
      message: DEFAULT_CODES.includes(code)
        ? `Es un avatar por defecto (${code}): quien no tenga imagen propia en ese nivel se queda sin avatar.`
        : `Se quita la imagen de ${code}. Mientras no haya otra, el alumno ve el avatar por defecto.`,
      confirmLabel: 'Quitar',
      danger: true,
    })
    if (!ok) return false
    try {
      const res = await api.admin.deleteAvatar(code, level)
      done(`Avatar del nivel ${level} quitado (${code}).`, { auditId: res.auditId }, afterUndo)
      return true
    } catch (e) {
      toast.error(errorMessage(e))
      return false
    }
  }

  return { save, remove }
}
