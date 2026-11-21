import { useCallback, useEffect, useRef, useState } from 'react'
import { api, errorMessage, type AuditFilters } from '../../../lib/api'
import type { AuditEntry } from '../../../lib/types'

const PAGE_SIZE = 50

interface FeedState {
  entries: AuditEntry[]
  nextBeforeId: number | null
  loading: boolean
  error: string
}

const EMPTY: FeedState = { entries: [], nextBeforeId: null, loading: true, error: '' }

export interface AuditFeed extends FeedState {
  loadingMore: boolean
  moreError: string
  /** Trae la página siguiente (beforeId) y la agrega al final. */
  loadMore: () => void
  /** Vuelve a empezar desde la primera página. */
  reload: () => void
  /** Ajusta una entrada en memoria (p. ej. tras deshacerla) sin volver a pedir la lista. */
  patch: (id: number, change: Partial<AuditEntry>) => void
}

// Bitácora paginada con "Cargar más". Al cambiar los filtros empieza de cero y descarta
// respuestas que lleguen tarde.
export function useAuditFeed(filters: Omit<AuditFilters, 'limit' | 'beforeId'>): AuditFeed {
  const key = JSON.stringify(filters)
  const [feed, setFeed] = useState<FeedState>(EMPTY)
  const [tick, setTick] = useState(0)
  const [prevKey, setPrevKey] = useState(key)
  const [loadingMore, setLoadingMore] = useState(false)
  const [moreError, setMoreError] = useState('')
  // Cambia con cada carga desde cero; "Cargar más" lo usa para saber si su respuesta sigue valiendo.
  const generation = useRef(0)

  if (key !== prevKey) {
    setPrevKey(key)
    setFeed(EMPTY)
    setLoadingMore(false)
    setMoreError('')
  }

  useEffect(() => {
    const gen = ++generation.current
    const params: AuditFilters = { ...JSON.parse(key), limit: PAGE_SIZE }
    api.admin.audit(params).then(
      (res) => {
        if (gen === generation.current) {
          setFeed({ entries: res.entries ?? [], nextBeforeId: res.nextBeforeId, loading: false, error: '' })
        }
      },
      (e: unknown) => {
        if (gen === generation.current) setFeed((f) => ({ ...f, loading: false, error: errorMessage(e) }))
      },
    )
  }, [key, tick])

  const reload = useCallback(() => {
    setFeed((f) => ({ ...f, loading: true, error: '' }))
    setLoadingMore(false)
    setMoreError('')
    setTick((t) => t + 1)
  }, [])

  const loadMore = () => {
    if (feed.nextBeforeId === null || loadingMore || feed.loading) return
    const gen = generation.current
    setLoadingMore(true)
    setMoreError('')
    api.admin.audit({ ...filters, limit: PAGE_SIZE, beforeId: feed.nextBeforeId }).then(
      (res) => {
        if (gen !== generation.current) return
        setFeed((f) => {
          const seen = new Set(f.entries.map((e) => e.id))
          return {
            ...f,
            entries: [...f.entries, ...(res.entries ?? []).filter((e) => !seen.has(e.id))],
            nextBeforeId: res.nextBeforeId,
          }
        })
        setLoadingMore(false)
      },
      (e: unknown) => {
        if (gen !== generation.current) return
        setMoreError(errorMessage(e))
        setLoadingMore(false)
      },
    )
  }

  const patch = useCallback((id: number, change: Partial<AuditEntry>) => {
    setFeed((f) => ({ ...f, entries: f.entries.map((e) => (e.id === id ? { ...e, ...change } : e)) }))
  }, [])

  return { ...feed, loadingMore, moreError, loadMore, reload, patch }
}
