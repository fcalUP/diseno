package app

import (
	"net/http"
	"slices"
	"testing"
)

func TestStuAutomaticSingleCourse(t *testing.T) {
	e := stuSetup(t)
	e.student(stuAD, "AUTO01", "AB23")
	rec := e.call("POST", "/api/login", map[string]string{"studentId": "AUTO01", "password": "AB23"})
	body := e.want(rec, http.StatusOK, "")
	if body["requiresCourseSelection"] != false || body["student"].(map[string]any)["career"] != stuAD ||
		!slices.Equal(stuNames(body["courses"]), []string{stuAD}) {
		t.Fatalf("una materia debe entrar directo: %v", body)
	}
	cookie := secCookie(rec, "diseno_alumno")
	if cookie == nil {
		t.Fatal("falta la sesión")
	}
	me := e.want(e.call("GET", "/api/me", nil, cookie), http.StatusOK, "")
	if me["requiresCourseSelection"] != false || me["student"].(map[string]any)["career"] != stuAD {
		t.Fatalf("recargar cambió la materia: %v", me)
	}
	for _, code := range []string{"AUTO01", "NOEXISTE"} {
		bad := e.want(e.call("POST", "/api/login", map[string]string{"studentId": code, "password": "ZZ99"}),
			http.StatusUnauthorized, "Credenciales inválidas.")
		if _, ok := bad["courses"]; ok {
			t.Fatal("credenciales malas revelaron materias")
		}
	}
}

func TestStuMultipleCoursesAndPersistentSelection(t *testing.T) {
	e := stuSetup(t)
	tar := e.student(stuTAR, "MULTI01", "AB23")
	ad := e.student(stuAD, "MULTI01", "CD45")
	ihm := e.student(stuIHM, "MULTI01", "EF67")
	e.exec("update students set monedas = 11, exp = 12 where id = $1", tar)
	e.exec("update students set monedas = 22, exp = 30 where id = $1", ad)
	e.exec("update students set monedas = 33, exp = 80 where id = $1", ihm)
	// La clave solo coincide con AD; la identidad por matrícula da las tres materias.
	rec := e.call("POST", "/api/login", map[string]string{"studentId": "MULTI01", "password": "CD45"})
	body := e.want(rec, http.StatusOK, "")
	cookie := secCookie(rec, "diseno_alumno")
	if cookie == nil {
		t.Fatal("falta sesión multicurso")
	}
	if body["requiresCourseSelection"] != true || body["levelUpOccurred"] != false ||
		!slices.Equal(stuNames(body["courses"]), []string{stuTAR, stuAD, stuIHM}) {
		t.Fatalf("selector: %v", body)
	}
	for _, item := range body["courses"].([]any) {
		c := item.(map[string]any)
		if c["id"].(float64) <= 0 || c["logoUrl"] == "" {
			t.Fatalf("tarjeta sin id/logo: %v", c)
		}
	}
	var original Session
	if !verifyToken(e.a.sessionKey, cookie.Value, &original) || original.AuthID != ad || original.Code != "MULTI01" {
		t.Fatalf("credencial autenticadora: %+v", original)
	}
	for range 2 {
		me := e.want(e.call("GET", "/api/me", nil, cookie), http.StatusOK, "")
		if me["requiresCourseSelection"] != true || me["levelUpOccurred"] != false {
			t.Fatalf("recargar omitió el selector: %v", me)
		}
	}
	if n := e.num("select count(*) from students where code = 'MULTI01' and (last_login_at is not null or last_login_level <> 0)"); n != 0 {
		t.Fatal("ver el selector no debe consumir la subida de nivel ni registrar visita")
	}
	choose := func(course string, coins float64) {
		t.Helper()
		rec := e.call("POST", "/api/switch-course", map[string]int64{"courseId": e.course(course)}, cookie)
		res := e.want(rec, http.StatusOK, "")
		st := res["student"].(map[string]any)
		if res["requiresCourseSelection"] != false || st["career"] != course || st["monedas"] != coins || res["levelUpOccurred"] != true {
			t.Fatalf("selección %s: %v", course, res)
		}
		cookie = secCookie(rec, "diseno_alumno")
		var switched Session
		if cookie == nil || !verifyToken(e.a.sessionKey, cookie.Value, &switched) || switched.AuthID != original.AuthID ||
			switched.PwV != original.PwV || switched.Exp != original.Exp || switched.SelectPending {
			t.Fatalf("el cambio perdió o renovó la credencial original: %+v", switched)
		}
		for range 2 {
			me := e.want(e.call("GET", "/api/me", nil, cookie), http.StatusOK, "")
			if me["requiresCourseSelection"] != false || me["student"].(map[string]any)["career"] != course {
				t.Fatalf("recargar cambió selección: %v", me)
			}
		}
	}
	choose(stuAD, 22)
	choose(stuIHM, 33)
	choose(stuTAR, 11)
	// El formato anterior con career sigue entrando al curso solicitado sin selector.
	legacy := e.want(e.loginReq(stuIHM, "MULTI01", "AB23"), http.StatusOK, "")
	if legacy["requiresCourseSelection"] != false || legacy["student"].(map[string]any)["career"] != stuIHM {
		t.Fatalf("login compatible con career: %v", legacy)
	}
}

func TestStuCoursesExcludeInactiveAndOtherStudents(t *testing.T) {
	e := stuSetup(t)
	e.student(stuTAR, "ACCESS01", "AB23")
	inactive := e.student(stuAD, "ACCESS01", "CD45")
	e.student("POO", "ACCESS01", "EF67") // curso inactivo
	e.student(stuIHM, "SOMEONE_ELSE", "AB23")
	e.exec("update students set active = false where id = $1", inactive)
	rec := e.call("POST", "/api/login", map[string]string{"studentId": "ACCESS01", "password": "AB23"})
	body := e.want(rec, http.StatusOK, "")
	cookie := secCookie(rec, "diseno_alumno")
	if !slices.Equal(stuNames(body["courses"]), []string{stuTAR}) || body["requiresCourseSelection"] != false {
		t.Fatalf("materias vigentes: %v", body)
	}
	for _, course := range []string{stuAD, stuIHM, "POO"} {
		e.want(e.call("POST", "/api/switch-course", map[string]int64{"courseId": e.course(course)}, cookie),
			http.StatusForbidden, "No tienes acceso a esa materia.")
	}
	e.want(e.call("POST", "/api/switch-course", map[string]int64{"courseId": 0}, cookie), http.StatusBadRequest, "")
	// Una clave de una inscripción desactivada no autentica las otras activas.
	e.want(e.call("POST", "/api/login", map[string]string{"studentId": "ACCESS01", "password": "CD45"}),
		http.StatusForbidden, stuMsgInactive)
	me := e.want(e.call("GET", "/api/me", nil, cookie), http.StatusOK, "")
	if me["student"].(map[string]any)["career"] != stuTAR {
		t.Fatal("cambio denegado alteró la selección")
	}
}

func TestStuCourseSessionRevokesWithAuthenticatingEnrollment(t *testing.T) {
	e := stuSetup(t)
	authID := e.student(stuTAR, "REVOKE01", "AB23")
	targetID := e.student(stuAD, "REVOKE01", "CD45")
	loginAndSwitch := func() *http.Cookie {
		t.Helper()
		rec := e.call("POST", "/api/login", map[string]string{"studentId": "REVOKE01", "password": "AB23"})
		e.want(rec, http.StatusOK, "")
		rec = e.call("POST", "/api/switch-course", map[string]int64{"courseId": e.course(stuAD)}, secCookie(rec, "diseno_alumno"))
		e.want(rec, http.StatusOK, "")
		return secCookie(rec, "diseno_alumno")
	}
	cookie := loginAndSwitch()
	// Las claves heredadas no se sincronizan: cambiar la de otra inscripción no
	// revoca una sesión que se autenticó con TAR.
	e.exec("update students set password_hash = 'otro-hash' where id = $1", targetID)
	e.want(e.call("GET", "/api/me", nil, cookie), http.StatusOK, "")
	for _, sql := range []string{
		"update students set active = false where id = $1",
		"update students set code = 'CHANGED' where id = $1",
		"update students set password_hash = 'revocado' where id = $1",
	} {
		e.exec(sql, authID)
		for _, methodPath := range [][2]string{{"GET", "/api/me"}, {"POST", "/api/switch-course"}, {"POST", "/api/purchase"}} {
			rec := e.call(methodPath[0], methodPath[1], map[string]int64{"courseId": e.course(stuTAR)}, cookie)
			e.want(rec, http.StatusUnauthorized, sessionExpired)
			if !secCleared(rec, "diseno_alumno") {
				t.Fatal("la sesión revocada no se borró")
			}
		}
		if sql == "update students set password_hash = 'revocado' where id = $1" {
			break
		}
		e.exec("update students set active = true, code = 'REVOKE01' where id = $1", authID)
		cookie = loginAndSwitch()
	}
}

func TestStuLoginLimitSharedAcrossCourseAndAutomatic(t *testing.T) {
	e := stuSetup(t)
	e.student(stuTAR, "LIMIT01", "AB23")
	e.student(stuAD, "LIMIT01", "CD45")
	for i := range stuLoginMax {
		career := ""
		if i%2 == 0 {
			career = stuAD
		}
		e.want(e.call("POST", "/api/login", map[string]string{"studentId": "LIMIT01", "password": "ZZ99", "career": career}),
			http.StatusUnauthorized, "Credenciales inválidas.")
	}
	for _, career := range []string{"", stuTAR, stuAD} {
		e.want(e.call("POST", "/api/login", map[string]string{"studentId": "LIMIT01", "password": "AB23", "career": career}),
			http.StatusTooManyRequests, stuMsgTooMany)
	}
}
