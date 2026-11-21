// Cliente HTTP del portal y del panel. Una función por endpoint (ver docs/spec.md §4).
import type {
  AdminAccount,
  AdminBadge,
  AdminCourse,
  AdminLoginResponse,
  AdminPurchase,
  AdminSettings,
  AdminStats,
  AdminStudent,
  AdminUser,
  ApiErrorBody,
  AuditListResponse,
  Avatar,
  BadgeInput,
  BadgesResponse,
  BatchUndoResponse,
  BulkStudentsRequest,
  BulkStudentsResponse,
  CourseInput,
  GenerateStudentsRequest,
  GenerateStudentsResponse,
  ImportJob,
  ImportReport,
  ImportRunRequest,
  ImportSource,
  ImportStatus,
  LeaderboardResponse,
  Level,
  LoginRequest,
  LoginResponse,
  MediaImportResponse,
  MessageResponse,
  PasswordResponse,
  PublicConfig,
  PurchaseRequest,
  PurchaseResponse,
  RegisterRequest,
  ResetPasswordRequest,
  SendResetCodeRequest,
  SettingsPatch,
  StudentInput,
  StudentMutationResponse,
  StudentsResponse,
  UndoResponse,
  UpdateStudentDataRequest,
  UpdateStudentDataResponse,
  UploadTicket,
} from './types'

export class ApiError extends Error {
  status: number
  data: unknown

  constructor(message: string, status: number, data: unknown = null) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.data = data
  }
}

// Las rutas de alumno devuelven el cuerpo tal cual, sea éxito o {error}, porque el código
// heredado hace `if (data.success) ... else alert(data.error)`.
export type StudentResult<T> =
  | (T & { error?: undefined })
  | (ApiErrorBody & { [K in keyof T]?: undefined })

type Method = 'POST' | 'PUT' | 'PATCH' | 'DELETE'

// El WAF compartido bloquea cuerpos de más de 8 KB; mejor avisar antes de mandarlos.
export const MAX_BODY_BYTES = 8 * 1024
// Filas por petición en operaciones masivas.
export const CHUNK_SIZE = 50

// Se dispara cuando una ruta del panel responde 401 (la sesión expiró o la cerraron).
export const ADMIN_SESSION_EXPIRED = 'admin-session-expired'

// En producción el servidor agrega el SDK del WAF (challenge.js). Su fetch adjunta el
// token y resuelve retos en segundo plano; sin él se usa el fetch normal. Solo para el
// API propio: a S3 no hay que mandarle el encabezado del token.
declare global {
  interface Window {
    AwsWafIntegration?: { fetch: typeof fetch }
  }
}

function apiFetch(path: string, init: RequestInit): Promise<Response> {
  const waf = window.AwsWafIntegration
  return waf ? waf.fetch(path, init) : fetch(path, init)
}

async function request(method: 'GET' | Method, path: string, body?: unknown) {
  const headers: Record<string, string> = { Accept: 'application/json' }
  let payload: string | undefined
  if (method !== 'GET') {
    headers['Content-Type'] = 'application/json'
    // DELETE va sin cuerpo salvo que lo pidan; POST/PUT/PATCH siempre llevan JSON.
    if (body !== undefined || method !== 'DELETE') payload = JSON.stringify(body ?? {})
  }
  if (payload && new Blob([payload]).size > MAX_BODY_BYTES) {
    throw new ApiError('Los datos son demasiado grandes para enviarlos de una vez (máximo 8 KB).', 413)
  }
  const res = await apiFetch(path, { method, headers, body: payload, credentials: 'same-origin' })
  const text = await res.text()
  let data: unknown = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      // Respuesta que no es JSON, p. ej. la página de error del proxy o del WAF.
    }
  }
  return { res, data }
}

function errorText(status: number, data: unknown): string {
  const msg = (data as Partial<ApiErrorBody> | null)?.error
  return typeof msg === 'string' && msg ? msg : `Error del servidor (${status}).`
}

function unwrap<T>(path: string, { res, data }: { res: Response; data: unknown }): T {
  if (res.ok) return data as T
  if (res.status === 401 && path.startsWith('/api/admin/')) {
    window.dispatchEvent(new Event(ADMIN_SESSION_EXPIRED))
  }
  throw new ApiError(errorText(res.status, data), res.status, data)
}

export async function apiGet<T>(path: string): Promise<T> {
  return unwrap<T>(path, await request('GET', path))
}

export async function apiSend<T>(method: Method, path: string, body?: unknown): Promise<T> {
  return unwrap<T>(path, await request(method, path, body))
}

async function studentCall<T>(method: 'GET' | Method, path: string, body?: unknown): Promise<StudentResult<T>> {
  const { res, data } = await request(method, path, body)
  if (data && typeof data === 'object') return data as StudentResult<T>
  return { error: errorText(res.status, data) } as StudentResult<T>
}

// Mensaje legible para mostrar en un toast o junto a un formulario.
export function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message
  // fetch falla con TypeError cuando no hay red ("Failed to fetch", "Load failed"...).
  if (e instanceof TypeError && /fetch|network|load failed/i.test(e.message)) {
    return 'No se pudo conectar con el servidor. Revisa tu conexión.'
  }
  if (e instanceof Error && e.message) return e.message
  return 'Ocurrió un error inesperado.'
}

// Parte un arreglo en grupos de `size` elementos.
export function chunk<T>(arr: readonly T[], size: number): T[][] {
  const n = Math.max(1, Math.floor(size))
  const out: T[][] = []
  for (let i = 0; i < arr.length; i += n) out.push(arr.slice(i, i + n))
  return out
}

type Query = Record<string, string | number | boolean | null | undefined>

function qs(params: Query): string {
  const s = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') s.set(k, String(v))
  }
  const str = s.toString()
  return str ? `?${str}` : ''
}

const seg = encodeURIComponent

type Ok = { success: true }
type WithAudit<T> = T & { auditId: number }

export interface StudentFilters {
  courseId?: number
  career?: string
  q?: string
  active?: 'true' | 'false' | 'all'
}

export interface PurchaseFilters {
  courseId?: number
  studentId?: number
  badgeId?: number
  q?: string
  reverted?: 'all' | 'true' | 'false'
  limit?: number
}

export interface AuditFilters {
  entity?: string
  q?: string
  actorType?: string
  batchId?: string
  limit?: number
  beforeId?: number
}

export const api = {
  health: () => apiGet<{ ok: true }>('/api/health'),

  // ---- Público / alumno: nunca lanzan por status, sólo por fallas de red ----
  config: () => studentCall<PublicConfig>('GET', '/api/config'),
  login: (b: LoginRequest) => studentCall<LoginResponse>('POST', '/api/login', b),
  me: () => studentCall<LoginResponse>('GET', '/api/me'),
  logout: () => studentCall<Ok>('POST', '/api/logout', {}),
  register: (b: RegisterRequest) => studentCall<MessageResponse>('POST', '/api/register', b),
  sendResetCode: (b: SendResetCodeRequest) =>
    studentCall<MessageResponse>('POST', '/api/send-reset-code', b),
  resetPassword: (b: ResetPasswordRequest) =>
    studentCall<MessageResponse>('POST', '/api/reset-password-with-code', b),
  badges: (career?: string) => studentCall<BadgesResponse>('GET', `/api/badges${qs({ career })}`),
  purchase: (b: PurchaseRequest) => studentCall<PurchaseResponse>('POST', '/api/purchase', b),
  leaderboard: () => studentCall<LeaderboardResponse>('GET', '/api/leaderboard'),

  // ---- Panel: lanzan ApiError si el status no es 2xx ----
  admin: {
    login: (b: { username: string; password: string }) =>
      apiSend<AdminLoginResponse>('POST', '/api/admin-login', b),
    logout: () => apiSend<Ok>('POST', '/api/admin/logout', {}),
    me: () => apiGet<{ admin: AdminUser }>('/api/admin/me'),
    stats: () => apiGet<AdminStats>('/api/admin/stats'),

    // Alumnos
    students: (f: StudentFilters = {}) => apiGet<StudentsResponse>(`/api/admin/students${qs({ ...f })}`),
    updateStudentData: (b: UpdateStudentDataRequest) =>
      apiSend<UpdateStudentDataResponse>('POST', '/api/admin/update-student-data', b),
    createStudent: (b: StudentInput) => apiSend<StudentMutationResponse>('POST', '/api/admin/students', b),
    generateStudents: (b: GenerateStudentsRequest) =>
      apiSend<GenerateStudentsResponse>('POST', '/api/admin/students/generate', b),
    updateStudent: (id: number, b: StudentInput) =>
      apiSend<WithAudit<{ student: AdminStudent }>>('PATCH', `/api/admin/students/${id}`, b),
    deleteStudent: (id: number) => apiSend<WithAudit<Ok>>('DELETE', `/api/admin/students/${id}`),
    // Sin contraseña el servidor genera una y la devuelve una sola vez.
    setStudentPassword: (id: number, password?: string) =>
      apiSend<PasswordResponse>('POST', `/api/admin/students/${id}/password`, password ? { password } : {}),
    bulkStudents: (b: BulkStudentsRequest) =>
      apiSend<BulkStudentsResponse>('POST', '/api/admin/students/bulk', b),

    // Cursos
    courses: () => apiGet<{ courses: AdminCourse[] }>('/api/admin/courses'),
    createCourse: (b: CourseInput) => apiSend<WithAudit<{ course: AdminCourse }>>('POST', '/api/admin/courses', b),
    updateCourse: (id: number, b: CourseInput) =>
      apiSend<WithAudit<{ course: AdminCourse }>>('PATCH', `/api/admin/courses/${id}`, b),
    deleteCourse: (id: number) => apiSend<WithAudit<Ok>>('DELETE', `/api/admin/courses/${id}`),
    setCourseHiddenBadges: (id: number, badgeIds: number[]) =>
      apiSend<WithAudit<{ course: AdminCourse }>>('PUT', `/api/admin/courses/${id}/hidden-badges`, { badgeIds }),
    reorderCourses: (ids: number[]) =>
      apiSend<Ok & { batchId: string }>('POST', '/api/admin/courses/reorder', { ids }),

    // Insignias
    badges: () => apiGet<{ badges: AdminBadge[] }>('/api/admin/badges'),
    createBadge: (b: BadgeInput) => apiSend<WithAudit<{ badge: AdminBadge }>>('POST', '/api/admin/badges', b),
    updateBadge: (id: number, b: BadgeInput) =>
      apiSend<WithAudit<{ badge: AdminBadge }>>('PATCH', `/api/admin/badges/${id}`, b),
    deleteBadge: (id: number) => apiSend<WithAudit<Ok>>('DELETE', `/api/admin/badges/${id}`),
    reorderBadges: (ids: number[]) =>
      apiSend<Ok & { batchId: string }>('POST', '/api/admin/badges/reorder', { ids }),

    // Niveles
    levels: () => apiGet<{ levels: Level[] }>('/api/admin/levels'),
    saveLevels: (levels: Level[]) => apiSend<WithAudit<{ levels: Level[] }>>('PUT', '/api/admin/levels', { levels }),

    // Avatares (sin código = todos)
    avatars: (code?: string) => apiGet<{ avatars: Avatar[] }>(`/api/admin/avatars${qs({ code })}`),
    setAvatar: (b: { code: string; level: number; url: string }) =>
      apiSend<WithAudit<{ avatar: Avatar }>>('PUT', '/api/admin/avatars', b),
    deleteAvatar: (code: string, level: number) =>
      apiSend<WithAudit<Ok>>('DELETE', `/api/admin/avatars${qs({ code, level })}`),

    // Compras
    purchases: (f: PurchaseFilters = {}) =>
      apiGet<{ purchases: AdminPurchase[] }>(`/api/admin/purchases${qs({ ...f })}`),
    revertPurchase: (id: number) =>
      apiSend<WithAudit<{ purchase: AdminPurchase }>>('POST', `/api/admin/purchases/${id}/revert`),
    restorePurchase: (id: number) =>
      apiSend<WithAudit<{ purchase: AdminPurchase }>>('POST', `/api/admin/purchases/${id}/restore`),

    // Ajustes
    settings: () => apiGet<{ settings: AdminSettings }>('/api/admin/settings'),
    updateSettings: (patch: SettingsPatch) =>
      apiSend<WithAudit<{ settings: AdminSettings }>>('PATCH', '/api/admin/settings', patch),

    // Administradores
    admins: () => apiGet<{ admins: AdminAccount[] }>('/api/admin/admins'),
    createAdmin: (b: { username: string; password: string }) =>
      apiSend<WithAudit<{ admin: AdminAccount }>>('POST', '/api/admin/admins', b),
    updateAdmin: (id: number, b: { username?: string; password?: string; active?: boolean }) =>
      apiSend<WithAudit<{ admin: AdminAccount }>>('PATCH', `/api/admin/admins/${id}`, b),
    deleteAdmin: (id: number) => apiSend<WithAudit<Ok>>('DELETE', `/api/admin/admins/${id}`),

    // Subidas: usar uploadImage(), que valida y sube el archivo.
    createUpload: (b: { filename: string; contentType: string; size: number }) =>
      apiSend<UploadTicket>('POST', '/api/admin/uploads', b),

    // Actividad, deshacer y rehacer
    audit: (f: AuditFilters = {}) => apiGet<AuditListResponse>(`/api/admin/audit${qs({ ...f })}`),
    undo: (id: number, force = false) =>
      apiSend<UndoResponse>('POST', `/api/admin/audit/${id}/undo`, force ? { force } : {}),
    redo: (id: number, force = false) =>
      apiSend<UndoResponse>('POST', `/api/admin/audit/${id}/redo`, force ? { force } : {}),
    undoBatch: (batchId: string, force = false) =>
      apiSend<BatchUndoResponse>('POST', `/api/admin/audit/batch/${seg(batchId)}/undo`, force ? { force } : {}),
    redoBatch: (batchId: string, force = false) =>
      apiSend<BatchUndoResponse>('POST', `/api/admin/audit/batch/${seg(batchId)}/redo`, force ? { force } : {}),

    // Importación desde Google Sheets
    importStatus: () => apiGet<ImportStatus>('/api/admin/import/status'),
    importPreview: (b: ImportSource = {}) => apiSend<ImportReport>('POST', '/api/admin/import/preview', b),
    importRun: (b: ImportRunRequest) => apiSend<{ jobId: string }>('POST', '/api/admin/import/run', b),
    importJob: (jobId: string) => apiGet<ImportJob>(`/api/admin/import/jobs/${seg(jobId)}`),
    importMedia: () => apiSend<MediaImportResponse>('POST', '/api/admin/import/media'),
  },
}

const IMAGE_TYPES = ['image/png', 'image/jpeg', 'image/gif', 'image/webp']
export const MAX_IMAGE_BYTES = 20 * 1024 * 1024

// Sube una imagen directo al almacenamiento (S3 o el disco local en desarrollo) y
// devuelve la url /media/... que se guarda en la entidad.
export async function uploadImage(file: File): Promise<string> {
  if (!IMAGE_TYPES.includes(file.type)) {
    throw new ApiError('Formato no permitido. Usa PNG, JPG, GIF o WEBP.', 0)
  }
  if (file.size > MAX_IMAGE_BYTES) throw new ApiError('La imagen pesa más de 20 MB.', 0)

  const ticket = await api.admin.createUpload({ filename: file.name, contentType: file.type, size: file.size })
  // El PUT a S3 es de otro origen: va sin cookies. El driver local responde en el mismo origen.
  const sameOrigin = ticket.uploadUrl.startsWith('/')
  let res: Response
  try {
    res = await fetch(ticket.uploadUrl, {
      method: 'PUT',
      headers: ticket.headers,
      body: file,
      credentials: sameOrigin ? 'same-origin' : 'omit',
    })
  } catch {
    throw new ApiError('No se pudo subir la imagen. Revisa tu conexión.', 0)
  }
  if (!res.ok) throw new ApiError(`No se pudo subir la imagen (${res.status}).`, res.status)
  return ticket.url
}
