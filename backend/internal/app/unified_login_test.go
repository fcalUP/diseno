package app

import (
	"net/http"
	"testing"
	"time"
)

func unifiedAdmin(e *stuEnv, username, password string) int64 {
	e.t.Helper()
	hash, err := hashPassword(password)
	if err != nil {
		e.t.Fatal(err)
	}
	var id int64
	e.scan(`insert into admins (username, password_hash) values ($1, $2) returning id`, []any{username, hash}, &id)
	return id
}

func TestUnifiedLoginRolesAndSessions(t *testing.T) {
	e := stuSetup(t)
	e.student(stuTAR, "001234", "AB12")
	adminID := unifiedAdmin(e, "profesor", " clave-admin-1 ")
	login := func(code, pw string, cookies ...*http.Cookie) *http.Cookie {
		t.Helper()
		rec := e.call("POST", "/api/login", map[string]string{"studentId": code, "password": pw}, cookies...)
		body := e.want(rec, http.StatusOK, "")
		if code == "001234" {
			if body["role"] != "student" || body["student"].(map[string]any)["id"] != "001234" || !secCleared(rec, "diseno_admin") {
				t.Fatalf("login alumno: %s", rec.Body)
			}
			return secCookie(rec, "diseno_alumno")
		}
		stuKeys(t, body, "success", "role", "admin")
		if body["role"] != "admin" || body["admin"].(map[string]any)["id"] != float64(adminID) || !secCleared(rec, "diseno_alumno") {
			t.Fatalf("login administrador: %s", rec.Body)
		}
		cookie := secCookie(rec, "diseno_admin")
		if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
			t.Fatalf("cookie admin: %+v", cookie)
		}
		return cookie
	}
	studentCookie := login("001234", "AB12")
	adminCookie := login(" profesor ", " clave-admin-1 ", studentCookie)
	e.want(e.call("GET", "/api/admin/me", nil, adminCookie), http.StatusOK, "")
	e.want(e.call("GET", "/api/me", nil, adminCookie), http.StatusUnauthorized, sessionExpired)
	e.want(e.call("GET", "/api/admin/me", nil, studentCookie), http.StatusUnauthorized, sessionExpired)
	if count := e.num(`select count(*) from audit_log where actor_type = 'admin' and actor_id = $1 and action = 'login' and not undoable`, adminID); count != 1 {
		t.Fatalf("faltó auditoría no reversible de login: %d", count)
	}
	var lastLogin *time.Time
	e.scan(`select last_login_at from admins where id = $1`, []any{adminID}, &lastLogin)
	if lastLogin == nil {
		t.Fatal("faltó last_login_at de administrador")
	}
	login("001234", "AB12", adminCookie)
	// La cookie emitida por el login único conserva la revocación por clave.
	e.exec(`update admins set password_hash = 'revocado' where id = $1`, adminID)
	rec := e.call("GET", "/api/admin/me", nil, adminCookie)
	e.want(rec, http.StatusUnauthorized, sessionExpired)
	if !secCleared(rec, "diseno_admin") {
		t.Fatal("la sesión revocada de administrador no se eliminó")
	}
}

func TestUnifiedLoginCollidingIdentifiers(t *testing.T) {
	e := stuSetup(t)
	studentID := e.student(stuTAR, "profesor", "AB12")
	adminID := unifiedAdmin(e, "profesor", "clave-admin-1")
	check := func(password, role string) {
		t.Helper()
		body := e.want(e.call("POST", "/api/login", map[string]string{"studentId": "profesor", "password": password}), http.StatusOK, "")
		if body["role"] != role {
			t.Fatalf("contraseña %s obtuvo rol %v, esperaba %s", password, body["role"], role)
		}
	}
	check("clave-admin-1", "admin")
	check("AB12", "student")
	// Con las mismas credenciales en ambos roles, el admin activo tiene prioridad.
	e.exec(`update students set password_hash = (select password_hash from admins where id = $1) where id = $2`, adminID, studentID)
	check("clave-admin-1", "admin")
	// Una cuenta admin desactivada nunca da permisos de administración; una
	// inscripción activa con sus propias credenciales sigue siendo utilizable.
	e.exec(`update admins set active = false where id = $1`, adminID)
	check("clave-admin-1", "student")
}

func TestUnifiedLoginInvalidAndInactive(t *testing.T) {
	e := stuSetup(t)
	adminID := unifiedAdmin(e, "profesor", "clave-admin-1")
	studentID := e.student(stuTAR, "001234", "AB12")
	for _, credentials := range []map[string]string{
		{"studentId": "profesor", "password": "incorrecta"},
		{"studentId": "001234", "password": "incorrecta"},
		{"studentId": "inexistente", "password": "clave-admin-1"},
	} {
		body := e.want(e.call("POST", "/api/login", credentials), http.StatusUnauthorized, "Credenciales inválidas.")
		stuKeys(t, body, "error")
	}
	for _, credentials := range []map[string]string{
		{"studentId": "", "password": "clave-admin-1"},
		{"studentId": "profesor", "password": ""},
		{"studentId": "001234", "password": "   "},
	} {
		e.want(e.call("POST", "/api/login", credentials), http.StatusBadRequest, "ID y contraseña requeridos.")
	}
	e.exec(`update admins set active = false where id = $1`, adminID)
	e.exec(`update students set active = false where id = $1`, studentID)
	e.want(e.call("POST", "/api/login", map[string]string{"studentId": "profesor", "password": "clave-admin-1"}), http.StatusForbidden, "Tu cuenta de administrador está desactivada.")
	e.want(e.call("POST", "/api/login", map[string]string{"studentId": "001234", "password": "AB12"}), http.StatusForbidden, stuMsgInactive)
	// Un rol enviado por el cliente no puede otorgar permisos de administrador.
	e.exec(`update students set active = true where id = $1`, studentID)
	rec := e.call("POST", "/api/login", map[string]string{"studentId": "001234", "password": "AB12", "role": "admin"})
	if body := e.want(rec, http.StatusOK, ""); body["role"] != "student" || secCookie(rec, "diseno_admin") != nil {
		t.Fatalf("el cliente alteró el rol: %s", rec.Body)
	}
}

func TestUnifiedLoginAdminLimitsShared(t *testing.T) {
	for _, firstPath := range []string{"/api/login", "/api/admin-login"} {
		t.Run(firstPath, func(t *testing.T) {
			e := stuSetup(t)
			unifiedAdmin(e, "profesor", "clave-admin-1")
			request := func(path, password, remote string) {
				t.Helper()
				key := "studentId"
				if path == "/api/admin-login" {
					key = "username"
				}
				rec := secPost(e.h, path, map[string]string{key: "profesor", "password": password}, remote)
				want := http.StatusUnauthorized
				if password == "clave-admin-1" {
					want = http.StatusTooManyRequests
				}
				e.want(rec, want, "")
			}
			for range admLoginUserMax {
				request(firstPath, "incorrecta", "192.0.2.1:1234")
			}
			for _, path := range []string{"/api/login", "/api/admin-login"} {
				request(path, "clave-admin-1", "192.0.2.1:5678")
			}
			// El tope global por cuenta se aplica también desde otra red.
			for range admLoginAcctMax {
				e.a.Limiter.Allow("admin-login-acct:profesor", admLoginAcctMax, admLoginWindow)
			}
			request("/api/login", "clave-admin-1", "198.51.100.1:1234")
		})
	}
}

func TestUnifiedLoginStudentKeepsNetworkCapacity(t *testing.T) {
	e := stuSetup(t)
	e.student(stuTAR, "001234", "AB12")
	unifiedAdmin(e, "profesor", "clave-admin-1")
	// Llenar el límite de red de administradores bloquea ese rol en ambos
	// endpoints; los alumnos todavía conservan su propio cupo de red.
	for range admLoginIPMax {
		e.a.Limiter.Allow("admin-login-ip:192.0.2.1", admLoginIPMax, admLoginWindow)
	}
	e.want(secPost(e.h, "/api/login", map[string]string{"studentId": "profesor", "password": "clave-admin-1"}, "192.0.2.1:1234"), http.StatusTooManyRequests, stuMsgTooMany)
	e.want(secPost(e.h, "/api/admin-login", map[string]string{"username": "profesor", "password": "clave-admin-1"}, "192.0.2.1:1234"), http.StatusTooManyRequests, stuMsgTooMany)
	// Repetir accesos correctos por encima de 30 no debe consumir ni exigir el
	// cupo de admin. El contador por cuenta sigue aumentando normalmente.
	for range admLoginIPMax + 1 {
		rec := secPost(e.h, "/api/login", map[string]string{"studentId": "001234", "password": "AB12"}, "192.0.2.1:1234")
		if body := e.want(rec, http.StatusOK, ""); body["role"] != "student" {
			t.Fatalf("login alumno: %s", rec.Body)
		}
	}
}

func TestSecUnifiedAdminLoginBurst(t *testing.T) {
	secWait(t, time.Minute)
	e := stuSetup(t)
	unifiedAdmin(e, "profesor", "clave-admin-1")
	got := secBurst(e.h, "/api/login", map[string]string{"studentId": "profesor", "password": "incorrecta"}, 40)
	if got[http.StatusUnauthorized] != admLoginUserMax || got[http.StatusTooManyRequests] != 40-admLoginUserMax {
		t.Fatalf("ráfaga de login único: %v", got)
	}
	e.want(secPost(e.h, "/api/admin-login", map[string]string{"username": "profesor", "password": "clave-admin-1"}, "[2001:db8:1:2::abcd]:443"), http.StatusTooManyRequests, stuMsgTooMany)
	e.want(secPost(e.h, "/api/login", map[string]string{"studentId": "profesor", "password": "clave-admin-1"}, "203.0.113.1:1234"), http.StatusOK, "")
}

func TestUnifiedSessionRoleRestoration(t *testing.T) {
	e := stuSetup(t)
	authID := e.student(stuTAR, "001234", "AB12")
	e.student(stuAD, "001234", "CD34")
	adminID := unifiedAdmin(e, "profesor", "clave-admin-1")
	studentLogin := e.call("POST", "/api/login", map[string]string{"studentId": "001234", "password": "AB12"})
	e.want(studentLogin, http.StatusOK, "")
	studentCookie := secCookie(studentLogin, "diseno_alumno")
	adminLogin := e.call("POST", "/api/login", map[string]string{"studentId": "profesor", "password": "clave-admin-1"})
	e.want(adminLogin, http.StatusOK, "")
	adminCookie := secCookie(adminLogin, "diseno_admin")
	checkStudent := func(cookies ...*http.Cookie) {
		t.Helper()
		body := e.want(e.call("GET", "/api/session", nil, cookies...), http.StatusOK, "")
		if body["role"] != "student" || body["requiresCourseSelection"] != true || body["levelUpOccurred"] != false {
			t.Fatalf("sesión de alumno perdió la selección pendiente: %v", body)
		}
	}
	checkStudent(studentCookie)
	for _, cookies := range [][]*http.Cookie{{adminCookie}, {studentCookie, adminCookie}} {
		rec := e.call("GET", "/api/session", nil, cookies...)
		body := e.want(rec, http.StatusOK, "")
		stuKeys(t, body, "success", "role", "admin")
		if body["role"] != "admin" || body["admin"].(map[string]any)["id"] != float64(adminID) || !secCleared(rec, "diseno_alumno") {
			t.Fatalf("no restauró el administrador con prioridad: %v", body)
		}
	}
	tampered := *adminCookie
	tampered.Value += "x"
	checkStudent(&tampered, studentCookie)
	// Una clave de admin cambiada no impide recuperar al alumno vigente.
	e.exec(`update admins set password_hash = 'revocado' where id = $1`, adminID)
	checkStudent(adminCookie, studentCookie)
	if rec := e.call("GET", "/api/session", nil, adminCookie, studentCookie); !secCleared(rec, "diseno_admin") {
		t.Fatal("no limpió la cookie de admin revocada")
	}
	// Sigue anclada a la inscripción cuya contraseña autenticó al alumno.
	e.exec(`update students set password_hash = 'revocado' where id = $1`, authID)
	for _, cookies := range [][]*http.Cookie{nil, {studentCookie}, {adminCookie}, {adminCookie, studentCookie}} {
		rec := e.call("GET", "/api/session", nil, cookies...)
		e.want(rec, http.StatusUnauthorized, sessionExpired)
		if !secCleared(rec, "diseno_alumno") || !secCleared(rec, "diseno_admin") {
			t.Fatal("restauración inválida no limpió ambas cookies")
		}
	}
}
