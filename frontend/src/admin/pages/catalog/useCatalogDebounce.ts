import { useEffect, useState } from 'react'

// Devuelve `value` cuando deja de cambiar durante `ms` milisegundos (búsquedas).
export function useCatalogDebounce<T>(value: T, ms = 300): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), ms)
    return () => clearTimeout(timer)
  }, [value, ms])
  return debounced
}
