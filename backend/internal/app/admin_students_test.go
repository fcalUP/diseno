package app

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

var admPwRe = regexp.MustCompile(`^[A-Z0-9]{4}$`)

type admStudentResp struct {
	Student  adminStudentJSON
	Password string
	AuditID  int64 `json:"auditId"`
}

func (e *admT) passwordOK(studentID int64, pw string) bool {
	e.t.Helper()
	var hash string
	e.one(&hash, `select password_hash from students where id = $1`, studentID)
	return checkPassword(hash, pw)
}

func TestAdmStudentCreate(t *testing.T) {
	e := admSetup(t)
	tar := e.courseID("TAR")

	got := admReq[admStudentResp](e, "POST", "/api/admin/students", map[string]any{
		"courseId": tar, "code": " 0100 ", "name": " Dana ", "sexo": "mujer", "monedas": 5,
	}, http.StatusCreated)
	st := got.Student
	if st.Code != "0100" || st.Name != "Dana" || st.Sexo != "Mujer" || st.Email != "0100@up.edu.mx" ||
		st.Monedas != 5 || !st.Active || st.CourseID != tar {
		t.Fatalf("alumno creado: %+v", st)
	}
	if !admPwRe.MatchString(got.Password) || !e.passwordOK(st.ID, got.Password) {
		t.Fatalf("contraseña generada %q no sirve", got.Password)
	}
	if a := e.audit(got.AuditID); a.Action != "create" || a.Entity != "student" || a.EntityID != fmt.Sprint(st.ID) ||
		a.Before != nil || a.After["code"] != "0100" || !a.Undoable {
		t.Fatalf("auditoría: %+v", a)
	}

	// Con contraseña propia no se devuelve nada en claro.
	raw := e.call("POST", "/api/admin/students", map[string]any{
		"courseId": tar, "code": "0101", "name": "Eli", "password": "AB12", "email": "eli@correo.mx",
	}, http.StatusCreated)
	if _, ok := raw["password"]; ok {
		t.Fatal("devolvió la contraseña que puso el admin")
	}
	eli := int64(raw["student"].(map[string]any)["id"].(float64))
	if !e.passwordOK(eli, "AB12") {
		t.Fatal("no guardó la contraseña indicada")
	}

	if msg := e.fails("POST", "/api/admin/students",
		map[string]any{"courseId": tar, "code": "0100", "name": "Otra"}, http.StatusConflict); msg != "El ID de alumno ya existe." {
		t.Fatalf("duplicado: %q", msg)
	}
	// El mismo ID en otro curso sí se puede (una fila por curso, como las hojas).
	e.call("POST", "/api/admin/students", map[string]any{"courseId": e.courseID("IHM"), "code": "0100", "name": "Dana"}, http.StatusCreated)

	for _, body := range []map[string]any{
		{"courseId": tar, "code": "0102"},
		{"courseId": tar, "name": "Sin ID"},
		{"code": "0102", "name": "Sin curso"},
		{"courseId": 99999, "code": "0102", "name": "Curso inexistente"},
		{"courseId": tar, "code": "0102", "name": "X", "sexo": "Otro"},
		{"courseId": tar, "code": "0102", "name": "X", "monedas": -1},
		{"courseId": tar, "code": "0102", "name": "X", "password": "abc"},
		{"courseId": tar, "code": "0102", "name": "X", "email": "no-es-correo"},
		{"courseId": tar, "code": "DefaultH", "name": "X"},
	} {
		e.fails("POST", "/api/admin/students", body, http.StatusBadRequest)
	}
	if n := e.count(`select count(*) from students where code = '0102'`); n != 0 {
		t.Fatalf("se guardaron alumnos inválidos: %d", n)
	}
}

func TestAdmStudentsGenerate(t *testing.T) {
	e := admSetup(t)
	tar := e.courseID("TAR")
	e.student(tar, "0200", "Ya estaba", 0, 0)
	batch := newBatchID()

	type genResp struct {
		Created []struct {
			Student  adminStudentJSON
			Password string
		}
		Skipped []admSkip
		BatchID string `json:"batchId"`
	}
	got := admReq[genResp](e, "POST", "/api/admin/students/generate", map[string]any{
		"courseId": tar,
		"batchId":  batch,
		"rows": []map[string]string{
			{"code": "0201", "name": "Eva"},
			{"code": "0202", "name": "Fer", "sexo": "Mujer", "email": "fer@correo.mx", "password": "ZZ99"},
			{"code": "0201", "name": "Eva otra vez"},
			{"code": "0200", "name": "Repetida"},
			{"code": " ", "name": "Sin ID"},
			{"code": "0203", "name": ""},
			{"code": "0204", "name": "Mala", "password": "abc"},
		},
	}, http.StatusOK)
	if got.BatchID != batch || len(got.Created) != 2 || len(got.Skipped) != 5 {
		t.Fatalf("resultado: %+v", got)
	}
	eva, fer := got.Created[0], got.Created[1]
	if eva.Student.Code != "0201" || eva.Student.Sexo != "Hombre" || eva.Student.Email != "0201@up.edu.mx" ||
		!admPwRe.MatchString(eva.Password) || !e.passwordOK(eva.Student.ID, eva.Password) {
		t.Fatalf("fila con contraseña generada: %+v", eva)
	}
	if fer.Student.Code != "0202" || fer.Password != "ZZ99" || fer.Student.Email != "fer@correo.mx" ||
		!e.passwordOK(fer.Student.ID, "ZZ99") {
		t.Fatalf("fila con contraseña propia: %+v", fer)
	}
	want := []admSkip{
		{"0201", "ID repetido en la lista."},
		{"0200", "El ID ya existe en este curso."},
		{"", "Falta el ID del alumno."},
		{"0203", "Falta el nombre del alumno."},
	}
	for i, s := range want {
		if got.Skipped[i] != s {
			t.Errorf("saltada %d: %+v, esperaba %+v", i, got.Skipped[i], s)
		}
	}
	if s := got.Skipped[4]; s.Code != "0204" || !strings.Contains(s.Reason, "regla") {
		t.Errorf("contraseña inválida: %+v", s)
	}
	if n := e.count(`select count(*) from audit_log where batch_id = $1::uuid and action = 'create' and entity = 'student'`, batch); n != 2 {
		t.Fatalf("entradas del lote: %d", n)
	}

	// Sin batchId se genera uno nuevo.
	again := admReq[genResp](e, "POST", "/api/admin/students/generate", map[string]any{
		"courseId": tar, "rows": []map[string]string{{"code": "0205", "name": "Gus"}},
	}, http.StatusOK)
	if len(again.Created) != 1 || again.BatchID == "" || again.BatchID == batch {
		t.Fatalf("lote nuevo: %+v", again)
	}

	tooMany := make([]map[string]string, 51)
	for i := range tooMany {
		tooMany[i] = map[string]string{"code": fmt.Sprintf("9%03d", i), "name": "X"}
	}
	for _, body := range []map[string]any{
		{"courseId": tar, "rows": tooMany},
		{"courseId": tar, "rows": []map[string]string{}},
		{"courseId": tar, "batchId": "no-es-uuid", "rows": []map[string]string{{"code": "0206", "name": "X"}}},
		{"courseId": 99999, "rows": []map[string]string{{"code": "0206", "name": "X"}}},
	} {
		e.fails("POST", "/api/admin/students/generate", body, http.StatusBadRequest)
	}
}

func TestAdmStudentUpdate(t *testing.T) {
	e := admSetup(t)
	tar, ad := e.courseID("TAR"), e.courseID("Analisis de Datos")
	id := e.student(tar, "0300", "Gil", 0, 0)
	e.student(tar, "0301", "Hugo", 0, 0)
	e.exec(`update students set last_login_level = 2 where id = $1`, id)
	path := fmt.Sprintf("/api/admin/students/%d", id)

	got := admReq[admStudentResp](e, "PATCH", path, map[string]any{
		"name": "Gil Pérez", "monedas": 15, "exp": 60, "comments": " Muy bien ", "active": false,
		"password": "ZZ11", // aquí se ignora: la contraseña tiene su propio endpoint
	}, http.StatusOK)
	st := got.Student
	if st.Name != "Gil Pérez" || st.Monedas != 15 || st.Level != 3 || st.Comments != "Muy bien" || st.Active ||
		st.LastLoginLevel != 2 || st.Code != "0300" {
		t.Fatalf("alumno editado: %+v", st)
	}
	if e.passwordOK(id, "ZZ11") {
		t.Fatal("PATCH no debe cambiar la contraseña")
	}
	a := e.audit(got.AuditID)
	if a.Action != "update" || a.Entity != "student" || !a.Undoable || a.Before["name"] != "Gil" ||
		a.After["name"] != "Gil Pérez" || a.Before["monedas"] != 0.0 || a.After["monedas"] != 15.0 {
		t.Fatalf("auditoría: %+v", a)
	}
	if !strings.Contains(a.Summary, "monedas 0 → 15") {
		t.Fatalf("resumen: %q", a.Summary)
	}

	if msg := e.fails("PATCH", path, map[string]any{"code": "0301"}, http.StatusConflict); msg != "El ID de alumno ya existe." {
		t.Fatalf("ID repetido: %q", msg)
	}
	moved := admReq[admStudentResp](e, "PATCH", path, map[string]any{"courseId": ad, "code": "0301"}, http.StatusOK)
	if moved.Student.CourseID != ad || moved.Student.Code != "0301" {
		t.Fatalf("cambio de curso: %+v", moved.Student)
	}

	// Un sexo o correo heredado raro se respeta si no cambia; uno nuevo inválido no.
	e.exec(`update students set sexo = 'N/A', email = 'sin correo' where id = $1`, id)
	e.call("PATCH", path, map[string]any{"sexo": "N/A", "email": "sin correo", "name": "Gil"}, http.StatusOK)

	for _, body := range []map[string]any{
		{"name": "  "}, {"code": ""}, {"monedas": -1}, {"exp": 3000000000}, {"courseId": 99999},
		{"sexo": "x"}, {"email": "mal@"}, {},
	} {
		e.fails("PATCH", path, body, http.StatusBadRequest)
	}
	e.fails("PATCH", "/api/admin/students/99999", map[string]any{"name": "Nadie"}, http.StatusNotFound)
	e.fails("PATCH", "/api/admin/students/abc", map[string]any{"name": "Nadie"}, http.StatusNotFound)
}

func TestAdmStudentDelete(t *testing.T) {
	e := admSetup(t)
	id := e.student(e.courseID("TAR"), "0400", "Iris", 3, 0)
	b := e.badge("Calculadora", 5, 2)
	p1 := e.purchase(id, b, 1, 2, false)
	e.purchase(id, b, 2, 2, true)
	path := fmt.Sprintf("/api/admin/students/%d", id)

	got := e.call("DELETE", path, nil, http.StatusOK)
	if got["success"] != true {
		t.Fatalf("respuesta: %v", got)
	}
	if n := e.count(`select count(*) from students where id = $1`, id); n != 0 {
		t.Fatal("el alumno sigue ahí")
	}
	if n := e.count(`select count(*) from purchases where student_id = $1`, id); n != 0 {
		t.Fatal("quedaron compras huérfanas")
	}
	a := e.audit(int64(got["auditId"].(float64)))
	row, _ := a.Before["row"].(map[string]any)
	purchases, _ := a.Before["purchases"].([]any)
	if a.Action != "delete" || a.Entity != "student" || !a.Undoable || a.After != nil || row["code"] != "0400" ||
		row["password_hash"] != "x" || row["monedas"] != 3.0 || len(purchases) != 2 {
		t.Fatalf("instantánea: %+v", a)
	}
	first := purchases[0].(map[string]any)
	if first["id"] != float64(p1) || first["badge_name"] != "Calculadora" || first["reverted_at"] != nil {
		t.Fatalf("compra en la instantánea: %v", first)
	}
	e.fails("DELETE", path, nil, http.StatusNotFound)
}

func TestAdmStudentPassword(t *testing.T) {
	e := admSetup(t)
	id := e.student(e.courseID("TAR"), "0450", "Juana", 0, 0)
	path := fmt.Sprintf("/api/admin/students/%d/password", id)
	type pwResp struct {
		Password string
		AuditID  int64 `json:"auditId"`
	}

	got := admReq[pwResp](e, "POST", path, map[string]any{}, http.StatusOK)
	if !admPwRe.MatchString(got.Password) || !e.passwordOK(id, got.Password) {
		t.Fatalf("contraseña generada: %q", got.Password)
	}
	a := e.audit(got.AuditID)
	if a.Action != "update" || a.Entity != "student" || a.Before["password_hash"] != "x" ||
		a.After["password_hash"] == "x" || !strings.Contains(a.Summary, "Contraseña") {
		t.Fatalf("auditoría: %+v", a)
	}

	got = admReq[pwResp](e, "POST", path, map[string]any{"password": " QW12 "}, http.StatusOK)
	if got.Password != "QW12" || !e.passwordOK(id, "QW12") {
		t.Fatalf("contraseña indicada: %q", got.Password)
	}
	e.fails("POST", path, map[string]any{"password": "qw12"}, http.StatusBadRequest)
	e.fails("POST", "/api/admin/students/99999/password", map[string]any{}, http.StatusNotFound)

	// Con una regla que no admite 2 letras + 2 dígitos hay que escribirla a mano.
	e.call("PATCH", "/api/admin/settings", map[string]any{"passwordPattern": "^[a-z]{6}$"}, http.StatusOK)
	e.fails("POST", path, map[string]any{}, http.StatusBadRequest)
	if got := admReq[pwResp](e, "POST", path, map[string]any{"password": "abcdef"}, http.StatusOK); got.Password != "abcdef" {
		t.Fatalf("con la regla nueva: %q", got.Password)
	}
}

func TestAdmStudentsBulk(t *testing.T) {
	e := admSetup(t)
	tar, ad := e.courseID("TAR"), e.courseID("Analisis de Datos")
	s1 := e.student(tar, "0500", "Juan", 5, 0)
	s2 := e.student(tar, "0501", "Karla", 1, 0)
	type bulkResp struct {
		Updated int
		BatchID string `json:"batchId"`
	}
	bulk := func(body map[string]any) bulkResp {
		t.Helper()
		return admReq[bulkResp](e, "POST", "/api/admin/students/bulk", body, http.StatusOK)
	}
	counters := func(id int64) (monedas, exp, tareas int) {
		t.Helper()
		if err := e.a.DB.QueryRow(e.ctx, `select monedas, exp, tareas from students where id = $1`, id).
			Scan(&monedas, &exp, &tareas); err != nil {
			t.Fatal(err)
		}
		return
	}

	got := bulk(map[string]any{"ids": []int64{s1, s2, 99999}, "action": "delta", "delta": map[string]int{"monedas": -3, "exp": 10}})
	if got.Updated != 2 || got.BatchID == "" {
		t.Fatalf("delta: %+v", got)
	}
	if m, x, _ := counters(s1); m != 2 || x != 10 {
		t.Fatalf("s1 tras delta: monedas=%d exp=%d", m, x)
	}
	if m, x, _ := counters(s2); m != 0 || x != 10 { // nunca por debajo de 0
		t.Fatalf("s2 tras delta: monedas=%d exp=%d", m, x)
	}
	if n := e.count(`select count(*) from audit_log where batch_id = $1::uuid and action = 'update' and undoable`, got.BatchID); n != 2 {
		t.Fatalf("una entrada por alumno con el mismo lote: %d", n)
	}

	// s2 ya está en 0: no cambia ni deja entrada.
	if got := bulk(map[string]any{"ids": []int64{s1, s2}, "action": "delta", "delta": map[string]int{"monedas": -100}}); got.Updated != 1 {
		t.Fatalf("delta hasta 0: %+v", got)
	}
	if m, _, _ := counters(s1); m != 0 {
		t.Fatalf("s1 debió quedar en 0: %d", m)
	}
	if got := bulk(map[string]any{"ids": []int64{s1, s2}, "action": "set", "set": map[string]int{"tareas": 4}}); got.Updated != 2 {
		t.Fatalf("set: %+v", got)
	}
	if _, _, ta := counters(s2); ta != 4 {
		t.Fatalf("tareas tras set: %d", ta)
	}
	if got := bulk(map[string]any{"ids": []int64{s1, s2}, "action": "deactivate"}); got.Updated != 2 {
		t.Fatalf("deactivate: %+v", got)
	}
	if got := bulk(map[string]any{"ids": []int64{s1, s2}, "action": "deactivate"}); got.Updated != 0 {
		t.Fatalf("deactivate otra vez: %+v", got)
	}
	batch := newBatchID()
	if got := bulk(map[string]any{"ids": []int64{s1, s2}, "action": "activate", "batchId": batch}); got.Updated != 2 || got.BatchID != batch {
		t.Fatalf("activate con lote del cliente: %+v", got)
	}

	// Mover: s1 choca con un 0500 que ya existe en el destino y se salta.
	e.student(ad, "0500", "Juan en Datos", 0, 0)
	if got := bulk(map[string]any{"ids": []int64{s1, s2}, "action": "move", "courseId": ad}); got.Updated != 1 {
		t.Fatalf("move: %+v", got)
	}
	var c1, c2 int64
	e.one(&c1, `select course_id from students where id = $1`, s1)
	e.one(&c2, `select course_id from students where id = $1`, s2)
	if c1 != tar || c2 != ad {
		t.Fatalf("cursos tras mover: %d %d", c1, c2)
	}

	got = bulk(map[string]any{"ids": []int64{s1}, "action": "delete"})
	if got.Updated != 1 || e.count(`select count(*) from students where id = $1`, s1) != 0 {
		t.Fatalf("delete: %+v", got)
	}
	var auditID int64
	e.one(&auditID, `select id from audit_log where batch_id = $1::uuid`, got.BatchID)
	if a := e.audit(auditID); a.Action != "delete" || a.Before["row"].(map[string]any)["code"] != "0500" {
		t.Fatalf("auditoría del borrado masivo: %+v", a)
	}

	tooMany := make([]int64, 501)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	for _, body := range []map[string]any{
		{"ids": []int64{}, "action": "delete"},
		{"ids": tooMany, "action": "activate"},
		{"ids": []int64{0}, "action": "activate"},
		{"ids": []int64{s2}, "action": "nope"},
		{"ids": []int64{s2}, "action": "delta"},
		{"ids": []int64{s2}, "action": "delta", "delta": map[string]int{"nivel": 1}},
		{"ids": []int64{s2}, "action": "set", "set": map[string]int{"monedas": -1}},
		{"ids": []int64{s2}, "action": "move"},
		{"ids": []int64{s2}, "action": "move", "courseId": 99999},
		{"ids": []int64{s2}, "action": "activate", "batchId": "x"},
	} {
		e.fails("POST", "/api/admin/students/bulk", body, http.StatusBadRequest)
	}
}
