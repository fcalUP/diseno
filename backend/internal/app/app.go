package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	Cfg     Config
	DB      *pgxpool.Pool
	Store   Storage
	Limiter *Limiter
	Log     *slog.Logger
	Mail    Mailer // lo arma New(); la versión real vive en mail.go (B1)

	sessionKey  []byte
	settingsKey []byte

	// bcryptSem limita cuántos bcrypt corren a la vez (ver withBcrypt en auth.go).
	bcryptSem chan struct{}

	settingsMu sync.Mutex
	settings   *Settings
	settingsAt time.Time

	levelsMu sync.Mutex
	levels   []Level
	levelsAt time.Time

	// Para importer.go (B3): importMu asegura un trabajo a la vez e importJobs
	// guarda los trabajos por id. B3 decide qué tipo guarda ahí.
	importMu   sync.Mutex
	importJobs sync.Map
}

type Mailer interface {
	Send(ctx context.Context, m Mail) error
}

type Mail struct {
	From          string
	To            []string
	Subject, HTML string
	Text          string
}

// noopMailer no envía nada; sirve mientras no haya proveedor configurado.
type noopMailer struct{}

func (noopMailer) Send(context.Context, Mail) error { return nil }

func New(ctx context.Context, cfg Config, db *pgxpool.Pool, log *slog.Logger) (*App, error) {
	a := &App{
		Cfg:         cfg,
		DB:          db,
		Log:         log,
		Limiter:     NewLimiter(),
		sessionKey:  deriveKey("diseno-session:", cfg.SessionSecret),
		settingsKey: deriveKey("diseno-settings:", cfg.SessionSecret),
		bcryptSem:   make(chan struct{}, max(2, runtime.NumCPU())),
	}
	var err error
	if cfg.StorageDriver == "s3" {
		a.Store, err = newS3Storage(ctx, cfg)
	} else {
		a.Store, err = newLocalStorage(cfg)
	}
	if err != nil {
		return nil, err
	}
	a.Mail = newResendMailer(a)
	return a, nil
}

func deriveKey(label, secret string) []byte {
	k := sha256.Sum256([]byte(label + secret))
	return k[:]
}

// Routes registra todas las rutas del spec §4. Los nombres de los handlers
// están fijados en docs/routes_contract.md.
func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()
	st, ad := a.requireStudent, a.requireAdmin

	mux.HandleFunc("GET /api/health", a.handleHealth)

	// Público y alumno (student.go)
	mux.HandleFunc("GET /api/config", a.handleConfig)
	mux.HandleFunc("POST /api/login", a.handleLogin)
	mux.HandleFunc("GET /api/me", st(a.handleMe))
	mux.HandleFunc("POST /api/switch-course", st(a.handleSwitchCourse))
	mux.HandleFunc("POST /api/logout", a.handleLogout)
	mux.HandleFunc("POST /api/register", a.handleRegister)
	mux.HandleFunc("POST /api/send-reset-code", a.handleSendResetCode)
	mux.HandleFunc("POST /api/reset-password-with-code", a.handleResetPassword)
	mux.HandleFunc("GET /api/badges", a.handleBadges)
	mux.HandleFunc("POST /api/purchase", st(a.handlePurchase))
	mux.HandleFunc("GET /api/leaderboard", st(a.handleLeaderboard))

	// Sesión de admin y tablero rápido heredado (admin_core.go)
	mux.HandleFunc("POST /api/admin-login", a.handleAdminLogin)
	mux.HandleFunc("POST /api/admin/logout", ad(a.handleAdminLogout))
	mux.HandleFunc("GET /api/admin/me", ad(a.handleAdminMe))
	mux.HandleFunc("GET /api/admin/students", ad(a.handleAdminStudentsList))
	mux.HandleFunc("POST /api/admin/update-student-data", ad(a.handleAdminUpdateStudentData))
	mux.HandleFunc("GET /api/admin/stats", ad(a.handleAdminStats))

	// Alumnos (admin_students.go)
	mux.HandleFunc("POST /api/admin/students", ad(a.handleAdminStudentCreate))
	mux.HandleFunc("POST /api/admin/students/generate", ad(a.handleAdminStudentsGenerate))
	mux.HandleFunc("POST /api/admin/students/bulk", ad(a.handleAdminStudentsBulk))
	mux.HandleFunc("PATCH /api/admin/students/{id}", ad(a.handleAdminStudentUpdate))
	mux.HandleFunc("DELETE /api/admin/students/{id}", ad(a.handleAdminStudentDelete))
	mux.HandleFunc("POST /api/admin/students/{id}/password", ad(a.handleAdminStudentPassword))

	// Cursos, insignias, niveles y avatares (admin_catalog.go)
	mux.HandleFunc("GET /api/admin/courses", ad(a.handleAdminCoursesList))
	mux.HandleFunc("POST /api/admin/courses", ad(a.handleAdminCoursesCreate))
	mux.HandleFunc("POST /api/admin/courses/reorder", ad(a.handleAdminCoursesReorder))
	mux.HandleFunc("PATCH /api/admin/courses/{id}", ad(a.handleAdminCoursesUpdate))
	mux.HandleFunc("DELETE /api/admin/courses/{id}", ad(a.handleAdminCoursesDelete))
	mux.HandleFunc("PUT /api/admin/courses/{id}/hidden-badges", ad(a.handleAdminCoursesHiddenBadges))
	mux.HandleFunc("GET /api/admin/badges", ad(a.handleAdminBadgesList))
	mux.HandleFunc("POST /api/admin/badges", ad(a.handleAdminBadgesCreate))
	mux.HandleFunc("POST /api/admin/badges/reorder", ad(a.handleAdminBadgesReorder))
	mux.HandleFunc("PATCH /api/admin/badges/{id}", ad(a.handleAdminBadgesUpdate))
	mux.HandleFunc("DELETE /api/admin/badges/{id}", ad(a.handleAdminBadgesDelete))
	mux.HandleFunc("GET /api/admin/levels", ad(a.handleAdminLevelsGet))
	mux.HandleFunc("PUT /api/admin/levels", ad(a.handleAdminLevelsPut))
	mux.HandleFunc("GET /api/admin/avatars", ad(a.handleAdminAvatarsList))
	mux.HandleFunc("PUT /api/admin/avatars", ad(a.handleAdminAvatarsPut))
	mux.HandleFunc("DELETE /api/admin/avatars", ad(a.handleAdminAvatarsDelete))

	// Compras, ajustes, administradores y subidas (admin_misc.go)
	mux.HandleFunc("GET /api/admin/purchases", ad(a.handleAdminPurchasesList))
	mux.HandleFunc("POST /api/admin/purchases/{id}/revert", ad(a.handleAdminPurchasesRevert))
	mux.HandleFunc("POST /api/admin/purchases/{id}/restore", ad(a.handleAdminPurchasesRestore))
	mux.HandleFunc("GET /api/admin/settings", ad(a.handleAdminSettingsGet))
	mux.HandleFunc("PATCH /api/admin/settings", ad(a.handleAdminSettingsPatch))
	mux.HandleFunc("GET /api/admin/admins", ad(a.handleAdminAdminsList))
	mux.HandleFunc("POST /api/admin/admins", ad(a.handleAdminAdminsCreate))
	mux.HandleFunc("PATCH /api/admin/admins/{id}", ad(a.handleAdminAdminsUpdate))
	mux.HandleFunc("DELETE /api/admin/admins/{id}", ad(a.handleAdminAdminsDelete))
	mux.HandleFunc("POST /api/admin/uploads", ad(a.handleAdminUpload))

	// Bitácora y deshacer (undo.go)
	mux.HandleFunc("GET /api/admin/audit", ad(a.handleAdminAuditList))
	mux.HandleFunc("POST /api/admin/audit/{id}/undo", ad(a.handleAdminAuditUndo))
	mux.HandleFunc("POST /api/admin/audit/{id}/redo", ad(a.handleAdminAuditRedo))
	mux.HandleFunc("POST /api/admin/audit/batch/{batchId}/undo", ad(a.handleAdminAuditBatchUndo))
	mux.HandleFunc("POST /api/admin/audit/batch/{batchId}/redo", ad(a.handleAdminAuditBatchRedo))

	// Importación desde Google Sheets (importer.go)
	mux.HandleFunc("GET /api/admin/import/status", ad(a.handleAdminImportStatus))
	mux.HandleFunc("POST /api/admin/import/preview", ad(a.handleAdminImportPreview))
	mux.HandleFunc("POST /api/admin/import/run", ad(a.handleAdminImportRun))
	mux.HandleFunc("GET /api/admin/import/jobs/{jobId}", ad(a.handleAdminImportJob))
	mux.HandleFunc("POST /api/admin/import/media", ad(a.handleAdminImportMedia))

	// Cualquier otra ruta del API: 404 en JSON.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "Ruta no encontrada.")
	})

	// Solo con almacenamiento local; en producción /media/* lo sirve CloudFront desde S3.
	if ls, ok := a.Store.(*localStorage); ok {
		mux.HandleFunc("PUT "+localUploadURL, a.handleLocalUpload)
		mux.Handle("GET /media/", mediaHandler(ls))
	}
	mux.Handle("/", a.spaHandler())

	return a.middleware(mux)
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := a.DB.Ping(ctx); err != nil {
		a.Log.Error("health: la base de datos no responde", "err", err)
		writeError(w, http.StatusServiceUnavailable, "Base de datos no disponible.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// BootstrapAdmin crea el primer administrador cuando la tabla está vacía (spec §4.2).
// Sin ADMIN_PASSWORD se genera una y se escribe una sola vez en el log.
func (a *App) BootstrapAdmin(ctx context.Context) error {
	var exists bool
	if err := a.DB.QueryRow(ctx, "select exists (select 1 from admins)").Scan(&exists); err != nil {
		return fmt.Errorf("revisar administradores: %w", err)
	}
	if exists {
		return nil
	}
	user, pw := a.Cfg.AdminUsername, a.Cfg.AdminPassword
	generated := pw == ""
	if generated {
		pw = randomToken(8)
	}
	if len(pw) < 8 {
		return errors.New("ADMIN_PASSWORD debe tener al menos 8 caracteres")
	}
	hash, err := hashPassword(pw)
	if err != nil {
		return err
	}

	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id int64
	err = tx.QueryRow(ctx, `insert into admins (username, password_hash) values ($1, $2)
		on conflict (username) do nothing returning id`, user, hash).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // otro proceso lo creó primero
	}
	if err != nil {
		return fmt.Errorf("crear administrador inicial: %w", err)
	}
	if _, err := a.Audit(ctx, tx, AuditEntry{
		ActorType:  "system",
		ActorLabel: "sistema",
		Action:     "create",
		Entity:     "admin",
		EntityID:   strconv.FormatInt(id, 10),
		Summary:    "Administrador inicial creado: " + user,
		After:      map[string]any{"id": id, "username": user, "active": true},
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if generated {
		a.Log.Warn("administrador inicial creado con contraseña generada; cámbiala al entrar",
			"username", user, "password", pw)
	} else {
		a.Log.Info("administrador inicial creado", "username", user)
	}
	return nil
}
