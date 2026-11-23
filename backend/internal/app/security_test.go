package app

// Regresiones de la revisión de seguridad R1 (H1, M1, M2, M3, L1, L4, L5) y del
// orden de las compras del alumno.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// secPost manda un POST JSON por el router completo desde remote ("ip:puerto").
// No usa t: también corre en gorutinas.
func secPost(h http.Handler, path string, body any, remote string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remote
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// secBurst manda n POST a la vez, cada uno desde otra dirección de la misma /64, y cuenta los estados.
func secBurst(h http.Handler, path string, body any, n int) map[int]int {
	var mu sync.Mutex
	var wg sync.WaitGroup
	got := map[int]int{}
	for i := range n {
		wg.Go(func() {
			rec := secPost(h, path, body, fmt.Sprintf("[2001:db8:1:2::%x]:443", i+1))
			mu.Lock()
			got[rec.Code]++
			mu.Unlock()
		})
	}
	wg.Wait()
	return got
}

// secWait cambia bcryptPublicWait mientras dura la prueba.
func secWait(t *testing.T, d time.Duration) {
	old := bcryptPublicWait
	bcryptPublicWait = d
	t.Cleanup(func() { bcryptPublicWait = old })
}

func secCookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name && c.Value != "" {
			return c
		}
	}
	return nil
}

func secCleared(rec *httptest.ResponseRecorder, name string) bool {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name && c.MaxAge < 0 {
			return true
		}
	}
	return false
}

// H1 y M1: los intentos se cuentan antes de bcrypt (una ráfaga en paralelo no pasa
// completa), una IPv6 cuenta como su /64 y bloquear una red no deja fuera al alumno
// desde otra. El tope de la cuenta sí aplica desde cualquier red.
func TestSecStudentLoginBurst(t *testing.T) {
	secWait(t, time.Minute) // con -race cada bcrypt tarda ~0.6 s
	e := stuSetup(t)
	e.student(stuTAR, "S001", "AB12")
	login := func(pw string) map[string]string {
		return map[string]string{"studentId": "S001", "password": pw, "career": stuTAR}
	}

	got := secBurst(e.h, "/api/login", login("ZZ99"), 40)
	if got[http.StatusUnauthorized] != stuLoginMax || got[http.StatusTooManyRequests] != 40-stuLoginMax {
		t.Fatalf("ráfaga de 40 desde una /64: %v; esperaba %d evaluados y el resto 429", got, stuLoginMax)
	}
	if rec := secPost(e.h, "/api/login", login("AB12"), "[2001:db8:1:2:ffff::1]:443"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("la buena desde la misma /64: %d", rec.Code)
	}
	if rec := secPost(e.h, "/api/login", login("AB12"), "[2001:db8:1:3::1]:443"); rec.Code != http.StatusOK {
		t.Fatalf("la buena desde otra /64: %d %s", rec.Code, rec.Body)
	}

	for range stuLoginAcctMax {
		e.a.Limiter.Allow(stuLoginAcctKey(e.course(stuTAR), "S001"), stuLoginAcctMax, stuLoginWindow)
	}
	if rec := secPost(e.h, "/api/login", login("AB12"), "198.51.100.9:1"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("con el tope de la cuenta lleno: %d", rec.Code)
	}

	// El tope del registro también va por /64.
	for range 20 {
		e.a.Limiter.Allow("stu-register-ip:2001:db8:5:5::", 20, time.Hour)
	}
	rec := secPost(e.h, "/api/register", map[string]string{"id": "S002", "name": "Sam", "sexo": "Hombre",
		"password": "AB12", "career": stuTAR}, "[2001:db8:5:5::99]:443")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("registro desde una /64 llena: %d", rec.Code)
	}
}

// H1 y M1 en el login del panel. Como en t4, "ROOT" cuenta para el usuario "root".
func TestSecAdminLoginBurst(t *testing.T) {
	secWait(t, time.Minute)
	e := admSetup(t) // root / clave-segura-1
	login := func(user, pw string) map[string]string { return map[string]string{"username": user, "password": pw} }

	// 30 pasan el tope de la red y, de esos, 10 el de (usuario, red).
	got := secBurst(e.h, "/api/admin-login", login("ROOT", "mala"), 40)
	if got[http.StatusUnauthorized] != admLoginUserMax || got[http.StatusTooManyRequests] != 40-admLoginUserMax {
		t.Fatalf("ráfaga de 40 desde una /64: %v", got)
	}
	if rec := secPost(e.h, "/api/admin-login", login("root", "clave-segura-1"), "[2001:db8:1:2::abcd]:443"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("la buena desde la red bloqueada: %d", rec.Code)
	}
	if rec := secPost(e.h, "/api/admin-login", login("root", "clave-segura-1"), "203.0.113.5:1"); rec.Code != http.StatusOK {
		t.Fatalf("el profesor desde otra red: %d %s", rec.Code, rec.Body)
	}

	for range admLoginAcctMax {
		e.a.Limiter.Allow("admin-login-acct:root", admLoginAcctMax, admLoginWindow)
	}
	if rec := secPost(e.h, "/api/admin-login", login("root", "clave-segura-1"), "203.0.113.6:1"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("con el tope del usuario lleno: %d", rec.Code)
	}
}

// M2: cambiar una contraseña cierra las sesiones abiertas con la anterior.
// (El caso del admin que cambia la suya está en TestAdmAdmins.)
func TestSecSessionsFollowPassword(t *testing.T) {
	e := admSetup(t)
	created := e.call("POST", "/api/admin/students",
		map[string]any{"courseId": e.courseID("TAR"), "code": "S100", "name": "Sesión", "password": "AB12"}, http.StatusCreated)
	sid := int64(created["student"].(map[string]any)["id"].(float64))
	stuLogin := func(pw string) *http.Cookie {
		t.Helper()
		rec := secPost(e.h, "/api/login", map[string]string{"studentId": "S100", "password": pw, "career": stuTAR}, "192.0.2.10:1")
		c := secCookie(rec, "diseno_alumno")
		if rec.Code != http.StatusOK || c == nil {
			t.Fatalf("login del alumno: %d %s", rec.Code, rec.Body)
		}
		return c
	}
	me := func(c *http.Cookie) *httptest.ResponseRecorder { return e.do("GET", "/api/me", nil, c) }

	c1 := stuLogin("AB12")
	if rec := me(c1); rec.Code != http.StatusOK {
		t.Fatalf("me: %d", rec.Code)
	}
	res := e.call("POST", fmt.Sprintf("/api/admin/students/%d/password", sid), map[string]any{}, http.StatusOK)
	if rec := me(c1); rec.Code != http.StatusUnauthorized || !secCleared(rec, "diseno_alumno") {
		t.Fatalf("sesión tras el cambio del profesor: %d %v", rec.Code, rec.Result().Cookies())
	}
	if rec := e.do("POST", "/api/purchase", map[string]any{"items": []any{}}, c1); rec.Code != http.StatusUnauthorized {
		t.Fatalf("compra con la sesión vieja: %d", rec.Code)
	}

	// Deshacer regresa el hash anterior: también es un cambio.
	c2 := stuLogin(res["password"].(string))
	e.call("POST", fmt.Sprintf("/api/admin/audit/%d/undo", int64(res["auditId"].(float64))), map[string]any{}, http.StatusOK)
	if rec := me(c2); rec.Code != http.StatusUnauthorized {
		t.Fatalf("sesión tras deshacer: %d", rec.Code)
	}
	stuLogin("AB12")

	// Un token sin versión (de antes de este cambio) ya no sirve.
	legacy := signToken(e.a.sessionKey, Session{Kind: kindStudent, ID: sid, Exp: time.Now().Add(time.Hour).Unix()})
	if rec := me(&http.Cookie{Name: "diseno_alumno", Value: legacy}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("token sin versión: %d", rec.Code)
	}

	// Admins: la sesión de otro cae cuando le cambian la contraseña; la propia no.
	e.call("POST", "/api/admin/admins", map[string]any{"username": "otro", "password": "clave-otro-1"}, http.StatusCreated)
	rec := secPost(e.h, "/api/admin-login", map[string]string{"username": "otro", "password": "clave-otro-1"}, "192.0.2.11:1")
	other := secCookie(rec, "diseno_admin")
	if rec.Code != http.StatusOK || other == nil {
		t.Fatalf("login de otro: %d %s", rec.Code, rec.Body)
	}
	var otherID int64
	e.one(&otherID, `select id from admins where username = 'otro'`)
	e.call("PATCH", fmt.Sprintf("/api/admin/admins/%d", otherID), map[string]any{"password": "clave-otro-2"}, http.StatusOK)
	if rec := e.do("GET", "/api/admin/me", nil, other); rec.Code != http.StatusUnauthorized {
		t.Fatalf("sesión de otro tras cambiarle la contraseña: %d", rec.Code)
	}
	if rec := e.do("GET", "/api/admin/me", nil, e.cookie); rec.Code != http.StatusOK {
		t.Fatalf("la sesión del que hizo el cambio: %d", rec.Code)
	}
}

// M3: bcrypt corre con turnos. Sin turno a tiempo las rutas públicas contestan 503
// (también con un alumno inexistente, que gasta su bcrypt igual); el panel espera.
func TestSecBcryptTurns(t *testing.T) {
	secWait(t, 50*time.Millisecond)
	e := admSetup(t)
	tar := e.courseID("TAR")
	hash, err := hashPassword("AB12")
	if err != nil {
		t.Fatal(err)
	}
	var sid int64
	e.one(&sid, `insert into students (course_id, code, name, password_hash) values ($1, 'S200', 'Turno', $2) returning id`, tar, hash)
	e.exec(`insert into reset_codes (student_id, code_hash, expires_at) values ($1, $2, now() + interval '10 minutes')`,
		sid, stuHashCode("123456"))

	sem := e.a.bcryptSem
	for range cap(sem) {
		sem <- struct{}{}
	}
	busy := []struct {
		path string
		body map[string]string
	}{
		{"/api/login", map[string]string{"studentId": "S200", "password": "AB12", "career": stuTAR}},
		{"/api/login", map[string]string{"studentId": "NOEXISTE", "password": "AB12", "career": stuTAR}},
		{"/api/admin-login", map[string]string{"username": "root", "password": "clave-segura-1"}},
		{"/api/register", map[string]string{"id": "S201", "name": "Nuevo", "sexo": "Hombre", "password": "AB12", "career": stuTAR}},
		{"/api/reset-password-with-code", map[string]string{"id": "S200", "code": "123456", "password": "CD34", "career": stuTAR}},
	}
	for _, c := range busy {
		rec := secPost(e.h, c.path, c.body, "192.0.2.20:1")
		if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), msgBusy) {
			t.Fatalf("%s %v sin turnos: %d %s", c.path, c.body, rec.Code, rec.Body)
		}
	}
	if e.count(`select count(*) from reset_codes where student_id = $1 and attempts = 0`, sid) != 1 ||
		e.count(`select count(*) from students where code = 'S201'`) != 0 {
		t.Fatal("un 503 no debe gastar el código ni crear al alumno")
	}

	// El panel no contesta 503: espera su turno mientras la petición siga viva.
	done := make(chan int, 1)
	go func() {
		done <- secPost(e.h, "/api/admin/students",
			map[string]any{"courseId": tar, "code": "S202", "name": "Paciente", "password": "AB12"}, "192.0.2.21:1", e.cookie).Code
	}()
	select {
	case code := <-done:
		t.Fatalf("el panel no esperó turno: %d", code)
	case <-time.After(4 * bcryptPublicWait):
	}
	<-sem
	if code := <-done; code != http.StatusCreated {
		t.Fatalf("el panel con un turno libre: %d", code)
	}
	for range cap(sem) - 1 {
		<-sem
	}
	if rec := secPost(e.h, "/api/login", busy[0].body, "192.0.2.20:1"); rec.Code != http.StatusOK {
		t.Fatalf("login con turnos libres: %d %s", rec.Code, rec.Body)
	}
}

// L1: pedir otro código con uno vigente conserva los intentos; vuelven a 0 solo al
// vencer o al usarse. M2: restablecer con código cierra la sesión que estaba abierta.
func TestSecResetAttemptsSurviveResend(t *testing.T) {
	e := stuSetup(t)
	mails := mailFake(t, e.a, http.StatusOK)
	id := e.student(stuTAR, "S300", "AB12")
	cookie, _ := e.login(stuTAR, "S300", "AB12")
	codeRe := regexp.MustCompile(`<h1[^>]*>(\d{6})</h1>`)
	send := func() *httptest.ResponseRecorder {
		return e.call("POST", "/api/send-reset-code", map[string]string{"id": "S300", "career": stuTAR})
	}
	sendOK := func() string {
		t.Helper()
		e.want(send(), http.StatusOK, "")
		m := codeRe.FindStringSubmatch(mailWait(t, mails).Body.HTML)
		if m == nil {
			t.Fatal("el correo no trae el código")
		}
		return m[1]
	}
	reset := func(code string) *httptest.ResponseRecorder {
		return e.call("POST", "/api/reset-password-with-code",
			map[string]string{"id": "S300", "code": code, "password": "CD34", "career": stuTAR})
	}
	wrong := func(code string) string {
		if code == "000000" {
			return "000001"
		}
		return "000000"
	}
	attempts := func() int { t.Helper(); return e.num("select attempts from reset_codes where student_id = $1", id) }

	code := sendOK()
	for range 3 {
		e.want(reset(wrong(code)), http.StatusUnauthorized, stuMsgBadCode)
	}
	code = sendOK()
	if n := attempts(); n != 3 {
		t.Fatalf("reenviar reinició los intentos: %d", n)
	}
	for range 2 {
		e.want(reset(wrong(code)), http.StatusUnauthorized, stuMsgBadCode)
	}
	// Ya van cinco: ni el bueno sirve ni se manda otro mientras no venza.
	e.want(reset(code), http.StatusUnauthorized, stuMsgBadCode)
	e.want(send(), http.StatusTooManyRequests, stuMsgTooMany)

	e.exec("update reset_codes set expires_at = now() - interval '1 second' where student_id = $1", id)
	code = sendOK()
	if n := attempts(); n != 0 {
		t.Fatalf("tras vencer debe empezar en 0: %d", n)
	}
	e.want(reset(code), http.StatusOK, "Contraseña actualizada correctamente.")

	e.want(e.call("GET", "/api/me", nil, cookie), http.StatusUnauthorized, sessionExpired)
	e.login(stuTAR, "S300", "CD34")
}

// L4: un ID nuevo solo lleva [A-Za-z0-9._-] (va en matrícula@dominio) y un nombre no
// lleva caracteres de control ni marcas de dirección. Lo viejo que no cambia se respeta.
func TestSecIDAndNameRules(t *testing.T) {
	e := admSetup(t)
	tar := e.courseID("TAR")
	reg := func(id, name string) *httptest.ResponseRecorder {
		return e.do("POST", "/api/register",
			map[string]string{"id": id, "name": name, "sexo": "Hombre", "password": "AB12", "career": stuTAR}, nil)
	}
	for _, code := range []string{"x\r\nBcc: otro@correo.mx", "ana lópez", "a@b.mx", "a,b", "ñu", "0251‮", strings.Repeat("1", 41)} {
		if rec := reg(code, "Prueba"); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), msgBadCode) {
			t.Fatalf("registro con ID %q: %d %s", code, rec.Code, rec.Body)
		}
	}
	for _, name := range []string{"Ana‮López", "Ana⁦x", "Ana\x00", "Ana\r\nBcc: x"} {
		if rec := reg("S410", name); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), msgBadName) {
			t.Fatalf("registro con nombre %q: %d %s", name, rec.Code, rec.Body)
		}
	}
	if rec := reg("A.b_c-1", "Ana López"); rec.Code != http.StatusCreated {
		t.Fatalf("registro válido: %d %s", rec.Code, rec.Body)
	}

	if msg := e.fails("POST", "/api/admin/students", map[string]any{"courseId": tar, "code": "x\r\nBcc", "name": "X"},
		http.StatusBadRequest); msg != msgBadCode {
		t.Fatalf("crear con ID malo: %q", msg)
	}
	if msg := e.fails("POST", "/api/admin/students", map[string]any{"courseId": tar, "code": "S400", "name": "X‮"},
		http.StatusBadRequest); msg != msgBadName {
		t.Fatalf("crear con nombre malo: %q", msg)
	}
	st := e.call("POST", "/api/admin/students", map[string]any{"courseId": tar, "code": "S400", "name": "Válido"}, http.StatusCreated)
	path := fmt.Sprintf("/api/admin/students/%d", int64(st["student"].(map[string]any)["id"].(float64)))
	if msg := e.fails("PATCH", path, map[string]any{"code": "S 400"}, http.StatusBadRequest); msg != msgBadCode {
		t.Fatalf("editar con ID malo: %q", msg)
	}
	if msg := e.fails("PATCH", path, map[string]any{"name": "Otro⁩"}, http.StatusBadRequest); msg != msgBadName {
		t.Fatalf("editar con nombre malo: %q", msg)
	}
	old := e.student(tar, "02 51", "Viejo‮", 0, 0)
	e.call("PATCH", fmt.Sprintf("/api/admin/students/%d", old),
		map[string]any{"code": "02 51", "name": "Viejo‮", "monedas": 3}, http.StatusOK)

	gen := e.call("POST", "/api/admin/students/generate", map[string]any{"courseId": tar,
		"rows": []map[string]string{{"code": "S401", "name": "Bien"}, {"code": "S 402", "name": "Mal"}}}, http.StatusOK)
	skipped := gen["skipped"].([]any)
	if len(gen["created"].([]any)) != 1 || len(skipped) != 1 || skipped[0].(map[string]any)["reason"] != msgBadCode {
		t.Fatalf("generar: %v", gen)
	}
}

// L4: la importación acepta los datos viejos tal cual, pero sin caracteres de control.
func TestSecImportStripsControl(t *testing.T) {
	if got := stripControl("a\tb\x00c\u0085d"); got != "a bc d" {
		t.Fatalf("stripControl: %q", got)
	}
	c := newImpTester(t)
	tabs := impLegacyTabs()
	tabs[0].rows[1][1] = "Ana\x00 López\u0085" // un NUL ni siquiera entra a Postgres
	tabs[0].rows[2][0] = "0254\a639"           // Beto, solo en TAR
	tabs[4].rows[1][1] = "0254\a639"           // una de sus dos compras
	newImpFake(t, tabs)
	job := c.run(impCreds(map[string]any{"include": impNoMedia}))
	if job.Status != "done" || job.Report == nil {
		t.Fatalf("importación: %+v", job)
	}
	if ana := c.student("TAR", "0251412"); ana.Name != "Ana López" {
		t.Fatalf("nombre: %q", ana.Name)
	}
	c.student("TAR", "0254639")
	if p := job.Report.Purchases; p.Created != 7 || p.Unmatched != 2 {
		t.Fatalf("las compras deben casar con el código limpio: %+v", p)
	}
}

// L5: los avatares van por matrícula, así que registrarse con una que ya está en otro
// curso pide la contraseña de esa inscripción.
func TestSecRegisterCodeFromOtherCourse(t *testing.T) {
	e := stuSetup(t)
	e.student(stuTAR, "S500", "AB12")
	reg := func(pw, career string) *httptest.ResponseRecorder {
		return e.call("POST", "/api/register",
			map[string]string{"id": "S500", "name": "Sara", "sexo": "Mujer", "password": pw, "career": career})
	}
	e.want(reg("ZZ99", stuAD), http.StatusConflict, stuMsgOtherCourse)
	if n := e.num("select count(*) from students where code = 'S500'"); n != 1 {
		t.Fatalf("inscripciones: %d", n)
	}
	e.want(reg("AB12", stuAD), http.StatusCreated, "Alumno registrado exitosamente.")
	e.login(stuAD, "S500", "AB12")
	e.want(reg("AB12", stuAD), http.StatusConflict, "El ID de alumno ya existe.")

	// Esos intentos tienen tope propio por matrícula: no es otra vía para adivinar.
	for range 10 {
		e.a.Limiter.Allow("stu-register-code:S500", 10, stuLoginWindow)
	}
	e.want(reg("AB12", stuIHM), http.StatusTooManyRequests, stuMsgTooMany)
}

// Las compras salen en el orden de la primera compra vigente de cada insignia, como en
// el portal viejo, no en orden alfabético. Sin compras es {} (nunca null).
func TestSecPurchasesOrder(t *testing.T) {
	e := stuSetup(t)
	id := e.student(stuTAR, "S600", "AB12")
	e.exec("update students set monedas = 50 where id = $1", id)
	e.exec(`insert into purchases (student_id, badge_name, quantity) values
		($1, 'Alfa', 1), ($1, 'Zeta', 1), ($1, 'Alfa', 2), ($1, 'Media', 1), ($1, 'Zeta', 3)`, id)
	e.exec(`update purchases set reverted_at = now()
		where id = (select min(id) from purchases where badge_name = 'Alfa') or badge_name = 'Media'`)

	const want = `"purchases":{"Zeta":4,"Alfa":2}`
	rec := e.loginReq(stuTAR, "S600", "AB12")
	cookie := secCookie(rec, "diseno_alumno")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("login: %d %s", rec.Code, rec.Body)
	}
	if rec := e.call("GET", "/api/me", nil, cookie); !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("me: %s", rec.Body)
	}
	b := e.badge("Beta", 5, 3)
	rec = e.call("POST", "/api/purchase", map[string]any{"items": []map[string]int64{{"badgeId": b, "quantity": 1, "unitCost": 5}}}, cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"purchases":{"Zeta":4,"Alfa":2,"Beta":1}`) {
		t.Fatalf("compra: %d %s", rec.Code, rec.Body)
	}

	e.student(stuTAR, "S601", "AB12")
	if rec := e.loginReq(stuTAR, "S601", "AB12"); !strings.Contains(rec.Body.String(), `"purchases":{}`) {
		t.Fatalf("sin compras: %s", rec.Body)
	}
}
