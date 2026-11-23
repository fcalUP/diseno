package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Regresión: Go manda null por un slice nil, pero types.ts promete arreglos y el
// panel hace .length o .map sin revisar (useUndo lee failed.length). Esta prueba
// recorre las respuestas con listas vacías y exige [] en cada llave del contrato.

var jsonArrayKeys = map[string]bool{
	"avatarLevels": true, "hiddenBadgeIds": true, "hiddenCourseIds": true, "sheetAliases": true,
	"hiddenBadgeKeywords": true, "emailCc": true, "created": true, "skipped": true, "failed": true,
	"warnings": true, "sheetsFound": true, "courses": true, "recentActivity": true, "lowStock": true,
	"perCourse": true, "entries": true, "students": true, "badges": true, "purchases": true,
	"admins": true, "levels": true, "leaderboard": true, "avatars": true, "fields": true,
}

// jsonNullArrays junta las rutas de las llaves de arreglo que llegaron en null.
func jsonNullArrays(v any, path string, out *[]string) {
	switch t := v.(type) {
	case []any:
		for i, x := range t {
			jsonNullArrays(x, fmt.Sprintf("%s[%d]", path, i), out)
		}
	case map[string]any:
		for k, x := range t {
			if k == "before" || k == "after" { // instantáneas de la bitácora: forma libre
				continue
			}
			if x == nil && jsonArrayKeys[k] {
				*out = append(*out, path+"."+k)
				continue
			}
			jsonNullArrays(x, path+"."+k, out)
		}
	}
}

func TestJSONArraysNeverNull(t *testing.T) {
	var probe []string
	jsonNullArrays(map[string]any{"x": []any{map[string]any{"failed": nil, "before": map[string]any{"fields": nil}}}}, "", &probe)
	if len(probe) != 1 || probe[0] != ".x[0].failed" {
		t.Fatalf("el recorrido no detecta bien los null: %v", probe)
	}

	e := admSetup(t)
	course := e.courseID("TAR")
	sid := e.student(course, "A001", "Sin Nada", 0, 0)
	rec := httptest.NewRecorder()
	e.a.setSession(rec, kindStudent, sid, "x") // e.student guarda el hash "x"
	stu := rec.Result().Cookies()[0]

	get := func(method, path string, body any, cookie *http.Cookie, want int) map[string]any {
		t.Helper()
		r := e.do(method, path, body, cookie)
		if r.Code != want {
			t.Fatalf("%s %s: esperaba %d, llegó %d %s", method, path, want, r.Code, r.Body)
		}
		var v map[string]any
		if err := json.Unmarshal(r.Body.Bytes(), &v); err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		var nulls []string
		jsonNullArrays(v, "", &nulls)
		if len(nulls) > 0 {
			t.Errorf("%s %s: arreglos en null: %v\n%s", method, path, nulls, r.Body)
		}
		return v
	}

	// Alumno sin compras, sin avatares y con la tienda vacía.
	get("GET", "/api/me", nil, stu, http.StatusOK)
	get("GET", "/api/badges", nil, stu, http.StatusOK)
	get("GET", "/api/leaderboard", nil, stu, http.StatusOK)

	// Panel con todo vacío o filtrado a nada.
	for _, p := range []string{"/api/admin/stats", "/api/admin/courses", "/api/admin/badges",
		"/api/admin/levels", "/api/admin/avatars", "/api/admin/avatars?code=nadie", "/api/admin/students",
		"/api/admin/students?q=nadie", "/api/admin/purchases", "/api/admin/settings", "/api/admin/admins",
		"/api/admin/audit?entity=nada", "/api/admin/import/status"} {
		get("GET", p, nil, e.cookie, http.StatusOK)
	}
	get("POST", "/api/admin/badges", map[string]any{"name": "Nada", "cost": 1, "stock": 1}, e.cookie, http.StatusCreated)
	get("PUT", fmt.Sprintf("/api/admin/courses/%d/hidden-badges", course), map[string]any{"badgeIds": []int{}}, e.cookie, http.StatusOK)
	get("PATCH", fmt.Sprintf("/api/admin/courses/%d", course),
		map[string]any{"sheetAliases": []string{}, "hiddenBadgeKeywords": []string{}}, e.cookie, http.StatusOK)
	get("PATCH", "/api/admin/settings", map[string]any{"emailCc": []string{}}, e.cookie, http.StatusOK)

	// Generar: solo repetidos (created vacío) y luego uno nuevo (skipped vacío).
	get("POST", "/api/admin/students/generate",
		map[string]any{"courseId": course, "rows": []map[string]string{{"code": "A001", "name": "Otra vez"}}}, e.cookie, http.StatusOK)
	gen := get("POST", "/api/admin/students/generate",
		map[string]any{"courseId": course, "rows": []map[string]string{{"code": "B001", "name": "Nuevo"}}}, e.cookie, http.StatusOK)
	// Deshacer el lote sin fallas: failed debe ser [].
	get("POST", fmt.Sprintf("/api/admin/audit/batch/%s/undo", gen["batchId"]), map[string]any{}, e.cookie, http.StatusOK)
	// Imágenes heredadas sin archivos: sin avisos.
	get("POST", "/api/admin/import/media", map[string]any{}, e.cookie, http.StatusOK)

	// Sin cursos visibles el selector del login queda vacío, no en null.
	e.exec(`update courses set active = false`)
	get("GET", "/api/config", nil, nil, http.StatusOK)
}
