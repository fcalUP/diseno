package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTokenTamper(t *testing.T) {
	key := deriveKey("diseno-session:", strings.Repeat("x", 32))
	tok := signToken(key, Session{Kind: kindStudent, ID: 7, Exp: time.Now().Add(time.Hour).Unix()})

	var s Session
	if !verifyToken(key, tok, &s) || s.ID != 7 || s.Kind != kindStudent {
		t.Fatalf("token válido rechazado: %+v", s)
	}
	payload, sig, _ := strings.Cut(tok, ".")
	forged := signToken(key, Session{Kind: kindAdmin, ID: 1, Exp: s.Exp})
	forgedPayload, _, _ := strings.Cut(forged, ".")
	bad := []string{
		forgedPayload + "." + sig,               // payload cambiado, firma vieja
		payload + "." + sig[:len(sig)-2] + "AA", // firma alterada
		payload,                                 // sin firma
		"",
		"basura.basura",
	}
	for _, b := range bad {
		if verifyToken(key, b, &Session{}) {
			t.Errorf("token alterado aceptado: %q", b)
		}
	}
	if verifyToken(deriveKey("diseno-session:", strings.Repeat("y", 32)), tok, &Session{}) {
		t.Error("token aceptado con otra clave")
	}
}

func requestWithCookie(name, value string) *http.Request {
	r := httptest.NewRequest("GET", "/api/me", nil)
	r.AddCookie(&http.Cookie{Name: name, Value: value})
	return r
}

func TestSessionCookie(t *testing.T) {
	a := appNoDB(t)
	a.Cfg.CookieSecure = true

	rec := httptest.NewRecorder()
	a.setSession(rec, kindStudent, 42)
	c := rec.Result().Cookies()[0]
	if c.Name != "diseno_alumno" || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode ||
		c.Path != "/" || c.MaxAge != 12*3600 {
		t.Fatalf("cookie con atributos incorrectos: %+v", c)
	}
	if id, ok := a.studentID(requestWithCookie(c.Name, c.Value)); !ok || id != 42 {
		t.Fatalf("studentID = %d, %v", id, ok)
	}
	// Un token de alumno no sirve como admin aunque se copie a la otra cookie.
	if _, ok := a.adminID(requestWithCookie("diseno_admin", c.Value)); ok {
		t.Fatal("token de alumno aceptado como admin")
	}
	expired := signToken(a.sessionKey, Session{Kind: kindStudent, ID: 42, Exp: time.Now().Add(-time.Second).Unix()})
	if _, ok := a.studentID(requestWithCookie("diseno_alumno", expired)); ok {
		t.Fatal("token vencido aceptado")
	}

	rec = httptest.NewRecorder()
	a.clearSession(rec, kindAdmin)
	if c := rec.Result().Cookies()[0]; c.Name != "diseno_admin" || c.MaxAge >= 0 {
		t.Fatalf("clearSession no borra la cookie: %+v", c)
	}
}

func TestRequireStudentWithoutCookie(t *testing.T) {
	a := appNoDB(t)
	called := false
	h := a.requireStudent(func(w http.ResponseWriter, r *http.Request, id int64) { called = true })
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest("GET", "/api/me", nil))
	if called || rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), sessionExpired) {
		t.Fatalf("esperaba 401, llegó %d %s", rec.Code, rec.Body)
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("debió borrar la cookie")
	}
}

func TestRequireActiveRow(t *testing.T) {
	a := isolatedApp(t)
	ctx := context.Background()
	var id int64
	err := a.DB.QueryRow(ctx, `insert into students (course_id, code, name, password_hash)
		select id, '0001', 'Prueba', 'x' from courses order by id limit 1 returning id`).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	a.setSession(rec, kindStudent, id)
	cookie := rec.Result().Cookies()[0]

	var got int64
	h := a.requireStudent(func(w http.ResponseWriter, r *http.Request, sid int64) {
		got = sid
		w.WriteHeader(http.StatusNoContent)
	})
	call := func() int {
		rec := httptest.NewRecorder()
		h(rec, requestWithCookie(cookie.Name, cookie.Value))
		return rec.Code
	}
	if code := call(); code != http.StatusNoContent || got != id {
		t.Fatalf("activo: %d, id=%d", code, got)
	}
	if _, err := a.DB.Exec(ctx, "update students set active = false where id = $1", id); err != nil {
		t.Fatal(err)
	}
	if code := call(); code != http.StatusUnauthorized {
		t.Fatalf("inactivo: %d", code)
	}
	if _, err := a.DB.Exec(ctx, "delete from students where id = $1", id); err != nil {
		t.Fatal(err)
	}
	if code := call(); code != http.StatusUnauthorized {
		t.Fatalf("borrado: %d", code)
	}
}

func TestPasswords(t *testing.T) {
	h, err := hashPassword("AB12")
	if err != nil {
		t.Fatal(err)
	}
	if !checkPassword(h, "AB12") || checkPassword(h, "AB13") || checkPassword("", "AB12") {
		t.Fatal("checkPassword no se comporta como se espera")
	}
	if len(randomToken(16)) != 32 {
		t.Fatal("randomToken(16) debe dar 32 caracteres hex")
	}
}
