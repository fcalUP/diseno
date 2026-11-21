// Hoja de Google y cuenta de servicio: lo comparten Importar y Ajustes.

// Acepta el ID o el enlace completo (https://docs.google.com/spreadsheets/d/<ID>/edit...).
export function parseSpreadsheetId(input: string): string {
  const text = input.trim()
  return /\/spreadsheets\/d\/([A-Za-z0-9_-]+)/.exec(text)?.[1] ?? text
}

export function spreadsheetIdError(id: string): string | undefined {
  if (!id) return 'Escribe el ID o pega el enlace de la hoja.'
  if (!/^[A-Za-z0-9_-]{10,200}$/.test(id)) {
    return 'El ID sólo lleva letras, números, guiones y guiones bajos (o pega el enlace de la hoja).'
  }
  return undefined
}

export function sheetUrl(id: string): string {
  return `https://docs.google.com/spreadsheets/d/${encodeURIComponent(id)}/edit`
}

export interface ServiceAccount {
  /** JSON compacto, listo para mandar al servidor. */
  json: string
  clientEmail: string
}

// Un JSON de cuenta de servicio pesa unos 2.4 KB; esto deja margen dentro del límite de 8 KB
// por petición aun con el escape de comillas y saltos de línea.
const MAX_FILE_BYTES = 16 * 1024
const MAX_JSON_CHARS = 6000

function readText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '')
    reader.onerror = () => reject(new Error('No se pudo leer el archivo.'))
    reader.readAsText(file)
  })
}

// Lee el .json de la cuenta de servicio y revisa que traiga client_email y private_key.
// Se queda en memoria: nunca se guarda en el navegador. Lanza Error con un mensaje listo
// para mostrar.
export async function readServiceAccount(file: File): Promise<ServiceAccount> {
  if (file.size > MAX_FILE_BYTES) throw new Error('El archivo es demasiado grande para ser una cuenta de servicio.')
  const text = await readText(file)
  let data: unknown
  try {
    data = JSON.parse(text)
  } catch {
    throw new Error('El archivo no es un JSON válido.')
  }
  if (!data || typeof data !== 'object' || Array.isArray(data)) {
    throw new Error('El archivo no es una cuenta de servicio de Google.')
  }
  const sa = data as Record<string, unknown>
  if (sa.type !== undefined && sa.type !== 'service_account') {
    throw new Error('El archivo no es de una cuenta de servicio (type debe ser "service_account").')
  }
  if (typeof sa.client_email !== 'string' || !sa.client_email.includes('@')) {
    throw new Error('Al archivo le falta client_email.')
  }
  if (typeof sa.private_key !== 'string' || !sa.private_key.includes('PRIVATE KEY')) {
    throw new Error('Al archivo le falta private_key.')
  }
  const json = JSON.stringify(sa)
  if (json.length > MAX_JSON_CHARS) throw new Error('El archivo es demasiado grande para enviarlo (máximo 8 KB).')
  return { json, clientEmail: sa.client_email }
}
