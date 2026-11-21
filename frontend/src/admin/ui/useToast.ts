import { createContext, useContext } from 'react'

export type ToastKind = 'success' | 'error' | 'info'

export interface ToastOptions {
  /** Si viene, el aviso muestra "Deshacer" y dura 10 s en vez de 6 s. */
  undo?: () => Promise<unknown>
}

export interface ToastApi {
  success: (message: string, options?: ToastOptions) => void
  error: (message: string, options?: ToastOptions) => void
  info: (message: string, options?: ToastOptions) => void
}

export const ToastContext = createContext<ToastApi | null>(null)

export function useToast(): ToastApi {
  const toast = useContext(ToastContext)
  if (!toast) throw new Error('useToast debe usarse dentro de <Toaster>')
  return toast
}
