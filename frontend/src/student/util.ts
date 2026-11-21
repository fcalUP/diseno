import { errorMessage, type StudentResult } from '../lib/api'

// Si la petición ni siquiera llega (sin red), se contesta como un error más del servidor
// para que los flujos de siempre, con alert(data.error), sigan igual.
export async function settle<T>(request: Promise<StudentResult<T>>): Promise<StudentResult<T>> {
  try {
    return await request
  } catch (e) {
    return { error: errorMessage(e) } as StudentResult<T>
  }
}

// Revisa la contraseña con el patrón configurado. Si el patrón no compila en JS, decide el
// servidor.
export function matchesPattern(value: string, pattern: string): boolean {
  try {
    return new RegExp(pattern).test(value)
  } catch {
    return true
  }
}
