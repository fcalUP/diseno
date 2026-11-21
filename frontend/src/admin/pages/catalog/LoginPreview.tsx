import { ImageOff } from 'lucide-react'

export interface LoginPreviewProps {
  name: string
  sideImageUrl: string
  logoUrl: string
}

// Maqueta chica del login partido: imagen a la izquierda, logo y formulario a la derecha.
export function LoginPreview({ name, sideImageUrl, logoUrl }: LoginPreviewProps) {
  return (
    <figure>
      <div
        aria-hidden="true"
        className="flex h-48 overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm"
      >
        <div
          className="relative w-1/2 bg-gray-100 bg-cover bg-center"
          style={sideImageUrl ? { backgroundImage: `url(${JSON.stringify(sideImageUrl)})` } : undefined}
        >
          {!sideImageUrl && (
            <span className="absolute inset-0 flex flex-col items-center justify-center gap-1 text-xs text-gray-500">
              <ImageOff className="h-5 w-5" />
              Sin imagen lateral
            </span>
          )}
        </div>
        <div className="flex w-1/2 min-w-0 flex-col items-center justify-center gap-1.5 px-4 text-center">
          {logoUrl ? (
            <img src={logoUrl} alt="" className="h-10 w-10 object-contain" />
          ) : (
            <span className="flex h-10 w-10 items-center justify-center rounded-full bg-gray-100 text-gray-400">
              <ImageOff className="h-4 w-4" />
            </span>
          )}
          <p className="line-clamp-2 break-words text-xs font-bold leading-tight text-gray-800">
            {name || 'Nombre del curso'}
          </p>
          <span className="h-3 w-full rounded border border-gray-300" />
          <span className="h-3 w-full rounded border border-gray-300" />
          <span className="h-3 w-full rounded border border-gray-300" />
          <span className="flex h-4 w-full items-center justify-center rounded-full bg-orange-500 text-[8px] font-semibold text-white">
            Log in
          </span>
        </div>
      </div>
      <figcaption className="mt-1 text-xs text-gray-500">
        Así se ve el login en computadora: la imagen lateral ocupa la mitad izquierda (mejor si es vertical) y el logo
        va arriba del nombre. En el celular sólo aparece el logo.
      </figcaption>
    </figure>
  )
}
