package app

// Endpoints públicos y del alumno (spec §4.1). Los textos de error son los del
// servidor viejo cuando existían.

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const (
	stuMsgInternal = "Error interno del servidor."
	stuMsgTooMany  = "Demasiados intentos. Intenta de nuevo en unos minutos."
	stuMsgCareer   = "Carrera no válida."
	stuMsgNoCareer = "Carrera requerida."
	stuMsgInactive = "Tu cuenta está desactivada."
	stuMsgResetOff = "La recuperación de contraseña está desactivada."
	stuMsgBadCode  = "Código incorrecto o no solicitado."
	stuMsgStoreOff = "La tienda está cerrada."
	stuMsgBadBuy   = "Datos inválidos o incompletos para la compra"

	stuLoginMaxFails = 10 // fallos por (curso, matrícula) dentro de la ventana
	stuLoginWindow   = 15 * time.Minute
	stuResetMaxFails = 5 // intentos fallidos que invalidan un código
)

// stuInternal registra el error y responde 500 sin dar detalles.
func (a *App) stuInternal(w http.ResponseWriter, where string, err error) {
	if writeCanceled(w, err) {
		return
	}
	a.Log.Error(where, "err", err)
	writeError(w, http.StatusInternalServerError, stuMsgInternal)
}

// stuCourseID busca un curso activo por su nombre, que es lo que manda el selector.
func (a *App) stuCourseID(ctx context.Context, career string) (int64, bool, error) {
	var id int64
	err := a.DB.QueryRow(ctx, "select id from courses where active and name = $1",
		strings.TrimSpace(career)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return id, err == nil, err
}

// stuPasswordOK revisa la contraseña con la regla de Ajustes. Si la regla
// guardada no compila se usa la predeterminada.
func (a *App) stuPasswordOK(pattern, pw string) bool {
	re, err := regexp.Compile(pattern)
	if err != nil {
		a.Log.Warn("regla de contraseña inválida; se usa la predeterminada", "err", err)
		re = regexp.MustCompile(defaultSettings().PasswordPattern)
	}
	return re.MatchString(pw)
}

// stuHashCode: el código de recuperación se guarda como SHA-256 en hex.
func stuHashCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

// ---------- GET /api/config ----------

type stuPublicCourse struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	SideImageURL string `json:"sideImageUrl"`
	LogoURL      string `json:"logoUrl"`
}

type stuFeatures struct {
	Registration     bool `json:"registration"`
	PasswordReset    bool `json:"passwordReset"`
	Store            bool `json:"store"`
	Leaderboard      bool `json:"leaderboard"`
	LevelUpAnimation bool `json:"levelUpAnimation"`
}

type stuConfigJSON struct {
	Courses              []stuPublicCourse `json:"courses"`
	Features             stuFeatures       `json:"features"`
	Levels               []Level           `json:"levels"`
	LevelUpGifURL        string            `json:"levelUpGifUrl"`
	PasswordPattern      string            `json:"passwordPattern"`
	PasswordHintRegister string            `json:"passwordHintRegister"`
	PasswordHintReset    string            `json:"passwordHintReset"`
	PasswordError        string            `json:"passwordError"`
}

func (a *App) handleConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s, err := a.Settings(ctx)
	if err != nil {
		a.stuInternal(w, "config: ajustes", err)
		return
	}
	levels, err := a.Levels(ctx)
	if err != nil {
		a.stuInternal(w, "config: niveles", err)
		return
	}
	if levels == nil {
		levels = []Level{}
	}
	// Solo los cursos activos salen en el selector del login y del registro.
	rows, _ := a.DB.Query(ctx, `select id, name, side_image_url, logo_url from courses
		where active order by sort_order, name`)
	courses, err := pgx.CollectRows(rows, pgx.RowToStructByPos[stuPublicCourse])
	if err != nil {
		a.stuInternal(w, "config: cursos", err)
		return
	}
	writeJSON(w, http.StatusOK, stuConfigJSON{
		Courses: courses,
		Features: stuFeatures{
			Registration:     s.RegistrationEnabled,
			PasswordReset:    s.PasswordResetEnabled,
			Store:            s.StoreEnabled,
			Leaderboard:      s.LeaderboardEnabled,
			LevelUpAnimation: s.LevelUpAnimationEnabled,
		},
		Levels:               levels,
		LevelUpGifURL:        s.LevelUpGifURL,
		PasswordPattern:      s.PasswordPattern,
		PasswordHintRegister: s.PasswordHintRegister,
		PasswordHintReset:    s.PasswordHintReset,
		PasswordError:        s.PasswordError,
	})
}

// ---------- Login, /api/me y logout ----------

// stuStudentJSON es Student de types.ts.
type stuStudentJSON struct {
	ID               string         `json:"id"` // matrícula
	Name             string         `json:"name"`
	Sexo             string         `json:"sexo"`
	Email            string         `json:"email"`
	Career           string         `json:"career"`
	Monedas          int            `json:"monedas"`
	Tareas           int            `json:"tareas"`
	Asistencias      int            `json:"asistencias"`
	Games            int            `json:"games"`
	Exp              int            `json:"exp"`
	Level            int            `json:"level"`
	Purchases        map[string]int `json:"purchases"`
	Comments         string         `json:"comments"`
	AvatarURL        string         `json:"avatarUrl"`
	DefaultAvatarURL string         `json:"defaultAvatarUrl"`
}

type stuLoginJSON struct {
	Success         bool           `json:"success"`
	Student         stuStudentJSON `json:"student"`
	LevelUpOccurred bool           `json:"levelUpOccurred"`
}

// stuPurchases suma por nombre de insignia lo comprado sin revertir.
func stuPurchases(ctx context.Context, q DBTX, studentID int64) (map[string]int, error) {
	out := map[string]int{}
	var name string
	var qty int
	rows, _ := q.Query(ctx, `select badge_name, sum(quantity) from purchases
		where student_id = $1 and reverted_at is null group by badge_name`, studentID)
	_, err := pgx.ForEachRow(rows, []any{&name, &qty}, func() error {
		out[name] = qty
		return nil
	})
	return out, err
}

// stuLoad arma el alumno de LoginResponse y revisa si subió de nivel: hubo
// subida cuando el nivel actual pasa del último que vio. Ese nivel se guarda
// cuando cambia y en login también se anota last_login_at. Si el alumno ya no
// existe devuelve pgx.ErrNoRows.
func (a *App) stuLoad(ctx context.Context, id int64, login bool) (stuStudentJSON, bool, error) {
	var st stuStudentJSON
	levels, err := a.Levels(ctx)
	if err != nil {
		return st, false, err
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return st, false, err
	}
	defer tx.Rollback(ctx)

	var last int
	err = tx.QueryRow(ctx, `select s.code, s.name, s.sexo, s.email, c.name, s.monedas, s.tareas,
			s.asistencias, s.games, s.exp, s.comments, s.last_login_level
		from students s join courses c on c.id = s.course_id
		where s.id = $1 for update of s`, id).Scan(&st.ID, &st.Name, &st.Sexo, &st.Email,
		&st.Career, &st.Monedas, &st.Tareas, &st.Asistencias, &st.Games, &st.Exp, &st.Comments, &last)
	if err != nil {
		return st, false, err
	}
	st.Level = CalcLevel(st.Exp, levels)
	if st.Level != last || login {
		// Sin updated_at: entrar no es editar al alumno.
		_, err = tx.Exec(ctx, `update students set last_login_level = $2,
			last_login_at = case when $3::boolean then now() else last_login_at end
			where id = $1`, id, st.Level, login)
		if err != nil {
			return st, false, err
		}
	}
	if st.Purchases, err = stuPurchases(ctx, tx, id); err != nil {
		return st, false, err
	}

	// Avatar propio del nivel; si falta, el predeterminado según sexo, como antes.
	def := "DefaultF"
	if st.Sexo == "Hombre" {
		def = "DefaultH"
	}
	var code, url string
	rows, _ := tx.Query(ctx, "select code, url from avatars where level = $1 and code = any($2)",
		st.Level, []string{st.ID, def})
	_, err = pgx.ForEachRow(rows, []any{&code, &url}, func() error {
		if code == st.ID {
			st.AvatarURL = url
		}
		if code == def {
			st.DefaultAvatarURL = url
		}
		return nil
	})
	if err != nil {
		return st, false, err
	}
	if st.AvatarURL == "" {
		st.AvatarURL = st.DefaultAvatarURL
	}
	if err := tx.Commit(ctx); err != nil {
		return st, false, err
	}
	return st, st.Level > last, nil
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StudentID string `json:"studentId"`
		Password  string `json:"password"`
		Career    string `json:"career"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	code, pw := strings.TrimSpace(req.StudentID), strings.TrimSpace(req.Password)
	if code == "" || pw == "" || strings.TrimSpace(req.Career) == "" {
		writeError(w, http.StatusBadRequest, "ID, contraseña y carrera requeridos.")
		return
	}
	if !a.Limiter.Allow("stu-login-ip:"+clientIP(r, a.Cfg.TrustProxy), 100, 5*time.Minute) {
		writeError(w, http.StatusTooManyRequests, stuMsgTooMany)
		return
	}
	ctx := r.Context()
	courseID, ok, err := a.stuCourseID(ctx, req.Career)
	if err != nil {
		a.stuInternal(w, "login: curso", err)
		return
	}
	if !ok {
		writeError(w, http.StatusBadRequest, stuMsgCareer)
		return
	}
	lockKey := "stu-login:" + strconv.FormatInt(courseID, 10) + ":" + code
	if a.Limiter.Blocked(lockKey, stuLoginMaxFails, stuLoginWindow) {
		writeError(w, http.StatusTooManyRequests, stuMsgTooMany)
		return
	}

	var id int64
	var hash string
	var active bool
	err = a.DB.QueryRow(ctx, "select id, password_hash, active from students where course_id = $1 and code = $2",
		courseID, code).Scan(&id, &hash, &active)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		a.stuInternal(w, "login: buscar alumno", err)
		return
	}
	// Sin alumno el hash queda vacío y checkPassword gasta el mismo tiempo.
	if !checkPassword(hash, pw) {
		a.Limiter.Fail(lockKey, stuLoginMaxFails, stuLoginWindow)
		writeError(w, http.StatusUnauthorized, "Credenciales inválidas.")
		return
	}
	if !active {
		writeError(w, http.StatusForbidden, stuMsgInactive)
		return
	}
	a.Limiter.Reset(lockKey)

	st, levelUp, err := a.stuLoad(ctx, id, true)
	if err != nil {
		a.stuInternal(w, "login: cargar alumno", err)
		return
	}
	a.setSession(w, kindStudent, id)
	writeJSON(w, http.StatusOK, stuLoginJSON{Success: true, Student: st, LevelUpOccurred: levelUp})
}

// handleMe repite la revisión de subida de nivel, así el GIF también sale al recargar.
func (a *App) handleMe(w http.ResponseWriter, r *http.Request, studentID int64) {
	st, levelUp, err := a.stuLoad(r.Context(), studentID, false)
	if errors.Is(err, pgx.ErrNoRows) {
		a.clearSession(w, kindStudent)
		writeError(w, http.StatusUnauthorized, sessionExpired)
		return
	}
	if err != nil {
		a.stuInternal(w, "me: cargar alumno", err)
		return
	}
	writeJSON(w, http.StatusOK, stuLoginJSON{Success: true, Student: st, LevelUpOccurred: levelUp})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	a.clearSession(w, kindStudent)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// ---------- POST /api/register ----------

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Sexo     string `json:"sexo"`
		Password string `json:"password"`
		Career   string `json:"career"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	s, err := a.Settings(ctx)
	if err != nil {
		a.stuInternal(w, "registro: ajustes", err)
		return
	}
	if !s.RegistrationEnabled {
		writeError(w, http.StatusForbidden, "El registro está cerrado.")
		return
	}
	code, name := strings.TrimSpace(req.ID), strings.TrimSpace(req.Name)
	sexo, pw := strings.TrimSpace(req.Sexo), strings.TrimSpace(req.Password)
	career := strings.TrimSpace(req.Career)
	// Los topes de largo evitan nombres enormes en el leaderboard. DefaultH y
	// DefaultF son los avatares predeterminados, no pueden ser matrículas.
	if code == "" || name == "" || sexo == "" || career == "" || !a.stuPasswordOK(s.PasswordPattern, pw) ||
		utf8.RuneCountInString(code) > 40 || utf8.RuneCountInString(name) > 120 || utf8.RuneCountInString(sexo) > 20 ||
		strings.EqualFold(code, "DefaultH") || strings.EqualFold(code, "DefaultF") {
		writeError(w, http.StatusBadRequest,
			"Datos inválidos o incompletos. Contraseña debe ser 4 HEX mayúsculas y carrera requerida.")
		return
	}
	courseID, ok, err := a.stuCourseID(ctx, career)
	if err != nil {
		a.stuInternal(w, "registro: curso", err)
		return
	}
	if !ok {
		writeError(w, http.StatusBadRequest, stuMsgCareer)
		return
	}
	if !a.Limiter.Allow("stu-register-ip:"+clientIP(r, a.Cfg.TrustProxy), 20, time.Hour) {
		writeError(w, http.StatusTooManyRequests, stuMsgTooMany)
		return
	}
	hash, err := hashPassword(pw)
	if err != nil {
		a.stuInternal(w, "registro: hash", err)
		return
	}

	tx, err := a.DB.Begin(ctx)
	if err != nil {
		a.stuInternal(w, "registro: transacción", err)
		return
	}
	defer tx.Rollback(ctx)
	var id int64
	err = tx.QueryRow(ctx, `insert into students (course_id, code, name, sexo, email, password_hash)
		values ($1, $2, $3, $4, $5, $6) on conflict (course_id, code) do nothing returning id`,
		courseID, code, name, sexo, code+"@"+s.StudentEmailDomain, hash).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "El ID de alumno ya existe.")
		return
	}
	if err != nil {
		a.stuInternal(w, "registro: insertar", err)
		return
	}
	snap, err := snapshotRow(ctx, tx, "students", id)
	if err != nil {
		a.stuInternal(w, "registro: instantánea", err)
		return
	}
	if _, err := a.Audit(ctx, tx, AuditEntry{
		ActorType:  "student",
		ActorID:    &id,
		ActorLabel: code,
		Action:     "create",
		Entity:     "student",
		EntityID:   strconv.FormatInt(id, 10),
		Summary:    "Alumno " + code + " se registró en " + career,
		After:      snap,
		Undoable:   true,
	}); err != nil {
		a.stuInternal(w, "registro: auditoría", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		a.stuInternal(w, "registro: commit", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"message": "Alumno registrado exitosamente."})
}

// ---------- Recuperación de contraseña ----------

func (a *App) handleSendResetCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID     string `json:"id"`
		Career string `json:"career"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	s, err := a.Settings(ctx)
	if err != nil {
		a.stuInternal(w, "código: ajustes", err)
		return
	}
	if !s.PasswordResetEnabled {
		writeError(w, http.StatusForbidden, stuMsgResetOff)
		return
	}
	code := strings.TrimSpace(req.ID)
	if code == "" || strings.TrimSpace(req.Career) == "" {
		writeError(w, http.StatusBadRequest, "ID de alumno y carrera son requeridos.")
		return
	}
	courseID, ok, err := a.stuCourseID(ctx, req.Career)
	if err != nil {
		a.stuInternal(w, "código: curso", err)
		return
	}
	if !ok {
		writeError(w, http.StatusBadRequest, stuMsgCareer)
		return
	}
	// Cuenta también las matrículas que no existen: frena a quien las prueba todas.
	if !a.Limiter.Allow("stu-reset-ip:"+clientIP(r, a.Cfg.TrustProxy), 20, time.Hour) {
		writeError(w, http.StatusTooManyRequests, stuMsgTooMany)
		return
	}
	var id int64
	var email string
	var active bool
	err = a.DB.QueryRow(ctx, "select id, email, active from students where course_id = $1 and code = $2",
		courseID, code).Scan(&id, &email, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "ID de alumno no encontrado en la carrera seleccionada.")
		return
	}
	if err != nil {
		a.stuInternal(w, "código: buscar alumno", err)
		return
	}
	if !active {
		writeError(w, http.StatusForbidden, stuMsgInactive)
		return
	}
	if !a.Limiter.Allow("stu-reset-send:"+strconv.FormatInt(id, 10), 3, time.Hour) {
		writeError(w, http.StatusTooManyRequests, stuMsgTooMany)
		return
	}

	resetCode := randomDigits(6)
	ttl := s.ResetCodeTTLMinutes
	if ttl < 1 {
		ttl = defaultSettings().ResetCodeTTLMinutes
	}
	// Un código nuevo reemplaza al anterior y reinicia los intentos.
	_, err = a.DB.Exec(ctx, `insert into reset_codes (student_id, code_hash, expires_at, attempts)
		values ($1, $2, now() + $3::integer * interval '1 minute', 0)
		on conflict (student_id) do update set code_hash = excluded.code_hash,
			expires_at = excluded.expires_at, attempts = 0`, id, stuHashCode(resetCode), ttl)
	if err != nil {
		a.stuInternal(w, "código: guardar", err)
		return
	}

	// Como antes: si el correo falla se registra y la respuesta sigue siendo 200.
	// Sin la cancelación del request para que un cliente que se va no lo corte.
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	m := mailResetCode(s.EmailFrom, mailRecipients(s, email, code), resetCode, ttl)
	if err := a.Mail.Send(sendCtx, m); err != nil {
		a.Log.Error("no se pudo enviar el código de recuperación", "student", id, "err", err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Código de restablecimiento enviado a tu correo electrónico."})
}

func (a *App) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID       string `json:"id"`
		Code     string `json:"code"`
		Password string `json:"password"`
		Career   string `json:"career"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	s, err := a.Settings(ctx)
	if err != nil {
		a.stuInternal(w, "restablecer: ajustes", err)
		return
	}
	if !s.PasswordResetEnabled {
		writeError(w, http.StatusForbidden, stuMsgResetOff)
		return
	}
	code, resetCode := strings.TrimSpace(req.ID), strings.TrimSpace(req.Code)
	pw := strings.TrimSpace(req.Password)
	if code == "" || resetCode == "" || strings.TrimSpace(req.Career) == "" || !a.stuPasswordOK(s.PasswordPattern, pw) {
		writeError(w, http.StatusBadRequest,
			"Datos inválidos. Asegúrate de usar un código, contraseña válida (4 HEX mayúsculas) y carrera.")
		return
	}
	courseID, ok, err := a.stuCourseID(ctx, req.Career)
	if err != nil {
		a.stuInternal(w, "restablecer: curso", err)
		return
	}
	if !ok {
		writeError(w, http.StatusBadRequest, stuMsgCareer)
		return
	}
	var id int64
	err = a.DB.QueryRow(ctx, "select id from students where course_id = $1 and code = $2",
		courseID, code).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "ID no encontrado en la carrera seleccionada.")
		return
	}
	if err != nil {
		a.stuInternal(w, "restablecer: buscar alumno", err)
		return
	}

	tx, err := a.DB.Begin(ctx)
	if err != nil {
		a.stuInternal(w, "restablecer: transacción", err)
		return
	}
	defer tx.Rollback(ctx)
	var hash string
	var expired bool
	var attempts int
	err = tx.QueryRow(ctx, `select code_hash, expires_at <= now(), attempts from reset_codes
		where student_id = $1 for update`, id).Scan(&hash, &expired, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, stuMsgBadCode)
		return
	}
	if err != nil {
		a.stuInternal(w, "restablecer: leer código", err)
		return
	}
	if expired || subtle.ConstantTimeCompare([]byte(hash), []byte(stuHashCode(resetCode))) != 1 {
		// Vencido o al quinto error el código se borra; antes solo se cuenta el intento.
		q := "update reset_codes set attempts = attempts + 1 where student_id = $1"
		if expired || attempts+1 >= stuResetMaxFails {
			q = "delete from reset_codes where student_id = $1"
		}
		if _, err := tx.Exec(ctx, q, id); err != nil {
			a.stuInternal(w, "restablecer: contar intento", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			a.stuInternal(w, "restablecer: commit", err)
			return
		}
		writeError(w, http.StatusUnauthorized, stuMsgBadCode)
		return
	}

	newHash, err := hashPassword(pw)
	if err != nil {
		a.stuInternal(w, "restablecer: hash", err)
		return
	}
	if _, err := tx.Exec(ctx, "update students set password_hash = $2, updated_at = now() where id = $1",
		id, newHash); err != nil {
		a.stuInternal(w, "restablecer: guardar", err)
		return
	}
	if _, err := tx.Exec(ctx, "delete from reset_codes where student_id = $1", id); err != nil {
		a.stuInternal(w, "restablecer: borrar código", err)
		return
	}
	// Deshacerlo devolvería una contraseña que el alumno ya olvidó: no se ofrece.
	if _, err := a.Audit(ctx, tx, AuditEntry{
		ActorType:  "student",
		ActorID:    &id,
		ActorLabel: code,
		Action:     "update",
		Entity:     "student",
		EntityID:   strconv.FormatInt(id, 10),
		Summary:    "Alumno " + code + " restableció su contraseña con un código",
		Undoable:   false,
	}); err != nil {
		a.stuInternal(w, "restablecer: auditoría", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		a.stuInternal(w, "restablecer: commit", err)
		return
	}
	// Demostró que lee su correo: se levanta el bloqueo de login si lo tenía.
	a.Limiter.Reset("stu-login:" + strconv.FormatInt(courseID, 10) + ":" + code)
	writeJSON(w, http.StatusOK, map[string]string{"message": "Contraseña actualizada correctamente."})
}

// ---------- GET /api/badges ----------

type stuBadgeJSON struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Quantity int    `json:"quantity"` // existencias
	Cost     int    `json:"cost"`
	ImageURL string `json:"imageUrl"`
}

func (a *App) handleBadges(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s, err := a.Settings(ctx)
	if err != nil {
		a.stuInternal(w, "insignias: ajustes", err)
		return
	}
	if !s.StoreEnabled {
		writeError(w, http.StatusForbidden, stuMsgStoreOff)
		return
	}
	var courseID int64
	if career := strings.TrimSpace(r.URL.Query().Get("career")); career != "" {
		id, ok, err := a.stuCourseID(ctx, career)
		if err != nil {
			a.stuInternal(w, "insignias: curso", err)
			return
		}
		if !ok {
			writeError(w, http.StatusBadRequest, stuMsgCareer)
			return
		}
		courseID = id
	} else {
		// Sin career se usa el curso del alumno con sesión.
		sid, ok := a.studentID(r)
		if !ok {
			writeError(w, http.StatusBadRequest, stuMsgNoCareer)
			return
		}
		err := a.DB.QueryRow(ctx, "select course_id from students where id = $1 and active", sid).Scan(&courseID)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusBadRequest, stuMsgNoCareer)
			return
		}
		if err != nil {
			a.stuInternal(w, "insignias: curso del alumno", err)
			return
		}
	}
	rows, _ := a.DB.Query(ctx, `select b.id, b.name, b.stock, b.cost, b.image_url from badges b
		where b.active and not exists (select 1 from course_hidden_badges h
			where h.course_id = $1 and h.badge_id = b.id)
		order by b.sort_order, b.name, b.id`, courseID)
	badges, err := pgx.CollectRows(rows, pgx.RowToStructByPos[stuBadgeJSON])
	if err != nil {
		a.stuInternal(w, "insignias: leer", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "badges": badges})
}

// ---------- POST /api/purchase ----------

type stuStockJSON struct {
	ID       int64 `json:"id"`
	Quantity int   `json:"quantity"`
}

type stuPurchaseJSON struct {
	Success   bool           `json:"success"`
	Message   string         `json:"message"`
	NewCoins  int            `json:"newCoins"`
	Badges    []stuStockJSON `json:"badges"`
	Purchases map[string]int `json:"purchases"`
}

// handlePurchase compra todo el carrito o nada. El alumno sale de la sesión y
// el precio de la base; unitCost solo sirve para avisar si el precio cambió.
func (a *App) handlePurchase(w http.ResponseWriter, r *http.Request, studentID int64) {
	var req struct {
		Items []struct {
			BadgeID  int64 `json:"badgeId"`
			Quantity int   `json:"quantity"`
			UnitCost int   `json:"unitCost"`
		} `json:"items"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	s, err := a.Settings(ctx)
	if err != nil {
		a.stuInternal(w, "compra: ajustes", err)
		return
	}
	if !s.StoreEnabled {
		writeError(w, http.StatusForbidden, stuMsgStoreOff)
		return
	}

	// Junta los renglones repetidos de una misma insignia. Si traen precios
	// distintos, alguno ya no es el actual.
	type wanted struct {
		qty, unitCost int
		mixed         bool
	}
	want := map[int64]*wanted{}
	var ids []int64
	for _, it := range req.Items {
		if it.BadgeID <= 0 || it.Quantity < 1 || it.Quantity > math.MaxInt32 || it.UnitCost < 0 {
			writeError(w, http.StatusBadRequest, stuMsgBadBuy)
			return
		}
		wt := want[it.BadgeID]
		if wt == nil {
			wt = &wanted{unitCost: it.UnitCost}
			want[it.BadgeID] = wt
			ids = append(ids, it.BadgeID)
		}
		wt.qty += it.Quantity
		wt.mixed = wt.mixed || wt.unitCost != it.UnitCost
	}
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, stuMsgBadBuy)
		return
	}
	slices.Sort(ids)

	tx, err := a.DB.Begin(ctx)
	if err != nil {
		a.stuInternal(w, "compra: transacción", err)
		return
	}
	defer tx.Rollback(ctx)

	// Primero el alumno y luego las insignias por id: mismo orden de bloqueo en
	// todas las compras, así dos carritos a la vez no se traban.
	var courseID int64
	var code, email, career string
	var coins int
	err = tx.QueryRow(ctx, `select s.course_id, s.code, s.email, c.name, s.monedas
		from students s join courses c on c.id = s.course_id
		where s.id = $1 for update of s`, studentID).Scan(&courseID, &code, &email, &career, &coins)
	if errors.Is(err, pgx.ErrNoRows) {
		a.clearSession(w, kindStudent)
		writeError(w, http.StatusUnauthorized, sessionExpired)
		return
	}
	if err != nil {
		a.stuInternal(w, "compra: bloquear alumno", err)
		return
	}
	type badgeRow struct {
		ID          int64
		Name        string
		Stock, Cost int
		Available   bool
	}
	rows, _ := tx.Query(ctx, `select b.id, b.name, b.stock, b.cost,
			b.active and not exists (select 1 from course_hidden_badges h
				where h.course_id = $2 and h.badge_id = b.id)
		from badges b where b.id = any($1) order by b.id for update of b`, ids, courseID)
	found, err := pgx.CollectRows(rows, pgx.RowToStructByPos[badgeRow])
	if err != nil {
		a.stuInternal(w, "compra: bloquear insignias", err)
		return
	}
	badges := make(map[int64]badgeRow, len(found))
	for _, b := range found {
		badges[b.ID] = b
	}

	total := 0
	for _, id := range ids {
		b, ok := badges[id]
		wt := want[id]
		if !ok || !b.Available {
			writeError(w, http.StatusForbidden, "Esta insignia no está disponible para tu grupo.")
			return
		}
		if wt.mixed || wt.unitCost != b.Cost {
			writeError(w, http.StatusConflict, fmt.Sprintf("El precio de %s cambió. Recarga la tienda.", b.Name))
			return
		}
		if b.Stock < wt.qty {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Sólo quedan %d unidades de %s", b.Stock, b.Name))
			return
		}
		// Deja de sumar al pasarse de las monedas: el total ya no importa y no se desborda.
		if total <= coins {
			total += wt.qty * b.Cost
		}
	}
	if total > coins {
		writeError(w, http.StatusBadRequest, "Monedas insuficientes")
		return
	}

	batch := newBatchID()
	coinsBefore := coins
	stock := make([]stuStockJSON, 0, len(ids))
	items := make([]mailItem, 0, len(ids))
	for _, id := range ids {
		b, qty := badges[id], want[id].qty
		cost := qty * b.Cost
		var left int
		if err := tx.QueryRow(ctx, `update badges set stock = stock - $2, updated_at = now()
			where id = $1 returning stock`, id, qty).Scan(&left); err != nil {
			a.stuInternal(w, "compra: existencias", err)
			return
		}
		// Un renglón por insignia; las monedas antes/después se encadenan.
		var pid int64
		if err := tx.QueryRow(ctx, `insert into purchases (student_id, badge_id, badge_name, quantity,
				unit_cost, total_cost, coins_before, coins_after, source)
			values ($1, $2, $3, $4, $5, $6, $7, $8, 'app') returning id`,
			studentID, id, b.Name, qty, b.Cost, cost, coins, coins-cost).Scan(&pid); err != nil {
			a.stuInternal(w, "compra: insertar", err)
			return
		}
		coins -= cost
		snap, err := snapshotRow(ctx, tx, "purchases", pid)
		if err != nil {
			a.stuInternal(w, "compra: instantánea", err)
			return
		}
		snap["student_coins"] = coins
		snap["badge_stock"] = left
		if _, err := a.Audit(ctx, tx, AuditEntry{
			ActorType:  "student",
			ActorID:    &studentID,
			ActorLabel: code,
			Action:     "purchase",
			Entity:     "purchase",
			EntityID:   strconv.FormatInt(pid, 10),
			Summary:    fmt.Sprintf("Alumno %s compró %d × %s (%d monedas)", code, qty, b.Name, cost),
			After:      snap,
			BatchID:    batch,
			Undoable:   true,
		}); err != nil {
			a.stuInternal(w, "compra: auditoría", err)
			return
		}
		stock = append(stock, stuStockJSON{ID: id, Quantity: left})
		items = append(items, mailItem{Name: b.Name, Quantity: qty})
	}
	if _, err := tx.Exec(ctx, "update students set monedas = $2, updated_at = now() where id = $1",
		studentID, coins); err != nil {
		a.stuInternal(w, "compra: monedas", err)
		return
	}
	purchases, err := stuPurchases(ctx, tx, studentID)
	if err != nil {
		a.stuInternal(w, "compra: resumen", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		a.stuInternal(w, "compra: commit", err)
		return
	}

	if s.PurchaseEmailEnabled {
		a.mailAsync(mailPurchase(s.EmailFrom, mailRecipients(s, email, code), career, items, coinsBefore, coins),
			"compra")
	}
	writeJSON(w, http.StatusOK, stuPurchaseJSON{
		Success:   true,
		Message:   fmt.Sprintf("Compra realizada correctamente. Monedas restantes: %d", coins),
		NewCoins:  coins,
		Badges:    stock,
		Purchases: purchases,
	})
}

// ---------- GET /api/leaderboard ----------

// stuLeaderRow es LeaderboardRow: sin matrícula, solo nombre, insignias y nivel.
type stuLeaderRow struct {
	Rank   int    `json:"rank"`
	Name   string `json:"name"`
	Sexo   string `json:"sexo"`
	Badges int    `json:"badges"`
	Exp    int    `json:"exp"`
	Level  int    `json:"level"`
}

func (a *App) handleLeaderboard(w http.ResponseWriter, r *http.Request, studentID int64) {
	ctx := r.Context()
	s, err := a.Settings(ctx)
	if err != nil {
		a.stuInternal(w, "leaderboard: ajustes", err)
		return
	}
	if !s.LeaderboardEnabled {
		writeError(w, http.StatusForbidden, "El leaderboard está desactivado.")
		return
	}
	levels, err := a.Levels(ctx)
	if err != nil {
		a.stuInternal(w, "leaderboard: niveles", err)
		return
	}
	board := []stuLeaderRow{}
	var row stuLeaderRow
	rows, _ := a.DB.Query(ctx, `select s.name, s.sexo, s.exp,
			coalesce((select sum(p.quantity) from purchases p
				where p.student_id = s.id and p.reverted_at is null), 0)
		from students s
		where s.active and s.course_id = (select course_id from students where id = $1)
		order by s.exp desc, s.id`, studentID)
	_, err = pgx.ForEachRow(rows, []any{&row.Name, &row.Sexo, &row.Exp, &row.Badges}, func() error {
		row.Rank = len(board) + 1
		row.Level = CalcLevel(row.Exp, levels)
		board = append(board, row)
		return nil
	})
	if err != nil {
		a.stuInternal(w, "leaderboard: leer", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "leaderboard": board})
}
