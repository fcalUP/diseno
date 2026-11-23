import {
  useId,
  useState,
  type InputHTMLAttributes,
  type ReactNode,
  type TextareaHTMLAttributes,
} from 'react'
import { Search, X } from 'lucide-react'
import { CustomSelect, type CustomSelectProps } from '../../components/CustomSelect'
import PasswordInput from '../../components/PasswordInput'
import { cx, focusRing, inputClass, inputErrorClass } from './styles'

interface FieldProps {
  label: string
  /** Oculta la etiqueta a la vista; los lectores de pantalla la siguen leyendo. */
  hideLabel?: boolean
  hint?: ReactNode
  error?: string
  /** Clases del contenedor (etiqueta + control + ayuda). */
  className?: string
}

// Etiqueta, ayuda y error que comparten todos los campos.
function Field({ id, label, hideLabel, hint, error, className, children }: FieldProps & { id: string; children: ReactNode }) {
  return (
    <div className={className}>
      <label htmlFor={id} className={hideLabel ? 'sr-only' : 'mb-1 block text-sm font-medium text-gray-700'}>
        {label}
      </label>
      {children}
      {error ? (
        <p id={`${id}-error`} className="mt-1 text-xs text-red-600">
          {error}
        </p>
      ) : (
        hint && (
          <p id={`${id}-hint`} className="mt-1 text-xs text-gray-500">
            {hint}
          </p>
        )
      )}
    </div>
  )
}

function a11y(id: string, hint: ReactNode, error?: string) {
  return {
    'aria-invalid': error ? true : undefined,
    'aria-describedby': error ? `${id}-error` : hint ? `${id}-hint` : undefined,
  }
}

export type TextFieldProps = FieldProps & Omit<InputHTMLAttributes<HTMLInputElement>, 'size' | 'className'>

export function TextField({ label, hideLabel, hint, error, className, id, ...rest }: TextFieldProps) {
  const autoId = useId()
  const fieldId = id ?? autoId
  const { type, ...inputProps } = rest
  return (
    <Field id={fieldId} label={label} hideLabel={hideLabel} hint={hint} error={error} className={className}>
      {type === 'password' ? (
        <PasswordInput id={fieldId} className={cx(inputClass, error && inputErrorClass)} {...a11y(fieldId, hint, error)} {...inputProps} />
      ) : (
        <input id={fieldId} type={type} className={cx(inputClass, error && inputErrorClass)} {...a11y(fieldId, hint, error)} {...inputProps} />
      )}
    </Field>
  )
}

export type NumberFieldProps = FieldProps &
  Omit<InputHTMLAttributes<HTMLInputElement>, 'size' | 'className' | 'type' | 'value' | 'onChange'> & {
    value: number
    onChange: (value: number) => void
  }

export function NumberField({ label, hideLabel, hint, error, className, id, value, onChange, onBlur, ...rest }: NumberFieldProps) {
  const autoId = useId()
  const fieldId = id ?? autoId
  // Texto mientras se escribe, para poder vaciar el campo sin que brinque a 0.
  const [draft, setDraft] = useState<string | null>(null)
  return (
    <Field id={fieldId} label={label} hideLabel={hideLabel} hint={hint} error={error} className={className}>
      <input
        {...rest}
        id={fieldId}
        type="number"
        value={draft ?? String(value)}
        onChange={(e) => {
          const text = e.target.value
          setDraft(text)
          if (text.trim() !== '' && Number.isFinite(Number(text))) onChange(Number(text))
        }}
        onBlur={(e) => {
          setDraft(null)
          onBlur?.(e)
        }}
        className={cx(inputClass, error && inputErrorClass)}
        {...a11y(fieldId, hint, error)}
      />
    </Field>
  )
}

export type TextAreaProps = FieldProps & Omit<TextareaHTMLAttributes<HTMLTextAreaElement>, 'className'>

export function TextArea({ label, hideLabel, hint, error, className, id, rows = 3, ...rest }: TextAreaProps) {
  const autoId = useId()
  const fieldId = id ?? autoId
  return (
    <Field id={fieldId} label={label} hideLabel={hideLabel} hint={hint} error={error} className={className}>
      <textarea
        id={fieldId}
        rows={rows}
        className={cx(inputClass, 'resize-y', error && inputErrorClass)}
        {...a11y(fieldId, hint, error)}
        {...rest}
      />
    </Field>
  )
}

export interface SelectOption {
  value: string | number
  label: string
  disabled?: boolean
}

export type SelectProps = FieldProps &
  Omit<CustomSelectProps, 'className' | 'value' | 'onChange' | 'children'> & {
    value: string | number
    onChange: (value: string) => void
    options: SelectOption[]
    /** Agrega una primera opción vacía con este texto. */
    placeholder?: string
  }

export function Select({
  label,
  hideLabel,
  hint,
  error,
  className,
  id,
  value,
  onChange,
  options,
  placeholder,
  ...rest
}: SelectProps) {
  const autoId = useId()
  const fieldId = id ?? autoId
  return (
    <Field id={fieldId} label={label} hideLabel={hideLabel} hint={hint} error={error} className={className}>
      <CustomSelect
        {...rest}
        id={fieldId}
        value={String(value)}
        onChange={(e) => onChange(e.target.value)}
        className={cx(inputClass, error && inputErrorClass)}
        {...a11y(fieldId, hint, error)}
      >
        {placeholder !== undefined && <option value="">{placeholder}</option>}
        {options.map((o) => (
          <option key={o.value} value={String(o.value)} disabled={o.disabled}>
            {o.label}
          </option>
        ))}
      </CustomSelect>
    </Field>
  )
}

export interface SearchInputProps {
  value: string
  onChange: (value: string) => void
  placeholder?: string
  /** Etiqueta para lectores de pantalla. */
  label?: string
  className?: string
  id?: string
}

export function SearchInput({ value, onChange, placeholder = 'Buscar…', label = 'Buscar', className, id }: SearchInputProps) {
  const autoId = useId()
  const fieldId = id ?? autoId
  return (
    <div className={cx('relative', className)}>
      <label htmlFor={fieldId} className="sr-only">
        {label}
      </label>
      <Search
        aria-hidden="true"
        className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
      />
      <input
        id={fieldId}
        type="search"
        value={value}
        placeholder={placeholder}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Escape' && value) {
            e.stopPropagation()
            onChange('')
          }
        }}
        className={cx(inputClass, 'pl-9 pr-9 [&::-webkit-search-cancel-button]:hidden')}
      />
      {value && (
        <button
          type="button"
          onClick={() => onChange('')}
          aria-label="Limpiar búsqueda"
          className={cx(
            'absolute right-2 top-1/2 -translate-y-1/2 rounded-full p-1 text-gray-400 hover:text-gray-600',
            focusRing,
          )}
        >
          <X aria-hidden="true" className="h-4 w-4" />
        </button>
      )}
    </div>
  )
}

export interface ToggleProps {
  checked: boolean
  onChange: (checked: boolean) => void
  label: string
  hideLabel?: boolean
  description?: string
  disabled?: boolean
  className?: string
  id?: string
}

// Interruptor accesible: es un botón con role="switch", así que Espacio y Enter lo activan.
export function Toggle({ checked, onChange, label, hideLabel, description, disabled, className, id }: ToggleProps) {
  const autoId = useId()
  const fieldId = id ?? autoId
  return (
    <div className={cx('inline-flex items-center gap-3', className)}>
      <button
        id={fieldId}
        type="button"
        role="switch"
        aria-checked={checked}
        aria-describedby={description ? `${fieldId}-desc` : undefined}
        disabled={disabled}
        onClick={() => onChange(!checked)}
        className={cx(
          'relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors disabled:cursor-not-allowed disabled:opacity-50',
          focusRing,
          checked ? 'bg-green-500' : 'bg-gray-300',
        )}
      >
        <span
          aria-hidden="true"
          className={cx(
            'inline-block h-5 w-5 rounded-full bg-white shadow transition-transform',
            checked ? 'translate-x-5' : 'translate-x-0.5',
          )}
        />
      </button>
      <span className={hideLabel ? 'sr-only' : 'min-w-0'}>
        <label htmlFor={fieldId} className="block cursor-pointer text-sm font-medium text-gray-700">
          {label}
        </label>
        {description && (
          <span id={`${fieldId}-desc`} className="block text-xs text-gray-500">
            {description}
          </span>
        )}
      </span>
    </div>
  )
}
