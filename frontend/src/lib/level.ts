import type { Level } from './types'

// Nivel = el más alto cuyo umbral ya se alcanzó. Con los umbrales de siempre
// (0, 10, 25, 50, 75, 100) da lo mismo que el calculateLevel anterior.
export function calcLevel(exp: number, levels: Level[]): number {
  let level = 0
  for (const l of levels) {
    if (exp >= l.minExp && l.level > level) level = l.level
  }
  return level
}

export interface LevelProgress {
  percentage: number
  currentExp: number
  requiredExp: number
}

// Misma cuenta que getLevelProgress de la versión anterior, pero en el nivel máximo la
// barra sale llena (antes quedaba vacía justo en el último umbral).
export function getLevelProgress(exp: number, level: number, levels: Level[]): LevelProgress {
  const threshold = (n: number) => levels.find((l) => l.level === n)?.minExp
  const current = threshold(level) ?? 0
  const next = threshold(level + 1)
  if (next === undefined) {
    const last = levels.reduce((max, l) => Math.max(max, l.minExp), 0)
    return { percentage: 100, currentExp: exp, requiredExp: last }
  }
  const pct = ((exp - current) / (next - current)) * 100
  return {
    percentage: Number.isFinite(pct) ? Math.min(100, Math.max(0, pct)) : 0,
    currentExp: exp,
    requiredExp: next,
  }
}
