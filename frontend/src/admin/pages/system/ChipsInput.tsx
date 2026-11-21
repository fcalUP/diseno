import { useId, useRef, useState } from 'react'
import { X } from 'lucide-react'
import { cx, focusRing } from '../../ui'

interface ChipsInputProps {
  label: string
  values: string[]
  onChange: (values: string[]) => void
  /** Devuelve un mensaje cuando el valor no sirve; ese valor se queda en el campo. */
  validate?: (value: string) => string | undefined
  placeholder?: string
  hint?: string
  disabled?: boolean
}

// Lista de valores como fichas (p. ej. correos en copia). Enter, coma, punto y coma o
// espacio agregan lo escrito; Retroceso con el campo vacío quita la última ficha.
export function ChipsInput({ label, values, onChange, validate, placeholder, hint, disabled }: ChipsInputProps) {
  const id = useId()
  const [text, setText] = useState('')
  const [error, setError] = useState('')
  const [announce, setAnnounce] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  const commit = (raw: string) => {
    const parts = raw
      .split(/[\s,;]+/)
      .map((p) => p.trim())
      .filter(Boolean)
    const next = [...values]
    const bad: string[] = []
    let firstError = ''
    for (const p of parts) {
      const err = validate?.(p)
      if (err) {
        bad.push(p)
        firstError ||= err
      } else if (!next.some((v) => v.toLowerCase() === p.toLowerCase())) {
        next.push(p)
      }
    }
    if (next.length !== values.length) {
      onChange(next)
      setAnnounce(`Se agregó ${next.slice(values.length).join(', ')}`)
    }
    setText(bad.join(', '))
    setError(firstError)
  }

  const remove = (index: number) => {
    setAnnounce(`Se quitó ${values[index]}`)
    onChange(values.filter((_, i) => i !== index))
  }

  const describedBy = error ? `${id}-error` : hint ? `${id}-hint` : undefined

  return (
    <div>
      <label htmlFor={id} className="mb-1 block text-sm font-medium text-gray-700">
        {label}
      </label>
      <div
        className={cx(
          'flex flex-wrap items-center gap-1.5 rounded-xl border bg-white px-2 py-1.5 shadow-sm transition-colors focus-within:border-blue-500 focus-within:ring-2 focus-within:ring-blue-500/30',
          error ? 'border-red-400' : 'border-gray-300',
          disabled && 'bg-gray-100',
        )}
      >
        {values.length > 0 && (
          <ul aria-label={`Lista de ${label}`} className="flex min-w-0 max-w-full flex-wrap gap-1.5">
            {values.map((v, i) => (
              <li
                key={`${i}:${v}`}
                className="inline-flex min-w-0 max-w-full items-center gap-1 rounded-full bg-purple-50 py-0.5 pl-2.5 pr-1 text-sm text-purple-700"
              >
                <span className="min-w-0 break-all">{v}</span>
                <button
                  type="button"
                  disabled={disabled}
                  onClick={() => {
                    remove(i)
                    // El botón desaparece con la ficha; el foco pasa al campo de texto.
                    inputRef.current?.focus()
                  }}
                  aria-label={`Quitar ${v}`}
                  className={cx('shrink-0 rounded-full p-0.5 hover:bg-purple-100 disabled:opacity-50', focusRing)}
                >
                  <X aria-hidden="true" className="h-3.5 w-3.5" />
                </button>
              </li>
            ))}
          </ul>
        )}
        <input
          ref={inputRef}
          id={id}
          type="text"
          inputMode="email"
          autoComplete="off"
          spellCheck={false}
          disabled={disabled}
          value={text}
          placeholder={values.length ? undefined : placeholder}
          aria-invalid={error ? true : undefined}
          aria-describedby={describedBy}
          onChange={(e) => {
            const t = e.target.value
            if (/[\s,;]/.test(t)) commit(t)
            else {
              setText(t)
              setError('')
            }
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && text.trim()) {
              // Sin esto Enter mandaría el formulario de la sección.
              e.preventDefault()
              commit(text)
            } else if (e.key === 'Backspace' && !text && values.length > 0) {
              remove(values.length - 1)
            }
          }}
          onBlur={() => {
            if (text.trim()) commit(text)
          }}
          className="min-w-[8rem] flex-1 border-0 bg-transparent px-1 py-1 text-sm text-gray-900 placeholder:text-gray-400 focus:outline-none disabled:cursor-not-allowed"
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
