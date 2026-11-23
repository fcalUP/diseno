package app

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestAdmPurchasesRevertRestore(t *testing.T) {
	e := admSetup(t)
	tar := e.courseID("TAR")
	luz := e.student(tar, "0600", "Luz", 10, 0)
	mar := e.student(tar, "0601", "Mar", 0, 0)
	b := e.badge("Kali Call", 3, 4)
	pid := e.purchase(luz, b, 2, 4, false) // 8 monedas
	other := e.purchase(mar, b, 1, 4, false)

	state := func(studentID int64) (coins, stock int) {
		t.Helper()
		e.one(&coins, `select monedas from students where id = $1`, studentID)
		e.one(&stock, `select coalesce((select stock from badges where id = $1), -1)`, b)
		return
	}
	type purResp struct {
		Purchase adminPurchaseJSON
		AuditID  int64 `json:"auditId"`
	}
	revert := fmt.Sprintf("/api/admin/purchases/%d/revert", pid)
	restore := fmt.Sprintf("/api/admin/purchases/%d/restore", pid)

	got := admReq[purResp](e, "POST", revert, map[string]any{}, http.StatusOK)
	if got.Purchase.RevertedAt == nil || got.Purchase.StudentCode != "0600" || got.Purchase.TotalCost != 8 ||
		got.Purchase.CourseID != tar || got.Purchase.BadgeName != "Kali Call" {
		t.Fatalf("compra revertida: %+v", got.Purchase)
	}
	if coins, stock := state(luz); coins != 18 || stock != 5 {
		t.Fatalf("tras revertir: monedas=%d stock=%d", coins, stock)
	}
	a := e.audit(got.AuditID)
	if a.Action != "revert" || a.Entity != "purchase" || a.EntityID != fmt.Sprint(pid) || !a.Undoable ||
		a.Before["student_coins"] != 10.0 || a.Before["badge_stock"] != 3.0 || a.Before["reverted_at"] != nil ||
		a.After["student_coins"] != 18.0 || a.After["badge_stock"] != 5.0 || a.After["reverted_at"] == nil ||
		a.After["quantity"] != 2.0 || a.After["id"] != float64(pid) {
		t.Fatalf("instantánea de la compra: %+v", a)
	}
	e.fails("POST", revert, map[string]any{}, http.StatusConflict)

	// Restaurar vuelve a cobrar; si ya no alcanza, 400.
	e.exec(`update students set monedas = 5 where id = $1`, luz)
	if msg := e.fails("POST", restore, map[string]any{}, http.StatusBadRequest); msg != "Monedas insuficientes" {
		t.Fatalf("sin monedas: %q", msg)
	}
	e.exec(`update students set monedas = 20 where id = $1`, luz)
	e.exec(`update badges set stock = 1 where id = $1`, b)
	if msg := e.fails("POST", restore, map[string]any{}, http.StatusBadRequest); msg != "Sólo quedan 1 unidades de Kali Call" {
		t.Fatalf("sin stock: %q", msg)
	}
	e.exec(`update badges set stock = 5 where id = $1`, b)
	got = admReq[purResp](e, "POST", restore, map[string]any{}, http.StatusOK)
	if got.Purchase.RevertedAt != nil {
		t.Fatal("la compra sigue revertida")
	}
	if coins, stock := state(luz); coins != 12 || stock != 3 {
		t.Fatalf("tras restaurar: monedas=%d stock=%d", coins, stock)
	}
	if a := e.audit(got.AuditID); a.Action != "restore" || a.Before["student_coins"] != 20.0 || a.After["student_coins"] != 12.0 {
		t.Fatalf("auditoría de restaurar: %+v", a)
	}
	e.fails("POST", restore, map[string]any{}, http.StatusConflict)
	e.fails("POST", "/api/admin/purchases/99999/revert", map[string]any{}, http.StatusNotFound)

	type listResp struct{ Purchases []adminPurchaseJSON }
	all := admReq[listResp](e, "GET", "/api/admin/purchases", nil, http.StatusOK)
	if len(all.Purchases) != 2 || all.Purchases[0].ID != other {
		t.Fatalf("lista (la más nueva primero): %+v", all.Purchases)
	}
	cases := map[string]int{
		fmt.Sprint("?studentId=", luz): 1,
		fmt.Sprint("?courseId=", tar):  2,
		fmt.Sprint("?badgeId=", b):     2,
		"?q=kali":                      2,
		"?q=0601":                      1,
		"?reverted=true":               0,
		"?reverted=false":              2,
		"?reverted=all&limit=1":        1,
	}
	for q, want := range cases {
		if got := admReq[listResp](e, "GET", "/api/admin/purchases"+q, nil, http.StatusOK); len(got.Purchases) != want {
			t.Errorf("%s: %d compras, esperaba %d", q, len(got.Purchases), want)
		}
	}
	for _, q := range []string{"?limit=0", "?limit=abc", "?reverted=x", "?studentId=abc"} {
		e.fails("GET", "/api/admin/purchases"+q, nil, http.StatusBadRequest)
	}

	// Si la insignia ya no existe se devuelven las monedas y no hay stock que tocar.
	e.exec(`delete from badges where id = $1`, b)
	got = admReq[purResp](e, "POST", fmt.Sprintf("/api/admin/purchases/%d/revert", other), map[string]any{}, http.StatusOK)
	if got.Purchase.BadgeID != nil {
		t.Fatalf("badge_id debió quedar en null: %+v", got.Purchase)
	}
	if coins, _ := state(mar); coins != 4 {
		t.Fatalf("monedas devueltas sin insignia: %d", coins)
	}
	if a := e.audit(got.AuditID); a.After["badge_stock"] != nil || a.After["student_coins"] != 4.0 {
		t.Fatalf("instantánea sin insignia: %+v", a)
	}
}

func TestAdmSettings(t *testing.T) {
	e := admSetup(t)
	type setResp struct {
		Settings map[string]any
		AuditID  *int64 `json:"auditId"`
	}
	patch := func(body map[string]any) setResp {
		t.Helper()
		return admReq[setResp](e, "PATCH", "/api/admin/settings", body, http.StatusOK)
	}

	got := admReq[setResp](e, "GET", "/api/admin/settings", nil, http.StatusOK)
	s := got.Settings
	if s["storeEnabled"] != true || s["resetCodeTtlMinutes"] != 10.0 || fmt.Sprint(s["emailCc"]) != "[fcal@up.edu.mx]" ||
		s["passwordPattern"] != "^[A-Z0-9]{4}$" {
		t.Fatalf("predeterminados: %v", s)
	}
	admSameJSON(t, s["resendApiKey"], `{"configured": false, "source": "none"}`)
	admSameJSON(t, s["googleServiceAccount"], `{"configured": false, "source": "none"}`)
	if len(s) != 18 {
		t.Fatalf("AdminSettings tiene 18 campos, llegaron %d: %v", len(s), s)
	}

	for _, bad := range []map[string]any{
		{"resetCodeTtlMinutes": 0}, {"resetCodeTtlMinutes": 121}, {"resetCodeTtlMinutes": 1.5},
		{"emailCc": []string{"no-es-correo"}}, {"emailCc": []string{""}}, {"emailFrom": ""}, {"emailFrom": "nadie"},
		{"studentEmailDomain": "no valido"}, {"studentEmailDomain": "a@up.edu.mx"},
		{"passwordPattern": "("}, {"passwordPattern": "(?i)^[a-z]{4}$"}, {"passwordPattern": `^\p{Lu}{4}$`},
		{"passwordPattern": `^[[:upper:]]{4}\z`}, {"passwordPattern": ""}, {"passwordHintRegister": "  "},
		{"levelUpGifUrl": "https://otro.sitio/x.gif"}, {"levelUpGifUrl": "/media/../x.gif"},
		{"levelUpGifUrl": "/media/u/2026/01/no-existe.gif"}, {"spreadsheetId": "con espacios"},
		{"storeEnabled": "sí"}, {"noExiste": true}, {"resendApiKey": 5}, {"resendApiKey": "con espacio"},
		{"googleServiceAccount": "{}"}, {"googleServiceAccount": "no es json"},
		{"storeEnabled": false, "resetCodeTtlMinutes": 0}, // uno malo tumba todo el parche
	} {
		e.fails("PATCH", "/api/admin/settings", bad, http.StatusBadRequest)
	}
	if n := e.count(`select count(*) from settings`); n != 0 {
		t.Fatalf("un parche inválido guardó %d ajustes", n)
	}

	got = patch(map[string]any{
		"storeEnabled": false, "resetCodeTtlMinutes": 15, "emailCc": []string{" a@up.edu.mx ", "b@up.edu.mx"},
		"studentEmailDomain": "UP.edu.mx",                      // queda igual tras normalizar
		"levelUpGifUrl":      "/media/legacy/gifs/levelup.gif", // igual al predeterminado
		"passwordPattern":    "^(?:[A-Z]{2}[0-9]{2})$",
		"emailFrom":          "Portal UP <portal@up.edu.mx>",
	})
	if got.AuditID == nil || got.Settings["storeEnabled"] != false || got.Settings["resetCodeTtlMinutes"] != 15.0 ||
		fmt.Sprint(got.Settings["emailCc"]) != "[a@up.edu.mx b@up.edu.mx]" {
		t.Fatalf("tras guardar: %+v", got)
	}
	a := e.audit(*got.AuditID)
	if a.Action != "update" || a.Entity != "setting" || !a.Undoable || a.EntityID != "" {
		t.Fatalf("auditoría: %+v", a)
	}
	admSameJSON(t, a.Before, `{"store_enabled": true, "reset_code_ttl_minutes": 10, "email_cc": ["fcal@up.edu.mx"],
		"password_pattern": "^[A-Z0-9]{4}$", "email_from": "onboarding@resend.dev"}`)
	admSameJSON(t, a.After, `{"store_enabled": false, "reset_code_ttl_minutes": 15, "email_cc": ["a@up.edu.mx", "b@up.edu.mx"],
		"password_pattern": "^(?:[A-Z]{2}[0-9]{2})$", "email_from": "Portal UP <portal@up.edu.mx>"}`)

	// El mismo parche otra vez no cambia nada: no hay entrada que deshacer.
	if again := patch(map[string]any{"storeEnabled": false, "resetCodeTtlMinutes": 15}); again.AuditID != nil {
		t.Fatalf("parche sin cambios dejó auditoría %d", *again.AuditID)
	}

	// Un secreto se guarda cifrado, nunca sale en claro y solo él no se puede deshacer.
	rec := e.do("PATCH", "/api/admin/settings", map[string]any{"resendApiKey": " re_secreto_123 "}, e.cookie)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "re_secreto_123") {
		t.Fatalf("guardar secreto: %d %s", rec.Code, rec.Body)
	}
	var stored string
	e.one(&stored, `select value #>> '{}' from settings where key = 'resend_api_key'`)
	if !strings.HasPrefix(stored, "enc:v1:") || strings.Contains(stored, "re_secreto") {
		t.Fatalf("el secreto no quedó cifrado: %q", stored)
	}
	got = admReq[setResp](e, "GET", "/api/admin/settings", nil, http.StatusOK)
	admSameJSON(t, got.Settings["resendApiKey"], `{"configured": true, "source": "db"}`)
	var secretAudit int64
	e.one(&secretAudit, `select max(id) from audit_log where entity = 'setting'`)
	a = e.audit(secretAudit)
	if a.Undoable {
		t.Fatal("cambiar solo un secreto no debe poder deshacerse")
	}
	admSameJSON(t, a.Before, `{"resend_api_key": {"configured": false}}`)
	admSameJSON(t, a.After, `{"resend_api_key": {"configured": true}}`)

	// Mezclado con un ajuste normal sí se puede deshacer; el correo de la cuenta se lee del JSON.
	sa := `{"type":"service_account","client_email":"bot@proyecto.iam.gserviceaccount.com","private_key":"-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n"}`
	got = patch(map[string]any{"googleServiceAccount": sa, "leaderboardEnabled": false})
	admSameJSON(t, got.Settings["googleServiceAccount"],
		`{"configured": true, "source": "db", "clientEmail": "bot@proyecto.iam.gserviceaccount.com"}`)
	a = e.audit(*got.AuditID)
	if !a.Undoable {
		t.Fatal("un parche con ajustes normales debe poder deshacerse")
	}
	admSameJSON(t, a.After, `{"google_service_account": {"configured": true}, "leaderboard_enabled": false}`)

	// null borra: el secreto y el ajuste vuelven a no estar guardados.
	got = patch(map[string]any{"resendApiKey": nil, "storeEnabled": nil})
	admSameJSON(t, got.Settings["resendApiKey"], `{"configured": false, "source": "none"}`)
	if got.Settings["storeEnabled"] != true {
		t.Fatal("storeEnabled debió volver al predeterminado")
	}
	if n := e.count(`select count(*) from settings where key in ('resend_api_key', 'store_enabled')`); n != 0 {
		t.Fatalf("quedaron %d valores que debieron borrarse", n)
	}
	admSameJSON(t, e.audit(*got.AuditID).After, `{"resend_api_key": {"configured": false}, "store_enabled": true}`)

	if n := e.count(`select count(*) from audit_log where coalesce(before::text, '') || coalesce(after::text, '')
		like any (array['%re_secreto%', '%PRIVATE KEY%'])`); n != 0 {
		t.Fatalf("la bitácora guardó %d secretos en claro", n)
	}
}

func TestAdmAdmins(t *testing.T) {
	e := admSetup(t)
	type adminResp struct {
		Admin   adminAccountJSON
		AuditID int64 `json:"auditId"`
	}
	list := admReq[struct{ Admins []adminAccountJSON }](e, "GET", "/api/admin/admins", nil, http.StatusOK)
	if len(list.Admins) != 1 || list.Admins[0].Username != "root" || !list.Admins[0].Active {
		t.Fatalf("lista: %+v", list)
	}

	e.fails("POST", "/api/admin/admins", map[string]any{"username": "Profe", "password": "corta"}, http.StatusBadRequest)
	e.fails("POST", "/api/admin/admins", map[string]any{"username": "  ", "password": "clave-larga-1"}, http.StatusBadRequest)
	created := admReq[adminResp](e, "POST", "/api/admin/admins",
		map[string]any{"username": " Profe ", "password": "clave-larga-1"}, http.StatusCreated)
	if created.Admin.Username != "Profe" || !created.Admin.Active {
		t.Fatalf("creado: %+v", created)
	}
	profe := created.Admin.ID
	if a := e.audit(created.AuditID); a.Action != "create" || a.Entity != "admin" || a.After["username"] != "Profe" || !a.Undoable {
		t.Fatalf("auditoría de crear: %+v", a)
	}
	if msg := e.fails("POST", "/api/admin/admins",
		map[string]any{"username": "profe", "password": "clave-larga-2"}, http.StatusConflict); msg != "Ese usuario ya existe." {
		t.Fatalf("usuario repetido: %q", msg)
	}

	rec := e.do("POST", "/api/admin-login", map[string]string{"username": "Profe", "password": "clave-larga-1"}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("el admin nuevo no puede entrar: %d", rec.Code)
	}
	profeCookie := rec.Result().Cookies()[0]

	self := fmt.Sprintf("/api/admin/admins/%d", e.id)
	other := fmt.Sprintf("/api/admin/admins/%d", profe)
	if msg := e.fails("PATCH", self, map[string]any{"active": false}, http.StatusBadRequest); msg != "No puedes desactivar tu propia cuenta." {
		t.Fatalf("desactivarse: %q", msg)
	}
	if msg := e.fails("DELETE", self, nil, http.StatusBadRequest); msg != "No puedes eliminar tu propia cuenta." {
		t.Fatalf("borrarse: %q", msg)
	}
	// Cambiar la propia contraseña cierra las otras sesiones; esta sigue con una cookie nueva (M2).
	oldCookie := e.cookie
	rec = e.do("PATCH", self, map[string]any{"password": "clave-nueva-123"}, oldCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("cambiar la propia contraseña: %d %s", rec.Code, rec.Body)
	}
	e.cookie = nil
	for _, c := range rec.Result().Cookies() {
		if c.Name == "diseno_admin" && c.Value != "" {
			e.cookie = c
		}
	}
	if e.cookie == nil {
		t.Fatal("cambiar la propia contraseña no renovó la cookie")
	}
	if rec := e.do("GET", "/api/admin/me", nil, oldCookie); rec.Code != http.StatusUnauthorized {
		t.Fatalf("la cookie de antes del cambio: %d", rec.Code)
	}
	if rec := e.do("GET", "/api/admin/me", nil, e.cookie); rec.Code != http.StatusOK {
		t.Fatalf("la cookie renovada: %d", rec.Code)
	}

	// Al desactivar a otro admin su sesión deja de servir.
	if got := admReq[adminResp](e, "PATCH", other, map[string]any{"active": false}, http.StatusOK); got.Admin.Active {
		t.Fatalf("sigue activo: %+v", got)
	}
	if rec := e.do("GET", "/api/admin/me", nil, profeCookie); rec.Code != http.StatusUnauthorized {
		t.Fatalf("la sesión del desactivado: %d", rec.Code)
	}
	e.fails("PATCH", other, map[string]any{"username": "ROOT"}, http.StatusConflict)
	upd := admReq[adminResp](e, "PATCH", other,
		map[string]any{"username": "Profe2", "password": "otra-clave-9", "active": true}, http.StatusOK)
	var hash string
	e.one(&hash, `select password_hash from admins where id = $1`, profe)
	if upd.Admin.Username != "Profe2" || !upd.Admin.Active || !checkPassword(hash, "otra-clave-9") {
		t.Fatalf("edición: %+v", upd)
	}
	if a := e.audit(upd.AuditID); a.Action != "update" || a.Before["username"] != "Profe" || a.After["username"] != "Profe2" {
		t.Fatalf("auditoría de editar: %+v", a)
	}
	e.fails("PATCH", other, map[string]any{}, http.StatusBadRequest)
	e.fails("PATCH", other, map[string]any{"password": "corta"}, http.StatusBadRequest)
	e.fails("PATCH", "/api/admin/admins/99999", map[string]any{"active": true}, http.StatusNotFound)

	del := e.call("DELETE", other, nil, http.StatusOK)
	if a := e.audit(int64(del["auditId"].(float64))); a.Action != "delete" || a.Before["username"] != "Profe2" || a.After != nil {
		t.Fatalf("auditoría de borrar: %+v", a)
	}
	e.fails("DELETE", other, nil, http.StatusNotFound)

	// Un admin inactivo se puede borrar aunque solo quede uno activo.
	viejo, _ := e.newAdmin("viejo", "clave-vieja-1")
	e.exec(`update admins set active = false where id = $1`, viejo)
	e.call("DELETE", fmt.Sprintf("/api/admin/admins/%d", viejo), nil, http.StatusOK)

	// La regla del último activo, por si alguna vez se llega sin pasar por la de uno mismo.
	if err := admCanRemove(2, 1, 1, "desactivar"); err == nil || err.Error() != "Debe quedar al menos un administrador activo." {
		t.Fatalf("último activo: %v", err)
	}
	if err := admCanRemove(2, 1, 2, "desactivar"); err != nil {
		t.Fatalf("con dos activos sí se puede: %v", err)
	}
}

func TestAdmUpload(t *testing.T) {
	e := admSetup(t)
	type ticket struct {
		UploadURL string `json:"uploadUrl"`
		Method    string
		Headers   map[string]string
		URL       string `json:"url"`
	}
	tk := admReq[ticket](e, "POST", "/api/admin/uploads",
		map[string]any{"filename": "foto.PNG", "contentType": "image/png", "size": 4}, http.StatusOK)
	now := time.Now().UTC()
	urlRe := regexp.MustCompile(fmt.Sprintf(`^/media/u/%04d/%02d/[0-9a-f]{16}\.png$`, now.Year(), int(now.Month())))
	if tk.Method != "PUT" || tk.Headers["Content-Type"] != "image/png" ||
		!strings.HasPrefix(tk.UploadURL, "/api/uploads/local?token=") || !urlRe.MatchString(tk.URL) {
		t.Fatalf("ticket: %+v", tk)
	}

	// El navegador sube el archivo con el ticket y la url queda lista para usarse.
	if rec := putUpload(e.h, tk.UploadURL, "image/png", "abcd"); rec.Code != http.StatusOK {
		t.Fatalf("subida: %d %s", rec.Code, rec.Body)
	}
	key, _ := mediaKey(tk.URL)
	if info, err := e.a.Store.Stat(e.ctx, key); err != nil || info.Size != 4 {
		t.Fatalf("Stat: %+v %v", info, err)
	}
	set := admReq[struct{ Settings map[string]any }](e, "PATCH", "/api/admin/settings",
		map[string]any{"levelUpGifUrl": tk.URL}, http.StatusOK)
	if set.Settings["levelUpGifUrl"] != tk.URL {
		t.Fatalf("GIF de subida de nivel: %v", set.Settings["levelUpGifUrl"])
	}

	if jpg := admReq[ticket](e, "POST", "/api/admin/uploads",
		map[string]any{"contentType": "image/jpeg", "size": 20 << 20}, http.StatusOK); !strings.HasSuffix(jpg.URL, ".jpg") {
		t.Fatalf("jpeg de 20 MB: %+v", jpg)
	}
	for _, body := range []map[string]any{
		{"contentType": "image/svg+xml", "size": 10},
		{"contentType": "text/html", "size": 10},
		{"contentType": "image/png", "size": 0},
		{"contentType": "image/png", "size": 20<<20 + 1},
		{},
	} {
		e.fails("POST", "/api/admin/uploads", body, http.StatusBadRequest)
	}
}

func TestAdmCheckPattern(t *testing.T) {
	for _, ok := range []string{`^[A-Z0-9]{4}$`, `^(?:[A-Z]{2}\d{2})$`, `^\(?[0-9]{4}\)?$`, `^\\p$`, `^[a-z]{6}$`} {
		if msg := admCheckPattern(ok); msg != "" {
			t.Errorf("%q debió aceptarse: %s", ok, msg)
		}
	}
	for _, bad := range []string{``, `(`, `(?i)^[a-z]{4}$`, `(?P<x>a)`, `^\pL{4}$`, `^a\z`, `\Qa\E`, `[[:alpha:]]`, strings.Repeat("a", 201)} {
		if admCheckPattern(bad) == "" {
			t.Errorf("%q debió rechazarse", bad)
		}
	}
}
