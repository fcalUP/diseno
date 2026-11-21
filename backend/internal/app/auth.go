package app

import (
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
)

// Session es el contenido firmado de la cookie: {k, i, e}.
type Session struct {
	Kind string `json:"k"` // "alumno" | "admin"
	ID   int64  `json:"i"`
	Exp  int64  `json:"e"` // unix, segundos
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

func (a *App) setSession(w http.ResponseWriter, kind string, id int64) {
	exp := time.Now().Add(sessionTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName(kind),
		Value:    signToken(a.sessionKey, Session{Kind: kind, ID: id, Exp: exp.Unix()}),
		Path:     "/",
		Expires:  exp,
		MaxAge:   int(sessionTTL.Seconds()),
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
	return a.requireActive(kindStudent, "select active from students where id = $1", h)
}

func (a *App) requireAdmin(h func(w http.ResponseWriter, r *http.Request, adminID int64)) http.HandlerFunc {
	return a.requireActive(kindAdmin, "select active from admins where id = $1", h)
}

// requireActive exige cookie válida y que la fila siga existiendo y activa.
func (a *App) requireActive(kind, query string, h func(http.ResponseWriter, *http.Request, int64)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s, ok := a.session(r, kind); ok {
			var active bool
			err := a.DB.QueryRow(r.Context(), query, s.ID).Scan(&active)
			if err == nil && active {
				h(w, r, s.ID)
				return
			}
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				a.Log.Error("verificar sesión", "kind", kind, "err", err)
				writeError(w, http.StatusInternalServerError, "Error interno del servidor.")
				return
			}
		}
		a.clearSession(w, kind)
		writeError(w, http.StatusUnauthorized, sessionExpired)
	}
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

// randomToken devuelve nBytes aleatorios en hex.
func randomToken(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b) // desde Go 1.24 nunca falla
	return hex.EncodeToString(b)
}
