package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// catUpload deja un archivo en MEDIA_DIR como si el navegador ya lo hubiera subido.
func catUpload(e *undoEnv, key string) string {
	e.t.Helper()
	p := filepath.Join(e.a.Cfg.MediaDir, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("png"), 0o644); err != nil {
		e.t.Fatal(err)
	}
	return mediaURL(key)
}

func catIDs(v any) string { return fmt.Sprint(undoIDs(v)) }

func TestCatCourses(t *testing.T) {
	e := undoSetup(t)
	logo := catUpload(e, "u/2026/11/logo.png")

	out := e.must(201, "POST", "/api/admin/courses", map[string]any{
		"name": "  Fotografía ", "sheetAliases": []string{" foto ", "", "foto", "Foto"},
		"hiddenBadgeKeywords": []string{"Cámara"}, "logoUrl": logo, "sideImageUrl": "/media/legacy/img/side-image.jpg",
	})
	c := out["course"].(map[string]any)
	id := e.idOf(c, "id")
	if c["name"] != "Fotografía" || c["sheetName"] != "Fotografía" || fmt.Sprint(c["sheetAliases"]) != "[foto Foto]" ||
		c["active"] != true || e.idOf(c, "sortOrder") != 6 || e.idOf(c, "studentCount") != 0 ||
		catIDs(c["hiddenBadgeIds"]) != "[]" || c["logoUrl"] != logo {
		t.Fatalf("curso creado: %v", c)
	}
	if e.str(`select action || '|' || entity || '|' || entity_id from audit_log where id = $1`, e.idOf(out, "auditId")) != "create|course|"+itoa64(id) {
		t.Fatal("falta la entrada de bitácora del alta")
	}

	bad := []map[string]any{
		{"name": ""},
		{"name": strings.Repeat("x", 121)},
		{"name": "Fotografía"}, // repetido
		{"name": "Otro", "sheetName": "TAR"},
		{"name": "Otro", "logoUrl": "https://ejemplo.com/logo.png"},
		{"name": "Otro", "logoUrl": "/media/../secreto"},
		{"name": "Otro", "sideImageUrl": "/media/u/2026/11/no-existe.png"},
		{"name": "Otro", "sortOrder": 1 << 40},
	}
	for _, b := range bad {
		if code, body := e.call("POST", "/api/admin/courses", b); code != 400 && code != 409 {
			t.Errorf("%v: esperaba 400/409, llegó %d %v", b, code, body)
		}
	}
	if code, body := e.call("POST", "/api/admin/courses", map[string]any{"name": "Fotografía"}); code != 409 ||
		body["error"] != "Ya existe un curso con ese nombre." {
		t.Fatalf("nombre repetido: %d %v", code, body)
	}

	list := e.must(200, "GET", "/api/admin/courses", nil)["courses"].([]any)
	if len(list) != 6 || list[5].(map[string]any)["name"] != "Fotografía" {
		t.Fatalf("lista de cursos: %v", list)
	}

	path := fmt.Sprintf("/api/admin/courses/%d", id)
	out = e.must(200, "PATCH", path, map[string]any{"active": false, "sheetName": "FOTO", "logoUrl": ""})
	c = out["course"].(map[string]any)
	if c["active"] != false || c["sheetName"] != "FOTO" || c["logoUrl"] != "" || c["name"] != "Fotografía" {
		t.Fatalf("curso editado: %v", c)
	}
	e.must(400, "PATCH", path, map[string]any{})
	e.must(400, "PATCH", path, map[string]any{"name": " "})
	e.must(400, "PATCH", path, map[string]any{"sheetName": ""})
	e.must(404, "PATCH", "/api/admin/courses/999", map[string]any{"name": "X"})

	// Con alumnos no se borra.
	e.newStudent("F1", 0)
	if code, body := e.call("DELETE", "/api/admin/courses/1", nil); code != 409 ||
		body["error"] != "El curso tiene alumnos. Muévelos o desactiva el curso." {
		t.Fatalf("borrar curso con alumnos: %d %v", code, body)
	}

	b := e.newBadge("Lente", 1, 1)
	e.must(200, "PUT", path+"/hidden-badges", map[string]any{"badgeIds": []int64{b}})
	out = e.must(200, "DELETE", path, nil)
	if out["success"] != true || e.num(`select count(*) from courses where id = $1`, id) != 0 {
		t.Fatalf("borrar curso: %v", out)
	}
	var snap map[string]any
	if err := e.a.DB.QueryRow(e.ctx, `select before from audit_log where id = $1`, e.idOf(out, "auditId")).Scan(&snap); err != nil {
		t.Fatal(err)
	}
	if snap["row"] == nil || catIDs(snap["hidden_badge_ids"]) != fmt.Sprint([]int64{b}) {
		t.Fatalf("instantánea del borrado: %v", snap)
	}
	e.must(404, "DELETE", path, nil)

	// Deshacer el borrado regresa el curso y sus insignias ocultas.
	e.undo(e.idOf(out, "auditId"), nil)
	if e.str(`select sheet_name from courses where id = $1`, id) != "FOTO" ||
		e.num(`select count(*) from course_hidden_badges where course_id = $1 and badge_id = $2`, id, b) != 1 {
		t.Fatal("deshacer el borrado del curso")
	}

	e.cookie = nil
	e.must(401, "GET", "/api/admin/courses", nil)
}

func TestCatHiddenBadges(t *testing.T) {
	e := undoSetup(t)
	b1, b2 := e.newBadge("Uno", 1, 1), e.newBadge("Dos", 1, 1)
	out := e.must(200, "PUT", "/api/admin/courses/3/hidden-badges", map[string]any{"badgeIds": []int64{b2, b1, b2}})
	if c := out["course"].(map[string]any); catIDs(c["hiddenBadgeIds"]) != fmt.Sprint([]int64{b1, b2}) {
		t.Fatalf("ocultas: %v", c)
	}
	var after map[string]any
	if err := e.a.DB.QueryRow(e.ctx, `select after from audit_log where id = $1 and entity = 'visibility'`,
		e.idOf(out, "auditId")).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if e.idOf(after, "course_id") != 3 || catIDs(after["badge_ids"]) != fmt.Sprint([]int64{b1, b2}) {
		t.Fatalf("instantánea de visibilidad: %v", after)
	}
	e.must(400, "PUT", "/api/admin/courses/3/hidden-badges", map[string]any{"badgeIds": []int64{b1, 9999}})
	e.must(400, "PUT", "/api/admin/courses/3/hidden-badges", map[string]any{})
	e.must(404, "PUT", "/api/admin/courses/999/hidden-badges", map[string]any{"badgeIds": []int64{}})
	out = e.must(200, "PUT", "/api/admin/courses/3/hidden-badges", map[string]any{"badgeIds": []int64{}})
	if c := out["course"].(map[string]any); catIDs(c["hiddenBadgeIds"]) != "[]" {
		t.Fatalf("sin ocultas: %v", c)
	}
}

func TestCatBadges(t *testing.T) {
	e := undoSetup(t)
	img := catUpload(e, "u/2026/11/internet.png")

	// "Analisis de Datos" (curso 2) oculta las insignias con "internet" o "calculadora".
	out := e.must(201, "POST", "/api/admin/badges", map[string]any{"name": "Internet  Gratis", "cost": 5, "stock": 10, "imageUrl": img})
	b := out["badge"].(map[string]any)
	id := e.idOf(b, "id")
	if b["slug"] != "internet_gratis" || catIDs(b["hiddenCourseIds"]) != "[2]" || e.idOf(b, "stock") != 10 ||
		b["active"] != true || e.idOf(b, "sortOrder") != 1 || b["imageUrl"] != img {
		t.Fatalf("insignia creada: %v", b)
	}
	out = e.must(201, "POST", "/api/admin/badges", map[string]any{"name": "Calculadora Científica", "hiddenCourseIds": []int{1}})
	if b := out["badge"].(map[string]any); catIDs(b["hiddenCourseIds"]) != "[1 2]" || e.idOf(b, "sortOrder") != 2 {
		t.Fatalf("palabras clave + cursos elegidos: %v", b)
	}

	bad := []map[string]any{
		{"name": ""},
		{"name": "¡¿?!"},
		{"name": "Nueva", "stock": -1},
		{"name": "Nueva", "cost": -3},
		{"name": "Nueva", "hiddenCourseIds": []int{99}},
		{"name": "Nueva", "imageUrl": "/media/u/2026/11/falta.png"},
		{"name": "Nueva", "imageUrl": "javascript:alert(1)"},
	}
	for _, body := range bad {
		if code, res := e.call("POST", "/api/admin/badges", body); code != 400 {
			t.Errorf("%v: esperaba 400, llegó %d %v", body, code, res)
		}
	}
	if code, res := e.call("POST", "/api/admin/badges", map[string]any{"name": "internet gratis"}); code != 409 ||
		res["error"] != "Ya existe una insignia con ese nombre." {
		t.Fatalf("slug repetido: %d %v", code, res)
	}

	path := fmt.Sprintf("/api/admin/badges/%d", id)
	out = e.must(200, "PATCH", path, map[string]any{"name": "Internet", "hiddenCourseIds": []int{}, "stock": 2})
	b = out["badge"].(map[string]any)
	if b["slug"] != "internet" || catIDs(b["hiddenCourseIds"]) != "[]" || e.idOf(b, "stock") != 2 || e.idOf(b, "cost") != 5 {
		t.Fatalf("insignia editada: %v", b)
	}
	editID := e.idOf(out, "auditId")
	e.must(400, "PATCH", path, map[string]any{})
	e.must(400, "PATCH", path, map[string]any{"cost": -1})
	e.must(400, "PATCH", path, map[string]any{"hiddenCourseIds": []int{77}})
	e.must(404, "PATCH", "/api/admin/badges/9999", map[string]any{"cost": 1})

	// Deshacer la edición también regresa la visibilidad.
	e.undo(editID, nil)
	if e.str(`select slug || '|' || stock from badges where id = $1`, id) != "internet_gratis|10" ||
		e.num(`select count(*) from course_hidden_badges where badge_id = $1 and course_id = 2`, id) != 1 {
		t.Fatal("deshacer edición de insignia")
	}

	// Compras: se cuentan las no revertidas.
	sid := e.newStudent("G1", 0)
	e.exec(`insert into purchases (student_id, badge_id, badge_name, quantity) values ($1, $2, 'Internet', 3)`, sid, id)
	e.exec(`insert into purchases (student_id, badge_id, badge_name, quantity, reverted_at) values ($1, $2, 'Internet', 4, now())`, sid, id)
	list := e.must(200, "GET", "/api/admin/badges", nil)["badges"].([]any)
	if len(list) != 2 || e.idOf(list[0].(map[string]any), "purchasedCount") != 3 {
		t.Fatalf("lista de insignias: %v", list)
	}

	// Borrar: las compras se quedan sin enlace y deshacer las vuelve a ligar.
	out = e.must(200, "DELETE", path, nil)
	if e.num(`select count(*) from purchases where student_id = $1 and badge_id is null`, sid) != 2 {
		t.Fatal("las compras deben quedarse sin insignia")
	}
	e.undo(e.idOf(out, "auditId"), nil)
	if e.num(`select count(*) from purchases where badge_id = $1`, id) != 2 ||
		e.num(`select count(*) from course_hidden_badges where badge_id = $1`, id) != 1 {
		t.Fatal("deshacer el borrado debe ligar compras y visibilidad")
	}
	e.must(404, "DELETE", "/api/admin/badges/9999", nil)
}

func TestCatReorder(t *testing.T) {
	e := undoSetup(t)
	out := e.must(200, "POST", "/api/admin/courses/reorder", map[string]any{"ids": []int{3, 1, 2, 4, 5}})
	batch, _ := out["batchId"].(string)
	order := func() string {
		return e.str(`select string_agg(id::text, ',' order by sort_order, id) from courses`)
	}
	if order() != "3,1,2,4,5" || e.num(`select count(*) from audit_log where batch_id = $1::uuid`, batch) != 3 {
		t.Fatalf("orden %s", order())
	}
	e.must(200, "POST", "/api/admin/audit/batch/"+batch+"/undo", nil)
	if order() != "1,2,3,4,5" {
		t.Fatalf("deshacer orden: %s", order())
	}
	e.must(400, "POST", "/api/admin/courses/reorder", map[string]any{"ids": []int{}})
	e.must(400, "POST", "/api/admin/courses/reorder", map[string]any{"ids": []int{1, 1}})
	e.must(400, "POST", "/api/admin/courses/reorder", map[string]any{"ids": []int{1, 99}})

	b1, b2 := e.newBadge("A", 1, 1), e.newBadge("B", 1, 1)
	e.must(200, "POST", "/api/admin/badges/reorder", map[string]any{"ids": []int64{b2, b1}})
	if e.str(`select string_agg(id::text, ',' order by sort_order, id) from badges`) != fmt.Sprintf("%d,%d", b2, b1) {
		t.Fatal("orden de insignias")
	}
}

func TestCatLevels(t *testing.T) {
	e := undoSetup(t)
	levels := e.must(200, "GET", "/api/admin/levels", nil)["levels"].([]any)
	if len(levels) != 6 || fmt.Sprint(levels[5]) != "map[level:5 minExp:100]" {
		t.Fatalf("niveles semilla: %v", levels)
	}
	lv := func(pairs ...int) map[string]any {
		var out []map[string]int
		for i := 0; i < len(pairs); i += 2 {
			out = append(out, map[string]int{"level": pairs[i], "minExp": pairs[i+1]})
		}
		return map[string]any{"levels": out}
	}
	var many []int
	for i := range 21 {
		many = append(many, i, i*10)
	}
	for name, body := range map[string]map[string]any{
		"vacío":        {"levels": []any{}},
		"21 niveles":   lv(many...),
		"hueco":        lv(0, 0, 2, 10),
		"repetido":     lv(0, 0, 1, 10, 1, 20),
		"no empieza 0": lv(0, 5, 1, 10),
		"no crece":     lv(0, 0, 1, 10, 2, 10),
		"sin nivel 0":  lv(1, 0, 2, 10),
	} {
		if code, out := e.call("PUT", "/api/admin/levels", body); code != 400 {
			t.Errorf("%s: esperaba 400, llegó %d %v", name, code, out)
		}
	}
	out := e.must(200, "PUT", "/api/admin/levels", lv(2, 40, 0, 0, 1, 15))
	if fmt.Sprint(out["levels"]) != "[map[level:0 minExp:0] map[level:1 minExp:15] map[level:2 minExp:40]]" {
		t.Fatalf("niveles guardados: %v", out["levels"])
	}
	if got, _ := e.a.Levels(e.ctx); CalcLevel(20, got) != 1 || len(got) != 3 {
		t.Fatalf("el caché de niveles no se invalidó: %v", got)
	}
	var snap []any
	if err := e.a.DB.QueryRow(e.ctx, `select before from audit_log where id = $1 and entity = 'level'`, e.idOf(out, "auditId")).Scan(&snap); err != nil {
		t.Fatal(err)
	}
	if len(snap) != 6 || fmt.Sprint(snap[1]) != "map[level:1 min_exp:10]" {
		t.Fatalf("instantánea de niveles: %v", snap)
	}
}

func TestCatAvatars(t *testing.T) {
	e := undoSetup(t)
	e.exec("delete from avatars") // el CRUD se comprueba desde un catálogo vacío
	e.newStudent("0241234", 0)
	up := catUpload(e, "u/2026/11/avatar.png")

	out := e.must(200, "PUT", "/api/admin/avatars", map[string]any{"code": "DefaultH", "level": 1, "url": "/media/legacy/avatars/DefaultH1.png"})
	if av := out["avatar"].(map[string]any); av["code"] != "DefaultH" || e.idOf(av, "level") != 1 {
		t.Fatalf("avatar: %v", av)
	}
	created := e.idOf(out, "auditId")
	out = e.must(200, "PUT", "/api/admin/avatars", map[string]any{"code": "DefaultH", "level": 1, "url": up})
	updated := e.idOf(out, "auditId")
	if e.str(`select string_agg(action || ':' || entity_id, ',' order by id) from audit_log where id in ($1, $2)`, created, updated) !=
		"create:DefaultH:1,update:DefaultH:1" {
		t.Fatal("la primera vez es alta y la segunda cambio")
	}
	e.must(200, "PUT", "/api/admin/avatars", map[string]any{"code": " 0241234 ", "level": 0, "url": up})

	bad := []map[string]any{
		{"code": "", "level": 1, "url": up},
		{"code": "DefaultH", "url": up},
		{"code": "9999999", "level": 1, "url": up},
		{"code": "DefaultF", "level": 99, "url": up},
		{"code": "DefaultF", "level": 1, "url": ""},
		{"code": "DefaultF", "level": 1, "url": "/media/u/2026/11/falta.png"},
		{"code": "DefaultF", "level": 1, "url": "/static/a.png"},
	}
	for _, body := range bad {
		if code, res := e.call("PUT", "/api/admin/avatars", body); code != 400 {
			t.Errorf("%v: esperaba 400, llegó %d %v", body, code, res)
		}
	}

	if list := e.must(200, "GET", "/api/admin/avatars?code=DefaultH", nil)["avatars"].([]any); len(list) != 1 {
		t.Fatalf("avatares de DefaultH: %v", list)
	}
	if list := e.must(200, "GET", "/api/admin/avatars", nil)["avatars"].([]any); len(list) != 2 {
		t.Fatalf("todos los avatares: %v", list)
	}

	// Deshacer el cambio regresa la url anterior.
	e.undo(updated, nil)
	if e.str(`select url from avatars where code = 'DefaultH' and level = 1`) != "/media/legacy/avatars/DefaultH1.png" {
		t.Fatal("deshacer cambio de avatar")
	}

	out = e.must(200, "DELETE", "/api/admin/avatars?code=DefaultH&level=1", nil)
	e.must(404, "DELETE", "/api/admin/avatars?code=DefaultH&level=1", nil)
	e.must(400, "DELETE", "/api/admin/avatars?code=DefaultH", nil)
	e.undo(e.idOf(out, "auditId"), nil)
	if e.num(`select count(*) from avatars where code = 'DefaultH' and level = 1`) != 1 {
		t.Fatal("deshacer el borrado del avatar")
	}
}

func TestCatStats(t *testing.T) {
	e := undoSetup(t)
	s1 := e.newStudent("H1", 10)
	e.newStudent("H2", 5)
	e.exec(`update students set exp = 30 where id = $1`, s1)
	off := e.newStudent("H3", 100)
	e.exec(`update students set active = false where id = $1`, off)
	low := e.newBadge("Poquitas", 2, 1)
	e.newBadge("Muchas", 50, 1)
	gone := e.newBadge("Apagada", 0, 1)
	e.exec(`update badges set active = false where id = $1`, gone)
	e.exec(`insert into purchases (student_id, badge_id, badge_name, quantity) values ($1, $2, 'Poquitas', 1)`, s1, low)
	e.exec(`insert into purchases (student_id, badge_id, badge_name, quantity, created_at) values ($1, $2, 'Poquitas', 1, now() - interval '30 days')`, s1, low)
	e.exec(`insert into purchases (student_id, badge_id, badge_name, quantity, reverted_at) values ($1, $2, 'Poquitas', 1, now())`, s1, low)
	for i := range 12 {
		e.audit(AuditEntry{Action: "update", Entity: "course", EntityID: "1", Summary: fmt.Sprintf("cambio %d", i),
			After: map[string]any{"password_hash": "x"}})
	}

	out := e.must(200, "GET", "/api/admin/stats", nil)
	if fmt.Sprint(out["students"]) != "map[active:2 total:3]" || fmt.Sprint(out["courses"]) != "map[active:3 total:5]" ||
		e.idOf(out, "coinsInCirculation") != 15 || fmt.Sprint(out["purchases"]) != "map[last7Days:1 total:2]" {
		t.Fatalf("totales: %v", out)
	}
	badges := out["badges"].(map[string]any)
	lowStock := badges["lowStock"].([]any)
	if e.idOf(badges, "total") != 3 || e.idOf(badges, "active") != 2 || len(lowStock) != 1 ||
		e.idOf(lowStock[0].(map[string]any), "id") != low {
		t.Fatalf("insignias: %v", badges)
	}
	per := out["perCourse"].([]any)
	first := per[0].(map[string]any)
	if len(per) != 5 || e.idOf(first, "students") != 2 || first["avgLevel"] != 1.0 || e.idOf(first, "totalExp") != 30 ||
		e.idOf(per[1].(map[string]any), "students") != 0 {
		t.Fatalf("por curso: %v", per)
	}
	recent := out["recentActivity"].([]any)
	if len(recent) != 10 || recent[0].(map[string]any)["summary"] != "cambio 11" {
		t.Fatalf("actividad reciente: %v", recent)
	}
	if strings.Contains(fmt.Sprint(recent), "password_hash") {
		t.Fatal("el resumen no debe enseñar hashes")
	}
}
