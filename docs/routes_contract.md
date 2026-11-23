# Routes contract

Implemented routes are registered in `backend/internal/app/app.go` (`Routes()`), pointing
at the methods below. When changing a route, update its handler, the TypeScript client and
this table together. Start with [AGENTS.md](../AGENTS.md), [architecture](arquitectura.md)
and [validation](validacion.md). There are no migration stubs left to replace.

Handler signatures by wrapper:

| Wrapper | Signature |
|---|---|
| none | `func (a *App) handleX(w http.ResponseWriter, r *http.Request)` |
| `requireStudent` | `func (a *App) handleX(w http.ResponseWriter, r *http.Request, studentID int64)` |
| `requireAdmin` | `func (a *App) handleX(w http.ResponseWriter, r *http.Request, adminID int64)` |

`requireStudent` / `requireAdmin` already checked the cookie, that the row exists with
`active = true`, and that the password hasn't changed since the cookie was issued. Otherwise
they answered 401 `Sesión expirada. Inicia sesión de nuevo.` and cleared the cookie.

## Routes

| METHOD path | Handler method | Wrapper | Source file |
|---|---|---|---|
| GET `/api/health` | `handleHealth` | none | app.go |
| GET `/api/config` | `handleConfig` | none | student.go |
| POST `/api/login` | `handleLogin` | none | student.go |
| GET `/api/me` | `handleMe` | requireStudent | student.go |
| POST `/api/switch-course` | `handleSwitchCourse` | requireStudent | student.go |
| POST `/api/logout` | `handleLogout` | none | student.go |
| POST `/api/register` | `handleRegister` | none | student.go |
| POST `/api/send-reset-code` | `handleSendResetCode` | none | student.go |
| POST `/api/reset-password-with-code` | `handleResetPassword` | none | student.go |
| GET `/api/badges` | `handleBadges` | none (use `a.studentID(r)` when `career` is missing) | student.go |
| POST `/api/purchase` | `handlePurchase` | requireStudent | student.go |
| GET `/api/leaderboard` | `handleLeaderboard` | requireStudent | student.go |
| POST `/api/admin-login` | `handleAdminLogin` | none (public) | admin_core.go |
| POST `/api/admin/logout` | `handleAdminLogout` | requireAdmin | admin_core.go |
| GET `/api/admin/me` | `handleAdminMe` | requireAdmin | admin_core.go |
| GET `/api/admin/students` | `handleAdminStudentsList` | requireAdmin | admin_core.go |
| POST `/api/admin/update-student-data` | `handleAdminUpdateStudentData` | requireAdmin | admin_core.go |
| GET `/api/admin/stats` | `handleAdminStats` | requireAdmin | admin_catalog.go |
| POST `/api/admin/students` | `handleAdminStudentCreate` | requireAdmin | admin_students.go |
| POST `/api/admin/students/generate` | `handleAdminStudentsGenerate` | requireAdmin | admin_students.go |
| POST `/api/admin/students/bulk` | `handleAdminStudentsBulk` | requireAdmin | admin_students.go |
| PATCH `/api/admin/students/{id}` | `handleAdminStudentUpdate` | requireAdmin | admin_students.go |
| DELETE `/api/admin/students/{id}` | `handleAdminStudentDelete` | requireAdmin | admin_students.go |
| POST `/api/admin/students/{id}/password` | `handleAdminStudentPassword` | requireAdmin | admin_students.go |
| GET `/api/admin/courses` | `handleAdminCoursesList` | requireAdmin | admin_catalog.go |
| POST `/api/admin/courses` | `handleAdminCoursesCreate` | requireAdmin | admin_catalog.go |
| POST `/api/admin/courses/reorder` | `handleAdminCoursesReorder` | requireAdmin | admin_catalog.go |
| PATCH `/api/admin/courses/{id}` | `handleAdminCoursesUpdate` | requireAdmin | admin_catalog.go |
| DELETE `/api/admin/courses/{id}` | `handleAdminCoursesDelete` | requireAdmin | admin_catalog.go |
| PUT `/api/admin/courses/{id}/hidden-badges` | `handleAdminCoursesHiddenBadges` | requireAdmin | admin_catalog.go |
| GET `/api/admin/badges` | `handleAdminBadgesList` | requireAdmin | admin_catalog.go |
| POST `/api/admin/badges` | `handleAdminBadgesCreate` | requireAdmin | admin_catalog.go |
| POST `/api/admin/badges/reorder` | `handleAdminBadgesReorder` | requireAdmin | admin_catalog.go |
| PATCH `/api/admin/badges/{id}` | `handleAdminBadgesUpdate` | requireAdmin | admin_catalog.go |
| DELETE `/api/admin/badges/{id}` | `handleAdminBadgesDelete` | requireAdmin | admin_catalog.go |
| GET `/api/admin/levels` | `handleAdminLevelsGet` | requireAdmin | admin_catalog.go |
| PUT `/api/admin/levels` | `handleAdminLevelsPut` | requireAdmin | admin_catalog.go |
| GET `/api/admin/avatars` | `handleAdminAvatarsList` | requireAdmin | admin_catalog.go |
| PUT `/api/admin/avatars` | `handleAdminAvatarsPut` | requireAdmin | admin_catalog.go |
| DELETE `/api/admin/avatars` | `handleAdminAvatarsDelete` | requireAdmin | admin_catalog.go |
| GET `/api/admin/purchases` | `handleAdminPurchasesList` | requireAdmin | admin_misc.go |
| POST `/api/admin/purchases/{id}/revert` | `handleAdminPurchasesRevert` | requireAdmin | admin_misc.go |
| POST `/api/admin/purchases/{id}/restore` | `handleAdminPurchasesRestore` | requireAdmin | admin_misc.go |
| GET `/api/admin/settings` | `handleAdminSettingsGet` | requireAdmin | admin_misc.go |
| PATCH `/api/admin/settings` | `handleAdminSettingsPatch` | requireAdmin | admin_misc.go |
| GET `/api/admin/admins` | `handleAdminAdminsList` | requireAdmin | admin_misc.go |
| POST `/api/admin/admins` | `handleAdminAdminsCreate` | requireAdmin | admin_misc.go |
| PATCH `/api/admin/admins/{id}` | `handleAdminAdminsUpdate` | requireAdmin | admin_misc.go |
| DELETE `/api/admin/admins/{id}` | `handleAdminAdminsDelete` | requireAdmin | admin_misc.go |
| POST `/api/admin/uploads` | `handleAdminUpload` | requireAdmin | admin_misc.go |
| GET `/api/admin/audit` | `handleAdminAuditList` | requireAdmin | undo.go |
| POST `/api/admin/audit/{id}/undo` | `handleAdminAuditUndo` | requireAdmin | undo.go |
| POST `/api/admin/audit/{id}/redo` | `handleAdminAuditRedo` | requireAdmin | undo.go |
| POST `/api/admin/audit/batch/{batchId}/undo` | `handleAdminAuditBatchUndo` | requireAdmin | undo.go |
| POST `/api/admin/audit/batch/{batchId}/redo` | `handleAdminAuditBatchRedo` | requireAdmin | undo.go |
| GET `/api/admin/import/status` | `handleAdminImportStatus` | requireAdmin | importer.go |
| POST `/api/admin/import/preview` | `handleAdminImportPreview` | requireAdmin | importer.go |
| POST `/api/admin/import/run` | `handleAdminImportRun` | requireAdmin | importer.go |
| GET `/api/admin/import/jobs/{jobId}` | `handleAdminImportJob` | requireAdmin | importer.go |
| POST `/api/admin/import/media` | `handleAdminImportMedia` | requireAdmin | importer.go |
| PUT `/api/uploads/local?token=` | `handleLocalUpload` | none, the token is the auth (local driver only) | storage.go |
| GET `/media/*` | `mediaHandler` | none (local driver only) | web.go |
| any other `/api/*` | 404 `{"error":"Ruta no encontrada."}` | none | app.go |
| GET `/*` | SPA (`spaHandler`) | none | web.go |

Path values: `pathID(r, "id")` for `{id}` (false when missing, not a number or ≤ 0),
`r.PathValue("batchId")` and `r.PathValue("jobId")` for the others.

`handleAdminStudentsList` and `handleAdminUpdateStudentData` currently live in
`admin_core.go` because the complete panel shares these handlers with the quick dashboard.
Do not create a second method with the same name.

## Student login and course selection

`POST /api/login` accepts `{studentId, password, career?}`. The normal login sends only
the student code and password. Student codes identify the same person across enrollments.
An active enrollment in an active course must validate the password; the response then
lists **all** active enrollments in active courses with that code, including enrollments
imported from Sheets with different passwords. Existing passwords stay independent: neither
login, import, the password reset nor an administrator password edit synchronizes them.
An inactive enrollment's password cannot authenticate another enrollment.

Successful login, `GET /api/me`, and `POST /api/switch-course` return:

```json
{
  "success": true,
  "student": {"id": "matricula", "career": "Selected course", "...": "existing student fields"},
  "levelUpOccurred": false,
  "courses": [{"id": 1, "name": "Course name", "sideImageUrl": "/media/...", "logoUrl": "/media/..."}],
  "requiresCourseSelection": true
}
```

The course `id` is the course ID, not the enrollment ID. One enrollment opens directly.
Multiple enrollments set `requiresCourseSelection = true`; the frontend shows course cards
before rendering a dashboard. The signed cookie keeps this pending state across reloads:
`/api/me` does not silently choose the first course. While selection is pending, the student
object is provided for compatibility but no visit or level animation is consumed.

`POST /api/switch-course` accepts `{courseId: number}`. It only selects an active enrollment
of that same code in an active course; another student's course, an inactive enrollment or
an inactive course returns 403. Success clears pending selection, returns the selected
student's own counters and purchases, and keeps that course selected on `/api/me` reloads.
The optional legacy `career` field selects the requested enrolled course directly.

The student cookie carries the current enrollment ID (`i`), the authenticating enrollment
ID (`a`), its password version (`v`), the code (`c`), expiration (`e`), and a pending selector
flag (`p`). Routes wrapped in `requireStudent` verify both enrollments still have the
same code, are active, and belong to active courses, and that the authenticating password
version still matches. Switching course keeps that source credential and the original
expiration. Resetting/changing that source password, removing or deactivating the source
enrollment, or changing its code revokes the whole session even while another course is
open. Changing a different enrollment's password does not revoke a session authenticated
with the original credential. This includes admin password edits, reset codes and imports.
Older single-enrollment cookies without `a` remain tied to their current enrollment.

`GET /api/badges` is a public catalog endpoint, not a `requireStudent` route. With an explicit
`career` it resolves that active course; without it the implementation reads the signed
cookie's enrollment ID and checks that student is active to infer the course. This branch
does not perform the complete source-password-version check described above. Purchases
still require `requireStudent` and validate authorization on the server.

## What the middleware already does

The order is recover, then request log, then security headers, then body limit, then the
CSRF check, then the mux. Handlers don't repeat any of it.

- Every POST/PUT/PATCH/DELETE under `/api/` must carry `Content-Type: application/json`,
  including requests with no body such as `DELETE /api/admin/avatars?code=&level=`; without
  it the answer is 403. Only `PUT /api/uploads/local` is exempt. When an `Origin` header is
  present it has to match `PUBLIC_URL`, and with a localhost `PUBLIC_URL` any
  `http://localhost:*` or `http://127.0.0.1:*` origin is accepted.
- Bodies are capped at 64 KB (25 MB for the local upload). `decodeJSON` answers 413 when the
  cap is hit and 400 for broken JSON. An empty body decodes as `{}`, and unknown fields are
  ignored (the legacy quick admin modal sends extras like `career` and `rowIndex`).
- `writeJSON` / `writeError` set `Cache-Control: no-store`.
- A panic becomes 500 `{"error":"Error interno del servidor."}` and the stack goes to the log.

## Shared helpers (verify exact signatures in their source files)

- Sessions: `a.setSession(w, kindStudent|kindAdmin, id, passwordHash)` and
  `a.clearSession(w, kind)`, where the constants are `"alumno"` and `"admin"`. The token
  carries `v`, a password version (first 8 bytes, hex, of HMAC-SHA256(session key,
  `"pwv:" + password_hash`)), and the `require*` wrappers reject it once the stored hash
  changes. So any password change (reset with a code, the panel setting one, an undo that
  restores an old hash, the importer writing a new one) logs out the sessions opened with the
  old password. Tokens without `v` are rejected too. When an admin changes their own password,
  `handleAdminAdminsUpdate` signs a fresh cookie so they stay in. `a.studentID(r)` /
  `a.adminID(r)` only check the cookie signature and never touch the DB.
- Passwords: `hashPassword` (bcrypt, cost 10) and `checkPassword(hash, pw)`. Pass
  `hash = ""` when the user doesn't exist: it still burns bcrypt time and returns false, so
  response timing doesn't reveal which IDs exist. Every bcrypt call in a request runs with a
  turn from `a.bcryptSem` (capacity `max(2, NumCPU)`): public routes wrap it in
  `a.publicBcrypt(w, r, fn)`, which waits up to 3 s and otherwise answers 503
  `El servidor está ocupado. Intenta de nuevo en unos segundos.`; panel and importer code use
  `a.hashPasswordCtx(ctx, pw)` or `a.withBcrypt(ctx, fn)`, which wait as long as the request
  lives.
- Rate limits: one window per key, so namespace keys. `Allow` checks and counts in one step
  (`count < max`) and `Reset` forgets the key. A login calls `Allow` before bcrypt and `Reset`
  on success; checking first and counting the failure afterwards let a parallel burst through
  whole, so that API is gone. Get the client with `clientIP(r, a.Cfg.TrustProxy)`: IPv4 as
  is, IPv6 collapsed to its /64 (one subscriber gets a whole /64, so per-address counting
  stops nobody). Current limits ("network" = IPv4 or IPv6 /64):

  | Route | Key | Limit |
  |---|---|---|
  | `POST /api/login` | network | 100 / 5 min |
  | `POST /api/login` | (code, network), all login forms | 10 / 15 min, cleared on success |
  | `POST /api/login` | code, any network and login form | 100 / 15 min |
  | `POST /api/login` with legacy career | (course, code, network) | 10 / 15 min, cleared on success |
  | `POST /api/login` with legacy career | (course, code), any network | 100 / 15 min |
  | `POST /api/admin-login` | network | 30 / 15 min |
  | `POST /api/admin-login` | (lowercased username, network) | 10 / 15 min, cleared on success |
  | `POST /api/admin-login` | lowercased username, any network | 100 / 15 min |
  | `POST /api/register` | network | 20 / hour |
  | `POST /api/register` | code already enrolled in another course | 10 / 15 min |
  | `POST /api/send-reset-code` | network | 20 / hour |
  | `POST /api/send-reset-code` | student | 3 / hour |
  | `POST /api/reset-password-with-code` | code | 5 wrong tries, then dead until it expires |
  | `POST /api/admin/import/preview` | admin | 10 / min |

  The per-account rows are checked after the per-network row, so a network that is already
  blocked doesn't use up the account's budget. A successful reset with a code clears that
  student's per-network and per-account login keys. Asking for a new reset code while the
  current one is still valid keeps its wrong-try count, and while a burnt code is still valid
  `send-reset-code` answers 429 without sending.
- Student IDs and names: new IDs (register, panel create/update/generate) must match
  `codeRe` (`^[A-Za-z0-9._-]{1,40}$`, the ID goes into `code@domain`), and names reject
  `badNameRune` (control characters and the U+202A–U+202E / U+2066–U+2069 direction marks).
  On update, an imported ID or name that isn't being changed is accepted as it is. The Sheets
  importer stays lenient but runs codes, names, sexo and emails through `stripControl`.
- Registering an ID that already exists in another course (avatars are keyed by ID) needs the
  password of one of those enrollments; otherwise 409 `Ese ID ya está registrado en otro
  curso. Usa la misma contraseña de ese curso o pide al profesor que te inscriba.`
- `Student.purchases` (login, `/api/me`, purchase) keeps the `Record<string, number>` shape
  but lists badges in the order they were first bought (lowest non-reverted purchase id),
  like the legacy portal, not alphabetically.
- Settings: `a.Settings(ctx)` returns a copy. Its struct has JSON tags equal to
  `AdminSettings`, with the secrets tagged `json:"-"`, so the admin handler can embed `Settings` in the
  response and add `resendApiKey` / `googleServiceAccount` status fields. `a.SaveSettings(ctx,
  tx, patch)` takes snake_case DB keys, checks each value fits the field type, and encrypts
  secrets. `nil` deletes the override (back to the default), and for secrets `""` deletes
  too. Range and format checks (TTL 1–120, email lists, regex, `/media/` urls) live in admin_misc.go.
  Call `a.invalidateSettings()` after the commit. `settingsFields()` lists `{Key, JSON,
  Secret}` for every key, which gives the camelCase to snake_case mapping; `settingsKeys()`
  lists just the DB keys.
- Levels: `a.Levels(ctx)` (cached, sorted) and `CalcLevel(exp, levels)`. Call
  `a.invalidateLevels()` after committing level changes. Both caches also expire on their
  own after one minute.
- Audit: `a.Audit(ctx, q, AuditEntry{...})` returns the id. `Undoable` defaults to false, so
  set it explicitly. `newBatchID()` returns a uuid v4. `snapshotRow(ctx, q, table, id)`
  works on `students`, `courses`, `badges`, `purchases` and `admins` (by `id`) and on
  `avatars`, where `id` is the string `"code:level"`. It returns `pgx.ErrNoRows` when the
  row is missing. JSON numbers come back as `float64`, the same thing you get when you scan
  `audit_log.before/after` into `map[string]any`, so the two compare cleanly.
- Storage: `a.Store.PresignUpload(ctx, key, contentType, size)` returns the upload URL and
  the headers the browser must send (signed headers minus `Host` and `Content-Length`, which
  the browser sets itself). `a.Store.Stat` returns `ErrNotFound` when the object is missing.
  `a.Store.List(ctx, prefix)` returns keys without `S3_PREFIX`. `mediaURL(key)` and
  `mediaKey(url)` convert between keys and `/media/...` urls; `mediaKey` rejects `..` and
  other invalid paths.
- Utilities: `normalizeLabel`, `slugify`, `parseIntLoose` (JavaScript `parseInt(x, 10)`,
  NaN becomes 0, out-of-range values saturate at int32 because the columns are `integer`),
  `randomDigits`, `randomToken(nBytes)` (hex) and `generatePassword(pattern)`. The last one
  produces 2 uppercase letters plus 2 digits, skips I, O, 0 and 1, and returns an error when
  the pattern can't match that shape.
- Mail: `mail.go` defines `newResendMailer(a *App) Mailer`, initialized by `New()`. Read
  mail.go for behavior when credentials are absent; a response alone does not prove delivery.
- Import: `a.importMu` (`sync.Mutex`) and `a.importJobs` (`sync.Map`) implement the in-memory
  one-job-at-a-time registry. Job status is not durable across an app restart; database data
  and audit entries are persistent.

## Test helpers (app_test.go, package `app`)

Don't redeclare these names in other test files.

- `isolatedDB(t)` gives a pgxpool on a brand-new schema with migrations and seed data
  applied. The schema is dropped at the end. The test is skipped when `DATABASE_URL` is
  unset.
- `isolatedApp(t)` builds a full `*App` on top of `isolatedDB`, with local storage in temp
  dirs.
- `appNoDB(t)` builds an `*App` with `DB == nil`, for code that doesn't touch the database.
- `freshSchema(t)` (empty schema, no migrations), `isolatedConfig(t)` and `quietLogger()`.

Run the suite with
`DATABASE_URL='postgres://diseno:diseno@localhost:5433/diseno?sslmode=disable' go test ./...`
from `backend/` against the local Compose database. See [validacion.md](validacion.md) for
setup and isolation; do not use production credentials.
