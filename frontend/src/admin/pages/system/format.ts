// Fechas y números en español de México para las páginas del panel.

const dateTime = new Intl.DateTimeFormat('es-MX', { dateStyle: 'medium', timeStyle: 'short' })
const relative = new Intl.RelativeTimeFormat('es-MX', { numeric: 'auto' })
const numbers = new Intl.NumberFormat('es-MX')

// "21 nov 2026, 10:03 a.m."; si la fecha no se entiende, la devuelve tal cual.
export function formatDateTime(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : dateTime.format(d)
}

const UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 365 * 86400],
  ['month', 30 * 86400],
  ['week', 7 * 86400],
  ['day', 86400],
  ['hour', 3600],
  ['minute', 60],
]

// "hace 5 minutos", "ayer"... `now` viene de useNow() para no leer el reloj al pintar.
export function formatRelative(iso: string, now: number): string {
  const t = new Date(iso).getTime()
  if (Number.isNaN(t)) return ''
  const secs = Math.round((t - now) / 1000)
  for (const [unit, size] of UNITS) {
    if (Math.abs(secs) >= size) return relative.format(Math.trunc(secs / size), unit)
  }
  return 'hace un momento'
}

export function formatNumber(n: number): string {
  return numbers.format(n)
}

// plural(3, 'alumno', 'alumnos') → "3 alumnos"
export function plural(n: number, one: string, many: string): string {
  return `${formatNumber(n)} ${n === 1 ? one : many}`
}
