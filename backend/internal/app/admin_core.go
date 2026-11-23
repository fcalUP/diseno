package app

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Límites del login de administradores (spec §4.2), con la misma idea que el
// de alumnos: el bloqueo fino va por (usuario, red), así nadie deja fuera al
// profesor desde otra red; el tope por usuario frena intentos repartidos.
const (
	admLoginUserMax = 10  // intentos por (usuario, red)
	admLoginAcctMax = 100 // intentos por usuario desde cualquier red
	admLoginIPMax   = 30  // intentos por red
	admLoginWindow  = 15 * time.Minute
)

type admUserJSON struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

// ---------- Ayudas comunes del panel (admin_*.go de B2a) ----------

// admErr es un error que ya trae su respuesta HTTP; admFail la contesta tal cual.
type admErr struct {
	status int
	msg    string
}

func (e *admErr) Error() string { return e.msg }

func admBad(status int, msg string) error { return &admErr{status, msg} }

// admFail contesta err: un admErr con su estado y mensaje; cualquier otro, 500 y al log.
func (a *App) admFail(w http.ResponseWriter, what string, err error) {
	var he *admErr
	if errors.As(err, &he) {
		writeError(w, he.status, he.msg)
		return
	}
	if writeCanceled(w, err) {
		return
	}
	a.Log.Error(what, "err", err)
	writeError(w, http.StatusInternalServerError, "Error interno del servidor.")
}

// admTx corre fn en una transacción: commit si devuelve nil, rollback si no.
// Si algo falla ya respondió y devuelve false.
func (a *App) admTx(w http.ResponseWriter, r *http.Request, what string, fn func(tx pgx.Tx) error) bool {
	if err := pgx.BeginFunc(r.Context(), a.DB, fn); err != nil {
		a.admFail(w, what, err)
		return false
	}
	return true
}

func admUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

var admUUIDRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// admBatchID usa el batchId que manda el cliente (para juntar varios lotes bajo
// un solo "Deshacer todo") o genera uno nuevo.
func admBatchID(v string) (string, error) {
	if v == "" {
		return newBatchID(), nil
	}
	if !admUUIDRe.MatchString(v) {
		return "", admBad(http.StatusBadRequest, "batchId inválido.")
	}
	return strings.ToLower(v), nil
}

// admQueryID lee un id positivo opcional de la query: 0 si no viene, ok=false si viene mal.
func admQueryID(r *http.Request, key string) (int64, bool) {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return 0, true
	}
	n, err := strconv.ParseInt(v, 10, 64)
	return n, err == nil && n > 0
}

// admWhere junta condiciones con parámetros numerados; "$?" se cambia por el
// número del argumento (puede aparecer varias veces en la misma condición).
type admWhere struct {
	conds []string
	args  []any
}

func (q *admWhere) add(cond string, v any) {
	q.args = append(q.args, v)
	q.conds = append(q.conds, strings.ReplaceAll(cond, "$?", "$"+strconv.Itoa(len(q.args))))
}

func (q *admWhere) sql() string {
	if len(q.conds) == 0 {
		return "true"
	}
	return strings.Join(q.conds, " and ")
}

// ---------- Sesión de admin ----------

func (a *App) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	ctx := r.Context()
	username := strings.TrimSpace(body.Username)
	ip := clientIP(r, a.Cfg.TrustProxy)
	user := strings.ToLower(username)
	// Todo se cuenta antes de bcrypt y en este orden (ver handleLogin). La red
	// va al final tras "@", que no sale en una IP.
	userKey := "admin-login:" + user + "@" + ip
	if !a.Limiter.Allow("admin-login-ip:"+ip, admLoginIPMax, admLoginWindow) ||
		!a.Limiter.Allow(userKey, admLoginUserMax, admLoginWindow) ||
		!a.Limiter.Allow("admin-login-acct:"+user, admLoginAcctMax, admLoginWindow) {
		writeError(w, http.StatusTooManyRequests, "Demasiados intentos. Intenta de nuevo en unos minutos.")
		return
	}

	var id int64
	var hash string
	var active bool
	err := a.DB.QueryRow(ctx, `select id, password_hash, active from admins where username = $1`, username).
		Scan(&id, &hash, &active)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		a.admFail(w, "login de admin", err)
		return
	}
	// Si el usuario no existe hash queda vacío y checkPassword tarda lo mismo.
	var good bool
	if !a.publicBcrypt(w, r, func() { good = checkPassword(hash, body.Password) }) {
		return
	}
	if !good {
		writeError(w, http.StatusUnauthorized, "Invalid admin credentials.")
		return
	}
	if !active {
		writeError(w, http.StatusForbidden, "Tu cuenta de administrador está desactivada.")
		return
	}
	ok := a.admTx(w, r, "login de admin", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `update admins set last_login_at = now() where id = $1`, id); err != nil {
			return err
		}
		_, err := a.auditAdmin(ctx, tx, id, AuditEntry{
			Action:   "login",
			Entity:   "admin",
			EntityID: itoa64(id),
			Summary:  "Inicio de sesión de " + username,
			Undoable: false,
		})
		return err
	})
	if !ok {
		return
	}
	a.Limiter.Reset(userKey) // el tope del usuario no, igual que en handleLogin
	a.setSession(w, kindAdmin, id, hash)
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Admin login successful.",
		"admin":   admUserJSON{ID: id, Username: username},
	})
}

func (a *App) handleAdminLogout(w http.ResponseWriter, r *http.Request, adminID int64) {
	a.clearSession(w, kindAdmin)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (a *App) handleAdminMe(w http.ResponseWriter, r *http.Request, adminID int64) {
	u := admUserJSON{ID: adminID}
	err := a.DB.QueryRow(r.Context(), `select username from admins where id = $1`, adminID).Scan(&u.Username)
	if errors.Is(err, pgx.ErrNoRows) { // lo borraron entre el middleware y aquí
		a.clearSession(w, kindAdmin)
		writeError(w, http.StatusUnauthorized, sessionExpired)
		return
	}
	if err != nil {
		a.admFail(w, "admin actual", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"admin": u})
}

// ---------- Tablero rápido heredado ----------

func (a *App) handleAdminStudentsList(w http.ResponseWriter, r *http.Request, adminID int64) {
	ctx := r.Context()
	q := r.URL.Query()
	var where admWhere

	courseID, ok := admQueryID(r, "courseId")
	if !ok {
		writeError(w, http.StatusBadRequest, "Curso inválido.")
		return
	}
	if courseID > 0 {
		where.add("s.course_id = $?", courseID)
	}
	// El modal heredado filtra por el nombre del curso que muestra el selector.
	if career := strings.TrimSpace(q.Get("career")); career != "" {
		var id int64
		err := a.DB.QueryRow(ctx, `select id from courses where name = $1`, career).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusBadRequest, "Carrera no válida.")
			return
		}
		if err != nil {
			a.admFail(w, "buscar curso", err)
			return
		}
		where.add("s.course_id = $?", id)
	}
	if term := strings.TrimSpace(q.Get("q")); term != "" {
		where.add(`(strpos(lower(s.code), lower($?)) > 0 or strpos(lower(s.name), lower($?)) > 0
			or strpos(lower(s.email), lower($?)) > 0)`, term)
	}
	switch q.Get("active") {
	case "", "all":
	case "true":
		where.conds = append(where.conds, "s.active")
	case "false":
		where.conds = append(where.conds, "not s.active")
	default:
		writeError(w, http.StatusBadRequest, "Filtro de estado inválido (true, false o all).")
		return
	}

	students, err := a.admStudents(ctx, a.DB, where.sql(), where.args...)
	if err != nil {
		a.admFail(w, "listar alumnos", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "students": students})
}

// handleAdminUpdateStudentData es el guardado del modal heredado. Nunca toca
// last_login_level: el alumno ve el GIF de subida de nivel en su siguiente login.
func (a *App) handleAdminUpdateStudentData(w http.ResponseWriter, r *http.Request, adminID int64) {
	var body struct {
		ID          int64 `json:"id"`
		Tareas      *int  `json:"tareas"`
		Monedas     *int  `json:"monedas"`
		Asistencias *int  `json:"asistencias"`
		Games       *int  `json:"games"`
		Exp         *int  `json:"exp"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	// Igual que el endpoint viejo: games puede faltar.
	if body.ID <= 0 || body.Tareas == nil || body.Monedas == nil || body.Asistencias == nil || body.Exp == nil {
		writeError(w, http.StatusBadRequest, "Incomplete data for update.")
		return
	}
	set := map[string]any{}
	for col, v := range map[string]*int{"tareas": body.Tareas, "monedas": body.Monedas,
		"asistencias": body.Asistencias, "games": body.Games, "exp": body.Exp} {
		if v == nil {
			continue
		}
		if !admCounterOK(*v) {
			writeError(w, http.StatusBadRequest, admCounterMsg)
			return
		}
		set[col] = *v
	}

	ctx := r.Context()
	var code string
	var auditID int64
	ok := a.admTx(w, r, "actualizar alumno (tablero rápido)", func(tx pgx.Tx) error {
		before, err := admLockStudent(ctx, tx, body.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return admBad(http.StatusNotFound, "Student row not found.")
		}
		if err != nil {
			return err
		}
		code = admStr(before, "code")
		_, auditID, err = a.admApplyStudent(ctx, tx, adminID, body.ID, before, set,
			"Tablero rápido, "+code+": "+admChanges(before, set), "")
		return err
	})
	if !ok {
		return
	}
	levels, err := a.Levels(ctx)
	if err != nil {
		a.admFail(w, "leer niveles", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"message":  "Student " + code + " data updated successfully.",
		"newLevel": CalcLevel(*body.Exp, levels),
		"auditId":  auditID,
	})
}
