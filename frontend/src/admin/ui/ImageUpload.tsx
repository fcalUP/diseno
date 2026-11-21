import { useId, useRef, useState, type DragEvent } from 'react'
import { CloudUpload, LoaderCircle, Trash } from 'lucide-react'
import { errorMessage, uploadImage } from '../../lib/api'
import { Button } from './Button'
import { cx, focusRing } from './styles'

export interface ImageUploadProps {
  label: string
  /** Url guardada (/media/...) o '' si no hay imagen. */
  value: string
  onChange: (url: string) => void
  hint?: string
  disabled?: boolean
  className?: string
  /** Tamaño de la vista previa. */
  previewClassName?: string
}

// Sube la imagen directo a S3 (presigned PUT) y entrega la url con onChange.
export function ImageUpload({
  label,
  value,
  onChange,
  hint = 'PNG, JPG, GIF o WEBP, máximo 20 MB.',
  disabled = false,
  className,
  previewClassName = 'h-40 w-full',
}: ImageUploadProps) {
  const id = useId()
  const inputRef = useRef<HTMLInputElement>(null)
  const [uploading, setUploading] = useState(false)
  const [dragging, setDragging] = useState(false)
  const [error, setError] = useState('')
  const locked = disabled || uploading

  const upload = async (file: File | undefined) => {
    if (!file || locked) return
    setError('')
    setUploading(true)
    try {
      onChange(await uploadImage(file))
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setUploading(false)
    }
  }

  const onDrop = (e: DragEvent<HTMLButtonElement>) => {
    e.preventDefault()
    setDragging(false)
    void upload(e.dataTransfer.files[0])
  }

  return (
    <div className={className}>
      <span id={`${id}-label`} className="mb-1 block text-sm font-medium text-gray-700">
        {label}
      </span>
      <button
        type="button"
        disabled={locked}
        aria-labelledby={`${id}-label ${id}-action`}
        aria-describedby={`${id}-hint`}
        onClick={() => inputRef.current?.click()}
        onDragOver={(e) => {
          e.preventDefault()
          if (!locked) setDragging(true)
        }}
        onDragLeave={(e) => {
          if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setDragging(false)
        }}
        onDrop={onDrop}
        className={cx(
          'relative flex items-center justify-center overflow-hidden rounded-xl border-2 border-dashed transition-colors disabled:cursor-not-allowed',
          focusRing,
          previewClassName,
          dragging ? 'border-orange-400 bg-orange-50' : 'border-gray-300 bg-gray-50 hover:border-orange-300',
        )}
      >
        {value ? (
          <img src={value} alt="" className="h-full w-full object-contain" />
        ) : (
          <span className="flex flex-col items-center gap-1 px-4 text-center text-sm text-gray-500">
            <CloudUpload aria-hidden="true" className="h-7 w-7 text-gray-400" />
            <span>
              Arrastra una imagen o <span className="font-semibold text-orange-600">elige un archivo</span>
            </span>
          </span>
        )}
        <span id={`${id}-action`} className="sr-only">
          {value ? 'Cambiar imagen' : 'Subir imagen'}
        </span>
        {uploading && (
          <span className="absolute inset-0 flex items-center justify-center gap-2 bg-white/80 text-sm font-medium text-gray-700">
            <LoaderCircle aria-hidden="true" className="h-5 w-5 animate-spin text-orange-500" />
            Subiendo…
          </span>
        )}
      </button>
      <input
        ref={inputRef}
        type="file"
        accept="image/png,image/jpeg,image/gif,image/webp"
        className="sr-only"
        tabIndex={-1}
        aria-hidden="true"
        onChange={(e) => {
          void upload(e.target.files?.[0])
          // Permite volver a elegir el mismo archivo.
          e.target.value = ''
        }}
      />
      <div className="mt-1 flex items-center justify-between gap-2">
        <p id={`${id}-hint`} className="text-xs text-gray-500">
          {hint}
        </p>
        {value && !locked && (
          <Button variant="ghost" size="sm" icon={Trash} onClick={() => onChange('')}>
            Quitar
          </Button>
        )}
      </div>
      {error && (
        <p role="alert" className="mt-1 text-xs text-red-600">
          {error}
        </p>
      )}
    </div>
  )
}
