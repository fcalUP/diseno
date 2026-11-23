package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"
	"net/mail"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// ---------- Compras ----------

// admPurchases lee compras con su alumno y curso, de la más nueva a la más vieja.
// where usa los alias p (purchases) y s (students).
func admPurchases(ctx context.Context, q DBTX, where string, limit int, args ...any) ([]adminPurchaseJSON, error) {
	args = append(args, limit)
	rows, err := q.Query(ctx, `select p.id, p.student_id, s.code, s.name, s.course_id, c.name, p.badge_id,
			p.badge_name, p.quantity, p.unit_cost, p.total_cost, p.coins_before, p.coins_after, p.source,
			p.created_at, p.reverted_at
		from purchases p join students s on s.id = p.student_id join courses c on c.id = s.course_id
		where `+where+`
		order by p.created_at desc, p.id desc
		limit $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[adminPurchaseJSON])
	if out == nil {
		out = []adminPurchaseJSON{}
	}
	return out, err
}

func (a *App) handleAdminPurchasesList(w http.ResponseWriter, r *http.Request, adminID int64) {
	q := r.URL.Query()
	var where admWhere
	for _, f := range []struct{ key, cond string }{
		{"courseId", "s.course_id = $?"},
		{"studentId", "p.student_id = $?"},
		{"badgeId", "p.badge_id = $?"},
	} {
		id, ok := admQueryID(r, f.key)
		if !ok {
			writeError(w, http.StatusBadRequest, "Filtro inválido: "+f.key+".")
			return
		}
		if id > 0 {
			where.add(f.cond, id)
		}
	}
	if term := strings.TrimSpace(q.Get("q")); term != "" {
		where.add(`(strpos(lower(s.code), lower($?)) > 0 or strpos(lower(s.name), lower($?)) > 0
			or strpos(lower(p.badge_name), lower($?)) > 0)`, term)
	}
	switch q.Get("reverted") {
	case "", "all":
	case "true":
		where.conds = append(where.conds, "p.reverted_at is not null")
	case "false":
		where.conds = append(where.conds, "p.reverted_at is null")
	default:
		writeError(w, http.StatusBadRequest, "Filtro de estado inválido (all, true o false).")
		return
	}
	limit := 500
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 5000 {
			writeError(w, http.StatusBadRequest, "El límite debe estar entre 1 y 5000.")
			return
		}
		limit = n
	}
	list, err := admPurchases(r.Context(), a.DB, where.sql(), limit, where.args...)
	if err != nil {
		a.admFail(w, "listar compras", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"purchases": list})
}

func (a *App) handleAdminPurchasesRevert(w http.ResponseWriter, r *http.Request, adminID int64) {
	a.admTogglePurchase(w, r, adminID, true)
}

func (a *App) handleAdminPurchasesRestore(w http.ResponseWriter, r *http.Request, adminID int64) {
	a.admTogglePurchase(w, r, adminID, false)
}

// admPurchaseSnapshot es la instantánea de §5.6: la compra más las monedas del
// alumno y el stock de la insignia en ese momento (null si ya no existe).
func admPurchaseSnapshot(ctx context.Context, tx pgx.Tx, id int64, coins int, stock *int) (map[string]any, error) {
	var snap map[string]any
	err := tx.QueryRow(ctx, `select to_jsonb(p) || jsonb_build_object('student_coins', $2::int, 'badge_stock', $3::int)
		from purchases p where p.id = $1`, id, coins, stock).Scan(&snap)
	return snap, err
}

// admTogglePurchase revierte (devuelve monedas y stock) o restaura (vuelve a
// cobrar) una compra. Bloquea en el mismo orden que la compra del alumno:
// alumno, insignia y al final la compra, para no cruzarse en un deadlock.
func (a *App) admTogglePurchase(w http.ResponseWriter, r *http.Request, adminID int64, revert bool) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusNotFound, "Compra no encontrada.")
		return
	}
	ctx := r.Context()
	notFound := admBad(http.StatusNotFound, "Compra no encontrada.")
	var auditID int64
	ok = a.admTx(w, r, "revertir o restaurar compra", func(tx pgx.Tx) error {
		var studentID int64
		var badgeID *int64
		err := tx.QueryRow(ctx, `select student_id, badge_id from purchases where id = $1`, id).Scan(&studentID, &badgeID)
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound
		}
		if err != nil {
			return err
		}
		var coins int
		var code string
		err = tx.QueryRow(ctx, `select monedas, code from students where id = $1 for update`, studentID).Scan(&coins, &code)
		if errors.Is(err, pgx.ErrNoRows) { // el alumno (y con él la compra) se borró
			return notFound
		}
		if err != nil {
			return err
		}
		var stock *int
		if badgeID != nil {
			var n int
			err := tx.QueryRow(ctx, `select stock from badges where id = $1 for update`, *badgeID).Scan(&n)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if err == nil {
				stock = &n
			}
		}
		var p struct {
			badgeID    *int64
			reverted   bool
			qty, total int
			name       string
		}
		err = tx.QueryRow(ctx, `select badge_id, reverted_at is not null, quantity, total_cost, badge_name
			from purchases where id = $1 for update`, id).Scan(&p.badgeID, &p.reverted, &p.qty, &p.total, &p.name)
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound
		}
		if err != nil {
			return err
		}
		if p.badgeID == nil { // la insignia se borró entre la primera lectura y ahora
			stock = nil
		}
		if revert && p.reverted {
			return admBad(http.StatusConflict, "Esta compra ya estaba revertida.")
		}
		if !revert && !p.reverted {
			return admBad(http.StatusConflict, "Esta compra no está revertida.")
		}

		before, err := admPurchaseSnapshot(ctx, tx, id, coins, stock)
		if err != nil {
			return err
		}
		if revert {
			if int64(coins)+int64(p.total) > math.MaxInt32 || (stock != nil && int64(*stock)+int64(p.qty) > math.MaxInt32) {
				return admBad(http.StatusBadRequest, "Revertir la compra pasaría el máximo de monedas o de stock.")
			}
			coins += p.total
			if stock != nil {
				*stock += p.qty
			}
		} else {
			if coins < p.total {
				return admBad(http.StatusBadRequest, "Monedas insuficientes")
			}
			if stock != nil && *stock < p.qty {
				return admBad(http.StatusBadRequest, fmt.Sprintf("Sólo quedan %d unidades de %s", *stock, p.name))
			}
			coins -= p.total
			if stock != nil {
				*stock -= p.qty
			}
		}
		if _, err := tx.Exec(ctx, `update students set monedas = $1, updated_at = now() where id = $2`, coins, studentID); err != nil {
			return err
		}
		if stock != nil {
			if _, err := tx.Exec(ctx, `update badges set stock = $1, updated_at = now() where id = $2`, *stock, *badgeID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `update purchases set reverted_at = case when $2::boolean then now() end where id = $1`,
			id, revert); err != nil {
			return err
		}
		after, err := admPurchaseSnapshot(ctx, tx, id, coins, stock)
		if err != nil {
			return err
		}
		action, summary := "restore", fmt.Sprintf("Compra restaurada: %s, %d × %s (%d monedas)", code, p.qty, p.name, p.total)
		if revert {
			action, summary = "revert", fmt.Sprintf("Compra revertida: %s, %d × %s (se devolvieron %d monedas)", code, p.qty, p.name, p.total)
		}
		auditID, err = a.auditAdmin(ctx, tx, adminID, AuditEntry{
			Action:   action,
			Entity:   "purchase",
			EntityID: itoa64(id),
			Summary:  summary,
			Before:   before,
			After:    after,
			Undoable: true,
		})
		return err
	})
	if !ok {
		return
	}
	list, err := admPurchases(ctx, a.DB, "p.id = $1", 1, id)
	if err == nil && len(list) == 0 {
		err = pgx.ErrNoRows
	}
	if err != nil {
		a.admFail(w, "leer compra", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"purchase": list[0], "auditId": auditID})
}

// ---------- Ajustes ----------

// admSettingsJSON es AdminSettings: los ajustes normales (Settings ya trae las
// etiquetas JSON y oculta los secretos) más el estado de cada secreto.
type admSettingsJSON struct {
	Settings
	ResendStatus secretStatusJSON `json:"resendApiKey"`
	GoogleStatus secretStatusJSON `json:"googleServiceAccount"`
}

func admSettingsView(s Settings) admSettingsJSON {
	if s.EmailCc == nil {
		s.EmailCc = []string{}
	}
	v := admSettingsJSON{
		Settings:     s,
		ResendStatus: secretStatusJSON{Configured: s.ResendAPIKey != "", Source: s.ResendSource},
		GoogleStatus: secretStatusJSON{Configured: s.GoogleServiceAccount != "", Source: s.GoogleSource},
	}
	if v.GoogleStatus.Configured {
		var sa struct {
			ClientEmail string `json:"client_email"`
		}
		if json.Unmarshal([]byte(s.GoogleServiceAccount), &sa) == nil {
			v.GoogleStatus.ClientEmail = sa.ClientEmail
		}
	}
	return v
}

// admSettingValues da los ajustes no secretos por clave de la tabla, como valores
// JSON genéricos (bool, string, float64, []any) para compararlos y auditarlos.
func admSettingValues(s Settings) map[string]any {
	b, _ := json.Marshal(s)
	var byJSON map[string]any
	_ = json.Unmarshal(b, &byJSON)
	out := map[string]any{}
	for _, f := range settingsFields() {
		if !f.Secret {
			out[f.Key] = byJSON[f.JSON]
		}
	}
	return out
}

// admStoredSecret es el secreto guardado en la tabla ("" si no hay o viene del env).
func admStoredSecret(s Settings, key string) string {
	switch {
	case key == "resend_api_key" && s.ResendSource == "db":
		return s.ResendAPIKey
	case key == "google_service_account" && s.GoogleSource == "db":
		return s.GoogleServiceAccount
	}
	return ""
}

var (
	admDomainRe  = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
	admSheetIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{0,200}$`)
)

// admCheckPattern exige que la regla compile en Go (RE2) y que se quede en la
// sintaxis que el navegador entiende igual: nada de banderas (?i), \p{..},
// clases POSIX, \A, \z, \Q..\E ni \C.
func admCheckPattern(p string) string {
	if p == "" || len(p) > 200 {
		return "La regla de contraseña debe tener entre 1 y 200 caracteres."
	}
	if _, err := regexp.Compile(p); err != nil {
		return "La regla de contraseña no es una expresión regular válida."
	}
	for i := 0; i < len(p); i++ {
		switch {
		case p[i] == '\\' && i+1 < len(p):
			if strings.IndexByte("pPAzQEC", p[i+1]) >= 0 {
				return `La regla de contraseña usa \` + p[i+1:i+2] + ", que el navegador no entiende igual; usa sintaxis simple."
			}
			i++ // lo escapado no cuenta como sintaxis
		case strings.HasPrefix(p[i:], "(?") && !strings.HasPrefix(p[i:], "(?:"):
			return "La regla de contraseña no puede usar banderas ni grupos con nombre; usa sintaxis simple."
		case strings.HasPrefix(p[i:], "[[:"):
			return "La regla de contraseña no puede usar clases POSIX como [[:alpha:]]; usa sintaxis simple."
		}
	}
	return ""
}

// admCheckMediaURL acepta "" o una url /media/; si es una subida (/media/u/) revisa que exista.
func (a *App) admCheckMediaURL(ctx context.Context, u string) (string, error) {
	if u == "" {
		return "", nil
	}
	key, ok := mediaKey(u)
	if !ok {
		return "La imagen debe ser una url que empiece con /media/.", nil
	}
	if strings.HasPrefix(key, "u/") {
		if _, err := a.Store.Stat(ctx, key); errors.Is(err, ErrNotFound) {
			return "La imagen no existe en el almacenamiento; vuelve a subirla.", nil
		} else if err != nil {
			return "", err
		}
	}
	return "", nil
}

// admCheckSetting valida (y limpia) el campo key de s, ya decodificado. Devuelve
// el mensaje para el admin, o un error interno.
func (a *App) admCheckSetting(ctx context.Context, key string, s *Settings) (string, error) {
	def, _ := findSetting(key)
	if p, ok := def.field(s).(*string); ok && key != "password_pattern" {
		*p = strings.TrimSpace(*p)
	}
	switch key {
	case "email_from":
		if _, err := mail.ParseAddress(s.EmailFrom); err != nil || len(s.EmailFrom) > 200 {
			return "El remitente no es un correo válido.", nil
		}
	case "email_cc":
		if len(s.EmailCc) > 20 {
			return "Máximo 20 correos en copia.", nil
		}
		for i, e := range s.EmailCc {
			s.EmailCc[i] = strings.TrimSpace(e)
			if !admPlainEmail(s.EmailCc[i]) {
				return fmt.Sprintf("Correo en copia inválido: %q.", e), nil
			}
		}
	case "student_email_domain":
		s.StudentEmailDomain = strings.ToLower(s.StudentEmailDomain)
		if !admDomainRe.MatchString(s.StudentEmailDomain) {
			return "El dominio de correo no es válido (ejemplo: up.edu.mx).", nil
		}
	case "password_pattern":
		return admCheckPattern(s.PasswordPattern), nil
	case "password_hint_register", "password_hint_reset", "password_error":
		if v := *def.field(s).(*string); v == "" || utf8.RuneCountInString(v) > 300 {
			return "Los textos de la contraseña no pueden quedar vacíos (máximo 300 caracteres).", nil
		}
	case "reset_code_ttl_minutes":
		if s.ResetCodeTTLMinutes < 1 || s.ResetCodeTTLMinutes > 120 {
			return "La vigencia del código debe ser de 1 a 120 minutos.", nil
		}
	case "level_up_gif_url":
		return a.admCheckMediaURL(ctx, s.LevelUpGifURL)
	case "spreadsheet_id":
		if !admSheetIDRe.MatchString(s.SpreadsheetID) {
			return "El ID de la hoja de cálculo no es válido.", nil
		}
	}
	return "", nil
}

// admCheckSecret valida un secreto nuevo ("" = borrar).
func admCheckSecret(key, v string) string {
	if v == "" {
		return ""
	}
	if key == "google_service_account" {
		var sa struct {
			ClientEmail string `json:"client_email"`
			PrivateKey  string `json:"private_key"`
		}
		if json.Unmarshal([]byte(v), &sa) != nil || sa.ClientEmail == "" || sa.PrivateKey == "" {
			return "Las credenciales deben ser el JSON de una cuenta de servicio de Google (con client_email y private_key)."
		}
		return ""
	}
	if len(v) > 500 || strings.ContainsAny(v, " \t\r\n") {
		return "La API key de Resend no es válida."
	}
	return ""
}

func (a *App) handleAdminSettingsGet(w http.ResponseWriter, r *http.Request, adminID int64) {
	s, err := a.Settings(r.Context())
	if err != nil {
		a.admFail(w, "leer ajustes", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": admSettingsView(s)})
}

// handleAdminSettingsPatch guarda solo lo que cambió. En la bitácora los secretos
// quedan como {"configured": bool}; si solo cambiaron secretos no se puede deshacer.
func (a *App) handleAdminSettingsPatch(w http.ResponseWriter, r *http.Request, adminID int64) {
	var patch map[string]json.RawMessage
	if !decodeJSON(w, r, &patch) {
		return
	}
	ctx := r.Context()
	cur, err := a.Settings(ctx)
	if err != nil {
		a.admFail(w, "leer ajustes", err)
		return
	}
	current, defaults := admSettingValues(cur), admSettingValues(defaultSettings())
	byJSON := map[string]SettingField{}
	for _, f := range settingsFields() {
		byJSON[f.JSON] = f
	}

	save := map[string]any{}
	before, after := map[string]any{}, map[string]any{}
	onlySecrets := true
	for name, raw := range patch {
		f, ok := byJSON[name]
		if !ok {
			writeError(w, http.StatusBadRequest, "Ajuste desconocido: "+name+".")
			return
		}
		if f.Secret {
			var v *string
			if err := json.Unmarshal(raw, &v); err != nil {
				writeError(w, http.StatusBadRequest, "Valor inválido para "+name+".")
				return
			}
			val := ""
			if v != nil {
				val = strings.TrimSpace(*v)
			}
			if msg := admCheckSecret(f.Key, val); msg != "" {
				writeError(w, http.StatusBadRequest, msg)
				return
			}
			old := admStoredSecret(cur, f.Key)
			if val == old {
				continue
			}
			save[f.Key] = val // "" borra
			before[f.Key] = map[string]bool{"configured": old != ""}
			after[f.Key] = map[string]bool{"configured": val != ""}
			continue
		}

		var val, stored any // stored nil = borrar el valor guardado y volver al predeterminado
		if string(bytes.TrimSpace(raw)) == "null" {
			val = defaults[f.Key]
		} else {
			probe := defaultSettings()
			def, _ := findSetting(f.Key)
			if err := json.Unmarshal(raw, def.field(&probe)); err != nil {
				writeError(w, http.StatusBadRequest, "Valor inválido para "+name+".")
				return
			}
			msg, err := a.admCheckSetting(ctx, f.Key, &probe)
			if err != nil {
				a.admFail(w, "validar ajustes", err)
				return
			}
			if msg != "" {
				writeError(w, http.StatusBadRequest, msg)
				return
			}
			val = admSettingValues(probe)[f.Key]
			stored = val
		}
		if reflect.DeepEqual(val, current[f.Key]) {
			continue
		}
		save[f.Key] = stored
		before[f.Key] = current[f.Key]
		after[f.Key] = val
		onlySecrets = false
	}
	if len(save) == 0 { // nada cambió: no hay entrada que deshacer
		writeJSON(w, http.StatusOK, map[string]any{"settings": admSettingsView(cur)})
		return
	}

	var auditID int64
	ok := a.admTx(w, r, "guardar ajustes", func(tx pgx.Tx) error {
		if err := a.SaveSettings(ctx, tx, save); err != nil {
			return err
		}
		var err error
		auditID, err = a.auditAdmin(ctx, tx, adminID, AuditEntry{
			Action:   "update",
			Entity:   "setting",
			Summary:  "Ajustes cambiados: " + strings.Join(slices.Sorted(maps.Keys(save)), ", "),
			Before:   before,
			After:    after,
			Undoable: !onlySecrets,
		})
		return err
	})
	a.invalidateSettings()
	if !ok {
		return
	}
	s, err := a.Settings(ctx)
	if err != nil {
		a.admFail(w, "leer ajustes", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": admSettingsView(s), "auditId": auditID})
}

// ---------- Administradores ----------

func admAccounts(ctx context.Context, q DBTX, where string, args ...any) ([]adminAccountJSON, error) {
	rows, err := q.Query(ctx, `select id, username, active, last_login_at, created_at from admins
		where `+where+` order by lower(username), id`, args...)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[adminAccountJSON])
	if out == nil {
		out = []adminAccountJSON{}
	}
	return out, err
}

func (a *App) admAccount(ctx context.Context, id int64) (adminAccountJSON, error) {
	list, err := admAccounts(ctx, a.DB, "id = $1", id)
	if err == nil && len(list) == 0 {
		err = pgx.ErrNoRows
	}
	if err != nil {
		return adminAccountJSON{}, err
	}
	return list[0], nil
}

func admCheckUsername(v string) (string, error) {
	u := strings.TrimSpace(v)
	if u == "" || utf8.RuneCountInString(u) > 60 {
		return "", admBad(http.StatusBadRequest, "El usuario es obligatorio (máximo 60 caracteres).")
	}
	return u, nil
}

// admAdminHash valida la contraseña de un admin (8+ caracteres, máximo 72 bytes por bcrypt) y saca su hash.
func (a *App) admAdminHash(ctx context.Context, pw string) (string, error) {
	if utf8.RuneCountInString(pw) < 8 {
		return "", admBad(http.StatusBadRequest, "La contraseña debe tener al menos 8 caracteres.")
	}
	if len(pw) > 72 {
		return "", admBad(http.StatusBadRequest, "La contraseña no puede pasar de 72 bytes.")
	}
	return a.hashPasswordCtx(ctx, pw)
}

// admUsernameTaken compara sin distinguir mayúsculas, para no tener "admin" y "Admin".
func admUsernameTaken(ctx context.Context, q DBTX, username string, exceptID int64) error {
	var taken bool
	err := q.QueryRow(ctx, `select exists (select 1 from admins where lower(username) = lower($1) and id <> $2)`,
		username, exceptID).Scan(&taken)
	if err == nil && taken {
		err = admBad(http.StatusConflict, "Ese usuario ya existe.")
	}
	return err
}

// admLockAdmin bloquea a todos los admins activos (así dos cambios a la vez no
// dejan el panel sin nadie) y la fila pedida. Devuelve la fila y cuántos activos hay.
func admLockAdmin(ctx context.Context, tx pgx.Tx, id int64) (map[string]any, int, error) {
	rows, err := tx.Query(ctx, `select id from admins where active order by id for update`)
	if err != nil {
		return nil, 0, err
	}
	active, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, 0, err
	}
	var row map[string]any
	err = tx.QueryRow(ctx, `select to_jsonb(t) from admins t where t.id = $1 for update`, id).Scan(&row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, admBad(http.StatusNotFound, "Administrador no encontrado.")
	}
	return row, len(active), err
}

// admCanRemove impide quedarse fuera: nadie se quita a sí mismo y siempre queda un admin activo.
func admCanRemove(id, self int64, activeCount int, verb string) error {
	if id == self {
		return admBad(http.StatusBadRequest, "No puedes "+verb+" tu propia cuenta.")
	}
	if activeCount <= 1 {
		return admBad(http.StatusBadRequest, "Debe quedar al menos un administrador activo.")
	}
	return nil
}

func (a *App) handleAdminAdminsList(w http.ResponseWriter, r *http.Request, adminID int64) {
	list, err := admAccounts(r.Context(), a.DB, "true")
	if err != nil {
		a.admFail(w, "listar administradores", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"admins": list})
}

func (a *App) handleAdminAdminsCreate(w http.ResponseWriter, r *http.Request, adminID int64) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	username, err := admCheckUsername(body.Username)
	if err != nil {
		a.admFail(w, "crear administrador", err)
		return
	}
	ctx := r.Context()
	hash, err := a.admAdminHash(ctx, body.Password)
	if err != nil {
		a.admFail(w, "crear administrador", err)
		return
	}
	var id, auditID int64
	ok := a.admTx(w, r, "crear administrador", func(tx pgx.Tx) error {
		if err := admUsernameTaken(ctx, tx, username, 0); err != nil {
			return err
		}
		var row map[string]any
		err := tx.QueryRow(ctx, `insert into admins as t (username, password_hash) values ($1, $2) returning to_jsonb(t)`,
			username, hash).Scan(&row)
		if admUnique(err) {
			return admBad(http.StatusConflict, "Ese usuario ya existe.")
		}
		if err != nil {
			return err
		}
		id = admNum(row, "id")
		auditID, err = a.auditAdmin(ctx, tx, adminID, AuditEntry{
			Action:   "create",
			Entity:   "admin",
			EntityID: itoa64(id),
			Summary:  "Administrador creado: " + username,
			After:    row,
			Undoable: true,
		})
		return err
	})
	if !ok {
		return
	}
	acct, err := a.admAccount(ctx, id)
	if err != nil {
		a.admFail(w, "leer administrador", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"admin": acct, "auditId": auditID})
}

func (a *App) handleAdminAdminsUpdate(w http.ResponseWriter, r *http.Request, adminID int64) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusNotFound, "Administrador no encontrado.")
		return
	}
	var body struct {
		Username *string `json:"username"`
		Password *string `json:"password"`
		Active   *bool   `json:"active"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	var sets []string
	var args []any
	var changed []string
	add := func(col string, v any, label string) {
		args = append(args, v)
		sets = append(sets, col+" = $"+strconv.Itoa(len(args)))
		changed = append(changed, label)
	}
	username := ""
	if body.Username != nil {
		var err error
		if username, err = admCheckUsername(*body.Username); err != nil {
			a.admFail(w, "editar administrador", err)
			return
		}
		add("username", username, "usuario")
	}
	newHash := ""
	if body.Password != nil {
		var err error
		if newHash, err = a.admAdminHash(r.Context(), *body.Password); err != nil {
			a.admFail(w, "editar administrador", err)
			return
		}
		add("password_hash", newHash, "contraseña")
	}
	if body.Active != nil {
		add("active", *body.Active, "activo")
	}
	if len(sets) == 0 {
		writeError(w, http.StatusBadRequest, "No hay cambios que guardar.")
		return
	}

	ctx := r.Context()
	var auditID int64
	ok = a.admTx(w, r, "editar administrador", func(tx pgx.Tx) error {
		before, activeCount, err := admLockAdmin(ctx, tx, id)
		if err != nil {
			return err
		}
		if body.Active != nil && !*body.Active && before["active"] == true {
			if err := admCanRemove(id, adminID, activeCount, "desactivar"); err != nil {
				return err
			}
		}
		if username != "" {
			if err := admUsernameTaken(ctx, tx, username, id); err != nil {
				return err
			}
		}
		var after map[string]any
		err = tx.QueryRow(ctx, "update admins t set "+strings.Join(sets, ", ")+", updated_at = now() where t.id = $"+
			strconv.Itoa(len(args)+1)+" returning to_jsonb(t)", append(args, id)...).Scan(&after)
		if admUnique(err) {
			return admBad(http.StatusConflict, "Ese usuario ya existe.")
		}
		if err != nil {
			return err
		}
		auditID, err = a.auditAdmin(ctx, tx, adminID, AuditEntry{
			Action:   "update",
			Entity:   "admin",
			EntityID: itoa64(id),
			Summary:  "Administrador " + admStr(before, "username") + " editado: " + strings.Join(changed, ", "),
			Before:   before,
			After:    after,
			Undoable: true,
		})
		return err
	})
	if !ok {
		return
	}
	// La contraseña nueva cierra las demás sesiones de esa cuenta; si es la
	// propia, esta se vuelve a firmar para seguir dentro.
	if id == adminID && newHash != "" {
		a.setSession(w, kindAdmin, adminID, newHash)
	}
	acct, err := a.admAccount(ctx, id)
	if err != nil {
		a.admFail(w, "leer administrador", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"admin": acct, "auditId": auditID})
}

func (a *App) handleAdminAdminsDelete(w http.ResponseWriter, r *http.Request, adminID int64) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusNotFound, "Administrador no encontrado.")
		return
	}
	ctx := r.Context()
	var auditID int64
	ok = a.admTx(w, r, "eliminar administrador", func(tx pgx.Tx) error {
		before, activeCount, err := admLockAdmin(ctx, tx, id)
		if err != nil {
			return err
		}
		if id == adminID || before["active"] == true {
			if err := admCanRemove(id, adminID, activeCount, "eliminar"); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `delete from admins where id = $1`, id); err != nil {
			return err
		}
		auditID, err = a.auditAdmin(ctx, tx, adminID, AuditEntry{
			Action:   "delete",
			Entity:   "admin",
			EntityID: itoa64(id),
			Summary:  "Administrador eliminado: " + admStr(before, "username"),
			Before:   before,
			Undoable: true,
		})
		return err
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "auditId": auditID})
}

// ---------- Subidas ----------

// Imágenes que se pueden subir y la extensión con que se guardan.
var admImageExt = map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/gif": "gif", "image/webp": "webp"}

const admMaxImageBytes = 20 << 20

// handleAdminUpload no recibe el archivo: firma un PUT para que el navegador lo
// suba directo al almacenamiento (el WAF no deja pasar cuerpos grandes).
func (a *App) handleAdminUpload(w http.ResponseWriter, r *http.Request, adminID int64) {
	var body struct {
		Filename    string `json:"filename"`
		ContentType string `json:"contentType"`
		Size        int64  `json:"size"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	ct := strings.ToLower(strings.TrimSpace(body.ContentType))
	ext, ok := admImageExt[ct]
	if !ok {
		writeError(w, http.StatusBadRequest, "Formato no permitido. Usa PNG, JPG, GIF o WEBP.")
		return
	}
	if body.Size <= 0 {
		writeError(w, http.StatusBadRequest, "El archivo está vacío.")
		return
	}
	if body.Size > admMaxImageBytes {
		writeError(w, http.StatusBadRequest, "La imagen pesa más de 20 MB.")
		return
	}
	now := time.Now().UTC()
	key := fmt.Sprintf("u/%04d/%02d/%s.%s", now.Year(), int(now.Month()), randomToken(8), ext)
	uploadURL, headers, err := a.Store.PresignUpload(r.Context(), key, ct, body.Size)
	if err != nil {
		a.admFail(w, "firmar subida", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"uploadUrl": uploadURL,
		"method":    http.MethodPut,
		"headers":   headers,
		"url":       mediaURL(key),
	})
}
