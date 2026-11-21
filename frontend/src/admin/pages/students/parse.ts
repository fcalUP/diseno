// Lee la lista que se pega en "Generar alumnos": una fila por línea con
// ID, Nombre, Sexo[, correo][, contraseña]. Columnas separadas por tabulador (lo que
// copia Sheets o Excel), punto y coma o coma. El encabezado es opcional y, si está,
// manda el orden de las columnas.
import { EMAIL_RE, fold } from './util'

export type PasteField = 'code' | 'name' | 'sexo' | 'email' | 'password'

export interface PastedRow {
  /** Línea del texto pegado (empieza en 1), para señalar errores. */
  line: number
  code: string
  name: string
  sexo: string
  email: string
  password: string
}

export interface PreviewRow extends PastedRow {
  errors: string[]
}

export interface PasteResult {
  rows: PastedRow[]
  /** La primera línea parece encabezado. */
  headerDetected: boolean
  /** Se tomó la primera línea como encabezado. */
  header: boolean
  error: string | null
}

const ORDER: PasteField[] = ['code', 'name', 'sexo', 'email', 'password']

// Nombres de columna aceptados, ya sin acentos y en minúsculas.
const HEADER_NAMES: Record<PasteField, string[]> = {
  code: ['id', 'matricula', 'codigo', 'student id', 'id alumno'],
  name: ['nombre', 'name', 'nombre completo', 'alumno', 'full name'],
  sexo: ['sexo', 'genero', 'sex', 'gender'],
  email: ['correo', 'email', 'e-mail', 'mail', 'correo electronico'],
  password: ['contrasena', 'password', 'pass', 'pwd'],
}

function headerField(cell: string): PasteField | null {
  const c = fold(cell)
  return ORDER.find((f) => HEADER_NAMES[f].includes(c)) ?? null
}

// Tabulador primero (pegado de una hoja), luego punto y coma, luego coma.
function pickSeparator(lines: string[]): string {
  return ['\t', ';', ','].find((sep) => lines.some((l) => l.includes(sep))) ?? ','
}

// Parte una línea respetando comillas dobles, p. ej. "López, Ana".
function splitLine(line: string, sep: string): string[] {
  const out: string[] = []
  let cur = ''
  let quoted = false
  for (let i = 0; i < line.length; i++) {
    const ch = line[i]
    if (quoted) {
      if (ch !== '"') cur += ch
      else if (line[i + 1] === '"') {
        cur += '"'
        i++
      } else quoted = false
    } else if (ch === '"' && cur.trim() === '') {
      quoted = true
      cur = ''
    } else if (ch === sep) {
      out.push(cur.trim())
      cur = ''
    } else cur += ch
  }
  out.push(cur.trim())
  return out
}

// H/M es lo usual en México; lo demás se respeta tal cual (sexo "otro").
function normalizeSexo(value: string): string {
  const s = fold(value)
  if (['h', 'hombre', 'masculino', 'male', 'man'].includes(s)) return 'Hombre'
  if (['m', 'mujer', 'f', 'femenino', 'female', 'woman'].includes(s)) return 'Mujer'
  return value.trim()
}

/** `headerOverride` null = detectar solo. */
export function parsePaste(text: string, headerOverride: boolean | null): PasteResult {
  const lines = text
    .split(/\r?\n/)
    .map((raw, i) => ({ raw, line: i + 1 }))
    .filter((l) => l.raw.trim() !== '')
  if (lines.length === 0) return { rows: [], headerDetected: false, header: false, error: null }

  const sep = pickSeparator(lines.map((l) => l.raw))
  const cells = lines.map((l) => splitLine(l.raw, sep))
  const first = cells[0].map(headerField)
  const headerDetected = first.includes('code') || first.includes('name')
  const header = headerOverride ?? headerDetected

  const index: Partial<Record<PasteField, number>> = {}
  if (header) {
    first.forEach((f, i) => {
      if (f && index[f] === undefined) index[f] = i
    })
    if (index.code === undefined || index.name === undefined) {
      return {
        rows: [],
        headerDetected,
        header,
        error: 'El encabezado necesita una columna de ID (o matrícula) y otra de nombre.',
      }
    }
  } else {
    ORDER.forEach((f, i) => {
      index[f] = i
    })
  }

  const skip = header ? 1 : 0
  const get = (row: string[], f: PasteField) => {
    const i = index[f]
    return i === undefined ? '' : (row[i] ?? '').trim()
  }
  const rows = cells.slice(skip).map((row, i) => ({
    line: lines[i + skip].line,
    code: get(row, 'code'),
    name: get(row, 'name').replace(/\s+/g, ' '),
    sexo: normalizeSexo(get(row, 'sexo')),
    email: get(row, 'email'),
    password: get(row, 'password'),
  }))
  return { rows, headerDetected, header, error: null }
}

// Marca cada fila con sus problemas. `existing` son las matrículas que ya hay en el curso.
export function validateRows(
  rows: PastedRow[],
  existing: ReadonlySet<string>,
  passwordPattern?: string,
): PreviewRow[] {
  let re: RegExp | null = null
  if (passwordPattern) {
    try {
      re = new RegExp(passwordPattern)
    } catch {
      re = null
    }
  }
  const seen = new Map<string, number>()
  return rows.map((r) => {
    const errors: string[] = []
    if (!r.code) errors.push('Falta el ID.')
    else if (/\s/.test(r.code)) errors.push('El ID no puede llevar espacios.')
    if (!r.name) errors.push('Falta el nombre.')
    if (r.email && !EMAIL_RE.test(r.email)) errors.push('El correo no es válido.')
    if (r.password && re && !re.test(r.password)) errors.push('La contraseña no cumple la regla de Ajustes.')
    if (r.code) {
      const first = seen.get(r.code)
      if (first === undefined) seen.set(r.code, r.line)
      else errors.push(`ID repetido (ya está en la línea ${first}).`)
      if (existing.has(r.code)) errors.push('Ya existe en este curso.')
    }
    return { ...r, errors }
  })
}
