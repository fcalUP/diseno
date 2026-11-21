import { useId, useRef, useState, type ClipboardEvent, type KeyboardEvent, type ReactNode } from 'react'
import { X } from 'lucide-react'
import { cx, focusRing } from '../../ui'

export interface ChipsInputProps {
  label: string
  values: string[]
  onChange: (values: string[]) => void
  /** Cómo se llama cada valor en el botón de quitar ("alias", "palabra"). */
  itemName: string
  hint?: ReactNode
  placeholder?: string
  /** Ajusta cada valor antes de agregarlo (p. ej. quitar acentos). */
  normalize?: (value: string) => string
  maxItems?: number
  maxLength?: number
  disabled?: boolean
}

const SEPARATORS = /[,;\n\t]/

// Lista de etiquetas. Enter, coma o salir del campo agrega lo escrito; también se pueden
// pegar varias separadas por comas. Retroceso con el campo vacío quita la última.
export function ChipsInput({
  label,
  values,
  onChange,
  itemName,
  hint,
  placeholder,
  normalize,
  maxItems = 50,
  maxLength = 120,
  disabled = false,
}: ChipsInputProps) {
  const id = useId()
  const inputRef = useRef<HTMLInputElement>(null)
  const [text, setText] = useState('')
  const [error, setError] = useState('')
  const [announce, setAnnounce] = useState('')

  const add = (raw: string) => {
    const fresh: string[] = []
    for (const part of raw.split(SEPARATORS)) {
      const trimmed = part.trim()
      const value = normalize ? normalize(trimmed) : trimmed
      if (value && !values.includes(value) && !fresh.includes(value)) fresh.push(value)
    }
    if (fresh.some((v) => v.length > maxLength)) {
      setError(`Cada ${itemName} debe tener ${maxLength} caracteres o menos.`)
      return
    }
    if (values.length + fresh.length > maxItems) {
      setError(`Caben ${maxItems} como máximo.`)
      return
    }
    setError('')
    setText('')
    if (fresh.length === 0) return
    onChange([...values, ...fresh])
    setAnnounce(fresh.length === 1 ? `Se agregó «${fresh[0]}».` : `Se agregaron ${fresh.length}.`)
  }

  const remove = (value: string) => {
    onChange(values.filter((v) => v !== value))
    setAnnounce(`Se quitó «${value}».`)
    // El botón desaparece; el foco regresa al campo.
    inputRef.current?.focus()
  }

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.nativeEvent.isComposing) return
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault()
      add(text)
    } else if (e.key === 'Backspace' && text === '' && values.length > 0) {
      e.preventDefault()
      remove(values[values.length - 1])
    }
  }

  const onPaste = (e: ClipboardEvent<HTMLInputElement>) => {
    const pasted = e.clipboardData.getData('text')
    if (!SEPARATORS.test(pasted)) return
    e.preventDefault()
    add(text + pasted)
  }

  const describedBy = error ? `${id}-error` : hint ? `${id}-hint` : undefined

  return (
    <div>
      <label htmlFor={id} className="mb-1 block text-sm font-medium text-gray-700">
        {label}
      </label>
      <div
        className={cx(
          'flex min-h-[2.625rem] flex-wrap items-center gap-1.5 rounded-xl border px-2 py-1.5 shadow-sm transition-colors focus-within:ring-2',
          error
            ? 'border-red-400 focus-within:border-red-500 focus-within:ring-red-500/30'
            : 'border-gray-300 focus-within:border-blue-500 focus-within:ring-blue-500/30',
          disabled ? 'bg-gray-100' : 'bg-white',
        )}
        onClick={(e) => {
          if (e.target === e.currentTarget) inputRef.current?.focus()
        }}
      >
        {values.map((v) => (
          <span
            key={v}
            className="inline-flex max-w-full items-center gap-1 rounded-full bg-purple-50 py-0.5 pl-2.5 pr-1 text-xs font-medium text-purple-700"
          >
            <span className="truncate">{v}</span>
            <button
              type="button"
              disabled={disabled}
              onClick={() => remove(v)}
              aria-label={`Quitar ${itemName} «${v}»`}
              title={`Quitar ${itemName}`}
              className={cx('shrink-0 rounded-full p-0.5 hover:bg-purple-100 disabled:opacity-50', focusRing)}
            >
              <X aria-hidden="true" className="h-3.5 w-3.5" />
            </button>
          </span>
        ))}
        <input
          ref={inputRef}
          id={id}
          value={text}
          disabled={disabled}
          maxLength={maxLength}
          placeholder={values.length === 0 ? placeholder : undefined}
          onChange={(e) => {
            setText(e.target.value)
            if (error) setError('')
          }}
          onKeyDown={onKeyDown}
          onPaste={onPaste}
          onBlur={() => {
            if (text.trim()) add(text)
          }}
          aria-invalid={error ? true : undefined}
          aria-describedby={describedBy}
          className="min-w-[8rem] flex-1 border-0 bg-transparent px-1 py-0.5 text-sm text-gray-900 placeholder:text-gray-400 focus:outline-none focus:ring-0 disabled:cursor-not-allowed"
        />
      </div>
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
      <span className="sr-only" aria-live="polite">
        {announce}
      </span>
    </div>
  )
}
