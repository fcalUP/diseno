import type { ButtonHTMLAttributes } from 'react'
import { LoaderCircle, type LucideIcon } from 'lucide-react'
import { cx, focusRing } from './styles'

const variants = {
  primary: 'bg-orange-500 text-white shadow-sm hover:bg-orange-600',
  secondary: 'bg-purple-600 text-white shadow-sm hover:bg-purple-700',
  ghost: 'bg-transparent text-gray-700 hover:bg-gray-100',
  danger: 'bg-red-600 text-white shadow-sm hover:bg-red-700',
}

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: keyof typeof variants
  size?: 'sm' | 'md'
  /** Muestra un spinner y deshabilita el botón. */
  loading?: boolean
  icon?: LucideIcon
}

export function Button({
  variant = 'primary',
  size = 'md',
  loading = false,
  icon: Icon,
  type = 'button',
  disabled,
  className,
  children,
  ...rest
}: ButtonProps) {
  return (
    <button
      type={type}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      className={cx(
        'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-full text-sm font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-60',
        focusRing,
        size === 'sm' ? 'px-3 py-1.5' : 'px-4 py-2',
        variants[variant],
        className,
      )}
      {...rest}
    >
      {loading ? (
        <LoaderCircle aria-hidden="true" className="h-4 w-4 animate-spin" />
      ) : (
        Icon && <Icon aria-hidden="true" className="h-4 w-4" />
      )}
      {children}
    </button>
  )
}

const iconVariants = {
  ghost: 'text-gray-500 hover:bg-gray-100 hover:text-gray-800',
  primary: 'bg-orange-500 text-white hover:bg-orange-600',
  danger: 'text-red-600 hover:bg-red-50',
}

export interface IconButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'children'> {
  icon: LucideIcon
  /** Texto para lectores de pantalla y tooltip; es obligatorio porque no hay texto visible. */
  label: string
  variant?: keyof typeof iconVariants
  size?: 'sm' | 'md'
}

export function IconButton({
  icon: Icon,
  label,
  variant = 'ghost',
  size = 'md',
  type = 'button',
  className,
  ...rest
}: IconButtonProps) {
  return (
    <button
      type={type}
      aria-label={label}
      title={label}
      className={cx(
        'inline-flex shrink-0 items-center justify-center rounded-full transition-colors disabled:cursor-not-allowed disabled:opacity-50',
        focusRing,
        size === 'sm' ? 'h-8 w-8' : 'h-10 w-10',
        iconVariants[variant],
        className,
      )}
      {...rest}
    >
      <Icon aria-hidden="true" className={size === 'sm' ? 'h-4 w-4' : 'h-5 w-5'} />
    </button>
  )
}
