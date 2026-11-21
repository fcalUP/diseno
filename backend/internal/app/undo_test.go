package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// undoEnv es un App sobre su propio esquema (isolatedApp) con un admin con sesión,
// para pegarle a las rutas reales. Cada prueba arranca con la base recién sembrada.
type undoEnv struct {
	t       *testing.T
	a       *App
	h       http.Handler
	ctx     context.Context
	adminID int64
	cookie  *http.Cookie
}

func undoSetup(t *testing.T) *undoEnv {
	t.Helper()
	a := isolatedApp(t)
	e := &undoEnv{t: t, a: a, h: a.Routes(), ctx: context.Background()}
	e.adminID = e.num(`insert into admins (username, password_hash) values ('profe', 'hash-profe') returning id`)
	rec := httptest.NewRecorder()
	a.setSession(rec, kindAdmin, e.adminID)
	e.cookie = rec.Result().Cookies()[0]
	return e
}

// call manda la petición con la cookie del admin y devuelve el status y el JSON.
func (e *undoEnv) call(method, path string, body any) (int, map[string]any) {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	if e.cookie != nil {
		req.AddCookie(e.cookie)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		e.t.Fatalf("%s %s: la respuesta no es JSON (%d): %s", method, path, rec.Code, rec.Body)
	}
	return rec.Code, out
}

// must exige el status y devuelve el JSON.
func (e *undoEnv) must(status int, method, path string, body any) map[string]any {
	e.t.Helper()
	code, out := e.call(method, path, body)
	if code != status {
		e.t.Fatalf("%s %s: esperaba %d, llegó %d %v", method, path, status, code, out)
	}
	return out
}

func (e *undoEnv) undo(id int64, body any) map[string]any {
	e.t.Helper()
	return e.must(http.StatusOK, "POST", fmt.Sprintf("/api/admin/audit/%d/undo", id), body)
}

func (e *undoEnv) redo(id int64, body any) map[string]any {
	e.t.Helper()
	return e.must(http.StatusOK, "POST", fmt.Sprintf("/api/admin/audit/%d/redo", id), body)
}

func (e *undoEnv) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.a.DB.Exec(e.ctx, sql, args...); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
}

// num lee un entero (también sirve para insert ... returning id).
func (e *undoEnv) num(sql string, args ...any) int64 {
	e.t.Helper()
	var n int64
	if err := e.a.DB.QueryRow(e.ctx, sql, args...).Scan(&n); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
	return n
}

func (e *undoEnv) str(sql string, args ...any) string {
	e.t.Helper()
	var s string
	if err := e.a.DB.QueryRow(e.ctx, sql, args...).Scan(&s); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
	return s
}

// row es snapshotRow sobre la base de la prueba.
func (e *undoEnv) row(table string, id any) map[string]any {
	e.t.Helper()
	r, err := snapshotRow(e.ctx, e.a.DB, table, id)
	if err != nil {
		e.t.Fatalf("snapshotRow %s %v: %v", table, id, err)
	}
	return r
}

// audit graba una entrada igual que lo harían los manejadores de otros archivos.
func (e *undoEnv) audit(entry AuditEntry) int64 {
	e.t.Helper()
	if entry.ActorType == "" {
		entry.ActorType, entry.ActorID, entry.ActorLabel = "admin", &e.adminID, "profe"
	}
	id, err := e.a.Audit(e.ctx, e.a.DB, entry)
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

// idOf lee un número de una respuesta JSON.
func (e *undoEnv) idOf(m map[string]any, key string) int64 {
	e.t.Helper()
	n, ok := undoInt(m[key])
	if !ok {
		e.t.Fatalf("falta %q en %v", key, m)
	}
	return n
}

func (e *undoEnv) undone(id int64) bool {
	e.t.Helper()
	return e.num(`select count(*) from audit_log where id = $1 and undone_at is not null`, id) == 1
}

// newStudent da de alta un alumno en el curso 1 como lo haría admin_students.go.
func (e *undoEnv) newStudent(code string, coins int) int64 {
	e.t.Helper()
	return e.num(`insert into students (course_id, code, name, password_hash, monedas)
		values (1, $1, $2, 'hash-alumno', $3) returning id`, code, "Alumno "+code, coins)
}

func (e *undoEnv) newBadge(name string, stock, cost int) int64 {
	e.t.Helper()
	return e.num(`insert into badges (name, slug, stock, cost) values ($1, $2, $3, $4) returning id`,
		name, slugify(name), stock, cost)
}

func (e *undoEnv) purchasesJSON(studentID int64) []any {
	e.t.Helper()
	var list []any
	if err := e.a.DB.QueryRow(e.ctx, `select coalesce(jsonb_agg(to_jsonb(p) order by p.id), '[]')
		from purchases p where p.student_id = $1`, studentID).Scan(&list); err != nil {
		e.t.Fatal(err)
	}
	return list
}

// La lista blanca del motor debe coincidir con el esquema real.
func TestUndoTablesMatchSchema(t *testing.T) {
	e := undoSetup(t)
	for entity, tb := range undoTables {
		rows, err := e.a.DB.Query(e.ctx, `select column_name::text from information_schema.columns
			where table_schema = current_schema() and table_name = $1 order by 1`, tb.name)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatal(err)
		}
		if want := slices.Sorted(slices.Values(tb.cols)); !slices.Equal(cols, want) {
			t.Errorf("%s: columnas %v, lista blanca %v", entity, cols, want)
		}
	}
}

func TestUndoUpdateUndoRedo(t *testing.T) {
	e := undoSetup(t)
	out := e.must(200, "PATCH", "/api/admin/courses/1", map[string]any{"name": "TAR 2", "active": false})
	id := e.idOf(out, "auditId")

	res := e.undo(id, nil)
	if got := e.str(`select name from courses where id = 1`); got != "Técnicas Avanzadas de Representación" {
		t.Fatalf("nombre tras deshacer: %q", got)
	}
	if e.num(`select count(*) from courses where id = 1 and active`) != 1 {
		t.Fatal("active no regresó a true")
	}
	if s, _ := res["summary"].(string); !strings.HasPrefix(s, "Se deshizo: Curso actualizado") {
		t.Fatalf("resumen: %q", s)
	}
	undoID := e.idOf(res, "auditId")
	if e.num(`select count(*) from audit_log where id = $1 and action = 'undo' and reverts_id = $2
		and not undoable and entity = 'course' and entity_id = '1'`, undoID, id) != 1 {
		t.Fatal("la entrada del deshacer no quedó bien")
	}
	if e.num(`select count(*) from audit_log where id = $1 and undone_at is not null and undone_by = $2`, id, e.adminID) != 1 {
		t.Fatal("la entrada original no quedó marcada como deshecha")
	}
	e.must(400, "POST", fmt.Sprintf("/api/admin/audit/%d/undo", id), nil)     // ya deshecha
	e.must(400, "POST", fmt.Sprintf("/api/admin/audit/%d/undo", undoID), nil) // un deshacer no se deshace
	e.must(404, "POST", "/api/admin/audit/999999/undo", nil)

	e.redo(id, nil)
	if got := e.str(`select name from courses where id = 1`); got != "TAR 2" {
		t.Fatalf("nombre tras rehacer: %q", got)
	}
	if e.undone(id) {
		t.Fatal("rehacer debe limpiar undone_at")
	}
	e.must(400, "POST", fmt.Sprintf("/api/admin/audit/%d/redo", id), nil) // no está deshecha

	// Sin sesión no hay nada.
	e.cookie = nil
	e.must(401, "POST", fmt.Sprintf("/api/admin/audit/%d/undo", id), nil)
}

func TestUndoConflictAndForce(t *testing.T) {
	e := undoSetup(t)
	out := e.must(200, "PATCH", "/api/admin/courses/2", map[string]any{"sheetName": "AD", "logoUrl": ""})
	id := e.idOf(out, "auditId")
	e.exec(`update courses set sheet_name = 'Otra' where id = 2`)

	code, body := e.call("POST", fmt.Sprintf("/api/admin/audit/%d/undo", id), nil)
	fields, _ := body["fields"].([]any)
	if code != 409 || body["conflict"] != true || len(fields) != 1 || fields[0] != "sheet_name" {
		t.Fatalf("esperaba conflicto en sheet_name: %d %v", code, body)
	}
	if e.undone(id) {
		t.Fatal("un conflicto no debe marcar la entrada")
	}
	e.undo(id, map[string]any{"force": true})
	if got := e.str(`select sheet_name || '|' || logo_url from courses where id = 2`); got != "Analisis de Datos|/media/legacy/img/ad-01.png" {
		t.Fatalf("tras forzar: %q", got)
	}
}

func TestUndoCreateWithDependents(t *testing.T) {
	e := undoSetup(t)
	sid := e.newStudent("A1", 50)
	createID := e.audit(AuditEntry{Action: "create", Entity: "student", EntityID: itoa64(sid),
		Summary: "Alumno creado: A1", After: e.row("students", sid), Undoable: true})
	bid := e.newBadge("Lupa", 5, 10)
	pid := e.num(`insert into purchases (student_id, badge_id, badge_name, quantity, unit_cost, total_cost)
		values ($1, $2, 'Lupa', 1, 10, 10) returning id`, sid, bid)

	code, body := e.call("POST", fmt.Sprintf("/api/admin/audit/%d/undo", createID), nil)
	if code != 409 || body["conflict"] != true {
		t.Fatalf("con compras debe pedir confirmación: %d %v", code, body)
	}
	if e.num(`select count(*) from students where id = $1`, sid) != 1 {
		t.Fatal("el alumno no debe borrarse sin force")
	}

	res := e.undo(createID, map[string]any{"force": true})
	if e.num(`select count(*) from students where id = $1`, sid) != 0 || e.num(`select count(*) from purchases where id = $1`, pid) != 0 {
		t.Fatal("force debe borrar al alumno con sus compras")
	}
	var snap map[string]any
	if err := e.a.DB.QueryRow(e.ctx, `select before from audit_log where id = $1`, e.idOf(res, "auditId")).Scan(&snap); err != nil {
		t.Fatal(err)
	}
	if list, _ := snap["purchases"].([]any); len(list) != 1 {
		t.Fatalf("la entrada del deshacer debe guardar las compras: %v", snap)
	}

	e.redo(createID, nil)
	if got := e.str(`select code || '|' || monedas || '|' || password_hash from students where id = $1`, sid); got != "A1|50|hash-alumno" {
		t.Fatalf("alumno restaurado: %q", got)
	}
	if e.num(`select badge_id from purchases where id = $1 and student_id = $2`, pid, sid) != bid {
		t.Fatal("rehacer debe regresar la compra con su insignia")
	}
}

func TestUndoDeleteStudentWithPurchases(t *testing.T) {
	e := undoSetup(t)
	sid := e.newStudent("B1", 30)
	bid := e.newBadge("Regla", 9, 2)
	e.exec(`insert into purchases (student_id, badge_id, badge_name, quantity, unit_cost, total_cost, coins_before, coins_after)
		values ($1, $2, 'Regla', 2, 2, 4, 34, 30)`, sid, bid)
	e.exec(`insert into purchases (student_id, badge_id, badge_name, quantity, unit_cost, total_cost, reverted_at, source)
		values ($1, $2, 'Regla', 1, 2, 2, now(), 'admin')`, sid, bid)
	// Borrado como lo registra admin_students.go: {"row", "purchases"}.
	row, purchases := e.row("students", sid), e.purchasesJSON(sid)
	e.exec(`delete from students where id = $1`, sid)
	delID := e.audit(AuditEntry{Action: "delete", Entity: "student", EntityID: itoa64(sid),
		Summary: "Alumno eliminado: B1", Before: map[string]any{"row": row, "purchases": purchases}, Undoable: true})

	// Si otro alumno ya ocupa la matrícula en el curso, no se puede restaurar.
	other := e.newStudent("B1", 0)
	code, body := e.call("POST", fmt.Sprintf("/api/admin/audit/%d/undo", delID), nil)
	if code != 409 || !strings.Contains(fmt.Sprint(body["error"]), "matrícula") {
		t.Fatalf("esperaba 409 por matrícula ocupada: %d %v", code, body)
	}
	e.exec(`delete from students where id = $1`, other)

	e.undo(delID, nil)
	if !undoSame(e.row("students", sid), row) {
		t.Fatalf("fila restaurada distinta:\n%v\n%v", e.row("students", sid), row)
	}
	if !undoSame(e.purchasesJSON(sid), purchases) {
		t.Fatalf("compras restauradas distintas:\n%v\n%v", e.purchasesJSON(sid), purchases)
	}

	e.redo(delID, nil)
	if e.num(`select count(*) from students where id = $1`, sid)+e.num(`select count(*) from purchases where student_id = $1`, sid) != 0 {
		t.Fatal("rehacer debe borrar otra vez al alumno y sus compras")
	}
	e.undo(delID, nil)
	if e.num(`select count(*) from purchases where student_id = $1`, sid) != 2 {
		t.Fatal("deshacer otra vez debe regresar las dos compras")
	}
}

func TestUndoBatchBulk(t *testing.T) {
	e := undoSetup(t)
	batch := newBatchID()
	var ids, entries []int64
	// Suma de monedas en lote como la registra admin_students.go: una entrada por alumno.
	for i, code := range []string{"L1", "L2", "L3"} {
		sid := e.newStudent(code, 10*(i+1))
		before := e.row("students", sid)
		e.exec(`update students set monedas = monedas + 5, updated_at = now() where id = $1`, sid)
		entries = append(entries, e.audit(AuditEntry{Action: "update", Entity: "student", EntityID: itoa64(sid),
			Summary: "Monedas +5: " + code, Before: before, After: e.row("students", sid), BatchID: batch, Undoable: true}))
		ids = append(ids, sid)
	}
	coins := func() string {
		return e.str(`select string_agg(monedas::text, ',' order by id) from students where id = any($1)`, ids)
	}

	out := e.must(200, "POST", "/api/admin/audit/batch/"+batch+"/undo", nil)
	if e.idOf(out, "done") != 3 || len(out["failed"].([]any)) != 0 || coins() != "10,20,30" {
		t.Fatalf("deshacer lote: %v, monedas %s", out, coins())
	}
	for _, id := range entries {
		if !e.undone(id) {
			t.Fatalf("la entrada %d no quedó deshecha", id)
		}
	}
	e.must(400, "POST", "/api/admin/audit/batch/"+batch+"/undo", nil)

	out = e.must(200, "POST", "/api/admin/audit/batch/"+batch+"/redo", nil)
	if e.idOf(out, "done") != 3 || coins() != "15,25,35" {
		t.Fatalf("rehacer lote: %v, monedas %s", out, coins())
	}

	// Un alumno cambió después: ese falla, los demás siguen.
	e.exec(`update students set monedas = 99 where id = $1`, ids[1])
	out = e.must(200, "POST", "/api/admin/audit/batch/"+batch+"/undo", nil)
	failed := out["failed"].([]any)
	if e.idOf(out, "done") != 2 || len(failed) != 1 || coins() != "10,99,30" {
		t.Fatalf("lote con conflicto: %v, monedas %s", out, coins())
	}
	f := failed[0].(map[string]any)
	if e.idOf(f, "id") != entries[1] || !strings.Contains(f["error"].(string), "monedas") {
		t.Fatalf("falla reportada: %v", f)
	}
	out = e.must(200, "POST", "/api/admin/audit/batch/"+batch+"/undo", map[string]any{"force": true})
	if e.idOf(out, "done") != 1 || coins() != "10,20,30" {
		t.Fatalf("forzar lo que faltaba: %v, monedas %s", out, coins())
	}

	e.must(404, "POST", "/api/admin/audit/batch/"+newBatchID()+"/undo", nil)
	e.must(400, "POST", "/api/admin/audit/batch/no-es-uuid/undo", nil)
}

func TestUndoPurchaseMath(t *testing.T) {
	e := undoSetup(t)
	sid := e.newStudent("C1", 20)
	bid := e.newBadge("Brújula", 5, 4)
	state := func() string {
		return e.str(`select s.monedas || '/' || b.stock from students s, badges b where s.id = $1 and b.id = $2`, sid, bid)
	}
	reverted := func(pid int64) bool {
		return e.num(`select count(*) from purchases where id = $1 and reverted_at is not null`, pid) == 1
	}
	// Compra del alumno como la registra student.go: cobra, descuenta stock y anota la fila.
	e.exec(`update students set monedas = monedas - 8 where id = $1`, sid)
	e.exec(`update badges set stock = stock - 2 where id = $1`, bid)
	pid := e.num(`insert into purchases (student_id, badge_id, badge_name, quantity, unit_cost, total_cost, coins_before, coins_after)
		values ($1, $2, 'Brújula', 2, 4, 8, 20, 12) returning id`, sid, bid)
	after := e.row("purchases", pid)
	after["student_coins"], after["badge_stock"] = 12, 3
	actor := sid
	buyID := e.audit(AuditEntry{ActorType: "student", ActorID: &actor, ActorLabel: "C1", Action: "purchase",
		Entity: "purchase", EntityID: itoa64(pid), Summary: "Compra: 2 x Brújula", After: after,
		BatchID: newBatchID(), Undoable: true})

	res := e.undo(buyID, nil)
	if state() != "20/5" || !reverted(pid) {
		t.Fatalf("deshacer compra: %s, revertida=%v", state(), reverted(pid))
	}
	var snap map[string]any
	if err := e.a.DB.QueryRow(e.ctx, `select after from audit_log where id = $1`, e.idOf(res, "auditId")).Scan(&snap); err != nil {
		t.Fatal(err)
	}
	if e.idOf(snap, "student_coins") != 20 || e.idOf(snap, "badge_stock") != 5 {
		t.Fatalf("instantánea del deshacer: %v", snap)
	}
	e.redo(buyID, nil)
	if state() != "12/3" || reverted(pid) {
		t.Fatalf("rehacer compra: %s", state())
	}
	e.undo(buyID, nil)

	// Ya no alcanzan las monedas ni el stock: falla limpio y no toca nada.
	e.exec(`update students set monedas = 5 where id = $1`, sid)
	code, body := e.call("POST", fmt.Sprintf("/api/admin/audit/%d/redo", buyID), nil)
	if code != 400 || !strings.Contains(fmt.Sprint(body["error"]), "monedas suficientes") {
		t.Fatalf("sin monedas: %d %v", code, body)
	}
	if state() != "5/5" || !reverted(pid) || !e.undone(buyID) {
		t.Fatalf("una falla no debe cambiar nada: %s", state())
	}
	e.exec(`update students set monedas = 100 where id = $1`, sid)
	e.exec(`update badges set stock = 1 where id = $1`, bid)
	code, body = e.call("POST", fmt.Sprintf("/api/admin/audit/%d/redo", buyID), nil)
	if code != 400 || !strings.Contains(fmt.Sprint(body["error"]), "stock") {
		t.Fatalf("sin stock: %d %v", code, body)
	}

	// Reversión hecha desde Compras (admin_misc.go): deshacerla la vuelve a cobrar.
	e.exec(`update students set monedas = 10 where id = $1`, sid)
	e.exec(`update badges set stock = 5 where id = $1`, bid)
	p2 := e.num(`insert into purchases (student_id, badge_id, badge_name, quantity, unit_cost, total_cost)
		values ($1, $2, 'Brújula', 1, 4, 4) returning id`, sid, bid)
	before := e.row("purchases", p2)
	before["student_coins"], before["badge_stock"] = 10, 5
	e.exec(`update students set monedas = monedas + 4 where id = $1`, sid)
	e.exec(`update badges set stock = stock + 1 where id = $1`, bid)
	e.exec(`update purchases set reverted_at = now() where id = $1`, p2)
	after = e.row("purchases", p2)
	after["student_coins"], after["badge_stock"] = 14, 6
	revID := e.audit(AuditEntry{Action: "revert", Entity: "purchase", EntityID: itoa64(p2),
		Summary: "Compra revertida", Before: before, After: after, Undoable: true})
	e.undo(revID, nil)
	if state() != "10/5" || reverted(p2) {
		t.Fatalf("deshacer reversión: %s", state())
	}
	e.redo(revID, nil)
	if state() != "14/6" || !reverted(p2) {
		t.Fatalf("rehacer reversión: %s", state())
	}
}

func TestUndoLevels(t *testing.T) {
	e := undoSetup(t)
	levelsNow := func() string {
		levels, err := e.a.Levels(e.ctx) // con caché: también prueba que se invalida
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprint(levels)
	}
	orig := levelsNow()
	out := e.must(200, "PUT", "/api/admin/levels", map[string]any{"levels": []map[string]int{
		{"level": 1, "minExp": 5}, {"level": 0, "minExp": 0}, {"level": 2, "minExp": 50}}})
	id := e.idOf(out, "auditId")
	if levelsNow() != "[{0 0} {1 5} {2 50}]" {
		t.Fatalf("niveles guardados: %s", levelsNow())
	}
	e.undo(id, nil)
	if levelsNow() != orig {
		t.Fatalf("deshacer niveles: %s, esperaba %s", levelsNow(), orig)
	}
	e.redo(id, nil)
	if levelsNow() != "[{0 0} {1 5} {2 50}]" {
		t.Fatalf("rehacer niveles: %s", levelsNow())
	}
	e.exec(`update levels set min_exp = 7 where level = 1`)
	code, body := e.call("POST", fmt.Sprintf("/api/admin/audit/%d/undo", id), nil)
	if code != 409 || body["conflict"] != true {
		t.Fatalf("niveles cambiados después: %d %v", code, body)
	}
	e.undo(id, map[string]any{"force": true})
	if levelsNow() != orig {
		t.Fatalf("forzar niveles: %s", levelsNow())
	}
}

func TestUndoVisibility(t *testing.T) {
	e := undoSetup(t)
	b1, b2 := e.newBadge("Uno", 1, 1), e.newBadge("Dos", 1, 1)
	e.exec(`insert into course_hidden_badges (course_id, badge_id) values (1, $1)`, b1)
	hidden := func() string {
		return e.str(`select coalesce(string_agg(badge_id::text, ',' order by badge_id), '') from course_hidden_badges where course_id = 1`)
	}
	out := e.must(200, "PUT", "/api/admin/courses/1/hidden-badges", map[string]any{"badgeIds": []int64{b2, b2}})
	id := e.idOf(out, "auditId")
	if hidden() != itoa64(b2) {
		t.Fatalf("ocultas: %s", hidden())
	}
	e.undo(id, nil)
	if hidden() != itoa64(b1) {
		t.Fatalf("deshacer visibilidad: %s", hidden())
	}
	e.redo(id, nil)
	if hidden() != itoa64(b2) {
		t.Fatalf("rehacer visibilidad: %s", hidden())
	}
	e.exec(`insert into course_hidden_badges (course_id, badge_id) values (1, $1)`, b1)
	code, body := e.call("POST", fmt.Sprintf("/api/admin/audit/%d/undo", id), nil)
	if code != 409 || body["conflict"] != true {
		t.Fatalf("visibilidad cambiada después: %d %v", code, body)
	}
}

func TestUndoSetting(t *testing.T) {
	e := undoSetup(t)
	// Cambio de ajustes como lo registra admin_misc.go: solo lo que cambió, en snake_case;
	// los secretos como {"configured": ...}.
	save := func(patch map[string]any, entry AuditEntry) int64 {
		tx, err := e.a.DB.Begin(e.ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(e.ctx)
		if err := e.a.SaveSettings(e.ctx, tx, patch); err != nil {
			t.Fatal(err)
		}
		entry.ActorType, entry.ActorID, entry.ActorLabel = "admin", &e.adminID, "profe"
		entry.Entity, entry.Action = "setting", "update"
		id, err := e.a.Audit(e.ctx, tx, entry)
		if err == nil {
			err = tx.Commit(e.ctx)
		}
		if err != nil {
			t.Fatal(err)
		}
		e.a.invalidateSettings()
		return id
	}
	settings := func() Settings {
		s, err := e.a.Settings(e.ctx)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	id := save(map[string]any{"store_enabled": false, "reset_code_ttl_minutes": 30, "resend_api_key": "re_123"},
		AuditEntry{Summary: "Ajustes actualizados", Undoable: true,
			Before: map[string]any{"store_enabled": true, "reset_code_ttl_minutes": 10, "resend_api_key": map[string]any{"configured": false}},
			After:  map[string]any{"store_enabled": false, "reset_code_ttl_minutes": 30, "resend_api_key": map[string]any{"configured": true}}})
	if s := settings(); s.StoreEnabled || s.ResetCodeTTLMinutes != 30 {
		t.Fatalf("ajustes guardados: %+v", s)
	}
	e.undo(id, nil)
	if s := settings(); !s.StoreEnabled || s.ResetCodeTTLMinutes != 10 || s.ResendAPIKey != "re_123" {
		t.Fatalf("deshacer ajustes (el secreto no se toca): store=%v ttl=%d key=%q", s.StoreEnabled, s.ResetCodeTTLMinutes, s.ResendAPIKey)
	}
	e.redo(id, nil)
	if s := settings(); s.StoreEnabled || s.ResetCodeTTLMinutes != 30 {
		t.Fatalf("rehacer ajustes: %+v", s)
	}

	// Claves en camelCase también sirven; si cambió después hay conflicto.
	id = save(map[string]any{"leaderboard_enabled": false}, AuditEntry{Summary: "Leaderboard apagado", Undoable: true,
		Before: map[string]any{"leaderboardEnabled": true}, After: map[string]any{"leaderboardEnabled": false}})
	save(map[string]any{"leaderboard_enabled": true}, AuditEntry{Summary: "Leaderboard prendido"})
	code, body := e.call("POST", fmt.Sprintf("/api/admin/audit/%d/undo", id), nil)
	if code != 409 || body["conflict"] != true {
		t.Fatalf("ajuste cambiado después: %d %v", code, body)
	}

	// Solo secretos: no hay nada que reponer.
	id = save(map[string]any{"resend_api_key": "re_456"}, AuditEntry{Summary: "Clave de Resend", Undoable: true,
		Before: map[string]any{"resend_api_key": map[string]any{"configured": true}},
		After:  map[string]any{"resend_api_key": map[string]any{"configured": true}}})
	e.must(400, "POST", fmt.Sprintf("/api/admin/audit/%d/undo", id), nil)
}

func TestUndoAdmin(t *testing.T) {
	e := undoSetup(t)
	other := e.num(`insert into admins (username, password_hash) values ('otro', 'hash-1') returning id`)
	// Cambio de contraseña como lo registra admin_misc.go: filas completas.
	before := e.row("admins", other)
	e.exec(`update admins set password_hash = 'hash-2', updated_at = now() where id = $1`, other)
	id := e.audit(AuditEntry{Action: "update", Entity: "admin", EntityID: itoa64(other),
		Summary: "Contraseña cambiada: otro", Before: before, After: e.row("admins", other), Undoable: true})
	e.undo(id, nil)
	if e.str(`select password_hash from admins where id = $1`, other) != "hash-1" {
		t.Fatal("deshacer debe regresar el hash anterior")
	}

	// Desactivarme a mí mismo deshaciendo una activación: no.
	e.exec(`update admins set active = false where id = $1`, e.adminID)
	off := e.row("admins", e.adminID)
	e.exec(`update admins set active = true where id = $1`, e.adminID)
	act := e.audit(AuditEntry{Action: "update", Entity: "admin", EntityID: itoa64(e.adminID),
		Summary: "Admin activado: profe", Before: off, After: e.row("admins", e.adminID), Undoable: true})
	code, body := e.call("POST", fmt.Sprintf("/api/admin/audit/%d/undo", act), nil)
	if code != 400 || !strings.Contains(fmt.Sprint(body["error"]), "propia cuenta") {
		t.Fatalf("auto-desactivación: %d %v", code, body)
	}
	if e.num(`select count(*) from admins where id = $1 and active`, e.adminID) != 1 {
		t.Fatal("el admin debe seguir activo")
	}

	// Borrado de otro admin: deshacer lo regresa con su hash.
	snap := e.row("admins", other)
	e.exec(`delete from admins where id = $1`, other)
	del := e.audit(AuditEntry{Action: "delete", Entity: "admin", EntityID: itoa64(other),
		Summary: "Admin eliminado: otro", Before: snap, Undoable: true})
	e.undo(del, nil)
	if e.str(`select username || '|' || password_hash from admins where id = $1`, other) != "otro|hash-1" {
		t.Fatal("deshacer el borrado debe regresar usuario y hash")
	}

	// La bitácora nunca enseña hashes.
	out := e.must(200, "GET", "/api/admin/audit?entity=admin", nil)
	if raw, _ := json.Marshal(out); strings.Contains(string(raw), "password_hash") || strings.Contains(string(raw), "hash-1") {
		t.Fatalf("la bitácora filtró hashes: %s", raw)
	}
}

func TestUndoImportBatch(t *testing.T) {
	e := undoSetup(t)
	batch := newBatchID()
	// Una importación (importer.go): actualiza un alumno, crea otro, crea una insignia
	// y una compra de Sheets, y cierra con la entrada resumen.
	old := e.newStudent("D1", 10)
	before := e.row("students", old)
	e.exec(`update students set monedas = 40, updated_at = now() where id = $1`, old)
	oldAfter := e.row("students", old)
	ns := e.newStudent("D2", 0)
	nb := e.newBadge("Compás", 3, 6)
	np := e.num(`insert into purchases (student_id, badge_id, badge_name, quantity, source, source_ref)
		values ($1, $2, 'Compás', 1, 'sheets', 'ref-1') returning id`, ns, nb)
	entries := []int64{
		e.audit(AuditEntry{Action: "update", Entity: "student", EntityID: itoa64(old), Summary: "Importado: D1",
			Before: before, After: oldAfter, BatchID: batch, Undoable: true}),
		e.audit(AuditEntry{Action: "create", Entity: "student", EntityID: itoa64(ns), Summary: "Importado: D2",
			After: e.row("students", ns), BatchID: batch, Undoable: true}),
		e.audit(AuditEntry{Action: "create", Entity: "badge", EntityID: itoa64(nb), Summary: "Importada: Compás",
			After: e.row("badges", nb), BatchID: batch, Undoable: true}),
		e.audit(AuditEntry{Action: "create", Entity: "purchase", EntityID: itoa64(np), Summary: "Compra importada",
			After: e.row("purchases", np), BatchID: batch, Undoable: true}),
	}
	summary := e.audit(AuditEntry{Action: "import", Entity: "import", Summary: "Importación desde Sheets",
		After: map[string]any{"students": 2}, BatchID: batch, Undoable: true})
	check := func(want string) {
		t.Helper()
		got := e.str(`select (select monedas from students where id = $1) || '|' ||
			(select count(*) from students where id = $2) || '|' ||
			(select count(*) from badges where id = $3) || '|' ||
			(select count(*) from purchases where id = $4 and badge_id = $3)`, old, ns, nb, np)
		if got != want {
			t.Fatalf("estado %s, esperaba %s", got, want)
		}
	}

	e.undo(summary, nil)
	check("10|0|0|0")
	for _, id := range append(entries, summary) {
		if !e.undone(id) {
			t.Fatalf("la entrada %d debió quedar deshecha", id)
		}
	}
	e.redo(summary, nil)
	check("40|1|1|1")

	// El endpoint de lote hace lo mismo a partir de la entrada resumen.
	out := e.must(200, "POST", "/api/admin/audit/batch/"+batch+"/undo", nil)
	if e.idOf(out, "done") != 5 || len(out["failed"].([]any)) != 0 {
		t.Fatalf("deshacer lote de importación: %v", out)
	}
	check("10|0|0|0")
	out = e.must(200, "POST", "/api/admin/audit/batch/"+batch+"/redo", nil)
	if e.idOf(out, "done") != 5 {
		t.Fatalf("rehacer lote de importación: %v", out)
	}
	check("40|1|1|1")

	// Si algo del lote cambió después, la importación no se deshace a medias.
	e.exec(`update students set monedas = 41 where id = $1`, old)
	code, body := e.call("POST", fmt.Sprintf("/api/admin/audit/%d/undo", summary), nil)
	if code != 409 || body["conflict"] != true || !strings.Contains(fmt.Sprint(body["error"]), "Importado: D1") {
		t.Fatalf("importación con conflicto: %d %v", code, body)
	}
	if e.num(`select count(*) from students where id = $1`, ns) != 1 {
		t.Fatal("un conflicto no debe dejar la importación a medias")
	}
}

func TestUndoAuditList(t *testing.T) {
	e := undoSetup(t)
	batch := newBatchID()
	sid := e.newStudent("E1", 0)
	actor := sid
	e.audit(AuditEntry{Action: "create", Entity: "student", EntityID: itoa64(sid), Summary: "Alumno creado: E1",
		After: e.row("students", sid), Undoable: true})
	e.audit(AuditEntry{ActorType: "student", ActorID: &actor, ActorLabel: "E1", Action: "update", Entity: "student",
		EntityID: itoa64(sid), Summary: "Código enviado",
		After: map[string]any{"reset": map[string]any{"code_hash": "abc", "attempts": 0}}})
	e.audit(AuditEntry{Action: "update", Entity: "course", EntityID: "1", Summary: "Descuento del 100%", BatchID: batch})
	e.audit(AuditEntry{Action: "update", Entity: "course", EntityID: "1", Summary: "Descuento del 1000", BatchID: batch})

	out := e.must(200, "GET", "/api/admin/audit", nil)
	entries := out["entries"].([]any)
	if len(entries) != 4 || out["nextBeforeId"] != nil {
		t.Fatalf("lista completa: %v", out)
	}
	if raw, _ := json.Marshal(out); strings.Contains(string(raw), "password_hash") || strings.Contains(string(raw), "code_hash") {
		t.Fatalf("la bitácora filtró hashes: %s", raw)
	}
	first := entries[0].(map[string]any)
	if first["summary"] != "Descuento del 1000" || first["batchId"] != batch || first["entityId"] != "1" {
		t.Fatalf("orden o campos: %v", first)
	}

	page := e.must(200, "GET", "/api/admin/audit?limit=3", nil)
	next := e.idOf(page, "nextBeforeId")
	if len(page["entries"].([]any)) != 3 {
		t.Fatalf("página 1: %v", page)
	}
	page = e.must(200, "GET", fmt.Sprintf("/api/admin/audit?limit=3&beforeId=%d", next), nil)
	if rest := page["entries"].([]any); len(rest) != 1 || rest[0].(map[string]any)["summary"] != "Alumno creado: E1" || page["nextBeforeId"] != nil {
		t.Fatalf("página 2: %v", page)
	}

	filters := map[string]int{
		"entity=student":    2,
		"actorType=student": 1,
		"q=100%25":          1, // el % del filtro es literal
		"q=descuento":       2,
		"batchId=" + batch:  2,
		"limit=500":         4,
	}
	for qs, want := range filters {
		if got := len(e.must(200, "GET", "/api/admin/audit?"+qs, nil)["entries"].([]any)); got != want {
			t.Errorf("%s: %d entradas, esperaba %d", qs, got, want)
		}
	}
	for _, qs := range []string{"batchId=xyz", "limit=0", "limit=abc", "beforeId=-1"} {
		e.must(400, "GET", "/api/admin/audit?"+qs, nil)
	}
}
