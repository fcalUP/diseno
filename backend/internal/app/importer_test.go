package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// impTestApp arma un App cuya media local lee también la carpeta seed/media del repo.
func impTestApp(t *testing.T) *App {
	t.Helper()
	cfg := isolatedConfig(t)
	cfg.MediaSeedDir = filepath.Join("..", "..", "..", "seed", "media")
	a, err := New(context.Background(), cfg, isolatedDB(t), quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	return a
}

type impTester struct {
	t      *testing.T
	a      *App
	h      http.Handler
	cookie *http.Cookie
}

func newImpTester(t *testing.T) impTester {
	t.Helper()
	a := impTestApp(t)
	var id int64
	err := a.DB.QueryRow(context.Background(),
		`insert into admins (username, password_hash) values ('profe', 'x') returning id`).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	a.setSession(rec, kindAdmin, id, "x")
	return impTester{t: t, a: a, h: a.Routes(), cookie: rec.Result().Cookies()[0]}
}

// do manda la petición como el panel (JSON + cookie de admin) y decodifica la respuesta en out.
func (c impTester) do(method, path string, body, out any) int {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			c.t.Fatalf("%s %s: %v (%d %s)", method, path, err, rec.Code, rec.Body)
		}
	}
	return rec.Code
}

// errMsg hace la petición y devuelve código y mensaje de error.
func (c impTester) errMsg(method, path string, body any) (int, string) {
	c.t.Helper()
	var e struct {
		Error string `json:"error"`
	}
	code := c.do(method, path, body, &e)
	return code, e.Error
}

func (c impTester) preview(body any) impReport {
	c.t.Helper()
	var rep impReport
	if code := c.do("POST", "/api/admin/import/preview", body, &rep); code != http.StatusOK {
		c.t.Fatalf("vista previa: %d %+v", code, rep)
	}
	return rep
}

// run lanza la importación y espera a que el trabajo termine.
func (c impTester) run(body map[string]any) impJobJSON {
	c.t.Helper()
	var res struct {
		JobID string `json:"jobId"`
		Error string `json:"error"`
	}
	if code := c.do("POST", "/api/admin/import/run", body, &res); code != http.StatusAccepted || res.JobID == "" {
		c.t.Fatalf("run: %d %+v", code, res)
	}
	return c.waitJob(res.JobID)
}

func (c impTester) waitJob(id string) impJobJSON {
	c.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		var job impJobJSON
		if code := c.do("GET", "/api/admin/import/jobs/"+id, nil, &job); code != http.StatusOK {
			c.t.Fatalf("job: %d", code)
		}
		if job.Status != "running" {
			return job
		}
		if time.Now().After(deadline) {
			c.t.Fatal("el trabajo no terminó a tiempo")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (c impTester) count(sql string, args ...any) int {
	c.t.Helper()
	var n int
	if err := c.a.DB.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		c.t.Fatalf("%s: %v", sql, err)
	}
	return n
}

func impCreds(body map[string]any) map[string]any {
	body["credentialsJson"] = impCredentials()
	return body
}

func impHasWarning(rep impReport, parts ...string) bool {
	return slices.ContainsFunc(rep.Warnings, func(w string) bool {
		for _, p := range parts {
			if !strings.Contains(w, p) {
				return false
			}
		}
		return true
	})
}

var impStudentHeader = []string{"ID", "NOMBRE", "Sexo", "CORREO", "PAS", "Homeworks", "Coins", "Attendance",
	"Badges", "EXP", "LAST_LOGIN_LEVEL", "Games", "", "Comentarios"}

// impLegacyTabs reproduce la hoja vieja: encabezado, filas cortas, "1,000",
// contraseña vacía, código repetido y la pestaña con acento.
func impLegacyTabs() []impFakeTab {
	return []impFakeTab{
		{"TAR", [][]string{
			impStudentHeader,
			{"0251412", "Ana López", "Mujer", "", "AB12", "3", "1,000", "2", "4", "30", "2", "1", "", "Va muy bien"},
			{" 0254639 ", "Beto Ruiz", "Hombre", "beto@correo.mx", "CD34", "5", "20", "4"}, // fila corta
			{"0255286", "Caro Díaz", "", "", "", "0", "7"},                                 // sin sexo ni contraseña
			{},
			{"0251412", "Ana Repetida", "Mujer", "", "ZZ99"},
			{"", "Sin Código", "Hombre"},
		}},
		{"Análisis de Datos", [][]string{ // el curso tiene sheet_name "Analisis de Datos"
			impStudentHeader,
			{"0259982", "Dani Pérez", "Hombre", "", "EF56", "1", "15", "1", "0", "12", "1", "0"},
			{"0251412", "Ana López", "Mujer", "", "AB12", "2", "5", "1", "0", "10", "1", "0"},
		}},
		{"IHM", [][]string{impStudentHeader}},
		{"Badges", [][]string{
			{"Insignia", "Cantidad", "Costo"},
			{"Internet Práctico", "10", "5"},
			{"Kali Call Práctico", "3", "8"},
			{"Salvavidas Teórico", "4", "2"},
			{"Badge Nueva", "2"},
		}},
		{"Purchases", [][]string{
			{"Fecha", "ID", "Insignia", "Cantidad", "Carrera"},
			{"1/2/2026, 3:04:05 PM", "0254639", "Kali Call Práctico", "1", "Técnicas Avanzadas de Representación"},
			{"1/2/2026, 3:04:05 PM", "0254639", "Kali Call Práctico", "1", "Técnicas Avanzadas de Representación"},
			{"3/2/2026 15:04:05", "0251412", "Internet Práctico", "2", "Análisis de datos"},
			{"5/2/2026, 10:00:00", "0251412", "Kali Call Práctico", "1", "tecnicas"},
			{"2026-02-07 09:00:00", "0259982", "Salvavidas Teórico", "1"},          // sin carrera
			{"ayer", "0255286", "Insignia Perdida", "1", "TAR"},                    // fecha rara, fuera de catálogo
			{"8/2/2026, 11:00:00 AM", "0251412", "Kali Call Práctico", "1"},        // en dos cursos, sin carrera
			{"9/2/2026, 11:00:00 AM", "9999999", "Kali Call Práctico", "1", "TAR"}, // no existe
			{"10/2/2026, 11:00:00 AM", "0259982", "Kali Call Práctico", "1", "Analisis de Datos (grupo 2)"},
		}},
		{"Hoja 1", nil},
	}
}

var impNoMedia = map[string]any{"students": true, "badges": true, "purchases": true, "media": false}

func impCourseRep(t *testing.T, rep impReport, name string) impCourseReport {
	t.Helper()
	for _, c := range rep.Courses {
		if c.CourseName == name {
			return c
		}
	}
	t.Fatalf("no hay informe para el curso %s", name)
	return impCourseReport{}
}

type impStudentRowDB struct {
	Name, Sexo, Email, Hash, Comments                   string
	Tareas, Monedas, Asistencias, Exp, LastLevel, Games int
}

func (c impTester) student(course, code string) impStudentRowDB {
	c.t.Helper()
	var s impStudentRowDB
	err := c.a.DB.QueryRow(context.Background(), `select s.name, s.sexo, s.email, s.password_hash, s.comments,
		s.tareas, s.monedas, s.asistencias, s.exp, s.last_login_level, s.games
		from students s join courses c on c.id = s.course_id where c.sheet_name = $1 and s.code = $2`, course, code).
		Scan(&s.Name, &s.Sexo, &s.Email, &s.Hash, &s.Comments, &s.Tareas, &s.Monedas, &s.Asistencias, &s.Exp, &s.LastLevel, &s.Games)
	if err != nil {
		c.t.Fatalf("alumno %s/%s: %v", course, code, err)
	}
	return s
}

func TestImportRunUpsertAndRerun(t *testing.T) {
	c := newImpTester(t)
	f := newImpFake(t, impLegacyTabs())
	ctx := context.Background()

	job := c.run(impCreds(map[string]any{"include": impNoMedia, "mode": "upsert"}))
	if job.Status != "done" || job.Error != nil || job.Report == nil || job.FinishedAt == nil {
		t.Fatalf("trabajo: %+v", job)
	}
	rep := *job.Report
	if rep.DryRun || rep.BatchID == nil || rep.SpreadsheetTitle != "Gamificación 2026" || len(rep.SheetsFound) != 6 {
		t.Fatalf("informe: %+v", rep)
	}

	// Alumnos: la pestaña con acento casa con el curso sin acento; filas cortas y repetidas.
	tar := impCourseRep(t, rep, "Técnicas Avanzadas de Representación")
	if tar.Sheet == nil || *tar.Sheet != "TAR" || tar.Rows != 5 || tar.Created != 3 || tar.Skipped != 2 {
		t.Fatalf("TAR: %+v", tar)
	}
	ad := impCourseRep(t, rep, "Analisis de Datos")
	if ad.Sheet == nil || *ad.Sheet != "Análisis de Datos" || ad.Rows != 2 || ad.Created != 2 {
		t.Fatalf("AD: %+v", ad)
	}
	if ihm := impCourseRep(t, rep, "Interfaces Hombre-Máquina"); ihm.Sheet == nil || ihm.Rows != 0 {
		t.Fatalf("IHM: %+v", ihm)
	}
	if poo := impCourseRep(t, rep, "POO"); poo.Sheet != nil || !impHasWarning(rep, "No hay pestaña para el curso POO") {
		t.Fatalf("POO: %+v", poo)
	}
	for _, w := range [][]string{
		{"TAR, fila 2 (0251412)", `monedas "1,000" se leyó como 1`},
		{"TAR, fila 6", "repetido"},
		{"TAR, fila 7", "sin código"},
		{"TAR, fila 4 (0255286)", "sin contraseña"},
	} {
		if !impHasWarning(rep, w...) {
			t.Errorf("falta el aviso %q en %q", w, rep.Warnings)
		}
	}

	ana := c.student("TAR", "0251412")
	if ana.Monedas != 1 || ana.Tareas != 3 || ana.Asistencias != 2 || ana.Exp != 30 || ana.LastLevel != 2 ||
		ana.Games != 1 || ana.Comments != "Va muy bien" || ana.Email != "0251412@up.edu.mx" || ana.Sexo != "Mujer" {
		t.Fatalf("Ana: %+v", ana)
	}
	if !checkPassword(ana.Hash, "AB12") {
		t.Fatal("la contraseña de la hoja no quedó como hash bcrypt")
	}
	beto := c.student("TAR", "0254639")
	if beto.Email != "beto@correo.mx" || beto.Asistencias != 4 || beto.Exp != 0 || beto.Comments != "" || beto.Monedas != 20 {
		t.Fatalf("Beto (fila corta): %+v", beto)
	}
	caro := c.student("TAR", "0255286")
	if caro.Sexo != "N/A" || caro.Email != "0255286@up.edu.mx" || !strings.HasPrefix(caro.Hash, "$2") || caro.Monedas != 7 {
		t.Fatalf("Caro: %+v", caro)
	}
	if c.count(`select count(*) from students`) != 5 {
		t.Fatal("se esperaban 5 alumnos")
	}

	// Insignias: visibilidad por palabras del curso e imagen heredada si existe.
	if rep.Badges != (impBadgeReport{Rows: 4, Created: 4}) {
		t.Fatalf("insignias: %+v", rep.Badges)
	}
	var kaliImg, nuevaImg string
	c.a.DB.QueryRow(ctx, `select image_url from badges where slug = 'kali_call_practico'`).Scan(&kaliImg)
	c.a.DB.QueryRow(ctx, `select image_url from badges where slug = 'badge_nueva'`).Scan(&nuevaImg)
	if kaliImg != "/media/legacy/badges/kali_call_practico.png" || nuevaImg != "" {
		t.Fatalf("imágenes: kali=%q nueva=%q", kaliImg, nuevaImg)
	}
	hidden := c.count(`select count(*) from course_hidden_badges h join badges b on b.id = h.badge_id
		join courses c on c.id = h.course_id where b.slug = 'internet_practico' and c.sheet_name = 'Analisis de Datos'`)
	if hidden != 1 || c.count(`select count(*) from course_hidden_badges`) != 1 {
		t.Fatal("Internet Práctico debió nacer oculta solo en Analisis de Datos")
	}

	// Compras: variantes de carrera, sin carrera, alumno desconocido o ambiguo.
	if rep.Purchases != (impPurchaseReport{Rows: 9, Created: 7, Unmatched: 2}) {
		t.Fatalf("compras: %+v %q", rep.Purchases, rep.Warnings)
	}
	if !impHasWarning(rep, "fila 8", "varios cursos") || !impHasWarning(rep, "fila 9", "no existe el alumno 9999999") ||
		!impHasWarning(rep, "Insignia Perdida", "no está en el catálogo") || !impHasWarning(rep, "fila 7", `fecha "ayer"`) {
		t.Fatalf("avisos de compras: %q", rep.Warnings)
	}
	perCourse := map[string]int{}
	rows, _ := c.a.DB.Query(ctx, `select c.sheet_name || ':' || s.code, count(*) from purchases p
		join students s on s.id = p.student_id join courses c on c.id = s.course_id group by 1`)
	for rows.Next() {
		var k string
		var n int
		rows.Scan(&k, &n)
		perCourse[k] = n
	}
	want := map[string]int{"TAR:0254639": 2, "Analisis de Datos:0251412": 1, "TAR:0251412": 1, "Analisis de Datos:0259982": 2, "TAR:0255286": 1}
	for k, n := range want {
		if perCourse[k] != n {
			t.Fatalf("compras por alumno = %v, esperaba %v", perCourse, want)
		}
	}
	var at time.Time
	var unit, total int
	var source, ref string
	err := c.a.DB.QueryRow(ctx, `select p.created_at, p.unit_cost, p.total_cost, p.source, p.source_ref from purchases p
		join students s on s.id = p.student_id where s.code = '0254639' order by p.id limit 1`).Scan(&at, &unit, &total, &source, &ref)
	if err != nil {
		t.Fatal(err)
	}
	if !at.Equal(time.Date(2026, 1, 2, 15, 4, 5, 0, impMexico)) || unit != 8 || total != 8 || source != "sheets" || !strings.HasPrefix(ref, "sheets:") {
		t.Fatalf("compra importada: %v %d %d %s %s", at, unit, total, source, ref)
	}
	var lostBadge *int64
	var lostCost int
	c.a.DB.QueryRow(ctx, `select badge_id, unit_cost from purchases where badge_name = 'Insignia Perdida'`).Scan(&lostBadge, &lostCost)
	if lostBadge != nil || lostCost != 0 {
		t.Fatal("la compra de una insignia fuera de catálogo debe quedar sin badge_id y costo 0")
	}
	// Las compras ya ocurrieron: ni monedas ni existencias cambian.
	if c.student("TAR", "0254639").Monedas != 20 || c.count(`select stock from badges where slug = 'kali_call_practico'`) != 3 {
		t.Fatal("la importación no debe descontar monedas ni existencias")
	}

	// Bitácora: una entrada por fila creada más el resumen, todas del admin y del mismo lote.
	batch := *rep.BatchID
	if n := c.count(`select count(*) from audit_log where batch_id = $1`, batch); n != 5+4+7+1 {
		t.Fatalf("entradas en el lote: %d", n)
	}
	if n := c.count(`select count(*) from audit_log where batch_id = $1 and (actor_type <> 'admin' or actor_label <> 'profe' or not undoable)`, batch); n != 0 {
		t.Fatal("todas las entradas deben ser del admin y deshacibles")
	}
	if n := c.count(`select count(*) from audit_log where batch_id = $1 and entity = 'purchase' and action = 'create'
		and after ? 'student_coins' and after ? 'badge_stock'`, batch); n != 7 {
		t.Fatalf("instantáneas de compra: %d", n)
	}
	var summaryAfter map[string]any
	c.a.DB.QueryRow(ctx, `select after from audit_log where batch_id = $1 and entity = 'import' and action = 'import'`, batch).Scan(&summaryAfter)
	if summaryAfter["dryRun"] != false || summaryAfter["batchId"] != batch {
		t.Fatalf("resumen: %v", summaryAfter)
	}

	// Reimportar lo mismo no duplica nada.
	job = c.run(impCreds(map[string]any{"include": impNoMedia, "mode": "upsert"}))
	rep = *job.Report
	if tar := impCourseRep(t, rep, "Técnicas Avanzadas de Representación"); tar.Unchanged != 3 || tar.Created+tar.Updated != 0 {
		t.Fatalf("TAR al reimportar: %+v", tar)
	}
	if ad := impCourseRep(t, rep, "Analisis de Datos"); ad.Unchanged != 2 {
		t.Fatalf("AD al reimportar: %+v", ad)
	}
	if rep.Badges != (impBadgeReport{Rows: 4, Unchanged: 4}) || rep.Purchases != (impPurchaseReport{Rows: 9, Skipped: 7, Unmatched: 2}) {
		t.Fatalf("reimportar: %+v %+v", rep.Badges, rep.Purchases)
	}
	if c.count(`select count(*) from purchases`) != 7 || c.count(`select count(*) from audit_log where batch_id = $1`, *rep.BatchID) != 1 {
		t.Fatal("reimportar duplicó compras o registró cambios inexistentes")
	}

	// Cambios en la hoja: se actualiza solo lo distinto y la bitácora guarda antes/después.
	tabs := impLegacyTabs()
	tabs[0].rows[2][6] = "25" // monedas de Beto
	tabs[3].rows[2][2] = "9"  // costo de Kali
	purchases := append(tabs[4].rows, []string{"11/2/2026, 9:00:00 AM", "0254639", "Kali Call Práctico", "2", "TAR"})
	f.setTab("TAR", tabs[0].rows)
	f.setTab("Badges", tabs[3].rows)
	f.setTab("Purchases", purchases)
	rep = *c.run(impCreds(map[string]any{"include": impNoMedia})).Report
	if tar := impCourseRep(t, rep, "Técnicas Avanzadas de Representación"); tar.Updated != 1 || tar.Unchanged != 2 {
		t.Fatalf("TAR tras cambiar monedas: %+v", tar)
	}
	if rep.Badges.Updated != 1 || rep.Purchases.Created != 1 {
		t.Fatalf("tras cambios: %+v %+v", rep.Badges, rep.Purchases)
	}
	var before, after map[string]any
	var summary string
	err = c.a.DB.QueryRow(ctx, `select before, after, summary from audit_log where batch_id = $1 and entity = 'student'`, *rep.BatchID).Scan(&before, &after, &summary)
	if err != nil || before["monedas"] != float64(20) || after["monedas"] != float64(25) || !strings.Contains(summary, "monedas") {
		t.Fatalf("auditoría de la actualización: %v %v %q %v", before["monedas"], after["monedas"], summary, err)
	}
	if c.count(`select unit_cost from purchases where quantity = 2 and badge_name = 'Kali Call Práctico'`) != 9 {
		t.Fatal("la compra nueva debió tomar el costo actual de la insignia")
	}
}

func TestImportCreateOnly(t *testing.T) {
	c := newImpTester(t)
	f := newImpFake(t, impLegacyTabs())
	c.run(impCreds(map[string]any{"include": impNoMedia}))

	tabs := impLegacyTabs()
	tabs[0].rows[2][6] = "30" // monedas de Beto: no debe tocarse
	tabs[0].rows = append(tabs[0].rows, []string{"0261111", "Eli Nuevo", "Mujer", "", "GH78", "1", "2"})
	tabs[3].rows[2][2] = "10" // costo de Kali: tampoco
	f.setTab("TAR", tabs[0].rows)
	f.setTab("Badges", tabs[3].rows)

	job := c.run(impCreds(map[string]any{"include": impNoMedia, "mode": "create_only"}))
	rep := *job.Report
	tar := impCourseRep(t, rep, "Técnicas Avanzadas de Representación")
	if tar.Rows != 6 || tar.Created != 1 || tar.Skipped != 5 || tar.Updated != 0 {
		t.Fatalf("TAR en create_only: %+v", tar)
	}
	if rep.Badges.Skipped != 4 || rep.Badges.Updated != 0 {
		t.Fatalf("insignias en create_only: %+v", rep.Badges)
	}
	if c.student("TAR", "0254639").Monedas != 20 || c.count(`select cost from badges where slug = 'kali_call_practico'`) != 8 {
		t.Fatal("create_only no debe cambiar lo que ya existe")
	}
	if eli := c.student("TAR", "0261111"); !checkPassword(eli.Hash, "GH78") {
		t.Fatal("create_only debió crear al alumno nuevo")
	}
}

func TestImportPreviewWritesNothing(t *testing.T) {
	c := newImpTester(t)
	newImpFake(t, impLegacyTabs())
	baseline := map[string]int{}
	for _, table := range []string{"students", "badges", "purchases", "avatars", "course_hidden_badges", "audit_log"} {
		baseline[table] = c.count(`select count(*) from ` + table)
	}

	// Sin include ni mode simula todo, incluida la media heredada.
	rep := c.preview(map[string]any{"credentialsJson": impCredentials()})
	if !rep.DryRun || rep.BatchID != nil {
		t.Fatalf("la vista previa no se marcó como simulación: %+v", rep)
	}
	tar := impCourseRep(t, rep, "Técnicas Avanzadas de Representación")
	if tar.Created != 3 || tar.Skipped != 2 || rep.Badges.Created != 4 {
		t.Fatalf("conteos simulados: %+v %+v", tar, rep.Badges)
	}
	// Las compras casan con alumnos que solo existen dentro de la simulación.
	if rep.Purchases != (impPurchaseReport{Rows: 9, Created: 7, Unmatched: 2}) {
		t.Fatalf("compras simuladas: %+v", rep.Purchases)
	}
	if rep.Media.Avatars == 0 || !impHasWarning(rep, "02798126.png", "nivel 6") {
		t.Fatalf("media simulada: %+v", rep.Media)
	}
	for table, before := range baseline {
		if n := c.count(`select count(*) from ` + table); n != before {
			t.Fatalf("la vista previa cambió %s de %d a %d filas", table, before, n)
		}
	}

	// En create_only con datos existentes, la simulación los cuenta como omitidos.
	c.run(impCreds(map[string]any{"include": impNoMedia}))
	before := c.count(`select count(*) from audit_log`)
	rep = c.preview(impCreds(map[string]any{"mode": "create_only", "include": impNoMedia}))
	if tar := impCourseRep(t, rep, "Técnicas Avanzadas de Representación"); tar.Skipped != 5 || tar.Created != 0 {
		t.Fatalf("vista previa create_only: %+v", tar)
	}
	if c.count(`select count(*) from audit_log`) != before {
		t.Fatal("la vista previa registró algo en la bitácora")
	}
}

func TestImportJobLifecycle(t *testing.T) {
	c := newImpTester(t)
	f := newImpFake(t, impLegacyTabs())
	f.block = make(chan struct{})
	released := false
	release := func() {
		if !released {
			released = true
			close(f.block)
		}
	}
	t.Cleanup(release)

	var res struct {
		JobID string `json:"jobId"`
	}
	if code := c.do("POST", "/api/admin/import/run", impCreds(map[string]any{"include": impNoMedia}), &res); code != http.StatusAccepted {
		t.Fatalf("run: %d", code)
	}
	var job map[string]any
	c.do("GET", "/api/admin/import/jobs/"+res.JobID, nil, &job)
	if job["status"] != "running" || job["report"] != nil || job["finishedAt"] != nil || job["error"] != nil || job["startedAt"] == nil {
		t.Fatalf("trabajo en curso: %v", job)
	}
	if code, msg := c.errMsg("POST", "/api/admin/import/run", impCreds(map[string]any{})); code != http.StatusConflict || msg != impBusy {
		t.Fatalf("segunda corrida: %d %q", code, msg)
	}
	if code, _ := c.errMsg("POST", "/api/admin/import/media", nil); code != http.StatusConflict {
		t.Fatalf("media durante una corrida: %d", code)
	}
	release()
	done := c.waitJob(res.JobID)
	if done.Status != "done" || done.Report == nil || done.Error != nil || done.FinishedAt == nil {
		t.Fatalf("trabajo terminado: %+v", done)
	}

	// El candado se soltó; un error de Google termina el trabajo con su mensaje.
	f.setFail(http.StatusForbidden)
	failed := c.run(impCreds(map[string]any{}))
	if failed.Status != "error" || failed.Report != nil || failed.Error == nil ||
		*failed.Error != "La cuenta de servicio "+impTestEmail+" no tiene acceso a la hoja. Compártela con ese correo." {
		t.Fatalf("trabajo con error: %+v", failed)
	}
	if code, _ := c.errMsg("GET", "/api/admin/import/jobs/no-existe", nil); code != http.StatusNotFound {
		t.Fatalf("trabajo inexistente: %d", code)
	}
}

func TestImportRequestErrors(t *testing.T) {
	c := newImpTester(t)
	f := newImpFake(t, impLegacyTabs())
	ctx := context.Background()

	anon := c
	anon.cookie = nil
	if code, _ := anon.errMsg("POST", "/api/admin/import/preview", map[string]any{}); code != http.StatusUnauthorized {
		t.Fatalf("sin sesión: %d", code)
	}
	// Validación: preview y run responden igual, sin llegar a Google.
	cases := []struct {
		body map[string]any
		want string
	}{
		{map[string]any{}, "Configura las credenciales de Google en Ajustes o sube el archivo JSON."},
		{map[string]any{"credentialsJson": `{"type":"authorized_user"}`}, "Las credenciales de Google no son válidas: se espera el JSON de una cuenta de servicio."},
		{impCreds(map[string]any{"mode": "todo"}), "Modo de importación inválido."},
		{impCreds(map[string]any{"include": map[string]bool{}}), "Elige al menos una parte para importar."},
		{impCreds(map[string]any{"spreadsheetId": "../../etc"}), "El ID de la hoja de cálculo no es válido."},
	}
	for _, tc := range cases {
		for _, path := range []string{"/api/admin/import/preview", "/api/admin/import/run"} {
			if code, msg := c.errMsg("POST", path, tc.body); code != http.StatusBadRequest || msg != tc.want {
				t.Errorf("%s: %d %q, esperaba %q", path, code, msg, tc.want)
			}
		}
	}
	// Los errores de Google llegan directo en la vista previa (en la corrida, por el trabajo).
	if code, msg := c.errMsg("POST", "/api/admin/import/preview", impCreds(map[string]any{"spreadsheetId": "1otraHojaQueNoExisteParaNadaXYZ"})); code != http.StatusBadRequest || msg != "No existe la hoja con ese ID." {
		t.Fatalf("404 de Google: %d %q", code, msg)
	}
	f.setFail(http.StatusForbidden)
	if code, msg := c.errMsg("POST", "/api/admin/import/preview", impCreds(map[string]any{})); code != http.StatusBadRequest ||
		msg != "La cuenta de servicio "+impTestEmail+" no tiene acceso a la hoja. Compártela con ese correo." {
		t.Fatalf("403 de Google: %d %q", code, msg)
	}
	f.setFail(0)

	// Credenciales guardadas en Ajustes y la url completa de la hoja.
	tx, err := c.a.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.a.SaveSettings(ctx, tx, map[string]any{"google_service_account": impCredentials()}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	c.a.invalidateSettings()
	rep := c.preview(map[string]any{"spreadsheetId": "https://docs.google.com/spreadsheets/d/" + impTestSheetID + "/edit#gid=0"})
	if rep.SpreadsheetTitle != "Gamificación 2026" {
		t.Fatalf("vista previa con credenciales de Ajustes: %+v", rep)
	}
	// Cada rango se pidió con el nombre de la pestaña entre comillas.
	f.mu.Lock()
	last := f.batches[len(f.batches)-1]
	f.mu.Unlock()
	if !slices.Contains(last, "'Análisis de Datos'!A:N") || !slices.Contains(last, "'Badges'!A:C") || !slices.Contains(last, "'Purchases'!A:E") {
		t.Fatalf("rangos pedidos: %q", last)
	}

	// La vista previa tiene un límite por admin; las que fallan en Google también cuentan.
	f.setFail(http.StatusForbidden)
	limited := false
	for range 12 {
		if code, _ := c.errMsg("POST", "/api/admin/import/preview", impCreds(map[string]any{})); code == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("la vista previa debió limitarse")
	}
}

func TestImportStatus(t *testing.T) {
	c := newImpTester(t)
	newImpFake(t, impLegacyTabs())

	var st map[string]any
	if code := c.do("GET", "/api/admin/import/status", nil, &st); code != http.StatusOK {
		t.Fatalf("status: %d", code)
	}
	media := st["legacyMedia"].(map[string]any)
	creds := st["credentials"].(map[string]any)
	if st["spreadsheetId"] != impTestSheetID || creds["configured"] != false || creds["source"] != "none" ||
		media["available"] != true || media["avatars"] != float64(impSeedFiles(t, "avatars")) ||
		media["badges"] != float64(impSeedFiles(t, "badges")) || st["lastImport"] != nil {
		t.Fatalf("estado inicial: %v", st)
	}

	job := c.run(impCreds(map[string]any{"include": impNoMedia}))
	c.do("GET", "/api/admin/import/status", nil, &st)
	last, _ := st["lastImport"].(map[string]any)
	if last == nil || last["action"] != "import" || last["batchId"] != *job.Report.BatchID || last["undoable"] != true {
		t.Fatalf("última importación: %v", st["lastImport"])
	}
	if after, _ := last["after"].(map[string]any); after["spreadsheetTitle"] != "Gamificación 2026" {
		t.Fatalf("el resumen debe guardar el informe: %v", last["after"])
	}
}

// impSeedFiles cuenta los archivos de seed/media/legacy/{dir}, sin ocultos
// (List los ignora y macOS deja .DS_Store).
func impSeedFiles(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join("..", "..", "..", "seed", "media", "legacy", dir))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			n++
		}
	}
	return n
}

func TestImportMedia(t *testing.T) {
	c := newImpTester(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := c.a.DB.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	// Un avatar subido por el admin y tres insignias: sin imagen, con imagen propia y sin archivo heredado.
	exec(`insert into avatars (code, level, url) values ('0251412', 0, '/media/u/2026/11/propia.png')`)
	exec(`insert into badges (name, slug, image_url) values ('Calculadora', 'calculadora', ''),
		('Pista Práctico', 'pista_practico', '/media/u/2026/11/pista.png'), ('Sin Archivo', 'sin_archivo', '')`)

	var res struct {
		Media    impMediaReport `json:"media"`
		Warnings []string       `json:"warnings"`
		BatchID  *string        `json:"batchId"`
	}
	if code := c.do("POST", "/api/admin/import/media", nil, &res); code != http.StatusOK {
		t.Fatalf("media: %d %+v", code, res)
	}
	// Omite el nivel 6, la casilla ocupada y los defaults que ya instaló la migración.
	total := impSeedFiles(t, "avatars")
	defaults := c.count(`select count(distinct url) from avatars where code in ('DefaultH', 'DefaultF')`)
	wantAvatars := total - 2 - defaults
	if res.Media.Avatars != wantAvatars || res.Media.BadgeImages != 1 || res.Media.CourseImages != 0 || res.BatchID == nil {
		t.Fatalf("media importada: %+v de %d archivos", res, total)
	}
	if !slices.ContainsFunc(res.Warnings, func(w string) bool { return strings.Contains(w, "02798126.png") && strings.Contains(w, "nivel 6") }) {
		t.Fatalf("falta el aviso del nivel 6: %q", res.Warnings)
	}
	url := func(code string, level int) string {
		var u string
		c.a.DB.QueryRow(ctx, `select url from avatars where code = $1 and level = $2`, code, level).Scan(&u)
		return u
	}
	if url("0251412", 0) != "/media/u/2026/11/propia.png" {
		t.Fatal("se pisó un avatar subido por el admin")
	}
	if url("0251412", 1) != "/media/legacy/avatars/02514121.png" || url("DefaultH", 3) != "/media/legacy/avatars/DefaultH3.png" {
		t.Fatal("faltan avatares heredados")
	}
	if url("0279812", 6) != "" {
		t.Fatal("no debe registrarse un avatar de un nivel que no existe")
	}
	images := map[string]string{}
	rows, _ := c.a.DB.Query(ctx, `select slug, image_url from badges`)
	for rows.Next() {
		var slug, img string
		rows.Scan(&slug, &img)
		images[slug] = img
	}
	if images["calculadora"] != "/media/legacy/badges/calculadora.png" || images["pista_practico"] != "/media/u/2026/11/pista.png" || images["sin_archivo"] != "" {
		t.Fatalf("imágenes de insignias: %v", images)
	}
	if n := c.count(`select count(*) from audit_log where batch_id = $1 and undoable and actor_label = 'profe'`, *res.BatchID); n != wantAvatars+1 {
		t.Fatalf("entradas de bitácora del lote: %d", n)
	}
	if n := c.count(`select count(*) from audit_log where batch_id = $1 and entity = 'avatar' and entity_id = '0251412:1'`, *res.BatchID); n != 1 {
		t.Fatal("el avatar se audita con entity_id código:nivel")
	}

	// Una segunda vez no hay nada nuevo y no se abre lote.
	res.BatchID = nil
	if code := c.do("POST", "/api/admin/import/media", nil, &res); code != http.StatusOK || res.Media.Avatars != 0 || res.Media.BadgeImages != 0 || res.BatchID != nil {
		t.Fatalf("segunda importación de media: %d %+v", code, res)
	}
}
