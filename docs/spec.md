# Migration spec (agent-facing source of truth)

This file is the contract every implementation agent follows. If something here conflicts
with your instincts, follow this file. If something is genuinely missing, choose the simplest
option consistent with it and note it in your final report.

## 0. What the product is

A gamification portal for Universidad Panamericana design courses. Students log in with
student ID + password, then open their only course automatically or choose among enrolled
courses ("career" in the legacy code) shown as cards with names and logos. A navbar selector
switches courses without logging in again. Within a course, students see their
avatar (one image per level), level, coins, EXP bar, counters, comments, a leaderboard and a
badge store where they spend coins. The professor administers everything.

Legacy implementation (read it, it is the behavioral reference):
- `legacy/server.js` — Express + Google Sheets as the database.
- `legacy/index.html` — React 18 UMD + Babel standalone + Tailwind Play CDN, all inline.
- Legacy media now lives in `seed/media/legacy/` (avatars, badges, img, gifs, logo.png).

New stack:
- `frontend/`: React 19 + Vite 8 + TypeScript 6.0 + ESLint 10 (flat config) + Tailwind CSS
  **3.4** (v3 on purpose: the legacy markup uses v3-only utilities like `bg-opacity-60`; v4
  would change the look). React Router 8 (declarative mode, `import ... from "react-router"`),
  `lucide-react` icons for the admin panel only.
- `backend/`: Go 1.26, stdlib `net/http` (Go 1.22+ method patterns), `pgx/v5` (pgxpool),
  `golang.org/x/crypto/bcrypt`, `golang.org/x/oauth2/google` + plain REST for Google Sheets,
  `aws-sdk-go-v2` (s3 + presign) for media. No web framework, no ORM, no codegen.
- PostgreSQL 18. Schema in `backend/internal/app/migrations/*.sql` (embedded, applied at boot).
- Deploy: one EC2 t4g.small (arm64) running Docker Compose: Caddy + app + Postgres. CloudFront
  with the account's shared WAF in front. Media in S3 behind CloudFront `/media/*`.

## 1. Hard rules

1. **Student-facing design must not change.** Port `legacy/index.html` markup and Tailwind
   classes verbatim (same texts, same alerts, same layout, same English copy). Only data
   sources change (API instead of hardcoded values) and real bugs get fixed (listed in §7).
   User-requested exception: remove course selection from login, add course cards after
   multi-course login and a course switcher in the student navbar, using the existing style.
2. **Nothing configurable lives in code.** Courses, which courses appear in the login
   dropdown, side image/logo per course, badges, per-course badge visibility, level
   thresholds, feature toggles, email sender/CC, password rule and its texts, reset-code TTL,
   level-up GIF, spreadsheet ID, Google credentials, Resend key, admin users: all editable
   from the admin UI. Code holds only *defaults* used to seed the DB.
3. **Everything an admin changes is logged and can be undone/redone** from the UI
   (activity log + "Deshacer" toast after each mutation). Activate/deactivate toggles
   instead of destructive deletes wherever a toggle makes sense.
4. **Every JSON request body must stay under 8 KB.** The shared WAF blocks larger bodies
   for this host. The frontend chunks bulk operations (≤ 50 rows per request) and never
   uploads files through the API: files go straight to S3 with a presigned PUT.
5. Admin panel copy is **Spanish** (Mexico). Student UI copy stays exactly as legacy
   (mostly English). Backend error messages keep the legacy Spanish strings where they exist.
6. Code comments in Spanish, short, written like a person (no AI filler, no emojis).
7. Laziest correct solution: stdlib first, no new dependencies beyond those listed, no
   speculative abstractions. Never skip validation, auth, error handling or accessibility.
8. Use the context7 MCP tools to check current library APIs before writing code against
   them (React Router 8, pgx v5, aws-sdk-go-v2, typescript-eslint, Tailwind 3).

## 2. Repository layout and file ownership

```
backend/
  go.mod                          module github.com/fcalUP/diseno/backend
  cmd/server/main.go              [BF] wiring, flags (-healthcheck), graceful shutdown
  internal/app/
    app.go                        [BF] App struct, New(), Routes() — ALL routes registered here
    config.go                     [BF] env config
    http.go                       [BF] JSON helpers, middleware, clientIP, errors
    auth.go                       [BF] tokens, cookies, password hashing, require* middleware
    ratelimit.go                  [BF] in-memory limiter
    settings.go                   [BF] typed settings with defaults + secret encryption
    levels.go                     [BF] levels cache + CalcLevel
    audit.go                      [BF] Audit() recording API (insert only)
    storage.go                    [BF] Storage interface + local + s3 implementations
    web.go                        [BF] SPA static serving + local /media serving
    migrate.go                    [BF] embedded migration runner
    migrations/0001_init.sql      [ORCH] schema (already written — do not change without reason)
    migrations/0002_seed.sql      [ORCH] default data
    util.go                       [BF] slugify, normalizeLabel, parseIntLoose, random helpers
    student.go                    [B1] public config + student endpoints
    mail.go                       [B1] Resend client + email templates
    admin_core.go                 [B2] admin login/logout/me, quick dashboard endpoints, stats
    admin_students.go             [B2] students CRUD, generate, bulk, password
    admin_catalog.go              [B2] courses, badges, visibility, levels, avatars
    admin_misc.go                 [B2] purchases admin, settings, admins, uploads
    undo.go                       [B2] undo/redo engine + audit list endpoints
    sheets.go                     [B3] Google Sheets REST client
    importer.go                   [B3] import preview/run/jobs + legacy media import
    *_test.go                     owner of the matching file
frontend/
  index.html, vite.config.ts, eslint.config.js, tailwind.config.js, postcss.config.js,
  tsconfig*.json                  [ORCH] already written
  src/main.tsx                    [FF]
  src/App.tsx                     [FF] router: "/" → student app, "/admin/*" → admin app
  src/index.css                   [ORCH] Tailwind + legacy custom CSS (already written)
  src/lib/types.ts                [ORCH] API types (the contract) — do not change shapes
  src/lib/api.ts                  [FF] typed fetch client, one function per endpoint
  src/lib/level.ts                [FF] calcLevel/getLevelProgress from levels config
  src/student/*                   [F1] legacy UI port (+ quick admin modal from legacy)
  src/admin/AdminApp.tsx          [FF] admin routes + auth guard + login page
  src/admin/AdminLayout.tsx       [FF] sidebar/topbar shell
  src/admin/ui/*                  [FF] shared kit (see §6)
  src/admin/pages/Dashboard.tsx, Students.tsx, StudentDrawer.tsx, GenerateStudents.tsx,
                  Avatars.tsx                                   [F2a]
  src/admin/pages/Courses.tsx, Badges.tsx, Purchases.tsx, Levels.tsx   [F2b]
  src/admin/pages/Import.tsx, Activity.tsx, Settings.tsx, Admins.tsx  [F2c]
deploy/                           [I1] CloudFormation, Caddyfile, compose, scripts
Dockerfile, .dockerignore, docker-compose.yml (local dev), .github/workflows/ci-cd.yml  [I1]
seed/media/legacy/                legacy images, synced to s3://<bucket>/media/legacy/
legacy/                           old code, reference only (deleted at the end)
```
Agents only edit files they own. If you need something from a file you don't own, use the
signature promised in §5/§6; if it's missing, write a minimal local helper in your own file.

## 3. Data model

See `backend/internal/app/migrations/0001_init.sql` (authoritative). Summary:
- `courses(id, name unique, sheet_name unique, sheet_aliases text[], side_image_url, logo_url,
  hidden_badge_keywords text[], active, sort_order, timestamps)`.
  `active` = appears in login/registration dropdown. `hidden_badge_keywords`: when a badge is
  created (admin or import) and its normalized name contains one of these, it starts hidden
  for that course. Seed: "Analisis de Datos" → {internet, calculadora} (legacy behavior).
- `students(id, course_id, code, name, sexo, email, password_hash, tareas, monedas,
  asistencias, exp, games, last_login_level, comments, active, last_login_at, timestamps)`,
  unique (course_id, code). One row = one legacy sheet row. A person in two courses has two
  rows (legacy did the same with two sheets).
- `badges(id, name, slug unique, stock, cost, image_url, active, sort_order, timestamps)`.
- `course_hidden_badges(course_id, badge_id)` — row present = badge hidden for that course.
- `purchases(id, student_id, badge_id null, badge_name, quantity, unit_cost, total_cost,
  coins_before, coins_after, source 'app'|'sheets'|'admin', source_ref, created_at,
  reverted_at)`. Reverted purchases don't count anywhere.
- `avatars(code, level, url, updated_at)` pk (code, level). `code` is the student code, or
  `DefaultH` / `DefaultF` for the defaults (same naming as legacy files).
- `levels(level pk, min_exp unique)`; seeded 0:0, 1:10, 2:25, 3:50, 4:75, 5:100.
- `settings(key pk, value jsonb, updated_at)`; only overrides are stored, defaults in code.
- `admins(id, username unique, password_hash, active, last_login_at, timestamps)`.
- `audit_log(...)` — see §5.6.
- `reset_codes(student_id pk, code_hash, expires_at, attempts)`.

Level of a student = highest `level` whose `min_exp <= exp` (legacy thresholds give the same
result as legacy `calculateLevel`).

## 4. HTTP API

General:
- JSON in and out. Errors: `{"error": "mensaje"}` with a proper status. Legacy endpoints keep
  `success: true` in successful responses.
- Mutating requests (POST/PUT/PATCH/DELETE under `/api/`) must send
  `Content-Type: application/json` (exception: `PUT /api/uploads/local`). If an `Origin`
  header is present it must equal `PUBLIC_URL` (or be a localhost origin when
  `PUBLIC_URL` is http://localhost...). Otherwise 403.
- Bodies limited to 64 KB server-side (WAF caps at 8 KB in production anyway).
- Sessions: HMAC-signed cookies. Student cookie `diseno_alumno`, admin cookie `diseno_admin`.
  HttpOnly, SameSite=Lax, Path=/, Secure when `COOKIE_SECURE=true`, 12 h lifetime.
- Every admin mutation response includes `auditId` (number) or `batchId` (string) so the UI
  can offer "Deshacer".
- TypeScript shapes for every response are in `frontend/src/lib/types.ts`. Go JSON tags must
  match those field names exactly (camelCase).

### 4.1 Public / student

| Method & path | Auth | Request | Response |
|---|---|---|---|
| GET `/api/health` | – | – | `{ok:true}` (checks DB ping) |
| GET `/api/config` | – | – | `PublicConfig` |
| POST `/api/login` | – | `{studentId, password, career}` | `LoginResponse`, sets student cookie |
| GET `/api/me` | student | – | `LoginResponse` |
| POST `/api/logout` | – | `{}` | `{success:true}` clears cookie |
| POST `/api/register` | – | `{id, name, sexo, password, career}` | 201 `{message}` |
| POST `/api/send-reset-code` | – | `{id, career}` | `{message}` |
| POST `/api/reset-password-with-code` | – | `{id, code, password, career}` | `{message}` |
| GET `/api/badges?career=` | – | – | `{success, badges: StoreBadge[]}` |
| POST `/api/purchase` | student | `PurchaseRequest` | `PurchaseResponse` |
| GET `/api/leaderboard` | student | – | `{success, leaderboard: LeaderboardRow[]}` |

Behavior details (B1):
- `career` is the course **name** (what the dropdown shows). Resolve it against active
  courses; unknown/inactive → 400 `{"error":"Carrera no válida."}`.
- Login: missing fields → 400 `ID, contraseña y carrera requeridos.`; wrong code/password →
  401 `Credenciales inválidas.`; inactive student → 403 `Tu cuenta está desactivada.`;
  rate limited → 429 `Demasiados intentos. Intenta de nuevo en unos minutos.`
  Lockout per (course, code): 10 failures in 15 min. Per IP: 100 attempts per 5 min.
  On success: compute level; `levelUpOccurred = level > last_login_level`; if
  `level != last_login_level` store the new value; set `last_login_at`; set cookie.
- `/api/me` does the same level-up check (so the GIF shows after a refresh too).
- Register: validates trimmed `id`, `name`, `sexo`, `password` against the
  `passwordPattern` setting, course active. Disabled → 403 `El registro está cerrado.`
  Invalid → 400 legacy text `Datos inválidos o incompletos. Contraseña debe ser 4 HEX
  mayúsculas y carrera requerida.` Duplicate (course, code) → 409 `El ID de alumno ya existe.`
  Email = `id@{studentEmailDomain}`. Counters 0. Audit actor `student`, action `create`.
  Rate limit 20/hour/IP.
- Reset: disabled → 403 `La recuperación de contraseña está desactivada.` Unknown student →
  404 legacy text. Code = 6 random digits (crypto/rand), stored as SHA-256 hex in
  `reset_codes` with TTL from settings, 5 wrong attempts invalidate it. Email to the
  student's `email` (fallback `code@domain`) plus `emailCc`, legacy HTML body. Email failure
  is logged, response is still 200 (legacy). Limits: 3 sends/hour per student, 20/hour/IP.
  Reset with code: legacy messages; success updates hash, deletes code, audit entry.
- Badges: active badges not hidden for the course, sorted by sort_order then name. If the
  store is disabled → 403 `La tienda está cerrada.` If `career` missing, use the student
  session's course, else 400.
- Purchase (all-or-nothing transaction): body `{items:[{badgeId, quantity, unitCost}]}`.
  Merge duplicate badgeIds; quantity integer ≥ 1. `SELECT ... FOR UPDATE` the student, then
  the badges ordered by id. Per badge: must exist and be active and visible for the student's
  course (403 `Esta insignia no está disponible para tu grupo.`), `unitCost` must equal the
  current cost (409 `El precio de {name} cambió. Recarga la tienda.`), stock ≥ quantity (400
  `Sólo quedan {n} unidades de {name}`). Total ≤ coins (400 `Monedas insuficientes`).
  Deduct coins and stock, insert one purchase row per item (coins_before/after chained),
  one audit entry per item sharing a batch id (actor student, action `purchase`, entity
  `purchase`). After commit, send the confirmation email asynchronously when
  `purchaseEmailEnabled` (legacy text body, all items listed). Store disabled → 403.
- Leaderboard: the session student's course, active students, `ORDER BY exp DESC, id ASC`,
  `badges` = sum of non-reverted purchase quantities. Disabled → 403
  `El leaderboard está desactivado.` Do not expose student codes.

### 4.2 Admin auth and legacy quick dashboard

| Method & path | Request | Response |
|---|---|---|
| POST `/api/admin-login` | `{username, password}` | `{success:true, message:"Admin login successful.", admin}` + cookie; 401 `{error:"Invalid admin credentials."}` |
| POST `/api/admin/logout` | `{}` | `{success:true}` |
| GET `/api/admin/me` | – | `{admin: AdminUser}` |
| GET `/api/admin/students?career=&courseId=&q=&active=` | – | `{success:true, students: AdminStudent[]}` |
| POST `/api/admin/update-student-data` | `{id, tareas, monedas, asistencias, games, exp}` | `{success:true, message, newLevel, auditId}` |

- Admin login limits: 10 failures/15 min per username, 30 attempts/15 min per IP.
- On boot, if `admins` is empty, create `ADMIN_USERNAME` (default `Admin`) with
  `ADMIN_PASSWORD`; if that env is empty generate a random one and log it once.
- Admin edits never touch `last_login_level`; the student sees the level-up GIF on the next
  login, like when the professor edited EXP in the sheet.

### 4.3 Admin panel

All under `/api/admin/`, admin cookie required (401 otherwise). Shapes in `types.ts`.

Students
- `GET students` (filters: `courseId`, `career`, `q` matches code/name/email, `active`
  = `true|false|all`, default all) → `{success, students}`.
- `POST students` `StudentInput` (+ optional `password`) → `{student, password?, auditId}`;
  when password is omitted one is generated and returned once.
- `POST students/generate` `{courseId, rows:[{code,name,sexo?,email?,password?}], batchId?}`
  (≤ 50 rows) → `{created:[{student,password}], skipped:[{code,reason}], batchId}`.
- `PATCH students/{id}` partial `StudentInput` → `{student, auditId}`.
- `DELETE students/{id}` → `{success, auditId}` (hard delete; the audit snapshot keeps the
  row and its purchases so undo restores them).
- `POST students/{id}/password` `{password?}` → `{password, auditId}`.
- `POST students/bulk` `BulkStudentsRequest` (≤ 500 ids) → `{updated, batchId}`.
  Actions: `delta` (add/subtract counters, never below 0), `set`, `activate`, `deactivate`,
  `move` (to `courseId`; skip conflicts), `delete`.

Courses / badges / visibility / levels / avatars
- `GET courses` → `{courses: AdminCourse[]}`; `POST courses`, `PATCH courses/{id}` →
  `{course, auditId}`; `DELETE courses/{id}` → 409 when it has students
  (`El curso tiene alumnos. Muévelos o desactiva el curso.`);
  `PUT courses/{id}/hidden-badges {badgeIds}` → `{course, auditId}`;
  `POST courses/reorder {ids}` → `{success, batchId}`.
- `GET badges` → `{badges: AdminBadge[]}`; `POST badges`, `PATCH badges/{id}` →
  `{badge, auditId}` (`hiddenCourseIds` sets visibility; on create, courses whose
  `hiddenBadgeKeywords` match also hide it); `DELETE badges/{id}`;
  `POST badges/reorder {ids}`.
- `GET levels` → `{levels}`; `PUT levels {levels}` → `{levels, auditId}`. Rules: levels are
  0..n contiguous, level 0 has minExp 0, minExp strictly increasing, 1–20 levels.
- `GET avatars?code=` → `{avatars: Avatar[]}` (no code = all);
  `PUT avatars {code, level, url}` → `{avatar, auditId}`;
  `DELETE avatars?code=&level=` → `{success, auditId}`.

Purchases, settings, admins, uploads, stats
- `GET purchases?courseId=&studentId=&badgeId=&q=&reverted=all|true|false&limit=` (default
  limit 500, newest first) → `{purchases: AdminPurchase[]}`.
- `POST purchases/{id}/revert` (refund coins, restock if the badge still exists, set
  reverted_at) and `POST purchases/{id}/restore` (re-apply; 400 if coins/stock no longer
  suffice) → `{purchase, auditId}`.
- `GET settings` → `{settings: AdminSettings}` (secrets only as status);
  `PATCH settings` `SettingsPatch` → `{settings, auditId}`. Secret fields accept a string to
  set or `null` to clear. Validate: email list, regex compiles in Go RE2 and in JS terms
  (keep to simple syntax), TTL 1–120, urls start with `/media/` or are empty.
- `GET admins` → `{admins}`; `POST admins {username,password}`;
  `PATCH admins/{id} {username?, password?, active?}`; `DELETE admins/{id}` → `{admin|success,
  auditId}`. Never deactivate/delete yourself or the last active admin. Password ≥ 8 chars.
- `POST uploads {filename, contentType, size}` → `UploadTicket`. Allowed types png, jpeg,
  gif, webp; size ≤ 20 MB. Key: `u/{yyyy}/{mm}/{random-hex-16}.{ext}`. The browser PUTs the
  file to `uploadUrl` with the returned headers, then saves `url` (`/media/u/...`) on the
  entity. Entity endpoints accept image urls that are empty or start with `/media/`; for
  `/media/u/` urls they confirm the object exists via `Storage.Stat`.
- `PUT /api/uploads/local?token=` (local driver only, not under `/api/admin`, token is the
  auth) stores the raw body.
- `GET stats` → `AdminStats`.

Activity log
- `GET audit?entity=&q=&actorType=&batchId=&limit=&beforeId=` → `{entries, nextBeforeId}`.
- `POST audit/{id}/undo {force?}`, `POST audit/{id}/redo {force?}` → `{success, auditId,
  summary}`; 409 `{error, conflict:true, fields}` when the record changed since.
- `POST audit/batch/{batchId}/undo|redo {force?}` → `{success, done, failed:[{id,error}]}`.

Import (B3)
- `GET import/status` → `ImportStatus`.
- `POST import/preview {spreadsheetId?, credentialsJson?}` → `ImportReport` (dry run,
  synchronous).
- `POST import/run {spreadsheetId?, credentialsJson?, include, mode}` → 202
  `{jobId}`; `GET import/jobs/{jobId}` → `ImportJob`. One job at a time (409 otherwise).
- `POST import/media` → `{media, warnings, batchId}`.

## 5. Backend internals

### 5.1 Config (env)
`PORT`=8080, `DATABASE_URL` (required), `SESSION_SECRET` (required, ≥ 32 chars),
`PUBLIC_URL`=http://localhost:5173, `COOKIE_SECURE`=false, `TRUST_PROXY_HEADERS`=false,
`WEB_DIR`=./web, `STORAGE_DRIVER`=local|s3, `MEDIA_DIR`=./data/media, `MEDIA_SEED_DIR`
(optional, local fallback for reads, e.g. ../seed/media), `S3_BUCKET`, `S3_PREFIX`=media/,
`AWS_REGION`=us-east-1, `UPLOAD_ORIGIN` (S3 endpoint origin for CSP connect-src, optional),
`ADMIN_USERNAME`=Admin, `ADMIN_PASSWORD`, `RESEND_API_KEY` and `GOOGLE_SERVICE_ACCOUNT`
(fallbacks when not set in the UI).

### 5.2 Signatures BF must provide (used by B1/B2/B3)
```go
type App struct {
    Cfg     Config
    DB      *pgxpool.Pool
    Store   Storage
    Limiter *Limiter
    Log     *slog.Logger
    Mail    Mailer          // set by New(); B1 implements resendMailer in mail.go
    // BF may add private fields (settings cache, levels cache, import jobs registry hooks)
}
func New(ctx context.Context, cfg Config, db *pgxpool.Pool, log *slog.Logger) (*App, error)
func (a *App) Routes() http.Handler

// http.go
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool // writes 400 on error
func writeJSON(w http.ResponseWriter, status int, v any)
func writeError(w http.ResponseWriter, status int, msg string)
func clientIP(r *http.Request, trustProxy bool) string
func pathID(r *http.Request, name string) (int64, bool)               // r.PathValue + parse

// auth.go
type Session struct{ Kind string; ID int64; Exp int64 }               // Kind "alumno"|"admin"
func (a *App) setSession(w http.ResponseWriter, kind string, id int64)
func (a *App) clearSession(w http.ResponseWriter, kind string)
func (a *App) studentID(r *http.Request) (int64, bool)
func (a *App) adminID(r *http.Request) (int64, bool)
func (a *App) requireStudent(h func(w http.ResponseWriter, r *http.Request, studentID int64)) http.HandlerFunc
func (a *App) requireAdmin(h func(w http.ResponseWriter, r *http.Request, adminID int64)) http.HandlerFunc
func hashPassword(pw string) (string, error)                          // bcrypt cost 10
func checkPassword(hash, pw string) bool
func randomToken(nBytes int) string                                   // hex
// requireStudent/requireAdmin also verify the row still exists and is active.

// ratelimit.go
func (l *Limiter) Allow(key string, max int, window time.Duration) bool   // counts attempts
func (l *Limiter) Fail(key string, max int, window time.Duration) bool    // counts failures; false when over
func (l *Limiter) Blocked(key string, max int, window time.Duration) bool // check without counting
func (l *Limiter) Reset(key string)

// settings.go
type Settings struct { /* every field of AdminSettings, plain values, plus: */
    ResendAPIKey string; GoogleServiceAccount string  // decrypted, "" when unset (env fallback applied)
    ResendSource, GoogleSource string                  // "db"|"env"|"none"
}
func (a *App) Settings(ctx context.Context) (Settings, error)         // cached, invalidated on write
func (a *App) SaveSettings(ctx context.Context, tx pgx.Tx, patch map[string]any) error // keys = settings keys below; secrets encrypted here
func (a *App) invalidateSettings()
// Settings keys (DB key → default):
// registration_enabled true, password_reset_enabled true, store_enabled true,
// leaderboard_enabled true, level_up_animation_enabled true, purchase_email_enabled true,
// email_from "onboarding@resend.dev", email_cc ["fcal@up.edu.mx"],
// student_email_domain "up.edu.mx", password_pattern "^[A-Z0-9]{4}$",
// password_hint_register "Password (2 uppercase letters and 2 numbers)",
// password_hint_reset "New password (4 uppercase letters/numbers)",
// password_error "Invalid password. Use only 4 uppercase letters/numbers.",
// reset_code_ttl_minutes 10, level_up_gif_url "/media/legacy/gifs/levelup.gif",
// spreadsheet_id "1I6pVLSBav-U7c86FLavh0tikPghLDVrWCFuru-qwQ4Y",
// resend_api_key (secret), google_service_account (secret)
// Secrets: AES-256-GCM, key = SHA-256("diseno-settings:" + SESSION_SECRET), stored as JSON
// string "enc:v1:" + base64(nonce|ciphertext).

// levels.go
type Level struct{ Level int `json:"level"`; MinExp int `json:"minExp"` }
func (a *App) Levels(ctx context.Context) ([]Level, error)            // cached, sorted
func (a *App) invalidateLevels()
func CalcLevel(exp int, levels []Level) int

// audit.go
type AuditEntry struct {
    ActorType  string  // "admin" | "student" | "system"
    ActorID    *int64
    ActorLabel string  // username, student code, or "sistema"
    Action     string  // create|update|delete|purchase|revert|restore|import|undo|redo|login...
    Entity     string  // student|course|badge|visibility|level|avatar|setting|admin|purchase|import
    EntityID   string  // "" when not applicable; avatar = "code:level"
    Summary    string  // Spanish, human readable
    Before     any     // snapshot or nil (marshalled to jsonb)
    After      any
    BatchID    string  // "" or uuid
    Undoable   bool
    RevertsID  *int64
}
func (a *App) Audit(ctx context.Context, q DBTX, e AuditEntry) (int64, error)
type DBTX interface { Exec(...); Query(...); QueryRow(...) } // satisfied by pgxpool.Pool and pgx.Tx
func newBatchID() string                                               // uuid v4 string
// Snapshot helper (rows as JSON objects with snake_case column names, as to_jsonb returns):
func snapshotRow(ctx context.Context, q DBTX, table string, id any) (map[string]any, error)

// storage.go
type ObjectInfo struct{ Key string; Size int64; ContentType string }
type Storage interface {
    PresignUpload(ctx context.Context, key, contentType string, size int64) (url string, headers map[string]string, err error)
    Stat(ctx context.Context, key string) (ObjectInfo, error)   // ErrNotFound when missing
    List(ctx context.Context, prefix string) ([]ObjectInfo, error)
}
var ErrNotFound = errors.New("not found")
func mediaURL(key string) string            // "/media/" + key
func mediaKey(url string) (string, bool)    // inverse; false if not a /media/ url

// util.go
func normalizeLabel(s string) string        // legacy normalizeLabel: NFD strip accents, lower, [_-]→space, collapse
func slugify(name string) string            // legacy badge file name: lower, spaces→_, strip accents, keep [a-z0-9_]
func parseIntLoose(s string) int            // JS parseInt semantics, NaN → 0
func randomDigits(n int) string
func generatePassword(pattern string) (string, error) // 2 letters + 2 digits shuffled, must match pattern
```
`Mailer` (B1, mail.go): `type Mailer interface{ Send(ctx context.Context, m Mail) error }`,
`type Mail struct{ From string; To []string; Subject, HTML, Text string }`. BF declares the
interface and a `noopMailer` in app.go so things compile before B1 lands; B1 provides
`newResendMailer(a *App) Mailer` and BF's `New()` calls it (B1 also edits that one line).

### 5.3 Routes
BF registers every route of §4 in `Routes()` pointing at methods named after the handler,
e.g. `a.handleLogin`, `a.handleAdminStudentsList`, and creates **stub implementations**
(`writeError(w, 501, "pendiente")`) in the owner's file so the package compiles. Owners
replace their stubs. Handler names are fixed in `routes_contract.md` (written by BF next to
this file) so owners know exactly which methods to implement.

### 5.4 Middleware order
recover → request log (method, path, status, duration; never log bodies or secrets) →
security headers (`X-Content-Type-Options: nosniff`, `Referrer-Policy:
strict-origin-when-cross-origin`, `X-Frame-Options: DENY`, CSP below) → body limit →
CSRF/origin check for /api mutations → mux.
CSP: `default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'
https://fonts.googleapis.com; font-src https://fonts.gstatic.com; connect-src 'self'
{UPLOAD_ORIGIN}; frame-ancestors 'none'; base-uri 'self'; form-action 'self'`.

### 5.5 Static serving (web.go)
- `/assets/*` from WEB_DIR with `Cache-Control: public, max-age=31536000, immutable`.
- Other existing files in WEB_DIR: `max-age=3600`. `index.html`: `no-cache`.
- Any other GET that is not `/api/...` or `/media/...` → `index.html` (SPA fallback).
- `GET /media/*` only when `STORAGE_DRIVER=local`: serve from MEDIA_DIR, then MEDIA_SEED_DIR.
  In production CloudFront serves `/media/*` from S3; the app never sees it.

### 5.6 Audit and undo (B2 implements undo.go; BF only records)
`audit_log(id, at, actor_type, actor_id, actor_label, action, entity, entity_id, summary,
before jsonb, after jsonb, batch_id uuid, undoable bool, undone_at, undone_by, reverts_id)`.
Snapshot formats (what goes in before/after):
- student/course/badge/admin/avatar: full row as `to_jsonb(row)` (snake_case).
  Student delete: `{"row": {...}, "purchases": [...]}`. Badge delete:
  `{"row": {...}, "hidden_course_ids": [...], "purchase_ids": [...]}`. Course delete:
  `{"row": {...}, "hidden_badge_ids": [...]}`.
- visibility: `{"course_id": n, "badge_ids": [...]}`.
- levels: `[{"level":0,"min_exp":0}, ...]`.
- setting: `{"key": value, ...}` with only the keys that changed; secrets recorded as
  `{"resend_api_key": {"configured": true}}` and marked not undoable when only secrets changed.
- purchase: full purchase row plus `{"student_coins": n, "badge_stock": n}` at that moment.
Undo semantics:
- create → delete the row (409 if other rows depend on it, e.g. student with purchases,
  unless force: then delete with dependents and keep them in the undo entry's snapshot).
- update → for each field that differs between before and after, current must equal after
  (else 409 conflict listing fields, unless force) then set back to before.
- delete → re-insert the snapshot (409 if the key is taken).
- purchase/restore → revert; revert → restore.
- Redo applies the opposite. Each undo/redo writes its own entry (`action` undo|redo,
  `reverts_id`), toggles `undone_at` on the original, and is itself not undoable.
- Batch undo/redo processes entries newest first, continues on errors, reports failures.
- Audit API never returns `password_hash` or secret values (strip keys named
  `password_hash`, `code_hash`).

## 6. Frontend internals

### 6.1 Contract
`src/lib/types.ts` holds every request/response type. `src/lib/api.ts` (FF) exports:
```ts
export class ApiError extends Error { status: number; data: unknown }
export async function apiGet<T>(path: string): Promise<T>
export async function apiSend<T>(method: 'POST'|'PUT'|'PATCH'|'DELETE', path: string, body?: unknown): Promise<T>
export async function uploadImage(file: File): Promise<string>  // returns /media/... url
export const api = { config(), login(b), me(), logout(), register(b), sendResetCode(b),
  resetPassword(b), badges(career), purchase(b), leaderboard(), admin: { login(b), logout(),
  me(), ...one method per admin endpoint } }
```
Legacy student code expects raw JSON even on errors (`data.error`), so `api` student helpers
return the parsed body for any status instead of throwing (keep legacy `alert(data.error)`
flows working); admin helpers throw `ApiError` with the server's `error` message.
`uploadImage` validates type/size client-side, asks for a ticket, PUTs the file, returns url.

### 6.2 Admin UI kit (`src/admin/ui/`, FF)
Tailwind only, Poppins inherited, consistent with legacy colors (orange-500 primary,
purple-600 secondary, blue-600 admin accents, gray-50 surfaces, rounded-2xl cards, shadow).
Components (named exports, props typed):
`Button` (variants primary|secondary|ghost|danger, size sm|md, loading), `IconButton`,
`Card`, `PageHeader` (title, subtitle, actions), `Toggle` (accessible switch, label),
`TextField`, `NumberField`, `TextArea`, `Select`, `SearchInput`, `Badge`/`Pill`,
`Modal` (focus trap-lite, Esc closes, aria), `Drawer` (right side panel), `ConfirmDialog`
(+ `useConfirm()` promise hook), `Toaster` + `useToast()` → `toast.success(msg, {undo?:
() => Promise<void>})` shows a "Deshacer" button when `undo` is given, `EmptyState`,
`Spinner`, `ImageUpload` (preview, drag & drop, calls `uploadImage`, shows progress
state, value = url, onChange(url), allows clearing), `DataTable` (columns config, sticky
header, row selection with checkboxes, empty state, responsive horizontal scroll),
`Tabs`, `Stat` (KPI tile).
Hooks: `useAsync(fn, deps)` → `{data, error, loading, reload}`, `useUndo()` →
`(ref: {auditId?: number; batchId?: string}) => Promise<void>` that calls the audit
undo endpoint and toasts the result (handles 409 by asking to force).

### 6.3 Admin pages (Spanish)
Sidebar order: Resumen, Alumnos, Generar alumnos, Avatares, Cursos, Insignias, Compras,
Niveles, Importar, Actividad, Ajustes, Administradores. Every list has search/filters,
toggles for `active`, and every mutation shows a toast with "Deshacer".
- Resumen: KPIs, per-course table, low stock badges, recent activity with undo.
- Alumnos: course tabs/filter, search, active filter, table (avatar thumb, ID, nombre, nivel,
  EXP, monedas, tareas, asistencias, juegos, insignias, activo toggle, acciones), row click
  opens `StudentDrawer` (all fields incl. comentarios, curso, sexo, correo; avatar slots per
  level with upload/replace/remove; reset password showing the new one once; delete with
  confirm). Bulk bar for selected rows: sumar/restar counters, fijar, activar, desactivar,
  mover de curso, eliminar, exportar CSV. "Nuevo alumno" button.
- Generar alumnos: pick course, paste lines (`ID, Nombre, Sexo[, correo][, contraseña]`,
  comma/tab/semicolon separated, header optional), preview with validation, create in chunks
  of 50 under one batchId, result table with generated passwords + copy + CSV download +
  "Deshacer todo".
- Avatares: defaults (`DefaultH`, `DefaultF`) per level, plus every student code with
  coverage per level (missing ones highlighted), upload per slot.
- Cursos: list with active toggle ("Visible en el login"), reorder up/down, drawer: nombre,
  hoja de Sheets + alias, imagen lateral, logo, palabras para ocultar insignias nuevas,
  checklist of visible badges, delete (blocked when it has students).
- Insignias: cards/table with image, nombre, costo, stock (+/-), activa toggle, visible por
  curso (chips), create/edit drawer with image upload, reorder.
- Compras: filters (curso, alumno, insignia, estado), table, "Deshacer compra" /
  "Rehacer compra", CSV export.
- Niveles: editable rows level/minExp, add/remove last, validation, save.
- Importar: spreadsheet ID, credentials status (+ upload a service-account .json once, or save
  it in Ajustes), preview report, run (include checkboxes, mode upsert/solo nuevos) with job
  polling and final report, "Importar imágenes heredadas", undo whole import.
- Actividad: filters, list with expandable before/after diff, Deshacer/Rehacer per entry,
  batch undo.
- Ajustes: feature toggles, email settings (remitente, copias, dominio, API key Resend write
  only), password rule + texts, TTL, level-up GIF upload, Google Sheets ID + credentials.
- Administradores: list, active toggle, create, change password, delete.

## 7. Legacy bugs to fix during the port (and nothing else)
- AdminDashboard called hooks after an early return (rules of hooks).
- After a purchase the UI read `data.newBadgeQuantity` (undefined); use the response.
- Purchase errors were ignored (UI crashed reading `data.newCoins`); alert the error instead.
- `/api/purchase` trusted client cost and studentId; server now uses the session and DB cost.
- Admin endpoints had no auth; admin password was hardcoded and public.
- Passwords stored in plain text; now bcrypt (imported plain values get hashed).
- Register form used `document.getElementById`; use controlled inputs (same markup).
- Max-level EXP bar showed empty at exactly the top threshold; show it full at max level.
