import { useCallback, useEffect, useState, type DependencyList } from 'react'

export interface AsyncState<T> {
  data: T | undefined
  error: Error | null
  loading: boolean
  /** Vuelve a pedir los datos. */
  reload: () => void
  /** Ajusta los datos en memoria (p. ej. tras guardar una fila) sin volver a pedirlos. */
  setData: (update: (prev: T | undefined) => T | undefined) => void
}

interface Snapshot<T> {
  data: T | undefined
  error: Error | null
  loading: boolean
}

function sameDeps(a: DependencyList, b: DependencyList) {
  return a.length === b.length && a.every((v, i) => Object.is(v, b[i]))
}

// Carga datos al montar y cada vez que cambian `deps`. Si llega una respuesta vieja
// (cambiaron los filtros a medio camino) se descarta.
export function useAsync<T>(fn: () => Promise<T>, deps: DependencyList): AsyncState<T> {
  const [state, setState] = useState<Snapshot<T>>({ data: undefined, error: null, loading: true })
  const [tick, setTick] = useState(0)
  const [prevDeps, setPrevDeps] = useState(deps)
  if (!sameDeps(prevDeps, deps)) {
    // Cambiaron las dependencias: se marca como cargando desde este mismo render.
    setPrevDeps(deps)
    setState((s) => ({ ...s, loading: true }))
  }

  useEffect(() => {
    let alive = true
    fn().then(
      (data) => {
        if (alive) setState({ data, error: null, loading: false })
      },
      (e: unknown) => {
        const error = e instanceof Error ? e : new Error(String(e))
        if (alive) setState((s) => ({ ...s, error, loading: false }))
      },
    )
    return () => {
      alive = false
    }
    // Las dependencias las decide quien llama, igual que en useEffect.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, tick])

  const reload = useCallback(() => {
    setState((s) => ({ ...s, loading: true }))
    setTick((t) => t + 1)
  }, [])

  const setData = useCallback((update: (prev: T | undefined) => T | undefined) => {
    setState((s) => ({ ...s, data: update(s.data) }))
  }, [])

  return { ...state, reload, setData }
}
