import type { HTMLAttributes, ReactNode } from 'react'
import { Inbox, LoaderCircle, type LucideIcon } from 'lucide-react'
import { cx } from './styles'

export interface CardProps extends HTMLAttributes<HTMLDivElement> {
  /** Con `false` la tarjeta no lleva padding (útil para tablas). */
  padded?: boolean
}

export function Card({ padded = true, className, ...rest }: CardProps) {
  return (
    <div
      className={cx('rounded-2xl border border-gray-100 bg-white shadow-sm', padded && 'p-5 sm:p-6', className)}
      {...rest}
    />
  )
}

export interface PageHeaderProps {
  title: ReactNode
  subtitle?: ReactNode
  actions?: ReactNode
}

export function PageHeader({ title, subtitle, actions }: PageHeaderProps) {
  return (
    <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
      <div className="min-w-0">
        <h1 className="text-2xl font-bold text-gray-800">{title}</h1>
        {subtitle && <p className="mt-1 text-sm text-gray-500">{subtitle}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  )
}

const statTones = {
  orange: 'bg-orange-100 text-orange-600',
  purple: 'bg-purple-100 text-purple-600',
  blue: 'bg-blue-100 text-blue-600',
  green: 'bg-green-100 text-green-600',
  gray: 'bg-gray-100 text-gray-600',
}

export interface StatProps {
  label: string
  value: ReactNode
  hint?: ReactNode
  icon?: LucideIcon
  tone?: keyof typeof statTones
}

export function Stat({ label, value, hint, icon: Icon, tone = 'orange' }: StatProps) {
  return (
    <Card className="flex items-start gap-4">
      {Icon && (
        <span className={cx('flex h-11 w-11 shrink-0 items-center justify-center rounded-xl', statTones[tone])}>
          <Icon aria-hidden="true" className="h-5 w-5" />
        </span>
      )}
      <div className="min-w-0">
        <p className="text-sm font-medium text-gray-500">{label}</p>
        <p className="mt-1 text-2xl font-bold text-gray-800">{value}</p>
        {hint && <p className="mt-1 text-xs text-gray-500">{hint}</p>}
      </div>
    </Card>
  )
}

export interface EmptyStateProps {
  title: string
  description?: ReactNode
  icon?: LucideIcon
  action?: ReactNode
  className?: string
}

export function EmptyState({ title, description, icon: Icon = Inbox, action, className }: EmptyStateProps) {
  return (
    <div className={cx('flex flex-col items-center justify-center px-6 py-12 text-center', className)}>
      <span className="mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 text-gray-400">
        <Icon aria-hidden="true" className="h-6 w-6" />
      </span>
      <p className="font-semibold text-gray-700">{title}</p>
      {description && <p className="mt-1 max-w-sm text-sm text-gray-500">{description}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}

const pillTones = {
  gray: 'bg-gray-100 text-gray-700',
  green: 'bg-green-100 text-green-700',
  red: 'bg-red-100 text-red-700',
  orange: 'bg-orange-100 text-orange-700',
  purple: 'bg-purple-100 text-purple-700',
  blue: 'bg-blue-100 text-blue-700',
  yellow: 'bg-yellow-100 text-yellow-800',
}

export interface PillProps {
  tone?: keyof typeof pillTones
  children: ReactNode
  title?: string
  className?: string
}

// Etiqueta pequeña de estado. También se exporta como `Badge`.
export function Pill({ tone = 'gray', children, title, className }: PillProps) {
  return (
    <span
      title={title}
      className={cx(
        'inline-flex items-center gap-1 whitespace-nowrap rounded-full px-2.5 py-0.5 text-xs font-medium',
        pillTones[tone],
        className,
      )}
    >
      {children}
    </span>
  )
}

const spinnerSizes = { sm: 'h-4 w-4', md: 'h-6 w-6', lg: 'h-10 w-10' }

export interface SpinnerProps {
  size?: keyof typeof spinnerSizes
  label?: string
  /** Muestra el texto junto al spinner; si no, sólo lo leen los lectores de pantalla. */
  showLabel?: boolean
  className?: string
}

export function Spinner({ size = 'md', label = 'Cargando…', showLabel = false, className }: SpinnerProps) {
  return (
    <span role="status" className={cx('inline-flex items-center gap-2 text-gray-500', className)}>
      <LoaderCircle aria-hidden="true" className={cx('animate-spin text-orange-500', spinnerSizes[size])} />
      <span className={showLabel ? 'text-sm' : 'sr-only'}>{label}</span>
    </span>
  )
}
