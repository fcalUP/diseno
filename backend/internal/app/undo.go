package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Bitácora del panel y motor de deshacer/rehacer (spec §5.6).

// ---------- Errores que ve el admin ----------

// undoError lleva el status y el mensaje para el panel. Con fields la respuesta
// es 409 con conflict: true, y el panel ofrece forzar.
type undoError struct {
	status int
	msg    string
	fields []string
}

func (e *undoError) Error() string { return e.msg }

func undoFail(status int, msg string) error { return &undoError{status: status, msg: msg} }

func undoConflict(msg string, fields []string) error {
	return &undoError{status: http.StatusConflict, msg: msg, fields: fields}
}

var undoErrData = undoFail(http.StatusBadRequest, "La entrada no tiene los datos necesarios para deshacerse.")

// Mensajes para las llaves únicas del esquema.
var undoUniqueMsgs = map[string]string{
	"courses_name_key":            "Ya existe un curso con ese nombre.",
	"courses_sheet_name_key":      "Ya existe un curso con esa hoja de Sheets.",
	"badges_slug_key":             "Ya existe una insignia con ese nombre.",
	"students_course_id_code_key": "Ya existe un alumno con esa matrícula en el curso.",
	"admins_username_key":         "Ya existe un administrador con ese usuario.",
	"levels_min_exp_key":          "Dos niveles no pueden pedir la misma EXP.",
	"purchases_source_ref_idx":    "Esa compra importada ya existe.",
}

// undoDBError traduce violaciones de restricciones a un 409 legible; lo demás pasa igual.
func undoDBError(err error) error {
	var pg *pgconn.PgError
	if err == nil || !errors.As(err, &pg) {
		return err
	}
	switch pg.Code {
	case "23505":
		if msg, ok := undoUniqueMsgs[pg.ConstraintName]; ok {
			return undoFail(http.StatusConflict, msg)
		}
		return undoFail(http.StatusConflict, "Ese registro ya existe.")
	case "23503":
		return undoFail(http.StatusConflict, "Falta un registro relacionado: el curso, el alumno o la insignia ya no existe.")
	case "23502", "23514", "22003", "22P02", "22007", "22008":
		return undoFail(http.StatusConflict, "Los datos guardados ya no son válidos para restaurarse.")
	}
	return err
}

// undoWriteError responde un undoError tal cual; lo inesperado va al log y sale como 500.
func (a *App) undoWriteError(w http.ResponseWriter, what string, err error) {
	var ue *undoError
	if !errors.As(err, &ue) {
		if writeCanceled(w, err) {
			return
		}
		a.Log.Error(what, "err", err)
		writeError(w, http.StatusInternalServerError, "Error interno del servidor.")
		return
	}
	if ue.fields != nil {
		writeJSON(w, ue.status, map[string]any{"error": ue.msg, "conflict": true, "fields": ue.fields})
		return
	}
	writeError(w, ue.status, ue.msg)
}

// ---------- Bitácora ----------

var (
	undoUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	undoLike = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
)

type undoFilter struct {
	entity, actorType, q, batchID string
	beforeID                      int64
	limit                         int
}

// undoEntries lee la bitácora de la más nueva a la más vieja. Devuelve hasta limit
// entradas y el id para pedir la página siguiente (0 si no hay más).
func undoEntries(ctx context.Context, q DBTX, f undoFilter) ([]auditEntryJSON, int64, error) {
	var conds []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		conds = append(conds, strings.Replace(cond, "?", "$"+strconv.Itoa(len(args)), 1))
	}
	if f.entity != "" {
		add("entity = ?", f.entity)
	}
	if f.actorType != "" {
		add("actor_type = ?", f.actorType)
	}
	if f.q != "" {
		add("summary ilike ?", "%"+undoLike.Replace(f.q)+"%")
	}
	if f.batchID != "" {
		add("batch_id = ?::uuid", f.batchID)
	}
	if f.beforeID > 0 {
		add("id < ?", f.beforeID)
	}
	sql := `select id, at, actor_type, actor_label, action, entity, entity_id, summary, before, after,
		batch_id::text, undoable, undone_at, reverts_id from audit_log`
	if len(conds) > 0 {
		sql += " where " + strings.Join(conds, " and ")
	}
	sql += " order by id desc limit " + strconv.Itoa(f.limit+1)
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, err
	}
	entries, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (auditEntryJSON, error) {
		var e auditEntryJSON
		err := r.Scan(&e.ID, &e.At, &e.ActorType, &e.ActorLabel, &e.Action, &e.Entity, &e.EntityID,
			&e.Summary, &e.Before, &e.After, &e.BatchID, &e.Undoable, &e.UndoneAt, &e.RevertsID)
		e.Before, e.After = undoStrip(e.Before), undoStrip(e.After)
		return e, err
	})
	if err != nil {
		return nil, 0, err
	}
	if entries == nil {
		entries = []auditEntryJSON{}
	}
	var next int64
	if len(entries) > f.limit {
		entries = entries[:f.limit]
		next = entries[f.limit-1].ID
	}
	return entries, next, nil
}

// undoStrip quita hashes y valores secretos de una instantánea antes de mandarla al panel.
func undoStrip(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			switch k {
			case "password_hash", "code_hash":
				delete(t, k)
			case "resend_api_key", "google_service_account", "resendApiKey", "googleServiceAccount":
				// Se guardan como {"configured": ...}; si alguien guardó el valor, no sale.
				if s, ok := val.(string); ok {
					t[k] = map[string]any{"configured": s != ""}
				}
			default:
				t[k] = undoStrip(val)
			}
		}
	case []any:
		for i := range t {
			t[i] = undoStrip(t[i])
		}
	}
	return v
}

func (a *App) handleAdminAuditList(w http.ResponseWriter, r *http.Request, adminID int64) {
	q := r.URL.Query()
	f := undoFilter{
		entity:    strings.TrimSpace(q.Get("entity")),
		actorType: strings.TrimSpace(q.Get("actorType")),
		q:         strings.TrimSpace(q.Get("q")),
		batchID:   strings.TrimSpace(q.Get("batchId")),
		limit:     50,
	}
	if f.batchID != "" && !undoUUID.MatchString(f.batchID) {
		writeError(w, http.StatusBadRequest, "El lote no es válido.")
		return
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "El límite no es válido.")
			return
		}
		f.limit = min(n, 200)
	}
	if v := q.Get("beforeId"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "beforeId no es válido.")
			return
		}
		f.beforeID = n
	}
	entries, next, err := undoEntries(r.Context(), a.DB, f)
	if err != nil {
		a.undoWriteError(w, "listar bitácora", err)
		return
	}
	var nextID *int64
	if next > 0 {
		nextID = &next
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "nextBeforeId": nextID})
}

// ---------- Endpoints de deshacer / rehacer ----------

type undoBody struct {
	Force bool `json:"force"`
}

type undoFailure struct {
	ID    int64  `json:"id"`
	Error string `json:"error"`
}

func (a *App) handleAdminAuditUndo(w http.ResponseWriter, r *http.Request, adminID int64) {
	a.undoOne(w, r, adminID, false)
}

func (a *App) handleAdminAuditRedo(w http.ResponseWriter, r *http.Request, adminID int64) {
	a.undoOne(w, r, adminID, true)
}

func (a *App) handleAdminAuditBatchUndo(w http.ResponseWriter, r *http.Request, adminID int64) {
	a.undoBatch(w, r, adminID, false)
}

func (a *App) handleAdminAuditBatchRedo(w http.ResponseWriter, r *http.Request, adminID int64) {
	a.undoBatch(w, r, adminID, true)
}

func (a *App) undoOne(w http.ResponseWriter, r *http.Request, adminID int64, redo bool) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusNotFound, "No se encontró esa entrada de la bitácora.")
		return
	}
	var body undoBody
	if !decodeJSON(w, r, &body) {
		return
	}
	auditID, summary, _, err := a.undoRun(r.Context(), id, adminID, redo, body.Force, true)
	if err != nil {
		a.undoWriteError(w, "deshacer/rehacer", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "auditId": auditID, "summary": summary})
}

func (a *App) undoBatch(w http.ResponseWriter, r *http.Request, adminID int64, redo bool) {
	batchID := r.PathValue("batchId")
	if !undoUUID.MatchString(batchID) {
		writeError(w, http.StatusBadRequest, "El lote no es válido.")
		return
	}
	var body undoBody
	if !decodeJSON(w, r, &body) {
		return
	}
	ctx := r.Context()
	// Deshacer va de la más nueva a la más vieja; rehacer va al revés para repetir
	// los cambios en su orden original (primero el alumno, luego sus compras).
	cond, order := "undone_at is null", "desc"
	if redo {
		cond, order = "undone_at is not null", "asc"
	}
	sql := "select id from audit_log where batch_id = $1::uuid and undoable and " + cond
	// En una importación la entrada resumen se encarga de todo el lote de una vez.
	ids, err := undoIDList(ctx, a.DB, sql+" and entity = 'import' order by id "+order, batchID)
	if err == nil && len(ids) == 0 {
		ids, err = undoIDList(ctx, a.DB, sql+" order by id "+order, batchID)
	}
	if err != nil {
		a.undoWriteError(w, "deshacer lote", err)
		return
	}
	if len(ids) == 0 {
		var exists bool
		err := a.DB.QueryRow(ctx, `select exists (select 1 from audit_log where batch_id = $1::uuid)`, batchID).Scan(&exists)
		switch {
		case err != nil:
			a.undoWriteError(w, "deshacer lote", err)
		case !exists:
			writeError(w, http.StatusNotFound, "No se encontró ese lote.")
		case redo:
			writeError(w, http.StatusBadRequest, "No queda nada por rehacer en ese lote.")
		default:
			writeError(w, http.StatusBadRequest, "No queda nada por deshacer en ese lote.")
		}
		return
	}
	done, failed := 0, []undoFailure{}
	for _, id := range ids {
		_, _, n, err := a.undoRun(ctx, id, adminID, redo, body.Force, false)
		if err != nil {
			failed = append(failed, undoFailure{ID: id, Error: a.undoText(err)})
			continue
		}
		done += n
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "done": done, "failed": failed})
}

// undoText da el mensaje de un error para la lista de fallas de un lote.
func (a *App) undoText(err error) string {
	var ue *undoError
	if !errors.As(err, &ue) {
		a.Log.Error("deshacer en lote", "err", err)
		return "Error interno del servidor."
	}
	if len(ue.fields) > 0 {
		return ue.msg + " Campos: " + strings.Join(ue.fields, ", ") + "."
	}
	return ue.msg
}

// undoRun deshace o rehace una entrada en su propia transacción. Con strict=false
// (lotes) las entradas que ya no aplican se saltan sin error. Devuelve el id de la
// nueva entrada, su resumen y cuántas entradas se procesaron.
func (a *App) undoRun(ctx context.Context, id, adminID int64, redo, force, strict bool) (int64, string, int, error) {
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return 0, "", 0, err
	}
	defer tx.Rollback(ctx)
	u := &undoCtx{a: a, tx: tx, adminID: adminID, redo: redo, force: force}
	it, err := u.load(ctx, id)
	if err != nil {
		return 0, "", 0, err
	}
	if err := u.check(it); err != nil {
		if strict {
			return 0, "", 0, err
		}
		return 0, "", 0, nil
	}
	auditID, summary, err := u.run(ctx, it)
	if err != nil {
		return 0, "", 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, "", 0, err
	}
	// Por si tocó ajustes o niveles (también puede pasar dentro de una importación).
	a.invalidateSettings()
	a.invalidateLevels()
	return auditID, summary, u.done, nil
}

// ---------- Motor ----------

type undoCtx struct {
	a       *App
	tx      pgx.Tx
	adminID int64
	redo    bool
	force   bool
	done    int
}

// undoItem es una fila de audit_log.
type undoItem struct {
	ID       int64
	Action   string
	Entity   string
	EntityID string
	Summary  string
	Before   any
	After    any
	BatchID  string
	Undoable bool
	Undone   bool
}

// load lee y bloquea la entrada, así dos peticiones no la deshacen a la vez.
func (u *undoCtx) load(ctx context.Context, id int64) (undoItem, error) {
	var it undoItem
	var entityID, batchID *string
	err := u.tx.QueryRow(ctx, `select id, action, entity, entity_id, summary, before, after, batch_id::text,
		undoable, undone_at is not null from audit_log where id = $1 for update`, id).Scan(
		&it.ID, &it.Action, &it.Entity, &entityID, &it.Summary, &it.Before, &it.After, &batchID,
		&it.Undoable, &it.Undone)
	if errors.Is(err, pgx.ErrNoRows) {
		return it, undoFail(http.StatusNotFound, "No se encontró esa entrada de la bitácora.")
	}
	if entityID != nil {
		it.EntityID = *entityID
	}
	if batchID != nil {
		it.BatchID = *batchID
	}
	return it, err
}

func (u *undoCtx) check(it undoItem) error {
	switch {
	case !it.Undoable:
		return undoFail(http.StatusBadRequest, "Este cambio no se puede deshacer.")
	case !u.redo && it.Undone:
		return undoFail(http.StatusBadRequest, "Este cambio ya se deshizo.")
	case u.redo && !it.Undone:
		return undoFail(http.StatusBadRequest, "Solo se puede rehacer un cambio que se deshizo.")
	}
	return nil
}

// run aplica la entrada, deja su propia entrada en la bitácora y marca la original.
func (u *undoCtx) run(ctx context.Context, it undoItem) (int64, string, error) {
	before, after, err := u.apply(ctx, it)
	if err != nil {
		return 0, "", err
	}
	action, summary := "undo", "Se deshizo: "+it.Summary
	if u.redo {
		action, summary = "redo", "Se rehízo: "+it.Summary
	}
	reverts := it.ID
	id, err := u.a.auditAdmin(ctx, u.tx, u.adminID, AuditEntry{
		Action:    action,
		Entity:    it.Entity,
		EntityID:  it.EntityID,
		Summary:   summary,
		Before:    before,
		After:     after,
		BatchID:   it.BatchID,
		Undoable:  false,
		RevertsID: &reverts,
	})
	if err != nil {
		return 0, "", err
	}
	if u.redo {
		_, err = u.tx.Exec(ctx, `update audit_log set undone_at = null, undone_by = null where id = $1`, it.ID)
	} else {
		_, err = u.tx.Exec(ctx, `update audit_log set undone_at = now(), undone_by = $2 where id = $1`, it.ID, u.adminID)
	}
	if err != nil {
		return 0, "", err
	}
	u.done++
	return id, summary, nil
}

// apply hace el cambio inverso (o lo repite al rehacer) y devuelve el antes y el
// después para la entrada nueva.
func (u *undoCtx) apply(ctx context.Context, it undoItem) (any, any, error) {
	switch it.Entity {
	case "import":
		return u.applyImport(ctx, it)
	case "level":
		return u.applyLevels(ctx, it)
	case "setting":
		return u.applySettings(ctx, it)
	case "visibility":
		return u.applyVisibility(ctx, it)
	case "purchase":
		switch it.Action {
		case "purchase", "restore":
			return u.togglePurchase(ctx, it, !u.redo)
		case "revert":
			return u.togglePurchase(ctx, it, u.redo)
		}
	}
	t, ok := undoTables[it.Entity]
	if !ok {
		return nil, nil, undoFail(http.StatusBadRequest, "Este tipo de cambio no se puede deshacer.")
	}
	switch it.Action {
	case "create":
		if u.redo {
			return u.recreate(ctx, t, it)
		}
		return u.remove(ctx, t, it, it.After, true)
	case "delete":
		if u.redo {
			return u.remove(ctx, t, it, it.Before, false)
		}
		return u.restore(ctx, t, it, it.Before)
	}
	return u.update(ctx, t, it)
}

// ---------- Filas genéricas (jsonb_populate_record) ----------

// undoTable describe una tabla que el motor sabe restaurar. Solo estas tablas y
// columnas llegan al SQL; las claves extra de una instantánea se ignoran.
type undoTable struct {
	name      string
	pk        []string
	cols      []string
	updatedAt bool
}

var undoTables = map[string]undoTable{
	"student": {"students", []string{"id"}, []string{"id", "course_id", "code", "name", "sexo", "email",
		"password_hash", "tareas", "monedas", "asistencias", "exp", "games", "last_login_level", "comments",
		"active", "last_login_at", "created_at", "updated_at"}, true},
	"course": {"courses", []string{"id"}, []string{"id", "name", "sheet_name", "sheet_aliases",
		"side_image_url", "logo_url", "hidden_badge_keywords", "active", "sort_order", "created_at",
		"updated_at"}, true},
	"badge": {"badges", []string{"id"}, []string{"id", "name", "slug", "stock", "cost", "image_url", "active",
		"sort_order", "created_at", "updated_at"}, true},
	"admin": {"admins", []string{"id"}, []string{"id", "username", "password_hash", "active", "last_login_at",
		"created_at", "updated_at"}, true},
	"avatar": {"avatars", []string{"code", "level"}, []string{"code", "level", "url", "updated_at"}, true},
	"purchase": {"purchases", []string{"id"}, []string{"id", "student_id", "badge_id", "badge_name", "quantity",
		"unit_cost", "total_cost", "coins_before", "coins_after", "source", "source_ref", "created_at",
		"reverted_at"}, false},
}

// Pseudo-campo de las instantáneas de insignia: cursos donde está oculta.
const undoHiddenKey = "hidden_course_ids"

func (t undoTable) from() string { return "jsonb_populate_record(null::" + t.name + ", $1) r" }

func (t undoTable) match() string {
	parts := make([]string, len(t.pk))
	for i, c := range t.pk {
		parts[i] = "t." + c + " = r." + c
	}
	return strings.Join(parts, " and ")
}

// key saca la llave primaria de la fila; si la instantánea no la trae, usa el
// entity_id ("12" o "code:level").
func (t undoTable) key(row map[string]any, entityID string) (map[string]any, error) {
	key := map[string]any{}
	for _, c := range t.pk {
		if v, ok := row[c]; ok && v != nil {
			key[c] = v
		}
	}
	if len(key) == len(t.pk) {
		return key, nil
	}
	if t.name == "avatars" {
		if i := strings.LastIndex(entityID, ":"); i > 0 {
			if lvl, err := strconv.Atoi(entityID[i+1:]); err == nil {
				return map[string]any{"code": entityID[:i], "level": lvl}, nil
			}
		}
	} else if id, err := strconv.ParseInt(entityID, 10, 64); err == nil && id > 0 {
		return map[string]any{"id": id}, nil
	}
	return nil, undoFail(http.StatusBadRequest, "La entrada no identifica el registro.")
}

// current lee y bloquea la fila actual; nil si ya no existe.
func (u *undoCtx) current(ctx context.Context, t undoTable, key map[string]any) (map[string]any, error) {
	var row map[string]any
	err := u.tx.QueryRow(ctx, "select to_jsonb(t) from "+t.name+" t, "+t.from()+" where "+t.match()+
		" for update of t", key).Scan(&row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return row, err
}

func (u *undoCtx) insertRow(ctx context.Context, t undoTable, row map[string]any) error {
	if t.name == "purchases" {
		return u.insertPurchases(ctx, []any{row})
	}
	var cols []string
	for _, c := range t.cols {
		if _, ok := row[c]; ok {
			cols = append(cols, c)
		}
	}
	list := strings.Join(cols, ", ")
	_, err := u.tx.Exec(ctx, "insert into "+t.name+" ("+list+") select "+list+
		" from jsonb_populate_record(null::"+t.name+", $1)", row)
	return undoDBError(err)
}

// insertPurchases restaura compras; las que apuntaban a una insignia borrada quedan
// sin enlace, igual que hace la base al borrar la insignia.
func (u *undoCtx) insertPurchases(ctx context.Context, list []any) error {
	if len(list) == 0 {
		return nil
	}
	var cols, exprs []string
	for _, c := range undoTables["purchase"].cols {
		present := false
		for _, item := range list {
			if m, ok := item.(map[string]any); ok {
				if _, ok := m[c]; ok {
					present = true
					break
				}
			}
		}
		if !present {
			continue
		}
		cols = append(cols, c)
		if c == "badge_id" {
			exprs = append(exprs, "(select b.id from badges b where b.id = r.badge_id)")
		} else {
			exprs = append(exprs, "r."+c)
		}
	}
	if len(cols) == 0 {
		return undoErrData
	}
	_, err := u.tx.Exec(ctx, "insert into purchases ("+strings.Join(cols, ", ")+") select "+
		strings.Join(exprs, ", ")+" from jsonb_populate_recordset(null::purchases, $1) r", list)
	return undoDBError(err)
}

// setCols pone en la fila los valores de vals para las columnas cols.
func (u *undoCtx) setCols(ctx context.Context, t undoTable, key, vals map[string]any, cols []string) error {
	m := maps.Clone(key)
	sets := make([]string, 0, len(cols)+1)
	for _, c := range cols {
		m[c] = vals[c]
		sets = append(sets, c+" = r."+c)
	}
	if t.updatedAt {
		sets = append(sets, "updated_at = now()")
	}
	_, err := u.tx.Exec(ctx, "update "+t.name+" t set "+strings.Join(sets, ", ")+" from "+t.from()+
		" where "+t.match(), m)
	return undoDBError(err)
}

// undoChanged lista las columnas (y pseudo-campos) que difieren entre antes y después,
// sin contar llave primaria ni marcas de tiempo.
func undoChanged(t undoTable, before, after map[string]any, extra ...string) []string {
	var out []string
	for _, c := range append(slices.Clone(t.cols), extra...) {
		if slices.Contains(t.pk, c) || c == "created_at" || c == "updated_at" {
			continue
		}
		_, inB := before[c]
		_, inA := after[c]
		if (inB || inA) && !undoSame(before[c], after[c]) {
			out = append(out, c)
		}
	}
	return out
}

// snapshot toma la fila junto con lo que depende de ella, en el formato de borrado
// del spec §5.6.
func (u *undoCtx) snapshot(ctx context.Context, entity string, row map[string]any) (map[string]any, error) {
	id, _ := undoInt(row["id"])
	switch entity {
	case "student":
		var purchases []any
		err := u.tx.QueryRow(ctx, `select coalesce(jsonb_agg(to_jsonb(p) order by p.id), '[]')
			from purchases p where p.student_id = $1`, id).Scan(&purchases)
		return map[string]any{"row": row, "purchases": purchases}, err
	case "course":
		hidden, err := catCourseHidden(ctx, u.tx, id)
		return map[string]any{"row": row, "hidden_badge_ids": hidden}, err
	case "badge":
		hidden, err := catBadgeHidden(ctx, u.tx, id)
		if err != nil {
			return nil, err
		}
		purchases, err := undoIDList(ctx, u.tx, `select id from purchases where badge_id = $1 order by id`, id)
		return map[string]any{"row": row, undoHiddenKey: hidden, "purchase_ids": purchases}, err
	}
	return row, nil
}

// undoRowOf devuelve la fila de una instantánea {"row": ...} o la instantánea misma.
func undoRowOf(snap map[string]any) map[string]any {
	if row, ok := snap["row"].(map[string]any); ok {
		return row
	}
	return snap
}

// canRemove revisa si se puede borrar la fila. needForce: al deshacer una creación,
// lo que dependa de la fila pide confirmación (force).
func (u *undoCtx) canRemove(ctx context.Context, entity string, id int64, needForce bool) error {
	count := func(sql string) (int, error) {
		var n int
		err := u.tx.QueryRow(ctx, sql, id).Scan(&n)
		return n, err
	}
	switch entity {
	case "course":
		n, err := count(`select count(*) from students where course_id = $1`)
		if err != nil {
			return err
		}
		if n > 0 {
			return undoFail(http.StatusConflict, "El curso tiene alumnos. Muévelos o desactiva el curso.")
		}
	case "student", "badge":
		if !needForce || u.force {
			return nil
		}
		sql, msg := `select count(*) from purchases where student_id = $1`,
			"El alumno tiene compras; si continúas se borran con él."
		if entity == "badge" {
			sql, msg = `select count(*) from purchases where badge_id = $1`,
				"La insignia tiene compras; si continúas quedan sin insignia enlazada."
		}
		n, err := count(sql)
		if err != nil {
			return err
		}
		if n > 0 {
			return undoConflict(msg, []string{"compras"})
		}
	}
	return nil
}

// remove borra la fila (deshacer una creación o rehacer un borrado) y devuelve lo
// que se llevó, con sus dependientes, como "antes".
func (u *undoCtx) remove(ctx context.Context, t undoTable, it undoItem, snapAny any, needForce bool) (any, any, error) {
	snap, _ := snapAny.(map[string]any)
	if snap == nil {
		return nil, nil, undoErrData
	}
	key, err := t.key(undoRowOf(snap), it.EntityID)
	if err != nil {
		return nil, nil, err
	}
	cur, err := u.current(ctx, t, key)
	if err != nil {
		return nil, nil, err
	}
	if cur == nil {
		return nil, nil, undoFail(http.StatusConflict, "El registro ya no existe.")
	}
	id, _ := undoInt(cur["id"])
	if err := u.canRemove(ctx, it.Entity, id, needForce); err != nil {
		return nil, nil, err
	}
	full, err := u.snapshot(ctx, it.Entity, cur)
	if err != nil {
		return nil, nil, err
	}
	if _, err := u.tx.Exec(ctx, "delete from "+t.name+" t using "+t.from()+" where "+t.match(), key); err != nil {
		return nil, nil, undoDBError(err)
	}
	if it.Entity == "admin" {
		if err := u.adminGuard(ctx); err != nil {
			return nil, nil, err
		}
	}
	return full, nil, nil
}

// restore vuelve a insertar una fila borrada con lo que dependía de ella.
func (u *undoCtx) restore(ctx context.Context, t undoTable, it undoItem, snapAny any) (any, any, error) {
	snap, _ := snapAny.(map[string]any)
	if snap == nil {
		return nil, nil, undoErrData
	}
	row := undoRowOf(snap)
	key, err := t.key(row, "")
	if err != nil {
		return nil, nil, err
	}
	if err := u.insertRow(ctx, t, row); err != nil {
		return nil, nil, err
	}
	id, _ := undoInt(key["id"])
	if err := u.restoreDeps(ctx, it.Entity, snap, id); err != nil {
		return nil, nil, err
	}
	cur, err := u.current(ctx, t, key)
	if err != nil {
		return nil, nil, err
	}
	if cur == nil {
		return nil, nil, undoErrData
	}
	full, err := u.snapshot(ctx, it.Entity, cur)
	return nil, full, err
}

// recreate rehace una creación con la instantánea que guardó el deshacer (incluye
// compras o visibilidad que se fueron con la fila); si no hay, usa el "después" original.
func (u *undoCtx) recreate(ctx context.Context, t undoTable, it undoItem) (any, any, error) {
	var snap map[string]any
	err := u.tx.QueryRow(ctx, `select before from audit_log where reverts_id = $1 and action = 'undo'
		and before is not null order by id desc limit 1`, it.ID).Scan(&snap)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, err
	}
	if snap == nil {
		return u.restore(ctx, t, it, it.After)
	}
	return u.restore(ctx, t, it, snap)
}

// restoreDeps repone compras y visibilidad de una fila restaurada. Lo que apunta a
// cursos o insignias que ya no existen se omite.
func (u *undoCtx) restoreDeps(ctx context.Context, entity string, snap map[string]any, id int64) error {
	switch entity {
	case "student":
		list, _ := snap["purchases"].([]any)
		return u.insertPurchases(ctx, list)
	case "course":
		if v, ok := snap["hidden_badge_ids"]; ok {
			_, err := u.tx.Exec(ctx, `insert into course_hidden_badges (course_id, badge_id)
				select $1, b.id from badges b where b.id = any($2) on conflict do nothing`, id, undoIDs(v))
			return err
		}
	case "badge":
		if v, ok := snap[undoHiddenKey]; ok {
			if _, err := u.tx.Exec(ctx, `insert into course_hidden_badges (course_id, badge_id)
				select c.id, $1 from courses c where c.id = any($2) on conflict do nothing`, id, undoIDs(v)); err != nil {
				return err
			}
		}
		if v, ok := snap["purchase_ids"]; ok {
			_, err := u.tx.Exec(ctx, `update purchases set badge_id = $1 where id = any($2) and badge_id is null`,
				id, undoIDs(v))
			return err
		}
	}
	return nil
}

// update regresa (o vuelve a aplicar) los campos que cambió la entrada. Cada campo
// debe seguir con el valor que dejó la entrada; si no, es conflicto salvo force.
func (u *undoCtx) update(ctx context.Context, t undoTable, it undoItem) (any, any, error) {
	before, _ := it.Before.(map[string]any)
	after, _ := it.After.(map[string]any)
	if before == nil || after == nil {
		return nil, nil, undoErrData
	}
	key, err := t.key(after, it.EntityID)
	if err != nil {
		return nil, nil, err
	}
	cur, err := u.current(ctx, t, key)
	if err != nil {
		return nil, nil, err
	}
	if cur == nil {
		return nil, nil, undoFail(http.StatusConflict, "El registro ya no existe.")
	}
	from, to := after, before
	if u.redo {
		from, to = before, after
	}
	id, _ := undoInt(key["id"])
	_, hb := before[undoHiddenKey]
	_, ha := after[undoHiddenKey]
	hidden := it.Entity == "badge" && hb && ha
	var extra []string
	if hidden {
		extra = []string{undoHiddenKey}
		if cur[undoHiddenKey], err = catBadgeHidden(ctx, u.tx, id); err != nil {
			return nil, nil, err
		}
		// Los cursos borrados desde entonces no cuentan como cambio.
		for _, m := range []map[string]any{before, after} {
			if m[undoHiddenKey], err = u.existing(ctx, "courses", undoIDs(m[undoHiddenKey])); err != nil {
				return nil, nil, err
			}
		}
	}
	fields := undoChanged(t, before, after, extra...)
	var conflicts []string
	for _, f := range fields {
		if !undoSame(cur[f], from[f]) {
			conflicts = append(conflicts, f)
		}
	}
	if len(conflicts) > 0 && !u.force {
		return nil, nil, undoConflict("El registro cambió desde entonces.", conflicts)
	}
	var cols []string
	for _, f := range fields {
		if f != undoHiddenKey {
			cols = append(cols, f)
			continue
		}
		if err := catSetBadgeHidden(ctx, u.tx, id, undoIDs(to[f])); err != nil {
			return nil, nil, err
		}
	}
	if len(cols) > 0 {
		if err := u.setCols(ctx, t, key, to, cols); err != nil {
			return nil, nil, err
		}
	}
	if it.Entity == "admin" {
		if err := u.adminGuard(ctx); err != nil {
			return nil, nil, err
		}
	}
	now, err := u.current(ctx, t, key)
	if err != nil {
		return nil, nil, err
	}
	if now == nil {
		return nil, nil, undoErrData
	}
	if hidden {
		if now[undoHiddenKey], err = catBadgeHidden(ctx, u.tx, id); err != nil {
			return nil, nil, err
		}
	}
	return cur, now, nil
}

// adminGuard impide que deshacer deje sin acceso al admin que lo está haciendo.
// Mientras él siga activo, siempre queda al menos un admin activo.
func (u *undoCtx) adminGuard(ctx context.Context) error {
	var ok bool
	if err := u.tx.QueryRow(ctx, `select exists (select 1 from admins where id = $1 and active)`,
		u.adminID).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return undoFail(http.StatusBadRequest, "No puedes desactivar ni eliminar tu propia cuenta.")
	}
	return nil
}

// existing filtra ids a los que siguen en la tabla (courses o badges), ordenados.
func (u *undoCtx) existing(ctx context.Context, table string, ids []int64) ([]int64, error) {
	return undoIDList(ctx, u.tx, "select id from "+table+" where id = any($1) order by id", ids)
}

// ---------- Compras ----------

// togglePurchase revierte una compra (devuelve monedas y stock) o la restaura
// (los vuelve a cobrar; falla si ya no alcanzan).
func (u *undoCtx) togglePurchase(ctx context.Context, it undoItem, revert bool) (any, any, error) {
	// La fila de la instantánea manda; entity_id queda de respaldo.
	var id int64
	ok := false
	for _, snap := range []any{it.After, it.Before} {
		if m, _ := snap.(map[string]any); m != nil && !ok {
			id, ok = undoInt(m["id"])
		}
	}
	if !ok {
		id, ok = undoInt(it.EntityID)
	}
	if !ok {
		return nil, nil, undoFail(http.StatusBadRequest, "La entrada no identifica la compra.")
	}
	before, err := u.purchaseState(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if before == nil {
		return nil, nil, undoFail(http.StatusConflict, "La compra ya no existe.")
	}
	studentID, _ := undoInt(before["student_id"])
	badgeID, _ := undoInt(before["badge_id"])
	_, hasBadge := before["badge_stock"] // sin stock: la insignia ya no existe
	qty, _ := undoInt(before["quantity"])
	total, _ := undoInt(before["total_cost"])
	coins, _ := undoInt(before["student_coins"])
	stock, _ := undoInt(before["badge_stock"])
	reverted := before["reverted_at"] != nil
	sign := int64(1)
	if revert {
		if reverted {
			return nil, nil, undoFail(http.StatusConflict, "La compra ya estaba revertida.")
		}
	} else {
		if !reverted {
			return nil, nil, undoFail(http.StatusConflict, "La compra ya estaba activa.")
		}
		if coins < total {
			return nil, nil, undoFail(http.StatusBadRequest,
				fmt.Sprintf("El alumno ya no tiene monedas suficientes: tiene %d y la compra cuesta %d.", coins, total))
		}
		if hasBadge && stock < qty {
			name, _ := before["badge_name"].(string)
			return nil, nil, undoFail(http.StatusBadRequest,
				fmt.Sprintf("Ya no hay stock suficiente de %s: quedan %d.", name, stock))
		}
		sign = -1
	}
	if _, err := u.tx.Exec(ctx, `update students set monedas = monedas + $2, updated_at = now() where id = $1`,
		studentID, sign*total); err != nil {
		return nil, nil, undoDBError(err)
	}
	if hasBadge {
		if _, err := u.tx.Exec(ctx, `update badges set stock = stock + $2, updated_at = now() where id = $1`,
			badgeID, sign*qty); err != nil {
			return nil, nil, undoDBError(err)
		}
	}
	if _, err := u.tx.Exec(ctx, `update purchases set reverted_at = case when $2 then now() end where id = $1`,
		id, revert); err != nil {
		return nil, nil, err
	}
	after, err := u.purchaseState(ctx, id)
	return before, after, err
}

// purchaseState devuelve la compra con las monedas del alumno y el stock de la insignia
// en ese momento (formato del spec §5.6) y bloquea las tres filas.
func (u *undoCtx) purchaseState(ctx context.Context, id int64) (map[string]any, error) {
	var p map[string]any
	err := u.tx.QueryRow(ctx, `select to_jsonb(p) from purchases p where p.id = $1 for update`, id).Scan(&p)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sid, _ := undoInt(p["student_id"])
	var coins int64
	if err := u.tx.QueryRow(ctx, `select monedas from students where id = $1 for update`, sid).Scan(&coins); err != nil {
		return nil, err
	}
	p["student_coins"] = coins
	if bid, ok := undoInt(p["badge_id"]); ok {
		var stock int64
		err := u.tx.QueryRow(ctx, `select stock from badges where id = $1 for update`, bid).Scan(&stock)
		if err == nil {
			p["badge_stock"] = stock
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	return p, nil
}

// ---------- Niveles, visibilidad y ajustes ----------

// applyLevels cambia la tabla de niveles completa por la de la entrada.
func (u *undoCtx) applyLevels(ctx context.Context, it undoItem) (any, any, error) {
	from, to := it.After, it.Before
	if u.redo {
		from, to = it.Before, it.After
	}
	var rows []struct {
		Level  int `json:"level"`
		MinExp int `json:"min_exp"`
	}
	raw, err := json.Marshal(to)
	if err == nil {
		err = json.Unmarshal(raw, &rows)
	}
	if err != nil || len(rows) == 0 {
		return nil, nil, undoErrData
	}
	target := make([]Level, len(rows))
	for i, r := range rows {
		target[i] = Level{Level: r.Level, MinExp: r.MinExp}
	}
	slices.SortFunc(target, func(x, y Level) int { return x.Level - y.Level })
	if catCheckLevels(target) != "" {
		return nil, nil, undoFail(http.StatusConflict, "Los niveles guardados en la entrada ya no son válidos.")
	}
	if _, err := u.tx.Exec(ctx, `lock table levels in share row exclusive mode`); err != nil {
		return nil, nil, err
	}
	cur, err := catReadLevels(ctx, u.tx)
	if err != nil {
		return nil, nil, err
	}
	if !undoSame(catLevelsSnapshot(cur), from) && !u.force {
		return nil, nil, undoConflict("Los niveles cambiaron desde entonces.", []string{"levels"})
	}
	if err := catWriteLevels(ctx, u.tx, target); err != nil {
		return nil, nil, undoDBError(err)
	}
	return catLevelsSnapshot(cur), catLevelsSnapshot(target), nil
}

// applyVisibility repone las insignias ocultas de un curso.
func (u *undoCtx) applyVisibility(ctx context.Context, it undoItem) (any, any, error) {
	before, _ := it.Before.(map[string]any)
	after, _ := it.After.(map[string]any)
	if before == nil || after == nil {
		return nil, nil, undoErrData
	}
	courseID, ok := undoInt(after["course_id"])
	if !ok {
		if courseID, ok = undoInt(before["course_id"]); !ok {
			return nil, nil, undoErrData
		}
	}
	from, to := after, before
	if u.redo {
		from, to = before, after
	}
	var exists bool
	if err := u.tx.QueryRow(ctx, `select exists (select 1 from courses where id = $1 for no key update)`,
		courseID).Scan(&exists); err != nil {
		return nil, nil, err
	}
	if !exists {
		return nil, nil, undoFail(http.StatusConflict, "El curso ya no existe.")
	}
	cur, err := catCourseHidden(ctx, u.tx, courseID)
	if err != nil {
		return nil, nil, err
	}
	// Las insignias borradas desde entonces no cuentan como cambio.
	want, err := u.existing(ctx, "badges", undoIDs(from["badge_ids"]))
	if err != nil {
		return nil, nil, err
	}
	if !slices.Equal(cur, want) && !u.force {
		return nil, nil, undoConflict("Las insignias ocultas del curso cambiaron desde entonces.", []string{"badge_ids"})
	}
	target, err := u.existing(ctx, "badges", undoIDs(to["badge_ids"]))
	if err != nil {
		return nil, nil, err
	}
	if err := catSetCourseHidden(ctx, u.tx, courseID, target); err != nil {
		return nil, nil, err
	}
	return map[string]any{"course_id": courseID, "badge_ids": cur},
		map[string]any{"course_id": courseID, "badge_ids": target}, nil
}

// applySettings repone los ajustes no secretos de la entrada. Los secretos solo se
// guardan como {"configured": ...}, así que no hay valor que reponer.
func (u *undoCtx) applySettings(ctx context.Context, it undoItem) (any, any, error) {
	byName := map[string]SettingField{}
	for _, f := range settingsFields() {
		byName[f.Key], byName[f.JSON] = f, f
	}
	// Claves de la entrada a snake_case, sin secretos ni claves desconocidas.
	norm := func(v any) map[string]any {
		out := map[string]any{}
		m, _ := v.(map[string]any)
		for k, val := range m {
			if f, ok := byName[k]; ok && !f.Secret {
				out[f.Key] = val
			}
		}
		return out
	}
	before, after := norm(it.Before), norm(it.After)
	from, to := after, before
	if u.redo {
		from, to = before, after
	}
	var keys []string
	for _, f := range settingsFields() {
		_, inB := before[f.Key]
		_, inA := after[f.Key]
		if (inB || inA) && !undoSame(undoSettingOr(f.Key, before[f.Key]), undoSettingOr(f.Key, after[f.Key])) {
			keys = append(keys, f.Key)
		}
	}
	if len(keys) == 0 {
		return nil, nil, undoFail(http.StatusBadRequest, "Este cambio solo tocó datos secretos y no se puede deshacer.")
	}
	cur := map[string]any{}
	var conflicts []string
	for _, k := range keys {
		v, err := u.settingValue(ctx, k)
		if err != nil {
			return nil, nil, err
		}
		cur[k] = v
		if !undoSame(v, undoSettingOr(k, from[k])) {
			conflicts = append(conflicts, k)
		}
	}
	if len(conflicts) > 0 && !u.force {
		return nil, nil, undoConflict("Los ajustes cambiaron desde entonces.", conflicts)
	}
	patch := map[string]any{}
	for _, k := range keys {
		patch[k] = to[k] // nil regresa al valor predeterminado
	}
	if err := u.a.SaveSettings(ctx, u.tx, patch); err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) {
			return nil, nil, err
		}
		return nil, nil, undoFail(http.StatusConflict, "No se pudo restaurar el ajuste: "+err.Error())
	}
	now := map[string]any{}
	for _, k := range keys {
		v, err := u.settingValue(ctx, k)
		if err != nil {
			return nil, nil, err
		}
		now[k] = v
	}
	return cur, now, nil
}

// settingValue es el valor vigente de una clave dentro de la transacción.
func (u *undoCtx) settingValue(ctx context.Context, key string) (any, error) {
	var v any
	err := u.tx.QueryRow(ctx, `select value from settings where key = $1`, key).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return undoSettingOr(key, nil), nil
	}
	return v, err
}

// undoSettingOr devuelve v, o el valor predeterminado de la clave si v es nil.
func undoSettingOr(key string, v any) any {
	if v != nil {
		return v
	}
	raw, _ := json.Marshal(defaultSettings())
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	for _, f := range settingsFields() {
		if f.Key == key {
			return m[f.JSON]
		}
	}
	return nil
}

// ---------- Importación ----------

// applyImport deshace (o rehace) todas las entradas del lote de la importación,
// menos el resumen, en la misma transacción: o queda todo o nada.
func (u *undoCtx) applyImport(ctx context.Context, it undoItem) (any, any, error) {
	if it.BatchID == "" {
		return nil, nil, undoFail(http.StatusBadRequest, "Esta importación no tiene lote para deshacer.")
	}
	cond, order := "undone_at is null", "desc"
	if u.redo {
		cond, order = "undone_at is not null", "asc"
	}
	ids, err := undoIDList(ctx, u.tx, `select id from audit_log where batch_id = $1::uuid and id <> $2
		and entity <> 'import' and undoable and `+cond+` order by id `+order, it.BatchID, it.ID)
	if err != nil {
		return nil, nil, err
	}
	n := 0
	for _, id := range ids {
		inner, err := u.load(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		if u.check(inner) != nil {
			continue
		}
		if _, _, err := u.run(ctx, inner); err != nil {
			var ue *undoError
			if errors.As(err, &ue) {
				return nil, nil, &undoError{status: ue.status, fields: ue.fields,
					msg: fmt.Sprintf("Entrada #%d (%s): %s", inner.ID, inner.Summary, ue.msg)}
			}
			return nil, nil, err
		}
		n++
	}
	return nil, map[string]any{"entries": n}, nil
}

// ---------- Utilidades ----------

// undoIDList corre una consulta que devuelve una columna bigint.
func undoIDList(ctx context.Context, q DBTX, sql string, args ...any) ([]int64, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if ids == nil {
		ids = []int64{}
	}
	return ids, err
}

// undoInt lee un entero que llegó de JSON (float64), de Go o como texto.
func undoInt(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), n == float64(int64(n))
	case int64:
		return n, true
	case int:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	case string:
		i, err := strconv.ParseInt(n, 10, 64)
		return i, err == nil
	}
	return 0, false
}

// undoIDs convierte una lista de JSON en ids, ordenados y sin repetir.
func undoIDs(v any) []int64 {
	ids := []int64{}
	switch list := v.(type) {
	case []int64:
		ids = append(ids, list...)
	case []any:
		for _, x := range list {
			if id, ok := undoInt(x); ok {
				ids = append(ids, id)
			}
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

// undoSame compara por JSON canónico: los números de la base llegan como float64 y
// los de Go como int, y así no importa.
func undoSame(a, b any) bool { return undoCanon(a) == undoCanon(b) }

func undoCanon(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	var x any
	if json.Unmarshal(raw, &x) != nil {
		return string(raw)
	}
	out, _ := json.Marshal(x)
	return string(out)
}
