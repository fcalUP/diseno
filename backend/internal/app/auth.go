package app

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionTTL     = 12 * time.Hour
	kindStudent    = "alumno"
	kindAdmin      = "admin"
	sessionExpired = "Sesión expirada. Inicia sesión de nuevo."
	msgBusy        = "El servidor está ocupado. Intenta de nuevo en unos segundos."
)

// Session es el contenido firmado de la cookie: {k, i, e, v}.
type Session struct {
	Kind          string `json:"k"` // "alumno" | "admin"
	ID            int64  `json:"i"`
	Exp           int64  `json:"e"`           // unix, segundos
	PwV           string `json:"v"`           // pwVersion de la contraseña con que entró
	AuthID        int64  `json:"a,omitempty"` // inscripción que comprobó la contraseña; no cambia al elegir materia
	Code          string `json:"c,omitempty"` // identidad del alumno, compartida por sus inscripciones
	SelectPending bool   `json:"p,omitempty"` // selector pendiente; persiste al recargar hasta elegir materia
}

// signToken firma v como base64url(json) + "." + base64url(HMAC-SHA256).
func signToken(key []byte, v any) string {
	payload, _ := json.Marshal(v)
	p := base64.RawURLEncoding.EncodeToString(payload)
	return p + "." + base64.RawURLEncoding.EncodeToString(tokenMAC(key, p))
}

// verifyToken revisa la firma en tiempo constante y decodifica en v.
// La expiración la revisa quien llama.
func verifyToken(key []byte, tok string, v any) bool {
	p, sig, ok := strings.Cut(tok, ".")
	if !ok {
		return false
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, tokenMAC(key, p)) {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(p)
	return err == nil && json.Unmarshal(payload, v) == nil
}

func tokenMAC(key []byte, payload string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(payload))
	return m.Sum(nil)
}

func cookieName(kind string) string { return "diseno_" + kind }

// pwVersion resume el hash de la contraseña sin revelarlo. Cambiar la contraseña
// (aunque sea a la misma: bcrypt usa otra sal) cambia la versión y las sesiones
// abiertas con la anterior dejan de servir.
func (a *App) pwVersion(hash string) string {
	return hex.EncodeToString(tokenMAC(a.sessionKey, "pwv:"+hash)[:8])
}

// setSession firma la cookie; pwHash es el password_hash actual de la cuenta.
func (a *App) setSession(w http.ResponseWriter, kind string, id int64, pwHash string) {
	a.writeSession(w, Session{Kind: kind, ID: id, Exp: time.Now().Add(sessionTTL).Unix(), PwV: a.pwVersion(pwHash)})
}

func (a *App) writeSession(w http.ResponseWriter, s Session) {
	exp := time.Unix(s.Exp, 0)
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName(s.Kind),
		Value:    signToken(a.sessionKey, s),
		Path:     "/",
		Expires:  exp,
		MaxAge:   max(1, int(s.Exp-time.Now().Unix())),
		HttpOnly: true,
		Secure:   a.Cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *App) clearSession(w http.ResponseWriter, kind string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName(kind),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   a.Cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// session lee y valida la cookie del tipo pedido. No consulta la base.
func (a *App) session(r *http.Request, kind string) (Session, bool) {
	c, err := r.Cookie(cookieName(kind))
	if err != nil {
		return Session{}, false
	}
	var s Session
	if !verifyToken(a.sessionKey, c.Value, &s) || s.Kind != kind || s.ID <= 0 || time.Now().Unix() >= s.Exp {
		return Session{}, false
	}
	return s, true
}

func (a *App) studentID(r *http.Request) (int64, bool) {
	s, ok := a.session(r, kindStudent)
	return s.ID, ok
}

func (a *App) adminID(r *http.Request) (int64, bool) {
	s, ok := a.session(r, kindAdmin)
	return s.ID, ok
}

func (a *App) requireStudent(h func(w http.ResponseWriter, r *http.Request, studentID int64)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, ok, err := a.validStudentSession(r)
		if err != nil {
			a.stuInternal(w, "verificar sesión de alumno", err)
			return
		}
		if ok {
			h(w, r, s.ID)
			return
		}
		a.clearSession(w, kindStudent)
		writeError(w, http.StatusUnauthorized, sessionExpired)
	}
}

// Estas comprobaciones completas también sirven al arranque unificado. Leer
// una cookie firmada por sí solo nunca basta para restaurar una sesión.
func (a *App) validStudentSession(r *http.Request) (Session, bool, error) {
	s, ok := a.session(r, kindStudent)
	if !ok {
		return Session{}, false, nil
	}
	authID := s.AuthID
	if authID == 0 {
		authID = s.ID // compatibilidad con las sesiones anteriores de una inscripción
	}
	var code, authCode, hash string
	var active bool
	err := a.DB.QueryRow(r.Context(), `select current.code, auth.code, auth.password_hash,
		current.active and selected_course.active and auth.active and auth_course.active
		from students current join courses selected_course on selected_course.id = current.course_id
		join students auth on auth.id = $2 join courses auth_course on auth_course.id = auth.course_id
		where current.id = $1`, s.ID, authID).Scan(&code, &authCode, &hash, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, false, nil
	}
	valid := err == nil && active && code == authCode && (s.Code == "" || s.Code == code) &&
		hmac.Equal([]byte(s.PwV), []byte(a.pwVersion(hash)))
	return s, valid, err
}

// validAdminSession exige cuenta activa y la misma versión de contraseña.
// Carga también el nombre en esa consulta para restaurar el panel sin repetirla.
func (a *App) validAdminSession(r *http.Request) (Session, admUserJSON, bool, error) {
	s, ok := a.session(r, kindAdmin)
	if !ok {
		return Session{}, admUserJSON{}, false, nil
	}
	user := admUserJSON{ID: s.ID}
	var active bool
	var hash string
	err := a.DB.QueryRow(r.Context(), "select username, active, password_hash from admins where id = $1", s.ID).
		Scan(&user.Username, &active, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, user, false, nil
	}
	return s, user, err == nil && active && hmac.Equal([]byte(s.PwV), []byte(a.pwVersion(hash))), err
}

func (a *App) requireAdmin(h func(w http.ResponseWriter, r *http.Request, adminID int64)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, _, ok, err := a.validAdminSession(r)
		if err != nil {
			a.stuInternal(w, "verificar sesión de administrador", err)
			return
		}
		if ok {
			h(w, r, s.ID)
			return
		}
		a.clearSession(w, kindAdmin)
		writeError(w, http.StatusUnauthorized, sessionExpired)
	}
}

// handleSession restaura el rol con una sola petición. Una cookie admin vieja
// no debe impedir recuperar una sesión de alumno que todavía sea válida.
func (a *App) handleSession(w http.ResponseWriter, r *http.Request) {
	_, user, ok, err := a.validAdminSession(r)
	if err != nil {
		a.stuInternal(w, "restaurar sesión de administrador", err)
		return
	}
	if ok {
		a.clearSession(w, kindStudent)
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "role": "admin", "admin": user})
		return
	}
	a.clearSession(w, kindAdmin)
	student, ok, err := a.validStudentSession(r)
	if err != nil {
		a.stuInternal(w, "restaurar sesión de alumno", err)
		return
	}
	if ok {
		a.handleMe(w, r, student.ID)
		return
	}
	a.clearSession(w, kindStudent)
	writeError(w, http.StatusUnauthorized, sessionExpired)
}

// dummyHash sirve para gastar el mismo tiempo cuando el usuario no existe.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("diseno-dummy-password"), 10)

func hashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), 10)
	return string(h), err
}

// checkPassword compara en tiempo de bcrypt. Con hash vacío (usuario
// inexistente) compara contra un hash falso y devuelve false.
func checkPassword(hash, pw string) bool {
	if hash == "" {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(pw))
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// bcryptPublicWait es lo más que una ruta pública espera turno de bcrypt
// (las pruebas lo acortan).
var bcryptPublicWait = 3 * time.Second

// withBcrypt corre fn (que usa bcrypt) con un turno de a.bcryptSem, para que
// una ráfaga no acapare todos los núcleos. Espera hasta que ctx termine.
func (a *App) withBcrypt(ctx context.Context, fn func()) error {
	select {
	case a.bcryptSem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-a.bcryptSem }()
	fn()
	return nil
}

// publicBcrypt es withBcrypt para las rutas públicas: sin turno en
// bcryptPublicWait responde 503. Devuelve false si ya respondió.
func (a *App) publicBcrypt(w http.ResponseWriter, r *http.Request, fn func()) bool {
	ctx, cancel := context.WithTimeout(r.Context(), bcryptPublicWait)
	defer cancel()
	if a.withBcrypt(ctx, fn) == nil {
		return true
	}
	if !writeCanceled(w, r.Context().Err()) {
		writeError(w, http.StatusServiceUnavailable, msgBusy)
	}
	return false
}

// hashPasswordCtx es hashPassword con turno; para el panel y el importador,
// que esperan lo que haga falta mientras la petición siga viva.
func (a *App) hashPasswordCtx(ctx context.Context, pw string) (string, error) {
	var hash string
	var err error
	if werr := a.withBcrypt(ctx, func() { hash, err = hashPassword(pw) }); werr != nil {
		return "", werr
	}
	return hash, err
}

// randomToken devuelve nBytes aleatorios en hex.
func randomToken(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b) // desde Go 1.24 nunca falla
	return hex.EncodeToString(b)
}
