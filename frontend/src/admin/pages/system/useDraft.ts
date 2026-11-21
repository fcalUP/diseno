import { useState } from 'react'

const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b)

export interface Draft<T extends object> {
  draft: T
  set: <K extends keyof T>(key: K, value: T[K]) => void
  /** Sólo las claves que cambiaron, para mandar en el PATCH. */
  patch: Partial<T>
  dirty: boolean
  reset: () => void
}

// Borrador de una sección de formulario. Si lo guardado cambia (se guardó, se deshizo o se
// recargó), el borrador vuelve a lo guardado; si cambia otra sección, éste no se toca.
export function useDraft<T extends object>(saved: T): Draft<T> {
  const savedKey = JSON.stringify(saved)
  const [base, setBase] = useState(savedKey)
  const [draft, setDraft] = useState<T>(saved)
  if (savedKey !== base) {
    setBase(savedKey)
    setDraft(saved)
  }

  const patch: Partial<T> = {}
  for (const key of Object.keys(saved) as (keyof T)[]) {
    if (!same(draft[key], saved[key])) patch[key] = draft[key]
  }

  return {
    draft,
    set: (key, value) => setDraft((d) => ({ ...d, [key]: value })),
    patch,
    dirty: Object.keys(patch).length > 0,
    reset: () => setDraft(saved),
  }
}
