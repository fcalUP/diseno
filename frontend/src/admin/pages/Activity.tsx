import { useState, type ReactNode } from 'react'
import { Bot, ChevronRight, Layers, Logs, Redo2, RefreshCw, ShieldCheck, Undo2, User, X } from 'lucide-react'
import type { AuditEntry } from '../../lib/types'
import {
  Button,
  Card,
  EmptyState,
  PageHeader,
  Pill,
  SearchInput,
  Select,
  cx,
  focusRing,
  useConfirm,
  useUndo,
} from '../ui'
import { AuditDiff } from './system/AuditDiff'
import { ACTOR_OPTIONS, ENTITY_OPTIONS, actorTypeLabel, entityInfo, isUndoRedo } from './system/audit'
import { formatDateTime, formatRelative, plural } from './system/format'
import { LoadError, PageLoading } from './system/PageState'
import { useAuditFeed } from './system/useAuditFeed'
import { useDebounced } from './system/useDebounced'
import { useNow } from './system/useNow'
import { useRedo } from './system/useRedo'

const ACTOR_ICONS = { admin: ShieldCheck, student: User, system: Bot }

type Kind = 'undo' | 'redo' | 'undoBatch' | 'redoBatch'

interface EntryItemProps {
  entry: AuditEntry
  now: number
  open: boolean
  /** Acción que corre sobre esta entrada, si hay. */
  running: Kind | null
  /** Hay otra acción corriendo en la página. */
  locked: boolean
  showBatchLink: boolean
  onToggle: () => void
  onAction: (kind: Kind) => void
  onBatch: (batchId: string) => void
}

function EntryItem({ entry, now, open, running, locked, showBatchLink, onToggle, onAction, onBatch }: EntryItemProps) {
  const { label, tone } = entityInfo(entry.entity)
  const ActorIcon = ACTOR_ICONS[entry.actorType] ?? User
  const summaryId = `audit-${entry.id}-summary`
  const diffId = `audit-${entry.id}-diff`
  const reversible = entry.undoable && !isUndoRedo(entry)
  const canUndo = reversible && !entry.undoneAt
  const canRedo = reversible && Boolean(entry.undoneAt)
  const batchId = entry.batchId
  const hasDetails = entry.before != null || entry.after != null

  const action = (kind: Kind, text: string, Icon: typeof Undo2) => (
    <Button
      size="sm"
      variant="ghost"
      icon={Icon}
      loading={running === kind}
      disabled={locked}
      aria-describedby={summaryId}
      onClick={() => onAction(kind)}
    >
      {text}
    </Button>
  )

  return (
    <li className="p-4 sm:p-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-1.5">
            <Pill tone={tone}>{label}</Pill>
            {entry.undoneAt && (
              <Pill tone="gray" title={`Deshecho el ${formatDateTime(entry.undoneAt)}`}>
                <Undo2 aria-hidden="true" className="h-3 w-3" />
                Deshecho
              </Pill>
            )}
            {isUndoRedo(entry) && entry.revertsId !== null && (
              <span className="text-xs text-gray-500">
                {entry.action === 'undo' ? 'Deshace' : 'Rehace'} el movimiento #{entry.revertsId}
              </span>
            )}
          </div>
          <p
            id={summaryId}
            className={cx('mt-1.5 break-words text-sm font-medium', entry.undoneAt ? 'text-gray-500' : 'text-gray-800')}
          >
            {entry.summary}
          </p>
          <p className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-gray-500">
            <span className="inline-flex items-center gap-1">
              <ActorIcon aria-hidden="true" className="h-3.5 w-3.5" />
              <span className="sr-only">{actorTypeLabel(entry.actorType)}:</span>
              <span className="break-all">{entry.actorLabel || actorTypeLabel(entry.actorType)}</span>
            </span>
            <span aria-hidden="true">·</span>
            <time dateTime={entry.at}>{formatRelative(entry.at, now)}</time>
            <span aria-hidden="true">·</span>
            <span>{formatDateTime(entry.at)}</span>
            <span aria-hidden="true">·</span>
            <span>#{entry.id}</span>
            {batchId && showBatchLink && (
              <>
                <span aria-hidden="true">·</span>
                <button
                  type="button"
                  onClick={() => onBatch(batchId)}
                  className={cx(
                    'inline-flex items-center gap-1 rounded font-medium text-purple-600 hover:underline',
                    focusRing,
                  )}
                >
                  <Layers aria-hidden="true" className="h-3.5 w-3.5" />
                  Ver lote
                </button>
              </>
            )}
          </p>
        </div>
        {(canUndo || canRedo) && (
          <div className="flex flex-wrap gap-1 sm:justify-end">
            {canUndo && action('undo', 'Deshacer', Undo2)}
            {canRedo && action('redo', 'Rehacer', Redo2)}
            {batchId && canUndo && action('undoBatch', 'Deshacer lote', Layers)}
            {batchId && canRedo && action('redoBatch', 'Rehacer lote', Layers)}
          </div>
        )}
      </div>

      {hasDetails && (
        <>
          <button
            type="button"
            aria-expanded={open}
            aria-controls={diffId}
            onClick={onToggle}
            className={cx(
              'mt-2 inline-flex items-center gap-1 rounded-full px-2 py-1 text-xs font-medium text-gray-600 hover:bg-gray-100',
              focusRing,
            )}
          >
            <ChevronRight aria-hidden="true" className={cx('h-4 w-4 transition-transform', open && 'rotate-90')} />
            {open ? 'Ocultar cambios' : 'Ver cambios'}
          </button>
          <div id={diffId} hidden={!open} className="mt-2">
            {open && (
              <AuditDiff before={entry.before} after={entry.after} caption={`Cambios del movimiento ${entry.id}`} />
            )}
          </div>
        </>
      )}
    </li>
  )
}

const CONFIRM_BATCH = {
  undoBatch: {
    title: '¿Deshacer todo el lote?',
    message: 'Se deshacen todos los cambios de esa misma operación, por ejemplo una importación o un cambio masivo.',
    confirmLabel: 'Deshacer lote',
  },
  redoBatch: {
    title: '¿Rehacer todo el lote?',
    message: 'Se vuelven a aplicar todos los cambios de esa misma operación.',
    confirmLabel: 'Rehacer lote',
  },
}

export default function Activity() {
  const undo = useUndo()
  const redo = useRedo()
  const confirm = useConfirm()
  const now = useNow()
  const [entity, setEntity] = useState('')
  const [actorType, setActorType] = useState('')
  const [text, setText] = useState('')
  const [batchId, setBatchId] = useState('')
  const q = useDebounced(text.trim())
  const feed = useAuditFeed({ entity, actorType, q, batchId })
  const [open, setOpen] = useState<ReadonlySet<number>>(new Set())
  const [running, setRunning] = useState<{ id: number; kind: Kind } | null>(null)

  const filtering = Boolean(entity || actorType || text.trim() || batchId)
  const clearFilters = () => {
    setEntity('')
    setActorType('')
    setText('')
    setBatchId('')
  }

  const toggle = (id: number) =>
    setOpen((s) => {
      const next = new Set(s)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  const showBatch = (id: string) => {
    setBatchId(id)
    window.scrollTo(0, 0)
  }

  const run = async (entry: AuditEntry, kind: Kind) => {
    if (running) return
    if (kind === 'undoBatch' || kind === 'redoBatch') {
      if (!(await confirm(CONFIRM_BATCH[kind]))) return
    }
    setRunning({ id: entry.id, kind })
    try {
      const batch = entry.batchId ?? undefined
      if (kind === 'undo') {
        if (await undo({ auditId: entry.id })) feed.patch(entry.id, { undoneAt: new Date().toISOString() })
      } else if (kind === 'redo') {
        if (await redo({ auditId: entry.id })) feed.patch(entry.id, { undoneAt: null })
      } else if (kind === 'undoBatch') {
        // Un lote toca varias entradas: más fácil volver a pedir la lista.
        if (await undo({ batchId: batch })) feed.reload()
      } else if (await redo({ batchId: batch })) {
        feed.reload()
      }
    } finally {
      setRunning(null)
    }
  }

  let content: ReactNode
  if (feed.entries.length === 0) {
    if (feed.error) content = <LoadError message={feed.error} onRetry={feed.reload} retrying={feed.loading} />
    else if (feed.loading) content = <PageLoading label="Cargando actividad…" />
    else
      content = (
        <Card padded={false}>
          {filtering ? (
            <EmptyState
              icon={Logs}
              title="No hay movimientos con estos filtros"
              action={
                <Button variant="ghost" icon={X} onClick={clearFilters}>
                  Limpiar filtros
                </Button>
              }
            />
          ) : (
            <EmptyState
              icon={Logs}
              title="Todavía no hay actividad"
              description="Aquí aparece cada cambio que hagan los administradores y los alumnos."
            />
          )}
        </Card>
      )
  } else {
    content = (
      <Card padded={false}>
        {feed.error && (
          <div
            role="alert"
            className="flex flex-wrap items-center gap-2 border-b border-red-100 bg-red-50 px-4 py-3 text-sm text-red-700"
          >
            <span className="min-w-0 flex-1">No se pudo actualizar: {feed.error}</span>
            <Button size="sm" variant="ghost" icon={RefreshCw} onClick={feed.reload}>
              Reintentar
            </Button>
          </div>
        )}
        <ul
          aria-label="Movimientos"
          aria-busy={feed.loading || undefined}
          className={cx('divide-y divide-gray-100', feed.loading && 'opacity-60')}
        >
          {feed.entries.map((entry) => (
            <EntryItem
              key={entry.id}
              entry={entry}
              now={now}
              open={open.has(entry.id)}
              running={running?.id === entry.id ? running.kind : null}
              locked={running !== null || feed.loading}
              showBatchLink={!batchId}
              onToggle={() => toggle(entry.id)}
              onAction={(kind) => void run(entry, kind)}
              onBatch={showBatch}
            />
          ))}
        </ul>
        <div className="border-t border-gray-100 p-4 text-center">
          {feed.nextBeforeId !== null ? (
            <Button variant="ghost" onClick={feed.loadMore} loading={feed.loadingMore} disabled={feed.loading}>
              Cargar más
            </Button>
          ) : (
            <p className="text-sm text-gray-500">No hay movimientos más antiguos.</p>
          )}
          {feed.moreError && (
            <p role="alert" className="mt-2 text-sm text-red-600">
              {feed.moreError}
            </p>
          )}
        </div>
      </Card>
    )
  }

  return (
    <>
      <PageHeader
        title="Actividad"
        subtitle="Cada cambio de administradores y alumnos. Desde aquí puedes deshacerlo o rehacerlo."
        actions={
          <Button
            variant="ghost"
            icon={RefreshCw}
            onClick={feed.reload}
            loading={feed.loading && feed.entries.length > 0}
          >
            Actualizar
          </Button>
        }
      />

      <Card className="mb-4">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <SearchInput
            value={text}
            onChange={setText}
            label="Buscar en la actividad"
            placeholder="Buscar por resumen o actor…"
            className="sm:col-span-2"
          />
          <Select
            label="Entidad"
            hideLabel
            value={entity}
            onChange={setEntity}
            options={ENTITY_OPTIONS}
            placeholder="Todas las entidades"
          />
          <Select
            label="Actor"
            hideLabel
            value={actorType}
            onChange={setActorType}
            options={ACTOR_OPTIONS}
            placeholder="Todos los actores"
          />
        </div>
        {filtering && (
          <div className="mt-3 flex flex-wrap items-center gap-2 text-sm">
            {batchId && (
              <Pill tone="purple" title={batchId}>
                <Layers aria-hidden="true" className="h-3 w-3" />
                Sólo el lote {batchId.slice(0, 8)}
              </Pill>
            )}
            <Button size="sm" variant="ghost" icon={X} onClick={clearFilters}>
              Limpiar filtros
            </Button>
          </div>
        )}
      </Card>

      <p aria-live="polite" className="mb-2 text-xs text-gray-500">
        {feed.loading && feed.entries.length === 0
          ? ''
          : `Mostrando ${plural(feed.entries.length, 'movimiento', 'movimientos')}${feed.nextBeforeId !== null ? ' (hay más)' : ''}.`}
      </p>

      {content}
    </>
  )
}
