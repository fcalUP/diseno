// Contrato con el backend. Los nombres coinciden con las etiquetas JSON de Go.
// Cambiar una forma aquí obliga a cambiarla también en el servidor.

export interface ApiErrorBody {
  error: string
  conflict?: boolean
  fields?: string[]
}

// ---------- Público / alumno ----------

export interface Level {
  level: number
  minExp: number
}

export interface PublicCourse {
  id: number
  name: string
  sideImageUrl: string
  logoUrl: string
}

export interface PublicConfig {
  courses: PublicCourse[]
  features: {
    registration: boolean
    passwordReset: boolean
    store: boolean
    leaderboard: boolean
    levelUpAnimation: boolean
  }
  levels: Level[]
  levelUpGifUrl: string
  passwordPattern: string
  passwordHintRegister: string
  passwordHintReset: string
  passwordError: string
}

export interface Student {
  id: string // matrícula, igual que en la versión de Sheets
  name: string
  sexo: string
  email: string
  career: string
  monedas: number
  tareas: number
  asistencias: number
  games: number
  exp: number
  level: number
  purchases: Record<string, number>
  comments: string
  avatarUrl: string
  defaultAvatarUrl: string
}

export interface LoginRequest {
  studentId: string
  password: string
  career: string
}

export interface LoginResponse {
  success: true
  student: Student
  levelUpOccurred: boolean
}

export interface RegisterRequest {
  id: string
  name: string
  sexo: string
  password: string
  career: string
}

export interface MessageResponse {
  message: string
}

export interface SendResetCodeRequest {
  id: string
  career: string
}

export interface ResetPasswordRequest {
  id: string
  code: string
  password: string
  career: string
}

export interface StoreBadge {
  id: number
  name: string
  quantity: number
  cost: number
  imageUrl: string
}

export interface BadgesResponse {
  success: true
  badges: StoreBadge[]
}

export interface PurchaseItem {
  badgeId: number
  quantity: number
  unitCost: number
}

export interface PurchaseRequest {
  items: PurchaseItem[]
}

export interface PurchaseResponse {
  success: true
  message: string
  newCoins: number
  badges: { id: number; quantity: number }[]
  purchases: Record<string, number>
}

export interface LeaderboardRow {
  rank: number
  name: string
  sexo: string
  badges: number
  exp: number
  level: number
}

export interface LeaderboardResponse {
  success: true
  leaderboard: LeaderboardRow[]
}

// ---------- Admin ----------

export interface AdminUser {
  id: number
  username: string
}

export interface AdminLoginResponse {
  success: true
  message: string
  admin: AdminUser
}

export interface AuditRef {
  auditId?: number
  batchId?: string
}

export interface AdminStudent {
  id: number
  code: string
  courseId: number
  courseName: string
  name: string
  sexo: string
  email: string
  tareas: number
  monedas: number
  asistencias: number
  games: number
  exp: number
  level: number
  lastLoginLevel: number
  badges: number
  comments: string
  active: boolean
  lastLoginAt: string | null
  createdAt: string
  updatedAt: string
  avatarLevels: number[]
}

export interface StudentInput {
  courseId?: number
  code?: string
  name?: string
  sexo?: string
  email?: string
  tareas?: number
  monedas?: number
  asistencias?: number
  games?: number
  exp?: number
  comments?: string
  active?: boolean
  password?: string // solo al crear
}

export interface StudentsResponse {
  success: true
  students: AdminStudent[]
}

export interface StudentMutationResponse {
  student: AdminStudent
  password?: string
  auditId: number
}

export interface UpdateStudentDataRequest {
  id: number
  tareas: number
  monedas: number
  asistencias: number
  games: number
  exp: number
}

export interface UpdateStudentDataResponse {
  success: true
  message: string
  newLevel: number
  auditId: number
}

export interface GenerateRow {
  code: string
  name: string
  sexo?: string
  email?: string
  password?: string
}

export interface GenerateStudentsRequest {
  courseId: number
  rows: GenerateRow[]
  batchId?: string
}

export interface GenerateStudentsResponse {
  created: { student: AdminStudent; password: string }[]
  skipped: { code: string; reason: string }[]
  batchId: string
}

export type CounterField = 'tareas' | 'monedas' | 'asistencias' | 'games' | 'exp'

export interface BulkStudentsRequest {
  ids: number[]
  action: 'delta' | 'set' | 'activate' | 'deactivate' | 'move' | 'delete'
  delta?: Partial<Record<CounterField, number>>
  set?: Partial<Record<CounterField, number>>
  courseId?: number
  batchId?: string
}

export interface BulkStudentsResponse {
  updated: number
  batchId: string
}

export interface PasswordResponse {
  password: string
  auditId: number
}

export interface AdminCourse {
  id: number
  name: string
  sheetName: string
  sheetAliases: string[]
  sideImageUrl: string
  logoUrl: string
  hiddenBadgeKeywords: string[]
  active: boolean
  sortOrder: number
  studentCount: number
  hiddenBadgeIds: number[]
  createdAt: string
  updatedAt: string
}

export interface CourseInput {
  name?: string
  sheetName?: string
  sheetAliases?: string[]
  sideImageUrl?: string
  logoUrl?: string
  hiddenBadgeKeywords?: string[]
  active?: boolean
  sortOrder?: number
}

export interface AdminBadge {
  id: number
  name: string
  slug: string
  stock: number
  cost: number
  imageUrl: string
  active: boolean
  sortOrder: number
  hiddenCourseIds: number[]
  purchasedCount: number
  createdAt: string
  updatedAt: string
}

export interface BadgeInput {
  name?: string
  stock?: number
  cost?: number
  imageUrl?: string
  active?: boolean
  sortOrder?: number
  hiddenCourseIds?: number[]
}

export interface Avatar {
  code: string
  level: number
  url: string
  updatedAt: string
}

export interface AdminPurchase {
  id: number
  studentId: number
  studentCode: string
  studentName: string
  courseId: number
  courseName: string
  badgeId: number | null
  badgeName: string
  quantity: number
  unitCost: number
  totalCost: number
  coinsBefore: number | null
  coinsAfter: number | null
  source: 'app' | 'sheets' | 'admin'
  createdAt: string
  revertedAt: string | null
}

export interface SecretStatus {
  configured: boolean
  source: 'db' | 'env' | 'none'
}

export interface AdminSettings {
  registrationEnabled: boolean
  passwordResetEnabled: boolean
  storeEnabled: boolean
  leaderboardEnabled: boolean
  levelUpAnimationEnabled: boolean
  purchaseEmailEnabled: boolean
  emailFrom: string
  emailCc: string[]
  studentEmailDomain: string
  passwordPattern: string
  passwordHintRegister: string
  passwordHintReset: string
  passwordError: string
  resetCodeTtlMinutes: number
  levelUpGifUrl: string
  spreadsheetId: string
  resendApiKey: SecretStatus
  googleServiceAccount: SecretStatus & { clientEmail?: string }
}

// Campos editables; los secretos aceptan texto para guardar o null para borrar.
export type SettingsPatch = Partial<
  Omit<AdminSettings, 'resendApiKey' | 'googleServiceAccount'>
> & {
  resendApiKey?: string | null
  googleServiceAccount?: string | null
}

export interface AdminAccount {
  id: number
  username: string
  active: boolean
  lastLoginAt: string | null
  createdAt: string
}

export interface UploadTicket {
  uploadUrl: string
  method: 'PUT'
  headers: Record<string, string>
  url: string // /media/... para guardar en la entidad
}

export interface AdminStats {
  students: { total: number; active: number }
  courses: { total: number; active: number }
  badges: { total: number; active: number; lowStock: AdminBadge[] }
  coinsInCirculation: number
  purchases: { last7Days: number; total: number }
  perCourse: {
    courseId: number
    courseName: string
    active: boolean
    students: number
    avgLevel: number
    totalExp: number
  }[]
  recentActivity: AuditEntry[]
}

export interface AuditEntry {
  id: number
  at: string
  actorType: 'admin' | 'student' | 'system'
  actorLabel: string
  action: string
  entity: string
  entityId: string | null
  summary: string
  before: unknown
  after: unknown
  batchId: string | null
  undoable: boolean
  undoneAt: string | null
  revertsId: number | null
}

export interface AuditListResponse {
  entries: AuditEntry[]
  nextBeforeId: number | null
}

export interface UndoResponse {
  success: true
  auditId: number
  summary: string
}

export interface BatchUndoResponse {
  success: true
  done: number
  failed: { id: number; error: string }[]
}

export interface ImportStatus {
  spreadsheetId: string
  credentials: SecretStatus & { clientEmail?: string }
  legacyMedia: { available: boolean; avatars: number; badges: number }
  lastImport: AuditEntry | null
}

export interface ImportInclude {
  students: boolean
  badges: boolean
  purchases: boolean
  media: boolean
}

export type ImportMode = 'upsert' | 'create_only'

export interface ImportSource {
  spreadsheetId?: string // también acepta la url completa de la hoja
  credentialsJson?: string // service account de un solo uso; no se guarda
  // Opcionales en la vista previa; si faltan se simula todo en modo upsert.
  include?: ImportInclude
  mode?: ImportMode
}

export interface ImportRunRequest extends ImportSource {
  include: ImportInclude
  mode: ImportMode
}

export interface ImportCourseReport {
  courseId: number
  courseName: string
  sheet: string | null
  rows: number
  created: number
  updated: number
  unchanged: number
  skipped: number
}

export interface MediaImportReport {
  avatars: number // casillas nuevas en avatars (nunca reemplaza las existentes)
  badgeImages: number // insignias sin imagen que recibieron legacy/badges/{slug}.png
  courseImages: number // siempre 0: los cursos ya traen sus imágenes desde la semilla
}

export interface ImportReport {
  dryRun: boolean
  batchId: string | null
  spreadsheetTitle: string
  sheetsFound: string[]
  courses: ImportCourseReport[]
  // skipped: repetidas, sin nombre válido o ya existentes en modo create_only.
  badges: { rows: number; created: number; updated: number; unchanged: number; skipped: number }
  // skipped: ya importadas antes o filas incompletas; unmatched: sin alumno.
  purchases: { rows: number; created: number; skipped: number; unmatched: number }
  media: MediaImportReport
  warnings: string[]
}

export interface ImportJob {
  id: string
  status: 'running' | 'done' | 'error'
  startedAt: string
  finishedAt: string | null
  report: ImportReport | null
  error: string | null
}

export interface MediaImportResponse {
  media: MediaImportReport
  warnings: string[]
  batchId: string | null
}
