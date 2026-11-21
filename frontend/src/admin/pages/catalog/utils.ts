// Utilidades de las páginas de catálogo: cursos, insignias, compras y niveles.

// Tope de las columnas integer de Postgres.
export const CATALOG_INT_MAX = 2_147_483_647

const numberFormat = new Intl.NumberFormat('es-MX')
const dateTimeFormat = new Intl.DateTimeFormat('es-MX', { dateStyle: 'medium', timeStyle: 'short' })
const dateFormat = new Intl.DateTimeFormat('es-MX', { dateStyle: 'medium' })
const timeFormat = new Intl.DateTimeFormat('es-MX', { timeStyle: 'short' })

export function catalogNumber(n: number): string {
  return numberFormat.format(n)
}

// "1 alumno", "24 alumnos".
export function catalogCount(n: number, one: string, many: string): string {
  return `${numberFormat.format(n)} ${n === 1 ? one : many}`
}

// Fecha y hora en la zona del navegador, p. ej. "21 nov 2026, 10:32 a.m.".
export function catalogDateTime(iso: string | null): string {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '' : dateTimeFormat.format(d)
}

// Fecha y hora por separado, para tablas angostas: ["21 nov 2026", "10:32 a.m."].
export function catalogDateParts(iso: string): [string, string] {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? ['', ''] : [dateFormat.format(d), timeFormat.format(d)]
}

// Para el CSV: "2026-11-21 10:32" en hora local, que Excel ordena bien.
export function catalogStamp(iso: string | null): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

// Igual que normalizeLabel del servidor: sin acentos, minúsculas, "_" y "-" como espacio.
export function catalogNormalize(s: string): string {
  return s
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .replace(/[_-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}

// Igual que slugify del servidor. Dos insignias no pueden compartir slug.
export function catalogSlug(name: string): string {
  return name
    .trim()
    .toLowerCase()
    .replace(/\s+/g, '_')
    .normalize('NFD')
    .replace(/[^a-z0-9_]/g, '')
}

// Primera palabra clave contenida en el nombre (comparando sin acentos ni mayúsculas), o null.
export function catalogKeywordHit(name: string, keywords: readonly string[]): string | null {
  const label = catalogNormalize(name)
  if (!label) return null
  return (
    keywords.find((k) => {
      const n = catalogNormalize(k)
      return n !== '' && label.includes(n)
    }) ?? null
  )
}

// Mueve un elemento de `from` a `to` sin tocar el arreglo original.
export function catalogMove<T>(list: readonly T[], from: number, to: number): T[] {
  const next = [...list]
  const [item] = next.splice(from, 1)
  next.splice(to, 0, item)
  return next
}

// Mismos elementos en el mismo orden.
export function catalogSameList<T>(a: readonly T[], b: readonly T[]): boolean {
  return a.length === b.length && a.every((v, i) => v === b[i])
}

// Mismos ids sin importar el orden.
export function catalogSameIds(a: Iterable<number>, b: Iterable<number>): boolean {
  const sa = new Set(a)
  const sb = new Set(b)
  return sa.size === sb.size && [...sa].every((id) => sb.has(id))
}

// Copia del conjunto con `id` agregado o quitado (para estados con Set).
export function catalogWith(set: ReadonlySet<number>, id: number, on: boolean): Set<number> {
  const next = new Set(set)
  if (on) next.add(id)
  else next.delete(id)
  return next
}
