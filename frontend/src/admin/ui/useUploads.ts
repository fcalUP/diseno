import { useCallback, useState } from 'react'
import type { ConfirmOptions } from './useConfirm'

// Cuenta las subidas en curso de uno o varios ImageUpload (pásale onUploadingChange a cada
// uno). Mientras uploading sea true la url todavía no llega: no hay que guardar ni cerrar
// sin preguntar, o la imagen se pierde.
export function useUploads(): [uploading: boolean, onUploadingChange: (busy: boolean) => void] {
  const [count, setCount] = useState(0)
  const onUploadingChange = useCallback((busy: boolean) => setCount((n) => Math.max(0, n + (busy ? 1 : -1))), [])
  return [count > 0, onUploadingChange]
}

// Pregunta para cerrar un formulario con una subida a medias.
export const CLOSE_WHILE_UPLOADING: ConfirmOptions = {
  title: '¿Cerrar mientras se sube la imagen?',
  message: 'La imagen todavía se está subiendo. Si cierras ahora, no se guarda.',
  confirmLabel: 'Cerrar sin la imagen',
  cancelLabel: 'Seguir subiendo',
  danger: true,
}
