package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

// ---------- Ayudas de las pruebas del panel (B2a) ----------

// admT junta lo que usan las pruebas del panel: la app sobre su propio esquema,
// las rutas reales y la cookie del admin principal ("root").
type admT struct {
	t      *testing.T
	a      *App
	h      http.Handler
	ctx    context.Context
	id     int64
	cookie *http.Cookie
}

func admSetup(t *testing.T) *admT {
	t.Helper()
	a := isolatedApp(t)
	e := &admT{t: t, a: a, h: a.Routes(), ctx: context.Background()}
	e.id, e.cookie = e.newAdmin("root", "clave-segura-1")
	return e
}

// newAdmin crea un administrador directo en la base y le arma una cookie de sesión.
func (e *admT) newAdmin(username, pw string) (int64, *http.Cookie) {
	e.t.Helper()
	hash, err := hashPassword(pw)
	if err != nil {
		e.t.Fatal(err)
	}
	var id int64
	e.one(&id, `insert into admins (username, password_hash) values ($1, $2) returning id`, username, hash)
	rec := httptest.NewRecorder()
	e.a.setSession(rec, kindAdmin, id, hash)
	return id, rec.Result().Cookies()[0]
}

// do hace la petición por Routes(); las mutaciones llevan Content-Type JSON, como exige el CSRF.
func (e *admT) do(method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

// admReq llama con la cookie del admin principal, exige el estado want y decodifica la respuesta.
func admReq[T any](e *admT, method, path string, body any, want int) T {
	e.t.Helper()
	rec := e.do(method, path, body, e.cookie)
	if rec.Code != want {
		e.t.Fatalf("%s %s: esperaba %d, llegó %d %s", method, path, want, rec.Code, rec.Body)
	}
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		e.t.Fatalf("%s %s: respuesta inesperada: %v %s", method, path, err, rec.Body)
	}
	return out
}

func (e *admT) call(method, path string, body any, want int) map[string]any {
	e.t.Helper()
	return admReq[map[string]any](e, method, path, body, want)
}

// fails exige un error con estado want y devuelve el mensaje.
func (e *admT) fails(method, path string, body any, want int) string {
	e.t.Helper()
	msg, _ := e.call(method, path, body, want)["error"].(string)
	if msg == "" {
		e.t.Fatalf("%s %s: la respuesta %d no trae error", method, path, want)
	}
	return msg
}

func (e *admT) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.a.DB.Exec(e.ctx, sql, args...); err != nil {
		e.t.Fatal(err)
	}
}

// one lee una fila de una sola columna en dst.
func (e *admT) one(dst any, sql string, args ...any) {
	e.t.Helper()
	if err := e.a.DB.QueryRow(e.ctx, sql, args...).Scan(dst); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
}

func (e *admT) count(sql string, args ...any) int {
	e.t.Helper()
	var n int
	e.one(&n, sql, args...)
	return n
}

func (e *admT) courseID(sheet string) int64 {
	e.t.Helper()
	var id int64
	e.one(&id, `select id from courses where sheet_name = $1`, sheet)
	return id
}

// student inserta un alumno directo (con un hash falso: aquí no hace falta bcrypt).
func (e *admT) student(courseID int64, code, name string, monedas, exp int) int64 {
	e.t.Helper()
	var id int64
	e.one(&id, `insert into students (course_id, code, name, password_hash, monedas, exp)
		values ($1, $2, $3, 'x', $4, $5) returning id`, courseID, code, name, monedas, exp)
	return id
}

func (e *admT) badge(name string, stock, cost int) int64 {
	e.t.Helper()
	var id int64
	e.one(&id, `insert into badges (name, slug, stock, cost) values ($1, $2, $3, $4) returning id`,
		name, slugify(name), stock, cost)
	return id
}

func (e *admT) purchase(studentID, badgeID int64, qty, unitCost int, reverted bool) int64 {
	e.t.Helper()
	var id int64
	e.one(&id, `insert into purchases (student_id, badge_id, badge_name, quantity, unit_cost, total_cost, reverted_at)
		select $1, b.id, b.name, $3::int, $4::int, $3::int * $4::int, case when $5::boolean then now() end from badges b where b.id = $2
		returning id`, studentID, badgeID, qty, unitCost, reverted)
	return id
}

type admAudit struct {
	Action, Entity, EntityID, Summary, BatchID string
	Before, After                              map[string]any
	Undoable                                   bool
}

func (e *admT) audit(id int64) admAudit {
	e.t.Helper()
	var r admAudit
	var before, after []byte
	err := e.a.DB.QueryRow(e.ctx, `select action, entity, coalesce(entity_id, ''), summary,
			coalesce(batch_id::text, ''), before, after, undoable
		from audit_log where id = $1`, id).
		Scan(&r.Action, &r.Entity, &r.EntityID, &r.Summary, &r.BatchID, &before, &after, &r.Undoable)
	if err != nil {
		e.t.Fatalf("auditoría %d: %v", id, err)
	}
	for _, p := range []struct {
		raw []byte
		dst *map[string]any
	}{{before, &r.Before}, {after, &r.After}} {
		if p.raw != nil {
			if err := json.Unmarshal(p.raw, p.dst); err != nil {
				e.t.Fatal(err)
			}
		}
	}
	return r
}

// admSameJSON compara got con el JSON want sin importar el orden de las llaves.
func admSameJSON(t *testing.T, got any, want string) {
	t.Helper()
	var w any
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatal(err)
	}
	g, _ := json.Marshal(got)
	wb, _ := json.Marshal(w)
	if string(g) != string(wb) {
		t.Fatalf("JSON distinto:\n got %s\nwant %s", g, wb)
	}
}

// ---------- Pruebas ----------

func TestAdmAuthRequired(t *testing.T) {
	e := admSetup(t)
	_, offCookie := e.newAdmin("apagado", "clave-segura-2")
	e.exec(`update admins set active = false where username = 'apagado'`)

	routes := []string{
		"POST /api/admin/logout", "GET /api/admin/me", "GET /api/admin/students",
		"POST /api/admin/update-student-data", "POST /api/admin/students", "POST /api/admin/students/generate",
		"POST /api/admin/students/bulk", "PATCH /api/admin/students/1", "DELETE /api/admin/students/1",
		"POST /api/admin/students/1/password", "GET /api/admin/purchases", "POST /api/admin/purchases/1/revert",
		"POST /api/admin/purchases/1/restore", "GET /api/admin/settings", "PATCH /api/admin/settings",
		"GET /api/admin/admins", "POST /api/admin/admins", "PATCH /api/admin/admins/1",
		"DELETE /api/admin/admins/1", "POST /api/admin/uploads",
	}
	for _, rt := range routes {
		method, path, _ := strings.Cut(rt, " ")
		var body any
		if method != http.MethodGet {
			body = map[string]any{}
		}
		if rec := e.do(method, path, body, nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s sin cookie: %d", rt, rec.Code)
		}
		if rec := e.do(method, path, body, offCookie); rec.Code != http.StatusUnauthorized ||
			!strings.Contains(rec.Body.String(), sessionExpired) {
			t.Errorf("%s con admin desactivado: %d %s", rt, rec.Code, rec.Body)
		}
	}
	got := admReq[struct{ Admin admUserJSON }](e, "GET", "/api/admin/me", nil, http.StatusOK)
	if got.Admin.ID != e.id || got.Admin.Username != "root" {
		t.Fatalf("me: %+v", got.Admin)
	}
}

func TestAdmLogin(t *testing.T) {
	e := admSetup(t)
	login := func(user, pw string) *httptest.ResponseRecorder {
		t.Helper()
		return e.do("POST", "/api/admin-login", map[string]string{"username": user, "password": pw}, nil)
	}

	if rec := login("root", "otra-cosa"); rec.Code != http.StatusUnauthorized ||
		!strings.Contains(rec.Body.String(), "Invalid admin credentials.") {
		t.Fatalf("contraseña mala: %d %s", rec.Code, rec.Body)
	}
	if rec := login("nadie", "clave-segura-1"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("usuario inexistente: %d", rec.Code)
	}

	rec := login(" root ", "clave-segura-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("login válido: %d %s", rec.Code, rec.Body)
	}
	var resp struct {
		Success bool
		Message string
		Admin   admUserJSON
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Success || resp.Message != "Admin login successful." || resp.Admin.ID != e.id || resp.Admin.Username != "root" {
		t.Fatalf("respuesta del login: %+v", resp)
	}
	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "diseno_admin" {
			cookie = c
		}
	}
	if cookie == nil || !cookie.HttpOnly {
		t.Fatalf("el login no dejó la cookie de admin: %v", rec.Result().Cookies())
	}
	if rec := e.do("GET", "/api/admin/me", nil, cookie); rec.Code != http.StatusOK {
		t.Fatalf("me con la cookie del login: %d", rec.Code)
	}
	var lastLogin *time.Time
	e.one(&lastLogin, `select last_login_at from admins where id = $1`, e.id)
	if lastLogin == nil {
		t.Fatal("no se guardó last_login_at")
	}
	var auditID int64
	e.one(&auditID, `select id from audit_log where action = 'login'`)
	if a := e.audit(auditID); a.Entity != "admin" || a.EntityID != fmt.Sprint(e.id) || a.Undoable {
		t.Fatalf("auditoría del login: %+v", a)
	}

	rec = e.do("POST", "/api/admin/logout", map[string]any{}, cookie)
	if rec.Code != http.StatusOK || len(rec.Result().Cookies()) == 0 || rec.Result().Cookies()[0].MaxAge >= 0 {
		t.Fatalf("logout: %d %v", rec.Code, rec.Result().Cookies())
	}

	e.newAdmin("apagado", "clave-segura-2")
	e.exec(`update admins set active = false where username = 'apagado'`)
	if rec := login("apagado", "clave-segura-2"); rec.Code != http.StatusForbidden {
		t.Fatalf("admin desactivado: %d", rec.Code)
	}

	// 10 fallos seguidos bloquean al usuario aunque luego la contraseña sea la buena.
	for i := range 10 {
		if rec := login("root", "mala"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("fallo %d: %d", i+1, rec.Code)
		}
	}
	if rec := login("root", "clave-segura-1"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("usuario bloqueado: %d", rec.Code)
	}

	// Por IP: 30 intentos en 15 minutos. Hasta aquí van 15 desde la misma IP.
	for i := 15; i < 30; i++ {
		if rec := login(fmt.Sprintf("otro%d", i), "x"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("intento %d: %d", i+1, rec.Code)
		}
	}
	if rec := login("apagado", "clave-segura-2"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("IP bloqueada: %d", rec.Code)
	}
}

func TestAdmStudentsList(t *testing.T) {
	e := admSetup(t)
	tar, ad := e.courseID("TAR"), e.courseID("Analisis de Datos")
	ana := e.student(tar, "0001", "Ana López", 10, 30) // nivel 2
	beto := e.student(tar, "0002", "Beto", 0, 0)
	e.student(ad, "0001", "Ana en Datos", 0, 100) // nivel 5
	e.exec(`update students set active = false, email = 'beto@up.edu.mx' where id = $1`, beto)
	e.exec(`insert into avatars (code, level, url) values ('0001', 2, '/media/a2.png'), ('0001', 0, '/media/a0.png')`)
	b := e.badge("Internet", 10, 1)
	e.purchase(ana, b, 2, 1, false)
	e.purchase(ana, b, 3, 1, true) // revertida: no cuenta

	type listResp struct {
		Success  bool
		Students []adminStudentJSON
	}
	all := admReq[listResp](e, "GET", "/api/admin/students", nil, http.StatusOK)
	if !all.Success || len(all.Students) != 3 {
		t.Fatalf("lista completa: %+v", all)
	}
	s := all.Students[0]
	if s.ID != ana || s.Level != 2 || s.Badges != 2 || !slices.Equal(s.AvatarLevels, []int{0, 2}) ||
		s.CourseID != tar || s.CourseName != "Técnicas Avanzadas de Representación" || s.Monedas != 10 {
		t.Fatalf("primer alumno: %+v", s)
	}
	if all.Students[1].AvatarLevels == nil || all.Students[2].Level != 5 {
		t.Fatalf("avatarLevels debe ser [] y el nivel salir de la tabla levels: %+v", all.Students[1:])
	}

	cases := map[string]int{
		fmt.Sprint("?courseId=", tar):                     2,
		"?career=" + url.QueryEscape("Analisis de Datos"): 1,
		"?q=ANA":                                 2,
		"?q=beto%40up":                           1, // por correo
		"?active=true":                           2,
		"?active=false":                          1,
		"?active=all":                            3,
		fmt.Sprint("?courseId=", tar, "&q=0001"): 1,
	}
	for q, want := range cases {
		if got := admReq[listResp](e, "GET", "/api/admin/students"+q, nil, http.StatusOK); len(got.Students) != want {
			t.Errorf("%s: %d alumnos, esperaba %d", q, len(got.Students), want)
		}
	}
	for _, q := range []string{"?career=Nada", "?active=si", "?courseId=abc", "?courseId=-1"} {
		e.fails("GET", "/api/admin/students"+q, nil, http.StatusBadRequest)
	}
}

func TestAdmUpdateStudentData(t *testing.T) {
	e := admSetup(t)
	id := e.student(e.courseID("TAR"), "0003", "Caro", 5, 0)
	e.exec(`update students set last_login_level = 1 where id = $1`, id)

	got := e.call("POST", "/api/admin/update-student-data", map[string]any{
		"id": id, "tareas": 2, "monedas": 7, "asistencias": 3, "games": 1, "exp": 55,
		"career": "TAR", "rowIndex": 9, // el modal heredado manda extras; se ignoran
	}, http.StatusOK)
	if got["success"] != true || got["newLevel"] != 3.0 || got["message"] != "Student 0003 data updated successfully." {
		t.Fatalf("respuesta: %v", got)
	}
	var tareas, monedas, asistencias, games, exp, lastLevel int
	err := e.a.DB.QueryRow(e.ctx, `select tareas, monedas, asistencias, games, exp, last_login_level
		from students where id = $1`, id).Scan(&tareas, &monedas, &asistencias, &games, &exp, &lastLevel)
	if err != nil {
		t.Fatal(err)
	}
	if tareas != 2 || monedas != 7 || asistencias != 3 || games != 1 || exp != 55 || lastLevel != 1 {
		t.Fatalf("fila: %d %d %d %d %d, last_login_level=%d", tareas, monedas, asistencias, games, exp, lastLevel)
	}
	a := e.audit(int64(got["auditId"].(float64)))
	if a.Action != "update" || a.Entity != "student" || !a.Undoable || a.Before["monedas"] != 5.0 ||
		a.After["monedas"] != 7.0 || a.After["last_login_level"] != 1.0 {
		t.Fatalf("auditoría: %+v", a)
	}

	// games puede faltar (como en el endpoint viejo) y se queda igual.
	e.call("POST", "/api/admin/update-student-data",
		map[string]any{"id": id, "tareas": 2, "monedas": 7, "asistencias": 3, "exp": 9}, http.StatusOK)
	e.one(&games, `select games from students where id = $1`, id)
	if games != 1 {
		t.Fatalf("games cambió sin venir en el cuerpo: %d", games)
	}

	if msg := e.fails("POST", "/api/admin/update-student-data",
		map[string]any{"id": id, "tareas": 1, "monedas": 1, "asistencias": 1}, http.StatusBadRequest); msg != "Incomplete data for update." {
		t.Fatalf("sin exp: %q", msg)
	}
	e.fails("POST", "/api/admin/update-student-data",
		map[string]any{"id": id, "tareas": -1, "monedas": 1, "asistencias": 1, "exp": 1}, http.StatusBadRequest)
	e.fails("POST", "/api/admin/update-student-data",
		map[string]any{"id": 99999, "tareas": 1, "monedas": 1, "asistencias": 1, "exp": 1}, http.StatusNotFound)
}
