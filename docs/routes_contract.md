# Routes contract

Every route is registered in `backend/internal/app/app.go` (`Routes()`), pointing at the
method named below. Owners replace the stubs in their own files; method names and
signatures must stay exactly as listed, or the package stops compiling.

Handler signatures by wrapper:

| Wrapper | Signature |
|---|---|
| none | `func (a *App) handleX(w http.ResponseWriter, r *http.Request)` |
| `requireStudent` | `func (a *App) handleX(w http.ResponseWriter, r *http.Request, studentID int64)` |
| `requireAdmin` | `func (a *App) handleX(w http.ResponseWriter, r *http.Request, adminID int64)` |

`requireStudent` / `requireAdmin` already checked the cookie and that the row exists with
`active = true`. Otherwise they answered 401 `Sesión expirada. Inicia sesión de nuevo.` and
cleared the cookie.

## Routes

| METHOD path | Handler method | Wrapper | Owner file |
|---|---|---|---|
| GET `/api/health` | `handleHealth` | none | app.go (BF, done) |
| GET `/api/config` | `handleConfig` | none | student.go (B1) |
| POST `/api/login` | `handleLogin` | none | student.go (B1) |
| GET `/api/me` | `handleMe` | requireStudent | student.go (B1) |
| POST `/api/logout` | `handleLogout` | none | student.go (B1) |
| POST `/api/register` | `handleRegister` | none | student.go (B1) |
| POST `/api/send-reset-code` | `handleSendResetCode` | none | student.go (B1) |
| POST `/api/reset-password-with-code` | `handleResetPassword` | none | student.go (B1) |
| GET `/api/badges` | `handleBadges` | none (use `a.studentID(r)` when `career` is missing) | student.go (B1) |
| POST `/api/purchase` | `handlePurchase` | requireStudent | student.go (B1) |
| GET `/api/leaderboard` | `handleLeaderboard` | requireStudent | student.go (B1) |
| POST `/api/admin-login` | `handleAdminLogin` | none (public) | admin_core.go (B2) |
| POST `/api/admin/logout` | `handleAdminLogout` | requireAdmin | admin_core.go (B2) |
| GET `/api/admin/me` | `handleAdminMe` | requireAdmin | admin_core.go (B2) |
| GET `/api/admin/students` | `handleAdminStudentsList` | requireAdmin | admin_core.go (B2) |
| POST `/api/admin/update-student-data` | `handleAdminUpdateStudentData` | requireAdmin | admin_core.go (B2) |
| GET `/api/admin/stats` | `handleAdminStats` | requireAdmin | admin_catalog.go (B2b) |
| POST `/api/admin/students` | `handleAdminStudentCreate` | requireAdmin | admin_students.go (B2) |
| POST `/api/admin/students/generate` | `handleAdminStudentsGenerate` | requireAdmin | admin_students.go (B2) |
| POST `/api/admin/students/bulk` | `handleAdminStudentsBulk` | requireAdmin | admin_students.go (B2) |
| PATCH `/api/admin/students/{id}` | `handleAdminStudentUpdate` | requireAdmin | admin_students.go (B2) |
| DELETE `/api/admin/students/{id}` | `handleAdminStudentDelete` | requireAdmin | admin_students.go (B2) |
| POST `/api/admin/students/{id}/password` | `handleAdminStudentPassword` | requireAdmin | admin_students.go (B2) |
| GET `/api/admin/courses` | `handleAdminCoursesList` | requireAdmin | admin_catalog.go (B2) |
| POST `/api/admin/courses` | `handleAdminCoursesCreate` | requireAdmin | admin_catalog.go (B2) |
| POST `/api/admin/courses/reorder` | `handleAdminCoursesReorder` | requireAdmin | admin_catalog.go (B2) |
| PATCH `/api/admin/courses/{id}` | `handleAdminCoursesUpdate` | requireAdmin | admin_catalog.go (B2) |
| DELETE `/api/admin/courses/{id}` | `handleAdminCoursesDelete` | requireAdmin | admin_catalog.go (B2) |
| PUT `/api/admin/courses/{id}/hidden-badges` | `handleAdminCoursesHiddenBadges` | requireAdmin | admin_catalog.go (B2) |
| GET `/api/admin/badges` | `handleAdminBadgesList` | requireAdmin | admin_catalog.go (B2) |
| POST `/api/admin/badges` | `handleAdminBadgesCreate` | requireAdmin | admin_catalog.go (B2) |
| POST `/api/admin/badges/reorder` | `handleAdminBadgesReorder` | requireAdmin | admin_catalog.go (B2) |
| PATCH `/api/admin/badges/{id}` | `handleAdminBadgesUpdate` | requireAdmin | admin_catalog.go (B2) |
| DELETE `/api/admin/badges/{id}` | `handleAdminBadgesDelete` | requireAdmin | admin_catalog.go (B2) |
| GET `/api/admin/levels` | `handleAdminLevelsGet` | requireAdmin | admin_catalog.go (B2) |
| PUT `/api/admin/levels` | `handleAdminLevelsPut` | requireAdmin | admin_catalog.go (B2) |
| GET `/api/admin/avatars` | `handleAdminAvatarsList` | requireAdmin | admin_catalog.go (B2) |
| PUT `/api/admin/avatars` | `handleAdminAvatarsPut` | requireAdmin | admin_catalog.go (B2) |
| DELETE `/api/admin/avatars` | `handleAdminAvatarsDelete` | requireAdmin | admin_catalog.go (B2) |
| GET `/api/admin/purchases` | `handleAdminPurchasesList` | requireAdmin | admin_misc.go (B2) |
| POST `/api/admin/purchases/{id}/revert` | `handleAdminPurchasesRevert` | requireAdmin | admin_misc.go (B2) |
| POST `/api/admin/purchases/{id}/restore` | `handleAdminPurchasesRestore` | requireAdmin | admin_misc.go (B2) |
| GET `/api/admin/settings` | `handleAdminSettingsGet` | requireAdmin | admin_misc.go (B2) |
| PATCH `/api/admin/settings` | `handleAdminSettingsPatch` | requireAdmin | admin_misc.go (B2) |
| GET `/api/admin/admins` | `handleAdminAdminsList` | requireAdmin | admin_misc.go (B2) |
| POST `/api/admin/admins` | `handleAdminAdminsCreate` | requireAdmin | admin_misc.go (B2) |
| PATCH `/api/admin/admins/{id}` | `handleAdminAdminsUpdate` | requireAdmin | admin_misc.go (B2) |
| DELETE `/api/admin/admins/{id}` | `handleAdminAdminsDelete` | requireAdmin | admin_misc.go (B2) |
| POST `/api/admin/uploads` | `handleAdminUpload` | requireAdmin | admin_misc.go (B2) |
| GET `/api/admin/audit` | `handleAdminAuditList` | requireAdmin | undo.go (B2) |
| POST `/api/admin/audit/{id}/undo` | `handleAdminAuditUndo` | requireAdmin | undo.go (B2) |
| POST `/api/admin/audit/{id}/redo` | `handleAdminAuditRedo` | requireAdmin | undo.go (B2) |
| POST `/api/admin/audit/batch/{batchId}/undo` | `handleAdminAuditBatchUndo` | requireAdmin | undo.go (B2) |
| POST `/api/admin/audit/batch/{batchId}/redo` | `handleAdminAuditBatchRedo` | requireAdmin | undo.go (B2) |
| GET `/api/admin/import/status` | `handleAdminImportStatus` | requireAdmin | importer.go (B3) |
| POST `/api/admin/import/preview` | `handleAdminImportPreview` | requireAdmin | importer.go (B3) |
| POST `/api/admin/import/run` | `handleAdminImportRun` | requireAdmin | importer.go (B3) |
| GET `/api/admin/import/jobs/{jobId}` | `handleAdminImportJob` | requireAdmin | importer.go (B3) |
| POST `/api/admin/import/media` | `handleAdminImportMedia` | requireAdmin | importer.go (B3) |
| PUT `/api/uploads/local?token=` | `handleLocalUpload` | none, the token is the auth (local driver only) | storage.go (BF, done) |
| GET `/media/*` | `mediaHandler` | none (local driver only) | web.go (BF, done) |
| any other `/api/*` | 404 `{"error":"Ruta no encontrada."}` | none | app.go (BF, done) |
| GET `/*` | SPA (`spaHandler`) | none | web.go (BF, done) |

Path values: `pathID(r, "id")` for `{id}` (false when missing, not a number or ≤ 0),
`r.PathValue("batchId")` and `r.PathValue("jobId")` for the others.

`handleAdminStudentsList` and `handleAdminUpdateStudentData` sit in admin_core.go because
spec §4.2 lists them with the quick dashboard. B2 can move them to admin_students.go; just
delete the stub so the method isn't declared twice.

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

## Helpers BF provides (see spec §5.2 for full signatures)

- Sessions: `a.setSession(w, kindStudent|kindAdmin, id)` and `a.clearSession(w, kind)`,
  where the constants are `"alumno"` and `"admin"`. `a.studentID(r)` / `a.adminID(r)` only
  check the cookie and never touch the DB.
- Passwords: `hashPassword` (bcrypt, cost 10) and `checkPassword(hash, pw)`. Pass
  `hash = ""` when the user doesn't exist: it still burns bcrypt time and returns false, so
  response timing doesn't reveal which IDs exist.
- Rate limits: one window per key, so namespace keys (`"login:" + courseID + ":" + code`,
  `"login-ip:" + ip`). `Allow` counts and checks (`count < max`). `Fail` records a failure
  and returns false once this failure reaches `max`. `Blocked` is `count >= max` and records
  nothing. `Reset` forgets the key. Typical login: `Blocked` first (answer 429), then `Fail`
  on a wrong password and `Reset` on success. Get the IP with
  `clientIP(r, a.Cfg.TrustProxy)`.
- Settings: `a.Settings(ctx)` returns a copy. Its struct has JSON tags equal to
  `AdminSettings`, with the secrets tagged `json:"-"`, so B2 can embed `Settings` in the
  response and add `resendApiKey` / `googleServiceAccount` status fields. `a.SaveSettings(ctx,
  tx, patch)` takes snake_case DB keys, checks each value fits the field type, and encrypts
  secrets. `nil` deletes the override (back to the default), and for secrets `""` deletes
  too. Range and format checks (TTL 1–120, email lists, regex, `/media/` urls) are B2's job.
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
- Mail (B1): `mail.go` defines `func newResendMailer(a *App) Mailer`, and `New()` already
  calls it, so app.go needs no edit. Until then `noopMailer` is used.
- Import (B3): `a.importMu` (`sync.Mutex`) and `a.importJobs` (`sync.Map`) are reserved for
  the one-job-at-a-time registry. Package-level state in importer.go works too.

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
`DATABASE_URL=postgres://localhost:5432/<your_db>?sslmode=disable go test ./...`.
