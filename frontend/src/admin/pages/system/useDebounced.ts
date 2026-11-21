import { useEffect, useState } from 'react'

// Devuelve `value` cuando deja de cambiar por `ms` milisegundos (búsquedas al escribir).
export function useDebounced<T>(value: T, ms = 350): T {
  const [settled, setSettled] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setSettled(value), ms)
    return () => clearTimeout(id)
  }, [value, ms])
  return settled
}
