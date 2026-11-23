package app

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"
	"net/mail"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const admCounterMsg = "Los contadores deben ser enteros entre 0 y 2147483647."

// Contadores que se pueden sumar o fijar en bloque.
var admCounters = []string{"tareas", "monedas", "asistencias", "games", "exp"}

// Columnas de students que el panel puede escribir, con su nombre para los resúmenes.
var admStudentCols = map[string]string{
	"course_id": "curso", "code": "ID", "name": "nombre", "sexo": "sexo", "email": "correo",
	"password_hash": "contraseña", "tareas": "tareas", "monedas": "monedas", "asistencias": "asistencias",
	"games": "juegos", "exp": "EXP", "comments": "comentarios", "active": "activo",
}

func admCounterOK(v int) bool { return v >= 0 && v <= math.MaxInt32 }

// admStr y admNum leen campos de una fila to_jsonb (los números llegan como float64).
func admStr(row map[string]any, key string) string {
	s, _ := row[key].(string)
	return s
}

func admNum(row map[string]any, key string) int64 {
	f, _ := row[key].(float64)
	return int64(f)
}

// admSame compara un valor de una fila to_jsonb con uno de Go.
func admSame(old, v any) bool {
	switch n := v.(type) {
	case int:
		v = float64(n)
	case int64:
		v = float64(n)
	}
	return old == v
}

// admChanges resume para la bitácora lo que cambia: "monedas 10 → 13, nombre".
func admChanges(before, set map[string]any) string {
	var parts []string
	for _, k := range slices.Sorted(maps.Keys(set)) {
		if admSame(before[k], set[k]) {
			continue
		}
		label := admStudentCols[k]
		if old, ok := before[k].(float64); ok && k != "course_id" {
			label = fmt.Sprintf("%s %d → %v", label, int64(old), set[k])
		}
		parts = append(parts, label)
	}
	if len(parts) == 0 {
		return "sin cambios"
	}
	return strings.Join(parts, ", ")
}

// admPlainEmail acepta solo "usuario@dominio", sin nombre ni <>.
func admPlainEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	return err == nil && addr.Address == s && len(s) <= 254
}

// admStudents lee alumnos con su curso, nivel, insignias y los niveles que
// tienen avatar propio. where usa el alias s (students).
func (a *App) admStudents(ctx context.Context, q DBTX, where string, args ...any) ([]adminStudentJSON, error) {
	levels, err := a.Levels(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `select s.id, s.code, s.course_id, c.name, s.name, s.sexo, s.email, s.tareas,
			s.monedas, s.asistencias, s.games, s.exp, s.last_login_level, s.comments, s.active,
			s.last_login_at, s.created_at, s.updated_at,
			(select coalesce(sum(p.quantity), 0) from purchases p where p.student_id = s.id and p.reverted_at is null),
			array(select v.level from avatars v where v.code = s.code order by v.level)
		from students s join courses c on c.id = s.course_id
		where `+where+`
		order by c.sort_order, c.id, s.id`, args...)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (adminStudentJSON, error) {
		var s adminStudentJSON
		err := row.Scan(&s.ID, &s.Code, &s.CourseID, &s.CourseName, &s.Name, &s.Sexo, &s.Email, &s.Tareas,
			&s.Monedas, &s.Asistencias, &s.Games, &s.Exp, &s.LastLoginLevel, &s.Comments, &s.Active,
			&s.LastLoginAt, &s.CreatedAt, &s.UpdatedAt, &s.Badges, &s.AvatarLevels)
		s.Level = CalcLevel(s.Exp, levels)
		if s.AvatarLevels == nil {
			s.AvatarLevels = []int{}
		}
		return s, err
	})
	if out == nil {
		out = []adminStudentJSON{}
	}
	return out, err
}

// admStudentByID lee un alumno ya guardado, para responder después del commit.
func (a *App) admStudentByID(ctx context.Context, id int64) (adminStudentJSON, error) {
	list, err := a.admStudents(ctx, a.DB, "s.id = $1", id)
	if err == nil && len(list) == 0 {
		err = pgx.ErrNoRows
	}
	if err != nil {
		return adminStudentJSON{}, err
	}
	return list[0], nil
}

func admCourseName(ctx context.Context, q DBTX, id int64) (string, error) {
	var name string
	err := q.QueryRow(ctx, `select name from courses where id = $1`, id).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", admBad(http.StatusBadRequest, "El curso no existe.")
	}
	return name, err
}

// admLockStudent bloquea al alumno hasta el fin de la transacción y devuelve su fila.
func admLockStudent(ctx context.Context, tx pgx.Tx, id int64) (map[string]any, error) {
	var row map[string]any
	err := tx.QueryRow(ctx, `select to_jsonb(s) from students s where s.id = $1 for update`, id).Scan(&row)
	return row, err
}

// admApplyStudent escribe set (columna → valor) en un alumno ya bloqueado y deja
// la entrada "update" en la bitácora con la fila de antes y la de después.
func (a *App) admApplyStudent(ctx context.Context, tx pgx.Tx, adminID, id int64, before, set map[string]any,
	summary, batchID string) (map[string]any, int64, error) {
	var sets []string
	var args []any
	for _, c := range slices.Sorted(maps.Keys(set)) {
		if _, ok := admStudentCols[c]; !ok {
			return nil, 0, fmt.Errorf("columna no permitida: %s", c)
		}
		args = append(args, set[c])
		sets = append(sets, c+" = $"+strconv.Itoa(len(args)))
	}
	sets = append(sets, "updated_at = now()")
	args = append(args, id)
	var after map[string]any
	err := tx.QueryRow(ctx, "update students s set "+strings.Join(sets, ", ")+
		" where s.id = $"+strconv.Itoa(len(args))+" returning to_jsonb(s)", args...).Scan(&after)
	if admUnique(err) {
		return nil, 0, admBad(http.StatusConflict, "El ID de alumno ya existe.")
	}
	if err != nil {
		return nil, 0, err
	}
	auditID, err := a.auditAdmin(ctx, tx, adminID, AuditEntry{
		Action:   "update",
		Entity:   "student",
		EntityID: itoa64(id),
		Summary:  summary,
		Before:   before,
		After:    after,
		BatchID:  batchID,
		Undoable: true,
	})
	return after, auditID, err
}

// admInsertStudent inserta un alumno y registra la entrada "create".
// created=false si ese ID ya existe en el curso (no se toca nada).
func (a *App) admInsertStudent(ctx context.Context, tx pgx.Tx, adminID int64, set map[string]any,
	course, batchID string) (id, auditID int64, created bool, err error) {
	cols := slices.Sorted(maps.Keys(set))
	ph := make([]string, len(cols))
	args := make([]any, len(cols))
	for i, c := range cols {
		if _, ok := admStudentCols[c]; !ok {
			return 0, 0, false, fmt.Errorf("columna no permitida: %s", c)
		}
		ph[i] = "$" + strconv.Itoa(i+1)
		args[i] = set[c]
	}
	var row map[string]any
	err = tx.QueryRow(ctx, "insert into students as s ("+strings.Join(cols, ", ")+") values ("+
		strings.Join(ph, ", ")+") on conflict (course_id, code) do nothing returning to_jsonb(s)", args...).Scan(&row)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	id = admNum(row, "id")
	auditID, err = a.auditAdmin(ctx, tx, adminID, AuditEntry{
		Action:   "create",
		Entity:   "student",
		EntityID: itoa64(id),
		Summary:  "Alumno creado: " + admStr(row, "code") + " – " + admStr(row, "name") + " (" + course + ")",
		After:    row,
		BatchID:  batchID,
		Undoable: true,
	})
	return id, auditID, err == nil, err
}

// admDeleteStudent borra un alumno ya bloqueado. La bitácora guarda la fila y
// sus compras ({"row", "purchases"}) para que deshacer pueda devolver todo.
func (a *App) admDeleteStudent(ctx context.Context, tx pgx.Tx, adminID int64, row map[string]any, batchID string) (int64, error) {
	id := admNum(row, "id")
	var purchases []map[string]any
	err := tx.QueryRow(ctx, `select coalesce(jsonb_agg(to_jsonb(p) order by p.id), '[]')
		from purchases p where p.student_id = $1`, id).Scan(&purchases)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `delete from students where id = $1`, id); err != nil {
		return 0, err
	}
	return a.auditAdmin(ctx, tx, adminID, AuditEntry{
		Action:   "delete",
		Entity:   "student",
		EntityID: itoa64(id),
		Summary:  "Alumno eliminado: " + admStr(row, "code") + " – " + admStr(row, "name"),
		Before:   map[string]any{"row": row, "purchases": purchases},
		BatchID:  batchID,
		Undoable: true,
	})
}

// admStudentInput es StudentInput de types.ts; los punteros distinguen "no viene" de "vacío".
type admStudentInput struct {
	CourseID    *int64  `json:"courseId"`
	Code        *string `json:"code"`
	Name        *string `json:"name"`
	Sexo        *string `json:"sexo"`
	Email       *string `json:"email"`
	Tareas      *int    `json:"tareas"`
	Monedas     *int    `json:"monedas"`
	Asistencias *int    `json:"asistencias"`
	Games       *int    `json:"games"`
	Exp         *int    `json:"exp"`
	Comments    *string `json:"comments"`
	Active      *bool   `json:"active"`
	Password    *string `json:"password"` // solo al crear
}

// admStudentSet valida los campos que vienen y los devuelve como columna → valor.
// current es la fila actual (nil al crear): un ID, nombre, sexo o correo
// importado raro que no cambia se respeta, para no impedir guardar lo demás.
func admStudentSet(in admStudentInput, current map[string]any) (map[string]any, error) {
	bad := func(msg string) (map[string]any, error) { return nil, admBad(http.StatusBadRequest, msg) }
	set := map[string]any{}
	if in.CourseID != nil {
		set["course_id"] = *in.CourseID // que exista se revisa dentro de la transacción
	}
	if in.Code != nil {
		code := strings.TrimSpace(*in.Code)
		switch {
		case code == "":
			return bad("Falta el ID del alumno.")
		case current != nil && code == admStr(current, "code"):
		case !codeRe.MatchString(code):
			return bad(msgBadCode)
		case strings.EqualFold(code, "DefaultH") || strings.EqualFold(code, "DefaultF"):
			return bad("Ese ID está reservado para los avatares predeterminados.")
		}
		set["code"] = code
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return bad("Falta el nombre del alumno.")
		}
		if utf8.RuneCountInString(name) > 200 {
			return bad("El nombre es demasiado largo (máximo 200 caracteres).")
		}
		if strings.ContainsFunc(name, badNameRune) && (current == nil || name != admStr(current, "name")) {
			return bad(msgBadName)
		}
		set["name"] = name
	}
	if in.Sexo != nil {
		switch v := strings.TrimSpace(*in.Sexo); {
		case strings.EqualFold(v, "Hombre"):
			set["sexo"] = "Hombre"
		case strings.EqualFold(v, "Mujer"):
			set["sexo"] = "Mujer"
		case current != nil && v == admStr(current, "sexo"):
		default:
			return bad("Sexo inválido. Usa Hombre o Mujer.")
		}
	}
	if in.Email != nil {
		email := strings.TrimSpace(*in.Email)
		if email != "" && !admPlainEmail(email) && (current == nil || email != admStr(current, "email")) {
			return bad("Correo inválido.")
		}
		set["email"] = email
	}
	for col, v := range map[string]*int{"tareas": in.Tareas, "monedas": in.Monedas,
		"asistencias": in.Asistencias, "games": in.Games, "exp": in.Exp} {
		if v == nil {
			continue
		}
		if !admCounterOK(*v) {
			return bad(admCounterMsg)
		}
		set[col] = *v
	}
	if in.Comments != nil {
		c := strings.TrimSpace(*in.Comments)
		if utf8.RuneCountInString(c) > 4000 {
			return bad("Los comentarios son demasiado largos (máximo 4000 caracteres).")
		}
		set["comments"] = c
	}
	if in.Active != nil {
		set["active"] = *in.Active
	}
	return set, nil
}

// admStudentPassword revisa la contraseña dada contra la regla de Ajustes o, si
// no viene, genera una. generated dice si hay que mostrársela al admin.
func admStudentPassword(given *string, pattern string) (pw string, generated bool, err error) {
	if given == nil || strings.TrimSpace(*given) == "" {
		pw, err := generatePassword(pattern)
		if err != nil {
			return "", false, admBad(http.StatusBadRequest,
				"La regla de contraseña de Ajustes no admite contraseñas generadas (2 letras y 2 números); escribe una a mano.")
		}
		return pw, true, nil
	}
	pw = strings.TrimSpace(*given)
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", false, err
	}
	if !re.MatchString(pw) || len(pw) > 72 { // bcrypt no acepta más de 72 bytes
		return "", false, admBad(http.StatusBadRequest, "La contraseña no cumple la regla de Ajustes ("+pattern+").")
	}
	return pw, false, nil
}

// admNewStudent valida un alumno nuevo, pone los predeterminados (sexo, correo
// como en el registro) y resuelve la contraseña. El hash lo saca quien llama.
func admNewStudent(in admStudentInput, s Settings) (set map[string]any, pw string, generated bool, err error) {
	if in.Sexo == nil || strings.TrimSpace(*in.Sexo) == "" {
		hombre := "Hombre"
		in.Sexo = &hombre
	}
	if set, err = admStudentSet(in, nil); err != nil {
		return nil, "", false, err
	}
	if set["email"] == nil || set["email"] == "" {
		set["email"] = set["code"].(string) + "@" + s.StudentEmailDomain
	}
	pw, generated, err = admStudentPassword(in.Password, s.PasswordPattern)
	return set, pw, generated, err
}

func (a *App) handleAdminStudentCreate(w http.ResponseWriter, r *http.Request, adminID int64) {
	var in admStudentInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.CourseID == nil || in.Code == nil || in.Name == nil {
		writeError(w, http.StatusBadRequest, "Curso, ID y nombre son obligatorios.")
		return
	}
	ctx := r.Context()
	settings, err := a.Settings(ctx)
	if err != nil {
		a.admFail(w, "leer ajustes", err)
		return
	}
	set, pw, generated, err := admNewStudent(in, settings)
	if err != nil {
		a.admFail(w, "crear alumno", err)
		return
	}
	hash, err := a.hashPasswordCtx(ctx, pw)
	if err != nil {
		a.admFail(w, "crear alumno", err)
		return
	}
	set["password_hash"] = hash

	var id, auditID int64
	ok := a.admTx(w, r, "crear alumno", func(tx pgx.Tx) error {
		course, err := admCourseName(ctx, tx, *in.CourseID)
		if err != nil {
			return err
		}
		var created bool
		id, auditID, created, err = a.admInsertStudent(ctx, tx, adminID, set, course, "")
		if err == nil && !created {
			err = admBad(http.StatusConflict, "El ID de alumno ya existe.")
		}
		return err
	})
	if !ok {
		return
	}
	st, err := a.admStudentByID(ctx, id)
	if err != nil {
		a.admFail(w, "leer alumno", err)
		return
	}
	resp := map[string]any{"student": st, "auditId": auditID}
	if generated {
		resp["password"] = pw
	}
	writeJSON(w, http.StatusCreated, resp)
}

type admSkip struct {
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

type admCreated struct {
	Student  adminStudentJSON `json:"student"`
	Password string           `json:"password"`
}

// handleAdminStudentsGenerate crea hasta 50 alumnos por petición. Las filas
// inválidas o repetidas se saltan con su motivo; el resto entra en una sola
// transacción y bajo un mismo batchId.
func (a *App) handleAdminStudentsGenerate(w http.ResponseWriter, r *http.Request, adminID int64) {
	var body struct {
		CourseID int64 `json:"courseId"`
		Rows     []struct {
			Code     string `json:"code"`
			Name     string `json:"name"`
			Sexo     string `json:"sexo"`
			Email    string `json:"email"`
			Password string `json:"password"`
		} `json:"rows"`
		BatchID string `json:"batchId"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if len(body.Rows) == 0 || len(body.Rows) > 50 {
		writeError(w, http.StatusBadRequest, "Manda entre 1 y 50 filas por solicitud.")
		return
	}
	batchID, err := admBatchID(body.BatchID)
	if err != nil {
		a.admFail(w, "generar alumnos", err)
		return
	}
	ctx := r.Context()
	course, err := admCourseName(ctx, a.DB, body.CourseID)
	if err != nil {
		a.admFail(w, "generar alumnos", err)
		return
	}
	settings, err := a.Settings(ctx)
	if err != nil {
		a.admFail(w, "leer ajustes", err)
		return
	}

	// Los que ya están en el curso se saltan antes de gastar bcrypt en ellos.
	codes := make([]string, len(body.Rows))
	for i, row := range body.Rows {
		codes[i] = strings.TrimSpace(row.Code)
	}
	rows, err := a.DB.Query(ctx, `select code from students where course_id = $1 and code = any($2)`, body.CourseID, codes)
	if err != nil {
		a.admFail(w, "generar alumnos", err)
		return
	}
	existing, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		a.admFail(w, "generar alumnos", err)
		return
	}
	taken := map[string]bool{}
	for _, c := range existing {
		taken[c] = true
	}

	type pending struct {
		set      map[string]any
		code, pw string
		err      error
	}
	skipped := []admSkip{}
	seen := map[string]bool{}
	var todo []*pending
	for i, row := range body.Rows {
		code := codes[i]
		in := admStudentInput{Code: &code, Name: &row.Name, Sexo: &row.Sexo, Email: &row.Email, Password: &row.Password}
		set, pw, _, err := admNewStudent(in, settings)
		var he *admErr
		switch {
		case errors.As(err, &he):
			skipped = append(skipped, admSkip{code, he.msg})
			continue
		case err != nil:
			a.admFail(w, "generar alumnos", err)
			return
		case taken[code]:
			skipped = append(skipped, admSkip{code, "El ID ya existe en este curso."})
			continue
		case seen[code]:
			skipped = append(skipped, admSkip{code, "ID repetido en la lista."})
			continue
		}
		seen[code] = true
		set["course_id"] = body.CourseID
		todo = append(todo, &pending{set: set, code: code, pw: pw})
	}

	// bcrypt tarda; en paralelo (con los turnos de la app), 50 filas no se
	// vuelven varios segundos.
	var wg sync.WaitGroup
	for _, p := range todo {
		wg.Go(func() {
			var hash string
			hash, p.err = a.hashPasswordCtx(ctx, p.pw)
			p.set["password_hash"] = hash
		})
	}
	wg.Wait()
	for _, p := range todo {
		if p.err != nil {
			a.admFail(w, "generar alumnos", p.err)
			return
		}
	}

	var ids []int64
	var pws []string
	ok := a.admTx(w, r, "generar alumnos", func(tx pgx.Tx) error {
		for _, p := range todo {
			id, _, created, err := a.admInsertStudent(ctx, tx, adminID, p.set, course, batchID)
			if err != nil {
				return err
			}
			if !created { // alguien lo creó entre la revisión y ahora
				skipped = append(skipped, admSkip{p.code, "El ID ya existe en este curso."})
				continue
			}
			ids = append(ids, id)
			pws = append(pws, p.pw)
		}
		return nil
	})
	if !ok {
		return
	}
	created := []admCreated{}
	if len(ids) > 0 {
		list, err := a.admStudents(ctx, a.DB, "s.id = any($1)", ids)
		if err != nil {
			a.admFail(w, "leer alumnos", err)
			return
		}
		byID := map[int64]adminStudentJSON{}
		for _, s := range list {
			byID[s.ID] = s
		}
		for i, id := range ids {
			created = append(created, admCreated{byID[id], pws[i]})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"created": created, "skipped": skipped, "batchId": batchID})
}

func (a *App) handleAdminStudentUpdate(w http.ResponseWriter, r *http.Request, adminID int64) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusNotFound, "Alumno no encontrado.")
		return
	}
	var in admStudentInput
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	var auditID int64
	ok = a.admTx(w, r, "editar alumno", func(tx pgx.Tx) error {
		before, err := admLockStudent(ctx, tx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return admBad(http.StatusNotFound, "Alumno no encontrado.")
		}
		if err != nil {
			return err
		}
		set, err := admStudentSet(in, before)
		if err != nil {
			return err
		}
		if len(set) == 0 {
			return admBad(http.StatusBadRequest, "No hay cambios que guardar.")
		}
		if cid, ok := set["course_id"].(int64); ok {
			if _, err := admCourseName(ctx, tx, cid); err != nil {
				return err
			}
		}
		_, auditID, err = a.admApplyStudent(ctx, tx, adminID, id, before, set,
			"Alumno "+admStr(before, "code")+" editado: "+admChanges(before, set), "")
		return err
	})
	if !ok {
		return
	}
	st, err := a.admStudentByID(ctx, id)
	if err != nil {
		a.admFail(w, "leer alumno", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"student": st, "auditId": auditID})
}

func (a *App) handleAdminStudentDelete(w http.ResponseWriter, r *http.Request, adminID int64) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusNotFound, "Alumno no encontrado.")
		return
	}
	ctx := r.Context()
	var auditID int64
	ok = a.admTx(w, r, "eliminar alumno", func(tx pgx.Tx) error {
		row, err := admLockStudent(ctx, tx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return admBad(http.StatusNotFound, "Alumno no encontrado.")
		}
		if err != nil {
			return err
		}
		auditID, err = a.admDeleteStudent(ctx, tx, adminID, row, "")
		return err
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "auditId": auditID})
}

// handleAdminStudentPassword fija la contraseña dada o genera una; la respuesta
// es la única vez que se ve en claro.
func (a *App) handleAdminStudentPassword(w http.ResponseWriter, r *http.Request, adminID int64) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusNotFound, "Alumno no encontrado.")
		return
	}
	var body struct {
		Password *string `json:"password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	ctx := r.Context()
	settings, err := a.Settings(ctx)
	if err != nil {
		a.admFail(w, "leer ajustes", err)
		return
	}
	pw, _, err := admStudentPassword(body.Password, settings.PasswordPattern)
	if err != nil {
		a.admFail(w, "cambiar contraseña", err)
		return
	}
	// La contraseña nueva cambia pwVersion: las sesiones abiertas del alumno caen.
	hash, err := a.hashPasswordCtx(ctx, pw)
	if err != nil {
		a.admFail(w, "cambiar contraseña", err)
		return
	}
	var auditID int64
	ok = a.admTx(w, r, "cambiar contraseña", func(tx pgx.Tx) error {
		before, err := admLockStudent(ctx, tx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return admBad(http.StatusNotFound, "Alumno no encontrado.")
		}
		if err != nil {
			return err
		}
		_, auditID, err = a.admApplyStudent(ctx, tx, adminID, id, before, map[string]any{"password_hash": hash},
			"Contraseña nueva para "+admStr(before, "code"), "")
		return err
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"password": pw, "auditId": auditID})
}

// handleAdminStudentsBulk aplica una acción a varios alumnos. Cada alumno que
// cambia deja su propia entrada en la bitácora, todas con el mismo batchId.
func (a *App) handleAdminStudentsBulk(w http.ResponseWriter, r *http.Request, adminID int64) {
	var body struct {
		IDs      []int64        `json:"ids"`
		Action   string         `json:"action"`
		Delta    map[string]int `json:"delta"`
		Set      map[string]int `json:"set"`
		CourseID int64          `json:"courseId"`
		BatchID  string         `json:"batchId"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if len(body.IDs) == 0 || len(body.IDs) > 500 {
		writeError(w, http.StatusBadRequest, "Selecciona entre 1 y 500 alumnos.")
		return
	}
	for _, id := range body.IDs {
		if id <= 0 {
			writeError(w, http.StatusBadRequest, "Lista de alumnos inválida.")
			return
		}
	}
	batchID, err := admBatchID(body.BatchID)
	if err != nil {
		a.admFail(w, "acción masiva", err)
		return
	}
	values := body.Delta
	switch body.Action {
	case "delta", "set":
		if body.Action == "set" {
			values = body.Set
		}
		if len(values) == 0 {
			writeError(w, http.StatusBadRequest, "Indica al menos un contador.")
			return
		}
		for k, v := range values {
			if !slices.Contains(admCounters, k) {
				writeError(w, http.StatusBadRequest, "Contador desconocido: "+k+".")
				return
			}
			if (body.Action == "set" && !admCounterOK(v)) || v < -math.MaxInt32 || v > math.MaxInt32 {
				writeError(w, http.StatusBadRequest, admCounterMsg)
				return
			}
		}
	case "activate", "deactivate", "delete":
	case "move":
		if body.CourseID <= 0 {
			writeError(w, http.StatusBadRequest, "Elige el curso de destino.")
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "Acción no válida.")
		return
	}

	ctx := r.Context()
	updated := 0
	ok := a.admTx(w, r, "acción masiva", func(tx pgx.Tx) error {
		updated = 0
		dest := ""
		if body.Action == "move" {
			var err error
			if dest, err = admCourseName(ctx, tx, body.CourseID); err != nil {
				return err
			}
		}
		rows, err := tx.Query(ctx, `select to_jsonb(s) from students s where s.id = any($1) order by s.id for update`, body.IDs)
		if err != nil {
			return err
		}
		students, err := pgx.CollectRows(rows, pgx.RowTo[map[string]any])
		if err != nil {
			return err
		}
		for _, before := range students {
			id, code := admNum(before, "id"), admStr(before, "code")
			if body.Action == "delete" {
				if _, err := a.admDeleteStudent(ctx, tx, adminID, before, batchID); err != nil {
					return err
				}
				updated++
				continue
			}
			set := map[string]any{}
			switch body.Action {
			case "delta": // nunca por debajo de 0
				for k, d := range values {
					set[k] = min(max(admNum(before, k)+int64(d), 0), math.MaxInt32)
				}
			case "set":
				for k, v := range values {
					set[k] = int64(v)
				}
			case "activate", "deactivate":
				set["active"] = body.Action == "activate"
			case "move": // si el ID ya existe en el curso destino se salta
				var taken bool
				err := tx.QueryRow(ctx, `select exists (select 1 from students where course_id = $1 and code = $2)`,
					body.CourseID, code).Scan(&taken)
				if err != nil {
					return err
				}
				if !taken {
					set["course_id"] = body.CourseID
				}
			}
			for k, v := range set {
				if admSame(before[k], v) {
					delete(set, k)
				}
			}
			if len(set) == 0 {
				continue
			}
			summary := "Acción masiva, " + code + ": " + admChanges(before, set)
			if body.Action == "move" {
				summary = "Alumno " + code + " movido a " + dest
			}
			if _, _, err := a.admApplyStudent(ctx, tx, adminID, id, before, set, summary, batchID); err != nil {
				return err
			}
			updated++
		}
		return nil
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": updated, "batchId": batchID})
}
