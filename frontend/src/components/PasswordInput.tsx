import { useId, useState, type InputHTMLAttributes } from 'react'
import { Eye, EyeOff } from 'lucide-react'

type PasswordInputProps = Omit<InputHTMLAttributes<HTMLInputElement>, 'type'> & {
  wrapperClassName?: string
  showLabel?: string
  hideLabel?: string
}

export default function PasswordInput({
  id,
  className = '',
  wrapperClassName = '',
  showLabel = 'Mostrar contraseña',
  hideLabel = 'Ocultar contraseña',
  disabled,
  ...props
}: PasswordInputProps) {
  const autoId = useId()
  const inputId = id ?? autoId
  const [visible, setVisible] = useState(false)

  return (
    <div className={`relative ${wrapperClassName}`}>
      <input {...props} id={inputId} type={visible ? 'text' : 'password'} disabled={disabled} className={`${className} pr-12`} />
      <button
        type="button"
        disabled={disabled}
        aria-label={visible ? hideLabel : showLabel}
        aria-pressed={visible}
        aria-controls={inputId}
        onClick={() => setVisible((current) => !current)}
        className="absolute right-2 top-1/2 -translate-y-1/2 rounded-full p-2 text-gray-500 hover:text-purple-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-purple-500 disabled:opacity-50"
      >
        {visible ? <EyeOff aria-hidden="true" className="h-5 w-5" /> : <Eye aria-hidden="true" className="h-5 w-5" />}
      </button>
    </div>
  )
}
