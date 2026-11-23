package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Cursos que trae la semilla (0002_seed.sql). POO existe pero está inactivo.
const (
	stuTAR = "Técnicas Avanzadas de Representación"
	stuAD  = "Analisis de Datos"
	stuIHM = "Interfaces Hombre-Máquina"
)

// stuEnv es una app con su propio esquema y el router completo.
type stuEnv struct {
	t *testing.T
	a *App
	h http.Handler
}

func stuSetup(t *testing.T) *stuEnv {
	t.Helper()
	a := isolatedApp(t)
	return &stuEnv{t: t, a: a, h: a.Routes()}
}

// call pasa por a.Routes() con todo el middleware; las mutaciones llevan JSON.
func (e *stuEnv) call(method, path string, body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
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
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

// want revisa el status y, si msg no está vacío, el error o el mensaje.
func (e *stuEnv) want(rec *httptest.ResponseRecorder, status int, msg string) map[string]any {
	e.t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		e.t.Fatalf("la respuesta no es un objeto JSON: %d %s", rec.Code, rec.Body)
	}
	if rec.Code != status {
		e.t.Fatalf("status %d, esperaba %d: %s", rec.Code, status, rec.Body)
	}
	if msg != "" && body["error"] != msg && body["message"] != msg {
		e.t.Fatalf("respuesta %s, esperaba el mensaje %q", rec.Body, msg)
	}
	return body
}

func (e *stuEnv) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.a.DB.Exec(context.Background(), sql, args...); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
}

func (e *stuEnv) scan(sql string, args []any, dst ...any) {
	e.t.Helper()
	if err := e.a.DB.QueryRow(context.Background(), sql, args...).Scan(dst...); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
}

func (e *stuEnv) num(sql string, args ...any) int {
	e.t.Helper()
	var n int
	e.scan(sql, args, &n)
	return n
}

func (e *stuEnv) course(name string) int64 {
	e.t.Helper()
	var id int64
	e.scan("select id from courses where name = $1", []any{name}, &id)
	return id
}

// student crea un alumno activo con todo en cero.
func (e *stuEnv) student(course, code, pw string) int64 {
	e.t.Helper()
	hash, err := hashPassword(pw)
	if err != nil {
		e.t.Fatal(err)
	}
	var id int64
	e.scan(`insert into students (course_id, code, name, sexo, email, password_hash)
		values ($1, $2, 'Alumno ' || $2, 'Hombre', $2 || '@up.edu.mx', $3) returning id`,
		[]any{e.course(course), code, hash}, &id)
	return id
}

func (e *stuEnv) badge(name string, cost, stock int) int64 {
	e.t.Helper()
	var id int64
	e.scan("insert into badges (name, slug, cost, stock) values ($1, $2, $3, $4) returning id",
		[]any{name, slugify(name), cost, stock}, &id)
	return id
}

func (e *stuEnv) loginReq(course, code, pw string) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.call("POST", "/api/login", map[string]string{"studentId": code, "password": pw, "career": course})
}

// login entra y devuelve la cookie de sesión y el cuerpo.
func (e *stuEnv) login(course, code, pw string) (*http.Cookie, map[string]any) {
	e.t.Helper()
	rec := e.loginReq(course, code, pw)
	body := e.want(rec, http.StatusOK, "")
	for _, c := range rec.Result().Cookies() {
		if c.Name == "diseno_alumno" && c.Value != "" {
			return c, body
		}
	}
	e.t.Fatal("el login no puso la cookie del alumno")
	return nil, nil
}

func (e *stuEnv) setting(key string, v any) {
	e.t.Helper()
	ctx := context.Background()
	tx, err := e.a.DB.Begin(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := e.a.SaveSettings(ctx, tx, map[string]any{key: v}); err != nil {
		e.t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		e.t.Fatal(err)
	}
	e.a.invalidateSettings()
}

// stuKeys exige exactamente esas claves: así se cuida el contrato con types.ts.
func stuKeys(t *testing.T, v any, keys ...string) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("no es un objeto: %#v", v)
	}
	if got, want := slices.Sorted(maps.Keys(m)), slices.Sorted(slices.Values(keys)); !slices.Equal(got, want) {
		t.Fatalf("claves %v, esperaba %v", got, want)
	}
	return m
}

func stuNames(list any) []string {
	var out []string
	for _, v := range list.([]any) {
		out = append(out, v.(map[string]any)["name"].(string))
	}
	return out
}

func TestStuConfig(t *testing.T) {
	e := stuSetup(t)
	cfg := e.want(e.call("GET", "/api/config", nil), http.StatusOK, "")
	stuKeys(t, cfg, "courses", "features", "levels", "levelUpGifUrl", "passwordPattern",
		"passwordHintRegister", "passwordHintReset", "passwordError")
	if got := stuNames(cfg["courses"]); !slices.Equal(got, []string{stuTAR, stuAD, stuIHM}) {
		t.Fatalf("cursos: %v", got)
	}
	first := stuKeys(t, cfg["courses"].([]any)[0], "id", "name", "sideImageUrl", "logoUrl")
	if first["id"] != float64(e.course(stuTAR)) || first["sideImageUrl"] != "/media/legacy/img/side-image.jpg" ||
		first["logoUrl"] != "/media/legacy/logo.png" {
		t.Fatalf("curso: %v", first)
	}
	features := stuKeys(t, cfg["features"], "registration", "passwordReset", "store", "leaderboard", "levelUpAnimation")
	for k, v := range features {
		if v != true {
			t.Errorf("features.%s = %v", k, v)
		}
	}
	levels := cfg["levels"].([]any)
	if len(levels) != 6 {
		t.Fatalf("niveles: %v", levels)
	}
	if last := stuKeys(t, levels[5], "level", "minExp"); last["level"] != 5.0 || last["minExp"] != 100.0 {
		t.Fatalf("último nivel: %v", last)
	}
	d := defaultSettings()
	if cfg["levelUpGifUrl"] != d.LevelUpGifURL || cfg["passwordPattern"] != d.PasswordPattern ||
		cfg["passwordHintRegister"] != d.PasswordHintRegister || cfg["passwordHintReset"] != d.PasswordHintReset ||
		cfg["passwordError"] != d.PasswordError {
		t.Fatalf("textos: %v", cfg)
	}

	// Lo que se cambia en el panel se ve en el portal.
	e.setting("registration_enabled", false)
	e.setting("password_hint_reset", "Cuatro letras o números")
	e.exec("update courses set active = false where name = $1", stuAD)
	e.exec("update courses set active = true, sort_order = 0 where name = 'POO'")
	cfg = e.want(e.call("GET", "/api/config", nil), http.StatusOK, "")
	if got := stuNames(cfg["courses"]); !slices.Equal(got, []string{"POO", stuTAR, stuIHM}) {
		t.Fatalf("cursos tras cambios: %v", got)
	}
	if f := cfg["features"].(map[string]any); f["registration"] != false || f["store"] != true {
		t.Fatalf("features tras cambios: %v", f)
	}
	if cfg["passwordHintReset"] != "Cuatro letras o números" {
		t.Fatalf("pista: %v", cfg["passwordHintReset"])
	}
}

func TestStuLogin(t *testing.T) {
	e := stuSetup(t)
	e.exec("delete from avatars") // este caso prueba explícitamente el comportamiento sin defaults
	id := e.student(stuTAR, "A001", "AB12")
	e.exec(`update students set name = 'Ana', sexo = 'Mujer', email = 'ana@up.edu.mx', monedas = 7, tareas = 3,
		asistencias = 4, games = 2, exp = 12, comments = 'Bien' where id = $1`, id)
	e.exec(`insert into purchases (student_id, badge_name, quantity)
		values ($1, 'Lápiz', 2), ($1, 'Lápiz', 1), ($1, 'Goma', 5)`, id)
	e.exec("update purchases set reverted_at = now() where badge_name = 'Goma'")
	e.exec(`insert into avatars (code, level, url) values
		('DefaultF', 1, '/media/f1.png'), ('DefaultH', 1, '/media/h1.png'), ('A001', 2, '/media/a2.png')`)

	const fields = "ID y contraseña requeridos."
	e.want(e.loginReq(stuTAR, "A001", ""), http.StatusBadRequest, fields)
	e.want(e.loginReq(stuTAR, "  ", "AB12"), http.StatusBadRequest, fields)
	e.want(e.loginReq("No existe", "A001", "AB12"), http.StatusBadRequest, stuMsgCareer)
	e.want(e.loginReq("POO", "A001", "AB12"), http.StatusBadRequest, stuMsgCareer) // curso inactivo
	e.want(e.loginReq(stuTAR, "A001", "AB13"), http.StatusUnauthorized, "Credenciales inválidas.")
	e.want(e.loginReq(stuTAR, "Z999", "AB12"), http.StatusUnauthorized, "Credenciales inválidas.")
	e.want(e.loginReq(stuAD, "A001", "AB12"), http.StatusUnauthorized, "Credenciales inválidas.") // otro curso

	rec := e.loginReq(stuTAR, " A001 ", "AB12")
	body := e.want(rec, http.StatusOK, "")
	stuKeys(t, body, "success", "role", "student", "levelUpOccurred", "courses", "requiresCourseSelection")
	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "diseno_alumno" {
			cookie = c
		}
	}
	if cookie == nil || !cookie.HttpOnly || cookie.Value == "" {
		t.Fatalf("cookie: %+v", cookie)
	}
	// Con 12 de EXP es nivel 1 y nunca había entrado: toca animación.
	if body["success"] != true || body["role"] != "student" || body["levelUpOccurred"] != true {
		t.Fatalf("login: %v", body)
	}
	st := stuKeys(t, body["student"], "id", "name", "sexo", "email", "career", "monedas", "tareas",
		"asistencias", "games", "exp", "level", "purchases", "comments", "avatarUrl", "defaultAvatarUrl")
	want := map[string]any{
		"id": "A001", "name": "Ana", "sexo": "Mujer", "email": "ana@up.edu.mx", "career": stuTAR,
		"monedas": 7.0, "tareas": 3.0, "asistencias": 4.0, "games": 2.0, "exp": 12.0, "level": 1.0,
		"purchases": map[string]any{"Lápiz": 3.0}, "comments": "Bien",
		"avatarUrl": "/media/f1.png", "defaultAvatarUrl": "/media/f1.png",
	}
	if !reflect.DeepEqual(st, want) {
		t.Fatalf("alumno:\n%v\nesperaba\n%v", st, want)
	}
	var lastLevel int
	var lastAt *time.Time
	e.scan("select last_login_level, last_login_at from students where id = $1", []any{id}, &lastLevel, &lastAt)
	if lastLevel != 1 || lastAt == nil {
		t.Fatalf("last_login_level=%d last_login_at=%v", lastLevel, lastAt)
	}

	// Con avatar propio del nivel se usa ese; el predeterminado va aparte ("" si falta).
	e.exec("update students set exp = 25 where id = $1", id)
	_, body = e.login(stuTAR, "A001", "AB12")
	if st := body["student"].(map[string]any); st["avatarUrl"] != "/media/a2.png" || st["defaultAvatarUrl"] != "" {
		t.Fatalf("avatar propio: %v", st)
	}
	// "Hombre" usa DefaultH; cualquier otro valor, DefaultF.
	e.exec("update students set sexo = 'Hombre', exp = 12 where id = $1", id)
	_, body = e.login(stuTAR, "A001", "AB12")
	if st := body["student"].(map[string]any); st["avatarUrl"] != "/media/h1.png" || st["defaultAvatarUrl"] != "/media/h1.png" {
		t.Fatalf("avatar predeterminado: %v", st)
	}

	// Cuenta desactivada: solo con la contraseña buena se dice por qué.
	e.exec("update students set active = false where id = $1", id)
	e.want(e.loginReq(stuTAR, "A001", "AB13"), http.StatusUnauthorized, "Credenciales inválidas.")
	e.want(e.loginReq(stuTAR, "A001", "AB12"), http.StatusForbidden, stuMsgInactive)
}

func TestStuLoginLockout(t *testing.T) {
	e := stuSetup(t)
	e.student(stuTAR, "B002", "CD34")
	e.student(stuTAR, "B003", "EF56")
	fail := func(n int) {
		t.Helper()
		for range n {
			e.want(e.loginReq(stuTAR, "B002", "XX00"), http.StatusUnauthorized, "Credenciales inválidas.")
		}
	}

	// Entrar bien reinicia la cuenta de fallos.
	fail(3)
	e.login(stuTAR, "B002", "CD34")
	fail(9)
	e.login(stuTAR, "B002", "CD34")

	// Diez fallos bloquean esa matrícula aunque luego llegue la contraseña buena.
	fail(10)
	e.want(e.loginReq(stuTAR, "B002", "CD34"), http.StatusTooManyRequests, stuMsgTooMany)
	e.login(stuTAR, "B003", "EF56") // las demás matrículas siguen entrando

	// Pasados los 15 minutos vuelve a entrar.
	l := e.a.Limiter
	l.mu.Lock()
	l.now = func() time.Time { return time.Now().Add(16 * time.Minute) }
	l.mu.Unlock()
	e.login(stuTAR, "B002", "CD34")

	// Tope por IP: 100 intentos cada 5 minutos.
	for range 100 {
		l.Allow("stu-login-ip:192.0.2.1", 100, 5*time.Minute)
	}
	e.want(e.loginReq(stuTAR, "B003", "EF56"), http.StatusTooManyRequests, stuMsgTooMany)
}

func TestStuLevelUp(t *testing.T) {
	e := stuSetup(t)
	id := e.student(stuTAR, "C003", "GH78")
	check := func(body map[string]any, up bool, level float64) {
		t.Helper()
		if body["levelUpOccurred"] != up || body["student"].(map[string]any)["level"] != level {
			t.Fatalf("levelUpOccurred=%v nivel=%v, esperaba %v y %v", body["levelUpOccurred"],
				body["student"].(map[string]any)["level"], up, level)
		}
	}
	_, body := e.login(stuTAR, "C003", "GH78")
	check(body, false, 0)

	// El profe sube la EXP en el panel (eso no toca last_login_level).
	e.exec("update students set exp = 30 where id = $1", id)
	_, body = e.login(stuTAR, "C003", "GH78")
	check(body, true, 2)
	cookie, body := e.login(stuTAR, "C003", "GH78")
	check(body, false, 2)

	// /api/me hace la misma revisión, así el GIF también sale al recargar.
	me := func() map[string]any {
		t.Helper()
		return e.want(e.call("GET", "/api/me", nil, cookie), http.StatusOK, "")
	}
	e.exec("update students set exp = 80 where id = $1", id)
	check(me(), true, 4)
	check(me(), false, 4)

	// Bajar de nivel no anima, pero se guarda; la siguiente subida sí cuenta.
	e.exec("update students set exp = 10 where id = $1", id)
	check(me(), false, 1)
	if n := e.num("select last_login_level from students where id = $1", id); n != 1 {
		t.Fatalf("last_login_level = %d", n)
	}
	e.exec("update students set exp = 25 where id = $1", id)
	check(me(), true, 2)
}

func TestStuMeAndLogout(t *testing.T) {
	e := stuSetup(t)
	id := e.student(stuTAR, "D004", "JK90")
	e.want(e.call("GET", "/api/me", nil), http.StatusUnauthorized, sessionExpired)

	cookie, login := e.login(stuTAR, "D004", "JK90")
	var before, after time.Time
	e.scan("select last_login_at from students where id = $1", []any{id}, &before)
	me := e.want(e.call("GET", "/api/me", nil, cookie), http.StatusOK, "")
	stuKeys(t, me, "success", "role", "student", "levelUpOccurred", "courses", "requiresCourseSelection")
	if me["success"] != true || me["role"] != "student" || !reflect.DeepEqual(me["student"], login["student"]) {
		t.Fatalf("/api/me: %v\nlogin: %v", me, login)
	}
	// Recargar no cuenta como entrar.
	e.scan("select last_login_at from students where id = $1", []any{id}, &after)
	if !after.Equal(before) {
		t.Fatalf("last_login_at cambió con /api/me: %v → %v", before, after)
	}

	rec := e.call("POST", "/api/logout", map[string]any{}, cookie)
	if body := e.want(rec, http.StatusOK, ""); body["success"] != true {
		t.Fatalf("logout: %v", body)
	}
	cleared := false
	for _, c := range rec.Result().Cookies() {
		cleared = cleared || (c.Name == "diseno_alumno" && c.MaxAge < 0)
	}
	if !cleared {
		t.Fatal("logout no borró la cookie")
	}

	// Un alumno desactivado pierde la sesión.
	e.exec("update students set active = false where id = $1", id)
	e.want(e.call("GET", "/api/me", nil, cookie), http.StatusUnauthorized, sessionExpired)
}

func TestStuRegister(t *testing.T) {
	e := stuSetup(t)
	reg := func(id, name, sexo, pw, career string) *httptest.ResponseRecorder {
		t.Helper()
		return e.call("POST", "/api/register",
			map[string]string{"id": id, "name": name, "sexo": sexo, "password": pw, "career": career})
	}
	const invalid = "Datos inválidos o incompletos. Contraseña debe ser 4 HEX mayúsculas y carrera requerida."
	const ok = "Alumno registrado exitosamente."

	e.want(reg(" R001 ", " Rita Pérez ", "Mujer", "ZX98", stuTAR), http.StatusCreated, ok)
	var id int64
	var code, name, sexo, email, hash, course string
	var counters int
	var active bool
	e.scan(`select s.id, s.code, s.name, s.sexo, s.email, s.password_hash, c.name, s.active,
			s.tareas + s.monedas + s.asistencias + s.exp + s.games + s.last_login_level
		from students s join courses c on c.id = s.course_id where s.code = 'R001'`, nil,
		&id, &code, &name, &sexo, &email, &hash, &course, &active, &counters)
	if code != "R001" || name != "Rita Pérez" || sexo != "Mujer" || email != "R001@up.edu.mx" ||
		course != stuTAR || !active || counters != 0 || hash == "ZX98" || !checkPassword(hash, "ZX98") {
		t.Fatalf("fila: %s %q %s %s %s activo=%v contadores=%d", code, name, sexo, email, course, active, counters)
	}
	var actorType, label, action, entity, entityID, afterCode string
	var actorID int64
	var undoable bool
	e.scan(`select actor_type, actor_id, actor_label, action, entity, entity_id, undoable, after->>'code'
		from audit_log where entity = 'student'`, nil,
		&actorType, &actorID, &label, &action, &entity, &entityID, &undoable, &afterCode)
	if actorType != "student" || actorID != id || label != "R001" || action != "create" || entity != "student" ||
		entityID != strconv.FormatInt(id, 10) || !undoable || afterCode != "R001" {
		t.Fatalf("auditoría: %s %d %s %s %s %s %v %s", actorType, actorID, label, action, entity, entityID, undoable, afterCode)
	}
	e.login(stuTAR, "R001", "ZX98")

	e.want(reg("R001", "Otra", "Hombre", "AB12", stuTAR), http.StatusConflict, "El ID de alumno ya existe.")
	e.want(reg("R001", "Rita Pérez", "Mujer", "ZX98", stuAD), http.StatusCreated, ok) // otro curso, otra fila

	for _, bad := range [][5]string{
		{"R002", "Rob", "Hombre", "zx98", stuTAR}, // minúsculas
		{"R002", "Rob", "Hombre", "ZX9", stuTAR},
		{"R002", "", "Hombre", "ZX98", stuTAR},
		{"", "Rob", "Hombre", "ZX98", stuTAR},
		{"R002", "Rob", " ", "ZX98", stuTAR},
		{"R002", "Rob", "Hombre", "ZX98", ""},
		{"DefaultH", "Rob", "Hombre", "ZX98", stuTAR}, // es el nombre de los avatares predeterminados
		{"R002", strings.Repeat("n", 121), "Hombre", "ZX98", stuTAR},
	} {
		e.want(reg(bad[0], bad[1], bad[2], bad[3], bad[4]), http.StatusBadRequest, invalid)
	}
	e.want(reg("R002", "Rob", "Hombre", "ZX98", "No existe"), http.StatusBadRequest, stuMsgCareer)
	e.want(reg("R002", "Rob", "Hombre", "ZX98", "POO"), http.StatusBadRequest, stuMsgCareer)

	// La regla de contraseña y el dominio del correo salen de Ajustes.
	e.setting("password_pattern", "^[0-9]{6}$")
	e.setting("student_email_domain", "alumnos.up.mx")
	e.want(reg("R003", "Rob", "Hombre", "ZX98", stuTAR), http.StatusBadRequest, invalid)
	e.want(reg("R003", "Rob", "Hombre", "123456", stuTAR), http.StatusCreated, ok)
	e.scan("select email from students where code = 'R003'", nil, &email)
	if email != "R003@alumnos.up.mx" {
		t.Fatalf("correo: %s", email)
	}

	e.setting("registration_enabled", false)
	e.want(reg("R004", "Rob", "Hombre", "123456", stuTAR), http.StatusForbidden, "El registro está cerrado.")
	e.setting("registration_enabled", true)

	// Tope de 20 registros por hora por IP.
	for range 20 {
		e.a.Limiter.Allow("stu-register-ip:192.0.2.1", 20, time.Hour)
	}
	e.want(reg("R005", "Rob", "Hombre", "123456", stuTAR), http.StatusTooManyRequests, stuMsgTooMany)
}

func TestStuPasswordReset(t *testing.T) {
	e := stuSetup(t)
	mails := mailFake(t, e.a, http.StatusOK)
	id := e.student(stuTAR, "E005", "LM12")
	e.exec("update students set email = 'e005@correo.mx' where id = $1", id)

	const sent = "Código de restablecimiento enviado a tu correo electrónico."
	const invalid = "Datos inválidos. Asegúrate de usar un código, contraseña válida (4 HEX mayúsculas) y carrera."
	send := func(code string) *httptest.ResponseRecorder {
		t.Helper()
		return e.call("POST", "/api/send-reset-code", map[string]string{"id": code, "career": stuTAR})
	}
	reset := func(code, pw string) *httptest.ResponseRecorder {
		t.Helper()
		return e.call("POST", "/api/reset-password-with-code",
			map[string]string{"id": "E005", "code": code, "password": pw, "career": stuTAR})
	}
	codeRe := regexp.MustCompile(`<h1[^>]*>(\d{6})</h1>`)
	sendOK := func() (string, mailFakeReq) {
		t.Helper()
		e.want(send("E005"), http.StatusOK, sent)
		m := mailWait(t, mails)
		match := codeRe.FindStringSubmatch(m.Body.HTML)
		if match == nil {
			t.Fatalf("el correo no trae el código: %s", m.Body.HTML)
		}
		return match[1], m
	}
	wrong := func(code string) string {
		if code == "000000" {
			return "000001"
		}
		return "000000"
	}
	codes := func() int { t.Helper(); return e.num("select count(*) from reset_codes") }

	// Se manda al correo del alumno más las copias; en la base solo queda el hash.
	code, m := sendOK()
	if !slices.Equal(m.Body.To, []string{"e005@correo.mx", "fcal@up.edu.mx"}) || m.Body.From != "onboarding@resend.dev" ||
		m.Body.Subject != "Código de restablecimiento de contraseña" || !strings.Contains(m.Body.HTML, "10 minutos") {
		t.Fatalf("correo: %+v", m.Body)
	}
	var hash string
	var attempts int
	var ttlOK bool
	e.scan(`select code_hash, attempts,
			expires_at between now() + interval '9 minutes' and now() + interval '11 minutes'
		from reset_codes where student_id = $1`, []any{id}, &hash, &attempts, &ttlOK)
	if hash != stuHashCode(code) || strings.Contains(hash, code) || attempts != 0 || !ttlOK {
		t.Fatalf("código guardado: %s intentos=%d vigencia=%v", hash, attempts, ttlOK)
	}

	// Cinco errores lo invalidan, aunque después llegue el bueno. La fila se queda
	// hasta vencer: mientras, no se manda otro código (ni gasta un envío).
	for range 5 {
		e.want(reset(wrong(code), "NP34"), http.StatusUnauthorized, stuMsgBadCode)
	}
	e.want(reset(code, "NP34"), http.StatusUnauthorized, stuMsgBadCode)
	e.want(send("E005"), http.StatusTooManyRequests, stuMsgTooMany)

	// Uno vencido tampoco sirve y se borra. Al vencer el quemado ya se puede pedir otro.
	e.exec("update reset_codes set expires_at = now() - interval '1 second' where student_id = $1", id)
	code, _ = sendOK()
	if n := e.num("select attempts from reset_codes where student_id = $1", id); n != 0 {
		t.Fatalf("un código nuevo tras vencer debe empezar en 0 intentos: %d", n)
	}
	e.exec("update reset_codes set expires_at = now() - interval '1 second' where student_id = $1", id)
	e.want(reset(code, "NP34"), http.StatusUnauthorized, stuMsgBadCode)
	if n := codes(); n != 0 {
		t.Fatalf("el código vencido no se borró (%d)", n)
	}

	// Datos mal formados no gastan intentos; con 4 errores el bueno todavía entra.
	code, _ = sendOK()
	e.want(reset(code, "np34"), http.StatusBadRequest, invalid)
	e.want(reset("", "NP34"), http.StatusBadRequest, invalid)
	for range 4 {
		e.want(reset(wrong(code), "NP34"), http.StatusUnauthorized, stuMsgBadCode)
	}
	if n := e.num("select attempts from reset_codes where student_id = $1", id); n != 4 {
		t.Fatalf("intentos = %d", n)
	}
	e.want(reset(code, "NP34"), http.StatusOK, "Contraseña actualizada correctamente.")
	if n := codes(); n != 0 {
		t.Fatalf("el código usado no se borró (%d)", n)
	}
	e.want(e.loginReq(stuTAR, "E005", "LM12"), http.StatusUnauthorized, "Credenciales inválidas.")
	e.login(stuTAR, "E005", "NP34")
	var actorType, action string
	var undoable bool
	e.scan("select actor_type, action, undoable from audit_log where entity = 'student' and entity_id = $1",
		[]any{strconv.FormatInt(id, 10)}, &actorType, &action, &undoable)
	if actorType != "student" || action != "update" || undoable {
		t.Fatalf("auditoría: %s %s deshacer=%v", actorType, action, undoable)
	}

	// Tres envíos por hora por alumno, y ya van tres.
	e.want(send("E005"), http.StatusTooManyRequests, stuMsgTooMany)

	e.want(send("NOEXISTE"), http.StatusNotFound, "ID de alumno no encontrado en la carrera seleccionada.")
	e.want(e.call("POST", "/api/send-reset-code", map[string]string{"id": "E005"}), http.StatusBadRequest,
		"ID de alumno y carrera son requeridos.")
	e.want(e.call("POST", "/api/send-reset-code", map[string]string{"id": "E005", "career": "POO"}),
		http.StatusBadRequest, stuMsgCareer)
	e.want(e.call("POST", "/api/reset-password-with-code",
		map[string]string{"id": "NOEXISTE", "code": "123456", "password": "NP34", "career": stuTAR}),
		http.StatusNotFound, "ID no encontrado en la carrera seleccionada.")
	e.want(reset("123456", "QR56"), http.StatusUnauthorized, stuMsgBadCode) // nunca se pidió

	// Si Resend falla la respuesta sigue siendo 200; sin correo guardado va a matrícula@dominio.
	other := e.student(stuTAR, "E006", "LM12")
	e.exec("update students set email = '' where id = $1", other)
	failing := mailFake(t, e.a, http.StatusInternalServerError)
	e.want(send("E006"), http.StatusOK, sent)
	if m := mailWait(t, failing); m.Body.To[0] != "E006@up.edu.mx" {
		t.Fatalf("destinatario: %v", m.Body.To)
	}

	// Una cuenta desactivada no recibe códigos.
	e.exec("update students set active = false where id = $1", other)
	e.want(send("E006"), http.StatusForbidden, stuMsgInactive)

	e.setting("password_reset_enabled", false)
	e.want(send("E005"), http.StatusForbidden, stuMsgResetOff)
	e.want(reset("123456", "QR56"), http.StatusForbidden, stuMsgResetOff)
}

func TestStuBadges(t *testing.T) {
	e := stuSetup(t)
	lapiz := e.badge("Lápiz", 10, 5)
	internet := e.badge("Internet", 30, 2)
	calc := e.badge("Calculadora", 20, 1)
	vieja := e.badge("Vieja", 5, 9)
	e.exec("update badges set sort_order = 1, image_url = '/media/legacy/badges/lapiz.png' where id = $1", lapiz)
	e.exec("update badges set active = false where id = $1", vieja)
	e.exec("insert into course_hidden_badges (course_id, badge_id) values ($1, $2), ($1, $3)",
		e.course(stuAD), internet, calc)

	get := func(query string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		return e.call("GET", "/api/badges"+query, nil, cookies...)
	}
	body := e.want(get("?career="+url.QueryEscape(stuTAR)), http.StatusOK, "")
	stuKeys(t, body, "success", "badges")
	// Activas y visibles, por sort_order y luego nombre.
	if got := stuNames(body["badges"]); body["success"] != true || !slices.Equal(got, []string{"Calculadora", "Internet", "Lápiz"}) {
		t.Fatalf("insignias TAR: %v", body)
	}
	b := stuKeys(t, body["badges"].([]any)[2], "id", "name", "quantity", "cost", "imageUrl")
	if b["id"] != float64(lapiz) || b["quantity"] != 5.0 || b["cost"] != 10.0 || b["imageUrl"] != "/media/legacy/badges/lapiz.png" {
		t.Fatalf("insignia: %v", b)
	}
	if got := stuNames(e.want(get("?career="+url.QueryEscape(stuAD)), http.StatusOK, "")["badges"]); !slices.Equal(got, []string{"Lápiz"}) {
		t.Fatalf("insignias de Análisis de Datos: %v", got)
	}

	// Sin career se usa el curso del alumno con sesión.
	e.student(stuAD, "F006", "ST78")
	cookie, _ := e.login(stuAD, "F006", "ST78")
	if got := stuNames(e.want(get("", cookie), http.StatusOK, "")["badges"]); !slices.Equal(got, []string{"Lápiz"}) {
		t.Fatalf("insignias por sesión: %v", got)
	}
	e.want(get(""), http.StatusBadRequest, stuMsgNoCareer)
	e.want(get("?career=No+existe"), http.StatusBadRequest, stuMsgCareer)
	e.want(get("?career=POO"), http.StatusBadRequest, stuMsgCareer)

	e.setting("store_enabled", false)
	e.want(get("?career="+url.QueryEscape(stuTAR)), http.StatusForbidden, stuMsgStoreOff)
}

func TestStuPurchase(t *testing.T) {
	e := stuSetup(t)
	ctx := context.Background()
	id := e.student(stuAD, "G007", "UV90")
	e.exec("update students set monedas = 100 where id = $1", id)
	lapiz := e.badge("Lápiz", 10, 5)
	goma := e.badge("Goma", 25, 2)
	regla := e.badge("Regla", 60, 3)
	calc := e.badge("Calculadora", 5, 9)
	vieja := e.badge("Vieja", 1, 9)
	e.exec("insert into course_hidden_badges (course_id, badge_id) values ($1, $2)", e.course(stuAD), calc)
	e.exec("update badges set active = false where id = $1", vieja)
	cookie, _ := e.login(stuAD, "G007", "UV90")

	type item = map[string]int64
	buy := func(items ...item) *httptest.ResponseRecorder {
		t.Helper()
		return e.call("POST", "/api/purchase", map[string]any{"items": items}, cookie)
	}
	const hidden = "Esta insignia no está disponible para tu grupo."
	unchanged := func() {
		t.Helper()
		if n := e.num("select monedas from students where id = $1", id); n != 100 {
			t.Fatalf("monedas = %d", n)
		}
		if n := e.num("select sum(stock) from badges where id = any($1)", []int64{lapiz, goma, regla}); n != 10 {
			t.Fatalf("existencias = %d", n)
		}
		if n := e.num("select count(*) from purchases") + e.num("select count(*) from audit_log where entity = 'purchase'"); n != 0 {
			t.Fatalf("quedaron %d compras o entradas de bitácora", n)
		}
	}

	e.want(e.call("POST", "/api/purchase", map[string]any{"items": []item{{"badgeId": lapiz, "quantity": 1, "unitCost": 10}}}),
		http.StatusUnauthorized, sessionExpired)
	e.want(buy(), http.StatusBadRequest, stuMsgBadBuy)
	e.want(buy(item{"badgeId": lapiz, "quantity": 0, "unitCost": 10}), http.StatusBadRequest, stuMsgBadBuy)
	e.want(buy(item{"badgeId": lapiz, "quantity": 3_000_000_000, "unitCost": 10}), http.StatusBadRequest, stuMsgBadBuy)
	e.want(buy(item{"badgeId": lapiz, "quantity": 1, "unitCost": -10}), http.StatusBadRequest, stuMsgBadBuy)
	e.want(e.call("POST", "/api/purchase", map[string]any{"items": []map[string]any{{"badgeId": lapiz, "quantity": 1.5, "unitCost": 10}}}, cookie),
		http.StatusBadRequest, "")
	e.want(buy(item{"badgeId": calc, "quantity": 1, "unitCost": 5}), http.StatusForbidden, hidden)
	e.want(buy(item{"badgeId": vieja, "quantity": 1, "unitCost": 1}), http.StatusForbidden, hidden)
	e.want(buy(item{"badgeId": 999999, "quantity": 1, "unitCost": 1}), http.StatusForbidden, hidden)
	e.want(buy(item{"badgeId": lapiz, "quantity": 1, "unitCost": 9}), http.StatusConflict, "El precio de Lápiz cambió. Recarga la tienda.")
	// Repetida con dos precios: alguno ya no es el actual.
	e.want(buy(item{"badgeId": lapiz, "quantity": 1, "unitCost": 10}, item{"badgeId": lapiz, "quantity": 1, "unitCost": 9}),
		http.StatusConflict, "El precio de Lápiz cambió. Recarga la tienda.")
	e.want(buy(item{"badgeId": goma, "quantity": 3, "unitCost": 25}), http.StatusBadRequest, "Sólo quedan 2 unidades de Goma")
	// Los renglones repetidos se suman antes de revisar existencias.
	e.want(buy(item{"badgeId": goma, "quantity": 2, "unitCost": 25}, item{"badgeId": goma, "quantity": 1, "unitCost": 25}),
		http.StatusBadRequest, "Sólo quedan 2 unidades de Goma")
	e.want(buy(item{"badgeId": regla, "quantity": 1, "unitCost": 60}, item{"badgeId": goma, "quantity": 2, "unitCost": 25}),
		http.StatusBadRequest, "Monedas insuficientes")
	// Un renglón malo y no se compra nada del carrito.
	e.want(buy(item{"badgeId": lapiz, "quantity": 1, "unitCost": 10}, item{"badgeId": calc, "quantity": 1, "unitCost": 5}),
		http.StatusForbidden, hidden)
	e.want(buy(item{"badgeId": lapiz, "quantity": 1, "unitCost": 10}, item{"badgeId": goma, "quantity": 1, "unitCost": 20}),
		http.StatusConflict, "El precio de Goma cambió. Recarga la tienda.")
	unchanged()

	// Lápiz 2 + 1 y Goma 1: 30 + 25 = 55 de 100.
	body := e.want(buy(item{"badgeId": lapiz, "quantity": 2, "unitCost": 10}, item{"badgeId": goma, "quantity": 1, "unitCost": 25},
		item{"badgeId": lapiz, "quantity": 1, "unitCost": 10}), http.StatusOK, "Compra realizada correctamente. Monedas restantes: 45")
	stuKeys(t, body, "success", "message", "newCoins", "badges", "purchases")
	wantBadges := []any{
		map[string]any{"id": float64(lapiz), "quantity": 2.0},
		map[string]any{"id": float64(goma), "quantity": 1.0},
	}
	if body["success"] != true || body["newCoins"] != 45.0 || !reflect.DeepEqual(body["badges"], wantBadges) ||
		!reflect.DeepEqual(body["purchases"], map[string]any{"Lápiz": 3.0, "Goma": 1.0}) {
		t.Fatalf("respuesta: %v", body)
	}
	if n := e.num("select monedas from students where id = $1", id); n != 45 {
		t.Fatalf("monedas = %d", n)
	}

	// Un renglón por insignia con las monedas encadenadas.
	type purchaseRow struct {
		BadgeID                         int64
		Qty, Unit, Total, Before, After int
		Source                          string
	}
	rowsOf := func() []purchaseRow {
		t.Helper()
		rows, _ := e.a.DB.Query(ctx, `select badge_id, quantity, unit_cost, total_cost, coins_before, coins_after, source
			from purchases order by id`)
		out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[purchaseRow])
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got, want := rowsOf(), []purchaseRow{{lapiz, 3, 10, 30, 100, 70, "app"}, {goma, 1, 25, 25, 70, 45, "app"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("compras:\n%v\nesperaba\n%v", got, want)
	}

	// Una entrada de bitácora por renglón, con el mismo lote.
	type auditRow struct {
		ActorType string
		ActorID   int64
		Label     string
		Action    string
		EntityID  string
		Batch     string
		Undoable  bool
		After     map[string]any
	}
	rows, _ := e.a.DB.Query(ctx, `select actor_type, actor_id, actor_label, action, entity_id, batch_id::text, undoable, after
		from audit_log where entity = 'purchase' order by id`)
	entries, err := pgx.CollectRows(rows, pgx.RowToStructByPos[auditRow])
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Batch == "" || entries[0].Batch != entries[1].Batch {
		t.Fatalf("bitácora: %+v", entries)
	}
	for i, wantAfter := range []map[string]float64{
		{"student_coins": 70, "badge_stock": 2, "quantity": 3, "coins_before": 100},
		{"student_coins": 45, "badge_stock": 1, "quantity": 1, "coins_before": 70},
	} {
		en := entries[i]
		if en.ActorType != "student" || en.ActorID != id || en.Label != "G007" || en.Action != "purchase" || !en.Undoable ||
			en.EntityID != strconv.FormatInt(int64(en.After["id"].(float64)), 10) {
			t.Fatalf("entrada %d: %+v", i, en)
		}
		for k, v := range wantAfter {
			if en.After[k] != v {
				t.Fatalf("entrada %d: after[%s] = %v, esperaba %v", i, k, en.After[k], v)
			}
		}
	}

	// La siguiente compra sigue desde lo que quedó.
	body = e.want(buy(item{"badgeId": goma, "quantity": 1, "unitCost": 25}), http.StatusOK, "")
	if body["newCoins"] != 20.0 || !reflect.DeepEqual(body["purchases"], map[string]any{"Lápiz": 3.0, "Goma": 2.0}) {
		t.Fatalf("segunda compra: %v", body)
	}
	if last := rowsOf()[2]; last.Before != 45 || last.After != 20 {
		t.Fatalf("encadenado: %+v", last)
	}
	e.want(buy(item{"badgeId": goma, "quantity": 1, "unitCost": 25}), http.StatusBadRequest, "Sólo quedan 0 unidades de Goma")

	// Lo revertido ya no cuenta para el alumno.
	e.exec("update purchases set reverted_at = now() where badge_id = $1", goma)
	me := e.want(e.call("GET", "/api/me", nil, cookie), http.StatusOK, "")
	if p := me["student"].(map[string]any)["purchases"]; !reflect.DeepEqual(p, map[string]any{"Lápiz": 3.0}) {
		t.Fatalf("compras tras revertir: %v", p)
	}

	e.setting("store_enabled", false)
	e.want(buy(item{"badgeId": lapiz, "quantity": 1, "unitCost": 10}), http.StatusForbidden, stuMsgStoreOff)
}

func TestStuPurchaseConcurrent(t *testing.T) {
	e := stuSetup(t)
	last := e.badge("Última", 10, 1)
	var cookies []*http.Cookie
	for _, code := range []string{"H008", "H009"} {
		id := e.student(stuTAR, code, "WX12")
		e.exec("update students set monedas = 50 where id = $1", id)
		c, _ := e.login(stuTAR, code, "WX12")
		cookies = append(cookies, c)
	}
	var wg sync.WaitGroup
	statuses := make(chan int, len(cookies))
	for _, c := range cookies {
		wg.Go(func() {
			rec := e.call("POST", "/api/purchase",
				map[string]any{"items": []map[string]int64{{"badgeId": last, "quantity": 1, "unitCost": 10}}}, c)
			statuses <- rec.Code
		})
	}
	wg.Wait()
	close(statuses)
	var got []int
	for s := range statuses {
		got = append(got, s)
	}
	slices.Sort(got)
	if !slices.Equal(got, []int{http.StatusOK, http.StatusBadRequest}) {
		t.Fatalf("dos compras de la última unidad: %v", got)
	}
	if n := e.num("select stock from badges where id = $1", last); n != 0 {
		t.Fatalf("existencias = %d", n)
	}
	if n := e.num("select count(*) from purchases"); n != 1 {
		t.Fatalf("compras = %d", n)
	}
}

func TestStuPurchaseEmail(t *testing.T) {
	e := stuSetup(t)
	mails := mailFake(t, e.a, http.StatusOK)
	e.setting("email_cc", []string{"profe@up.edu.mx", "coordinacion@up.edu.mx"})
	id := e.student(stuAD, "J010", "YZ34")
	e.exec("update students set monedas = 80, email = '' where id = $1", id)
	lapiz := e.badge("Lápiz", 10, 5)
	goma := e.badge("Goma", 25, 2)
	cookie, _ := e.login(stuAD, "J010", "YZ34")
	buy := func(items ...map[string]int64) map[string]any {
		t.Helper()
		return e.want(e.call("POST", "/api/purchase", map[string]any{"items": items}, cookie), http.StatusOK, "")
	}

	buy(map[string]int64{"badgeId": lapiz, "quantity": 2, "unitCost": 10},
		map[string]int64{"badgeId": goma, "quantity": 1, "unitCost": 25})
	m := mailWait(t, mails)
	if m.Auth != "Bearer re_prueba" || m.Path != "/emails" {
		t.Fatalf("petición: %+v", m)
	}
	if !slices.Equal(m.Body.To, []string{"J010@up.edu.mx", "profe@up.edu.mx", "coordinacion@up.edu.mx"}) ||
		m.Body.From != "onboarding@resend.dev" || m.Body.Subject != "Confirmación de compra de insignias" || m.Body.HTML != "" {
		t.Fatalf("correo: %+v", m.Body)
	}
	want := "Hola,\n\nSe ha realizado una compra de insignias:\n\n- Alumno: J010@up.edu.mx\n- Carrera: Analisis de Datos\n" +
		"- Insignia: Lápiz\n- Cantidad: 2\n- Insignia: Goma\n- Cantidad: 1\n- Costo total: 45 monedas\n" +
		"- Monedas anteriores: 80\n- Monedas restantes: 35\n\nGracias."
	if m.Body.Text != want {
		t.Fatalf("texto:\n%s\nesperaba\n%s", m.Body.Text, want)
	}

	// Si Resend falla, la compra igual se hace.
	failing := mailFake(t, e.a, http.StatusInternalServerError)
	if body := buy(map[string]int64{"badgeId": lapiz, "quantity": 1, "unitCost": 10}); body["newCoins"] != 25.0 {
		t.Fatalf("compra con Resend caído: %v", body)
	}
	mailWait(t, failing)

	// Con el aviso apagado no sale nada.
	e.setting("purchase_email_enabled", false)
	buy(map[string]int64{"badgeId": lapiz, "quantity": 1, "unitCost": 10})
	select {
	case m := <-failing:
		t.Fatalf("no debió mandar correo: %+v", m)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestStuLeaderboard(t *testing.T) {
	e := stuSetup(t)
	add := func(course, code, name, sexo string, exp int, active bool) int64 {
		t.Helper()
		id := e.student(course, code, "LB12")
		e.exec("update students set name = $2, sexo = $3, exp = $4, active = $5 where id = $1", id, name, sexo, exp, active)
		return id
	}
	ana := add(stuTAR, "K001", "Ana", "Mujer", 30, true)
	beto := add(stuTAR, "K002", "Beto", "Hombre", 80, true)
	caro := add(stuTAR, "K003", "Caro", "Mujer", 30, true) // empata con Ana y va después por id
	add(stuTAR, "K004", "Dani", "Hombre", 200, false)      // inactivo
	add(stuAD, "K005", "Eva", "Mujer", 500, true)          // otro curso
	e.exec("insert into purchases (student_id, badge_name, quantity) values ($1, 'Lápiz', 2), ($1, 'Goma', 1), ($2, 'Lápiz', 4)",
		ana, caro)
	e.exec("insert into purchases (student_id, badge_name, quantity, reverted_at) values ($1, 'Goma', 7, now())", beto)

	e.want(e.call("GET", "/api/leaderboard", nil), http.StatusUnauthorized, sessionExpired)
	cookie, _ := e.login(stuTAR, "K001", "LB12")
	rec := e.call("GET", "/api/leaderboard", nil, cookie)
	body := e.want(rec, http.StatusOK, "")
	stuKeys(t, body, "success", "leaderboard")
	rows := body["leaderboard"].([]any)
	want := []map[string]any{
		{"rank": 1.0, "name": "Beto", "sexo": "Hombre", "badges": 0.0, "exp": 80.0, "level": 4.0},
		{"rank": 2.0, "name": "Ana", "sexo": "Mujer", "badges": 3.0, "exp": 30.0, "level": 2.0},
		{"rank": 3.0, "name": "Caro", "sexo": "Mujer", "badges": 4.0, "exp": 30.0, "level": 2.0},
	}
	if body["success"] != true || len(rows) != len(want) {
		t.Fatalf("leaderboard: %v", body)
	}
	for i, w := range want {
		if got := stuKeys(t, rows[i], "rank", "name", "sexo", "badges", "exp", "level"); !reflect.DeepEqual(got, w) {
			t.Fatalf("fila %d: %v, esperaba %v", i, got, w)
		}
	}
	for _, code := range []string{"K001", "K002", "K003", "K004", "K005"} {
		if strings.Contains(rec.Body.String(), code) {
			t.Fatalf("el leaderboard expone la matrícula %s: %s", code, rec.Body)
		}
	}

	// Cada quien ve el de su curso.
	other, _ := e.login(stuAD, "K005", "LB12")
	if got := stuNames(e.want(e.call("GET", "/api/leaderboard", nil, other), http.StatusOK, "")["leaderboard"]); !slices.Equal(got, []string{"Eva"}) {
		t.Fatalf("leaderboard de otro curso: %v", got)
	}

	e.setting("leaderboard_enabled", false)
	e.want(e.call("GET", "/api/leaderboard", nil, cookie), http.StatusForbidden, "El leaderboard está desactivado.")
}
