// Piezas chicas que repiten las páginas de alumnos.
import { useId, useState, type ReactNode } from 'react'
import { Link, type LinkProps } from 'react-router'
import { Check, Copy, RefreshCw, TriangleAlert } from 'lucide-react'
import { calcLevel, getLevelProgress } from '../../../lib/level'
import type { Level } from '../../../lib/types'
import { Button, Spinner, cx, focusRing, useToast } from '../../ui'
import { copyText, formatNumber, initials } from './util'

const avatarSizes = { sm: 'h-9 w-9 text-xs', md: 'h-12 w-12 text-sm', lg: 'h-16 w-16 text-lg' }

// Imagen del nivel actual o, si no hay (o no carga), un círculo con iniciales.
export function StudentAvatar({ name, url, size = 'sm' }: { name: string; url?: string; size?: keyof typeof avatarSizes }) {
  const [broken, setBroken] = useState<string | null>(null)
  const base = cx('shrink-0 rounded-full', avatarSizes[size])
  if (url && broken !== url) {
    return (
      <img
        src={url}
        alt=""
        loading="lazy"
        onError={() => setBroken(url)}
        className={cx(base, 'border border-gray-100 bg-gray-50 object-cover')}
      />
    )
  }
  return (
    <span aria-hidden="true" className={cx(base, 'flex items-center justify-center bg-gray-200 font-semibold text-gray-600')}>
      {initials(name)}
    </span>
  )
}

export function CopyButton({
  text,
  label = 'Copiar',
  variant = 'ghost',
}: {
  text: string
  label?: string
  variant?: 'ghost' | 'secondary'
}) {
  const toast = useToast()
  const [copied, setCopied] = useState(false)
  return (
    <>
      <Button
        variant={variant}
        size="sm"
        icon={copied ? Check : Copy}
        onClick={async () => {
          if (await copyText(text)) {
            setCopied(true)
            setTimeout(() => setCopied(false), 2000)
          } else {
            toast.error('No se pudo copiar. Selecciona el texto y cópialo a mano.')
          }
        }}
      >
        {copied ? 'Copiado' : label}
      </Button>
      <span role="status" className="sr-only">
        {copied ? 'Copiado al portapapeles.' : ''}
      </span>
    </>
  )
}

// Contraseña recién creada: se ve una sola vez.
export function PasswordReveal({ password, title = 'Contraseña nueva' }: { password: string; title?: string }) {
  return (
    <div className="rounded-xl border border-orange-200 bg-orange-50 p-4">
      <p className="text-sm font-medium text-orange-800">{title}</p>
      <div className="mt-2 flex flex-wrap items-center gap-3">
        <code className="select-all rounded-lg bg-white px-3 py-1.5 font-mono text-xl font-bold tracking-widest text-gray-900">
          {password}
        </code>
        <CopyButton text={password} />
      </div>
      <p className="mt-2 text-xs text-orange-800">Cópiala ahora: no se vuelve a mostrar.</p>
    </div>
  )
}

const isPreset = (v: string) => v === 'Hombre' || v === 'Mujer'

// Las altas usan Hombre / Mujer; una edición puede conservar su valor importado original.
export function SexoField({ value, onChange, error, legacyValue }: { value: string; onChange: (value: string) => void; error?: string; legacyValue?: string }) {
  const id = useId()
  const legacy = legacyValue !== undefined && !isPreset(legacyValue) ? legacyValue : undefined
  const radio = cx('h-4 w-4 cursor-pointer accent-orange-500', focusRing)
  return (
    <fieldset aria-describedby={error ? `${id}-error` : undefined}>
      <legend className="mb-1 block text-sm font-medium text-gray-700">Sexo</legend>
      <div className="flex min-h-[2.375rem] flex-wrap items-center gap-x-5 gap-y-2">
        {(['Hombre', 'Mujer'] as const).map((s) => (
          <label key={s} className="inline-flex cursor-pointer items-center gap-2 text-sm text-gray-700">
            <input
              type="radio"
              name={id}
              className={radio}
              checked={value === s}
              onChange={() => onChange(s)}
            />
            {s}
          </label>
        ))}
        {legacy !== undefined && <label className="inline-flex cursor-pointer items-center gap-2 text-sm text-gray-700">
          <input
            type="radio"
            name={id}
            className={radio}
            checked={value === legacy}
            onChange={() => onChange(legacy)}
          />
          {legacy || 'Sin valor'} (heredado)
        </label>}
      </div>
      {error && (
        <p id={`${id}-error`} className="mt-1 text-xs text-red-600">
          {error}
        </p>
      )}
    </fieldset>
  )
}

// Nivel que tendría el alumno con esta EXP, con la barra hacia el siguiente.
export function LevelPreview({
  exp,
  levels,
  savedLevel,
  className,
}: {
  exp: number
  levels: Level[]
  savedLevel?: number
  className?: string
}) {
  if (levels.length === 0 || !Number.isFinite(exp)) return null
  const level = calcLevel(exp, levels)
  const { percentage, requiredExp } = getLevelProgress(exp, level, levels)
  const isMax = !levels.some((l) => l.level === level + 1)
  return (
    <div className={cx('rounded-xl bg-gray-50 p-3', className)}>
      <div className="flex flex-wrap items-center justify-between gap-2 text-sm" aria-live="polite">
        <p className="font-semibold text-gray-800">
          Nivel {level}
          {savedLevel !== undefined && savedLevel !== level && (
            <span className="ml-2 font-normal text-gray-500">(guardado: nivel {savedLevel})</span>
          )}
        </p>
        <p className="text-gray-500">
          {isMax ? 'Nivel máximo' : `${formatNumber(exp)} / ${formatNumber(requiredExp)} EXP para el nivel ${level + 1}`}
        </p>
      </div>
      <div className="mt-2 h-2 overflow-hidden rounded-full bg-gray-200" aria-hidden="true">
        <div className="h-full rounded-full bg-purple-600 transition-all" style={{ width: `${percentage}%` }} />
      </div>
    </div>
  )
}

export function ProgressBar({ value, max, label }: { value: number; max: number; label: string }) {
  const pct = max > 0 ? Math.round((value / max) * 100) : 0
  return (
    <div>
      <div className="mb-1 flex justify-between gap-2 text-sm text-gray-600">
        <span>{label}</span>
        <span className="tabular-nums">
          {value} de {max}
        </span>
      </div>
      <div
        role="progressbar"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={max}
        aria-valuenow={value}
        className="h-2.5 overflow-hidden rounded-full bg-gray-200"
      >
        <div className="h-full rounded-full bg-orange-500 transition-all" style={{ width: `${pct}%` }} />
      </div>
    </div>
  )
}

const linkVariants = {
  primary: 'bg-orange-500 text-white shadow-sm hover:bg-orange-600',
  secondary: 'bg-purple-600 text-white shadow-sm hover:bg-purple-700',
  ghost: 'bg-transparent text-gray-700 hover:bg-gray-100',
  link: 'text-orange-600 hover:bg-orange-50',
}

// Enlace del router con aspecto de botón.
export function LinkButton({
  variant = 'ghost',
  size = 'md',
  className,
  ...rest
}: LinkProps & { variant?: keyof typeof linkVariants; size?: 'sm' | 'md' }) {
  return (
    <Link
      className={cx(
        'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-full text-sm font-semibold transition-colors',
        focusRing,
        size === 'sm' ? 'px-3 py-1.5' : 'px-4 py-2',
        linkVariants[variant],
        className,
      )}
      {...rest}
    />
  )
}

export function Section({
  title,
  description,
  actions,
  children,
}: {
  title: string
  description?: ReactNode
  actions?: ReactNode
  children: ReactNode
}) {
  const id = useId()
  return (
    <section aria-labelledby={id}>
      <div className="mb-3 flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <h3 id={id} className="text-base font-semibold text-gray-800">
            {title}
          </h3>
          {description && <p className="mt-0.5 text-xs text-gray-500">{description}</p>}
        </div>
        {actions}
      </div>
      {children}
    </section>
  )
}

export function ErrorBanner({ message, onRetry, className }: { message: string; onRetry?: () => void; className?: string }) {
  return (
    <div
      role="alert"
      className={cx(
        'flex flex-wrap items-center gap-3 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700',
        className,
      )}
    >
      <TriangleAlert aria-hidden="true" className="h-4 w-4 shrink-0" />
      <p className="min-w-0 flex-1">{message}</p>
      {onRetry && (
        <Button size="sm" variant="ghost" icon={RefreshCw} onClick={onRetry}>
          Reintentar
        </Button>
      )}
    </div>
  )
}

export function LoadingBlock({ label = 'Cargando…' }: { label?: string }) {
  return (
    <div className="flex justify-center py-12">
      <Spinner size="lg" label={label} />
    </div>
  )
}
