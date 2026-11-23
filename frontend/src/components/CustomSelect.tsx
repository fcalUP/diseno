import {
  Children,
  isValidElement,
  useEffect,
  useId,
  useRef,
  useState,
  type ButtonHTMLAttributes,
  type KeyboardEvent,
  type ReactNode,
} from 'react'
import { createPortal } from 'react-dom'
import { Check, ChevronDown } from 'lucide-react'

interface Choice {
  value: string
  label: string
  disabled: boolean
  group?: string
}

interface ValueChange {
  target: { value: string }
  currentTarget: { value: string }
}

export interface CustomSelectProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'value' | 'defaultValue' | 'onChange' | 'children' | 'type'> {
  value?: string | number
  defaultValue?: string | number
  onChange?: (event: ValueChange) => void
  children: ReactNode
  placeholder?: string
  required?: boolean
}

function textContent(children: ReactNode): string {
  return Children.toArray(children).map((child) => {
    if (typeof child === 'string' || typeof child === 'number') return String(child)
    return isValidElement<{ children?: ReactNode }>(child) ? textContent(child.props.children) : ''
  }).join('')
}

function readChoices(children: ReactNode, group?: string, groupDisabled = false): Choice[] {
  return Children.toArray(children).flatMap((child) => {
    if (!isValidElement<{ value?: string | number; children?: ReactNode; disabled?: boolean; label?: string }>(child)) return []
    if (child.type === 'option') {
      const label = child.props.label ?? textContent(child.props.children)
      return [{ value: String(child.props.value ?? label), label, disabled: groupDisabled || !!child.props.disabled, group }]
    }
    return readChoices(child.props.children, child.type === 'optgroup' ? child.props.label : group,
      groupDisabled || !!child.props.disabled)
  })
}

// El foco se queda en el botón aunque la lista se monte fuera de un diálogo con
// scroll. El descendiente activo comunica la navegación al lector de pantalla.
export function CustomSelect({
  value, defaultValue, onChange, children, placeholder = 'Seleccionar…', required,
  disabled, id, className, name, form, onKeyDown, onBlur, onClick, ...rest
}: CustomSelectProps) {
  const autoId = useId()
  const controlId = id ?? autoId
  const listId = `${controlId}-list`
  const triggerRef = useRef<HTMLButtonElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const typeahead = useRef({ text: '', at: 0 })
  const [localValue, setLocalValue] = useState(String(defaultValue ?? ''))
  const selectedValue = String(value ?? localValue)
  const choices = readChoices(children)
  const selectedIndex = choices.findIndex((choice) => choice.value === selectedValue)
  const [active, setActive] = useState(-1)
  const [position, setPosition] = useState<{ left: number; top: number; width: number; maxHeight: number } | null>(null)
  const open = !!position && !disabled
  const enabled = choices.map((choice, index) => choice.disabled ? -1 : index).filter((index) => index >= 0)

  const place = () => {
    const rect = triggerRef.current?.getBoundingClientRect()
    if (!rect) return
    const viewport = window.visualViewport
    const width = viewport?.width ?? window.innerWidth
    const height = viewport?.height ?? window.innerHeight
    const offsetTop = viewport?.offsetTop ?? 0
    const offsetLeft = viewport?.offsetLeft ?? 0
    const below = height + offsetTop - rect.bottom - 12
    const above = rect.top - offsetTop - 12
    const desiredHeight = Math.min(288, choices.length * 40 + 16)
    const upward = below < desiredHeight && above > below
    const maxHeight = Math.max(40, Math.min(desiredHeight, upward ? above : below))
    const menuWidth = Math.min(Math.max(rect.width, 180), width - 16)
    setPosition({
      left: Math.max(offsetLeft + 8, Math.min(rect.left, width + offsetLeft - menuWidth - 8)),
      top: upward ? Math.max(offsetTop + 8, rect.top - maxHeight - 6) : rect.bottom + 6,
      width: menuWidth,
      maxHeight,
    })
  }

  const show = (index = selectedIndex) => {
    if (disabled || !enabled.length) return
    setActive(enabled.includes(index) ? index : enabled[0])
    place()
  }

  const choose = (index: number) => {
    const choice = choices[index]
    if (!choice || choice.disabled) return
    setLocalValue(choice.value)
    if (choice.value !== selectedValue) onChange?.({ target: { value: choice.value }, currentTarget: { value: choice.value } })
    setPosition(null)
    triggerRef.current?.focus({ preventScroll: true })
  }

  useEffect(() => {
    if (!open) return
    const outside = (event: PointerEvent) => {
      if (event.target instanceof Node && !triggerRef.current?.contains(event.target) && !listRef.current?.contains(event.target)) setPosition(null)
    }
    const dismiss = (event: Event) => {
      if (event.target instanceof Node && listRef.current?.contains(event.target)) return
      setPosition(null)
    }
    document.addEventListener('pointerdown', outside, true)
    window.addEventListener('resize', dismiss)
    window.addEventListener('scroll', dismiss, true)
    window.visualViewport?.addEventListener('resize', dismiss)
    return () => {
      document.removeEventListener('pointerdown', outside, true)
      window.removeEventListener('resize', dismiss)
      window.removeEventListener('scroll', dismiss, true)
      window.visualViewport?.removeEventListener('resize', dismiss)
    }
  }, [open])

  useEffect(() => {
    if (open) document.getElementById(`${listId}-${active}`)?.scrollIntoView({ block: 'nearest' })
  }, [active, listId, open])

  const handleKey = (event: KeyboardEvent<HTMLButtonElement>) => {
    onKeyDown?.(event)
    if (event.defaultPrevented || disabled) return
    if (event.key === 'Escape' && open) {
      event.preventDefault()
      event.stopPropagation()
      setPosition(null)
    } else if (event.key === 'Tab') {
      setPosition(null)
    } else if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
      event.preventDefault()
      if (!open) {
        show(event.key === 'End' || event.key === 'ArrowUp' ? enabled.at(-1) : selectedIndex)
      } else {
        const current = enabled.indexOf(active)
        const next = event.key === 'Home' ? 0 : event.key === 'End' ? enabled.length - 1
          : (current + (event.key === 'ArrowUp' ? -1 : 1) + enabled.length) % enabled.length
        setActive(enabled[next])
      }
    } else if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      if (open) choose(active)
      else show()
    } else if (event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
      event.preventDefault()
      const now = Date.now()
      const text = (now - typeahead.current.at < 700 ? typeahead.current.text : '') + event.key.toLocaleLowerCase()
      typeahead.current = { text, at: now }
      const start = open ? active : selectedIndex
      const ordered = [...enabled.filter((index) => index > start), ...enabled.filter((index) => index <= start)]
      const next = ordered.find((index) => choices[index].label.toLocaleLowerCase().startsWith(text))
      if (next !== undefined) {
        if (!open) show(next)
        else setActive(next)
      }
    }
  }

  return (
    <>
      <button
        {...rest}
        ref={triggerRef}
        id={controlId}
        type="button"
        role="combobox"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? listId : undefined}
        aria-activedescendant={open && active >= 0 ? `${listId}-${active}` : undefined}
        aria-required={required || undefined}
        disabled={disabled}
        form={form}
        className={`custom-select-trigger ${className ?? ''}`}
        onKeyDown={handleKey}
        onBlur={(event) => {
          setPosition(null)
          onBlur?.(event)
        }}
        onClick={(event) => {
          onClick?.(event)
          if (!event.defaultPrevented) {
            if (open) setPosition(null)
            else show()
          }
        }}
      >
        <span className="min-w-0 flex-1 truncate text-left">{choices[selectedIndex]?.label ?? placeholder}</span>
        <ChevronDown aria-hidden="true" className={`h-4 w-4 shrink-0 text-gray-400 transition-transform ${open ? 'rotate-180' : ''}`} />
      </button>
      {name && <input type="hidden" name={name} form={form} value={selectedValue} disabled={disabled} />}
      {open && position && createPortal(
        <div
          ref={listRef}
          id={listId}
          role="listbox"
          aria-label={rest['aria-label']}
          aria-labelledby={rest['aria-label'] ? undefined : controlId}
          className="fixed z-[100] overflow-y-auto overscroll-contain rounded-xl border border-gray-200 bg-white p-1.5 text-sm text-gray-900 shadow-xl"
          style={position}
          onMouseDown={(event) => event.preventDefault()}
        >
          {choices.map((choice, index) => (
            <div key={`${choice.value}-${index}`}>
              {choice.group && choice.group !== choices[index - 1]?.group && <div className="px-3 py-2 text-xs font-semibold text-gray-500">{choice.group}</div>}
              <div
                id={`${listId}-${index}`}
                role="option"
                aria-selected={choice.value === selectedValue}
                aria-disabled={choice.disabled || undefined}
                tabIndex={-1}
                onPointerMove={() => { if (!choice.disabled) setActive(index) }}
                onClick={() => choose(index)}
                className={`flex min-h-10 cursor-pointer items-center gap-3 rounded-lg px-3 py-2 ${choice.disabled ? 'cursor-not-allowed text-gray-400' : index === active ? 'bg-blue-50 text-blue-800' : 'hover:bg-gray-50'} ${choice.value === selectedValue ? 'font-medium' : ''}`}
              >
                <span className="min-w-0 flex-1 break-words">{choice.label}</span>
                {choice.value === selectedValue && <Check aria-hidden="true" className="h-4 w-4 shrink-0 text-blue-600" />}
              </div>
            </div>
          ))}
        </div>, document.body,
      )}
    </>
  )
}
