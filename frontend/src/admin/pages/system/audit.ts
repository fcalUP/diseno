import type { AuditEntry } from '../../../lib/types'

type Tone = 'gray' | 'green' | 'red' | 'orange' | 'purple' | 'blue' | 'yellow'

// Entidades de la bitácora (docs/spec.md §5.2): singular para la etiqueta, plural para el filtro.
const ENTITIES: Record<string, { one: string; many: string; tone: Tone }> = {
  student: { one: 'Alumno', many: 'Alumnos', tone: 'blue' },
  course: { one: 'Curso', many: 'Cursos', tone: 'purple' },
  badge: { one: 'Insignia', many: 'Insignias', tone: 'orange' },
  visibility: { one: 'Visibilidad', many: 'Visibilidad de insignias', tone: 'yellow' },
  level: { one: 'Niveles', many: 'Niveles', tone: 'green' },
  avatar: { one: 'Avatar', many: 'Avatares', tone: 'blue' },
  purchase: { one: 'Compra', many: 'Compras', tone: 'green' },
  setting: { one: 'Ajustes', many: 'Ajustes', tone: 'gray' },
  admin: { one: 'Administrador', many: 'Administradores', tone: 'red' },
  import: { one: 'Importación', many: 'Importaciones', tone: 'purple' },
}

export const ENTITY_OPTIONS = Object.entries(ENTITIES).map(([value, e]) => ({ value, label: e.many }))

export function entityInfo(entity: string): { label: string; tone: Tone } {
  const e = ENTITIES[entity]
  return e ? { label: e.one, tone: e.tone } : { label: entity, tone: 'gray' }
}

const ACTORS: Record<AuditEntry['actorType'], { one: string; many: string }> = {
  admin: { one: 'administrador', many: 'Administradores' },
  student: { one: 'alumno', many: 'Alumnos' },
  system: { one: 'sistema', many: 'Sistema' },
}

export const ACTOR_OPTIONS = Object.entries(ACTORS).map(([value, a]) => ({ value, label: a.many }))

export function actorTypeLabel(type: AuditEntry['actorType']): string {
  return ACTORS[type]?.one ?? type
}

// Las entradas de deshacer/rehacer no se pueden deshacer a su vez.
export function isUndoRedo(entry: AuditEntry): boolean {
  return entry.action === 'undo' || entry.action === 'redo'
}
