// Utilidades que comparten Resumen, Alumnos, Generar alumnos y Avatares.
import { api } from '../../../lib/api'
import type { CsvColumn } from '../../../lib/csv'
import type { AdminCourse, AdminStudent, CounterField } from '../../../lib/types'

// Contadores en el orden en que se muestran en todo el panel.
export const COUNTERS: { key: CounterField; label: string }[] = [
  { key: 'tareas', label: 'Tareas' },
  { key: 'monedas', label: 'Monedas' },
  { key: 'asistencias', label: 'Asistencias' },
  { key: 'games', label: 'Juegos' },
  { key: 'exp', label: 'EXP' },
]

export const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

// El alumno ve los comentarios en su perfil. Con este tope el PATCH no llega a los 8 KB del WAF.
export const MAX_COMMENTS = 3000

// Minúsculas y sin acentos, para buscar sin importar cómo se escribió.
export function fold(s: string): string {
  return s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase().trim()
}

export function matchesStudent(s: Pick<AdminStudent, 'code' | 'name' | 'email'>, query: string): boolean {
  const q = fold(query)
  return !q || [s.code, s.name, s.email].some((v) => fold(v).includes(q))
}

export function initials(name: string): string {
  const [a = '', b = ''] = name.trim().split(/\s+/)
  return (a.charAt(0) + b.charAt(0)).toUpperCase() || '?'
}

export function avatarKey(code: string, level: number): string {
  return `${code}:${level}`
}

export function sortCourses(courses: readonly AdminCourse[]): AdminCourse[] {
  return [...courses].sort((a, b) => a.sortOrder - b.sortOrder || a.name.localeCompare(b.name, 'es'))
}

// Un lote por operación masiva: todas las partes comparten el mismo id para deshacerlas juntas.
export function newBatchId(): string {
  return crypto.randomUUID()
}

// El portapapeles solo existe en https o localhost; si falla, quien llama avisa.
export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return false
  }
}

export function pl(n: number, one: string, many: string): string {
  return n === 1 ? one : many
}

export function formatNumber(n: number): string {
  return n.toLocaleString('es-MX')
}

export function alumnos(n: number): string {
  return `${formatNumber(n)} ${pl(n, 'alumno', 'alumnos')}`
}

export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString('es-MX', { dateStyle: 'medium', timeStyle: 'short' })
}

const rtf = new Intl.RelativeTimeFormat('es-MX', { numeric: 'auto' })
const UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 31_536_000],
  ['month', 2_592_000],
  ['week', 604_800],
  ['day', 86_400],
  ['hour', 3_600],
  ['minute', 60],
]

// "hace 5 minutos", "ayer"... `now` viene de fuera para no leer el reloj al pintar.
export function timeAgo(iso: string, now: number): string {
  const secs = (new Date(iso).getTime() - now) / 1000
  if (Number.isNaN(secs)) return ''
  for (const [unit, size] of UNITS) {
    if (Math.abs(secs) >= size) return rtf.format(Math.round(secs / size), unit)
  }
  return 'hace un momento'
}

// Fecha local AAAA-MM-DD para nombres de archivo.
export function today(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

export function fileSlug(s: string): string {
  return fold(s).replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '') || 'curso'
}

// No hay GET de un solo alumno: se busca su matrícula dentro de su curso y, si ya no
// aparece ahí (cambió de curso o de matrícula), en la lista completa.
export async function fetchStudent(id: number, hint?: Pick<AdminStudent, 'courseId' | 'code'>): Promise<AdminStudent> {
  if (hint) {
    const { students } = await api.admin.students({ courseId: hint.courseId, q: hint.code })
    const found = students.find((s) => s.id === id)
    if (found) return found
  }
  const { students } = await api.admin.students()
  const found = students.find((s) => s.id === id)
  if (!found) throw new Error('El alumno ya no existe.')
  return found
}

// Campos editables de un alumno (formulario de alta y panel de detalle).
export interface StudentForm {
  courseId: number
  code: string
  name: string
  sexo: string
  email: string
  tareas: number
  monedas: number
  asistencias: number
  games: number
  exp: number
  comments: string
  active: boolean
}

export type FormErrors = Partial<Record<keyof StudentForm, string>>

export function toForm(s: AdminStudent): StudentForm {
  return {
    courseId: s.courseId,
    code: s.code,
    name: s.name,
    sexo: s.sexo,
    email: s.email,
    tareas: s.tareas,
    monedas: s.monedas,
    asistencias: s.asistencias,
    games: s.games,
    exp: s.exp,
    comments: s.comments,
    active: s.active,
  }
}

// Textos sin espacios de sobra, listos para mandar.
export function cleanForm(f: StudentForm): StudentForm {
  return {
    ...f,
    code: f.code.trim(),
    name: f.name.trim().replace(/\s+/g, ' '),
    sexo: f.sexo.trim(),
    email: f.email.trim(),
    comments: f.comments.trim(),
  }
}

export function validateStudent(f: StudentForm): FormErrors {
  const e: FormErrors = {}
  if (!f.courseId) e.courseId = 'Elige un curso.'
  const code = f.code.trim()
  if (!code) e.code = 'Escribe la matrícula.'
  else if (/\s/.test(code)) e.code = 'La matrícula no puede llevar espacios.'
  if (!f.name.trim()) e.name = 'Escribe el nombre.'
  if (!f.sexo.trim()) e.sexo = 'Elige Hombre o Mujer, o escribe otro valor.'
  const email = f.email.trim()
  if (email && !EMAIL_RE.test(email)) e.email = 'El correo no es válido.'
  for (const { key } of COUNTERS) {
    if (!Number.isInteger(f[key]) || f[key] < 0) e[key] = 'Usa un número entero de 0 en adelante.'
  }
  return e
}

export const STUDENT_CSV: CsvColumn<AdminStudent>[] = [
  { header: 'Curso', value: (s) => s.courseName },
  { header: 'ID', value: (s) => s.code },
  { header: 'Nombre', value: (s) => s.name },
  { header: 'Sexo', value: (s) => s.sexo },
  { header: 'Correo', value: (s) => s.email },
  { header: 'Nivel', value: (s) => s.level },
  { header: 'EXP', value: (s) => s.exp },
  { header: 'Monedas', value: (s) => s.monedas },
  { header: 'Tareas', value: (s) => s.tareas },
  { header: 'Asistencias', value: (s) => s.asistencias },
  { header: 'Juegos', value: (s) => s.games },
  { header: 'Insignias', value: (s) => s.badges },
  { header: 'Activo', value: (s) => (s.active ? 'Sí' : 'No') },
  { header: 'Comentarios', value: (s) => s.comments },
  { header: 'Último acceso', value: (s) => s.lastLoginAt ?? '' },
]
