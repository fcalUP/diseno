package app

import (
	"context"
	"errors"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// Cursos, insignias, visibilidad, niveles, avatares y el resumen del panel (spec §4.3).
// Cada cambio queda en la bitácora con las instantáneas del spec §5.6.

// catFail responde un error de validación o de restricción de la base; lo demás es 500.
func (a *App) catFail(w http.ResponseWriter, what string, err error) {
	a.undoWriteError(w, what, undoDBError(err))
}

// catTx corre fn en una transacción y la confirma si no hubo error. Para cortar con
// un 400/404 dentro de fn basta devolver undoFail.
func (a *App) catTx(ctx context.Context, fn func(tx pgx.Tx) (any, error)) (any, error) {
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	body, err := fn(tx)
	if err == nil {
		err = tx.Commit(ctx)
	}
	return body, err
}

// catRespond escribe el resultado de catTx.
func (a *App) catRespond(w http.ResponseWriter, what string, status int, body any, err error) {
	if err != nil {
		a.catFail(w, what, err)
		return
	}
	writeJSON(w, status, body)
}

func catBad(msg string) error { return undoFail(http.StatusBadRequest, msg) }

// catCheckImage valida una url de imagen: vacía (si se permite) o /media/...; las
// subidas (/media/u/) deben existir en el almacenamiento.
func (a *App) catCheckImage(ctx context.Context, raw string, required bool) (string, error) {
	u := strings.TrimSpace(raw)
	if u == "" {
		if required {
			return "", catBad("Falta la imagen.")
		}
		return "", nil
	}
	key, ok := mediaKey(u)
	if !ok {
		return "", catBad("La imagen debe ser una ruta /media/ válida.")
	}
	if strings.HasPrefix(key, "u/") {
		if _, err := a.Store.Stat(ctx, key); errors.Is(err, ErrNotFound) {
			return "", catBad("La imagen no existe; súbela de nuevo.")
		} else if err != nil {
			return "", err
		}
	}
	return u, nil
}

// catImages valida las urls de imagen que trae el cuerpo, antes de abrir la transacción.
func (a *App) catImages(ctx context.Context, urls ...*string) error {
	for _, p := range urls {
		if p == nil {
			continue
		}
		clean, err := a.catCheckImage(ctx, *p, false)
		if err != nil {
			return err
		}
		*p = clean
	}
	return nil
}

// catList recorta, quita vacíos y repetidos, y revisa tamaños.
func catList(in []string, what string) ([]string, error) {
	out := []string{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || slices.Contains(out, s) {
			continue
		}
		if utf8.RuneCountInString(s) > 120 {
			return nil, catBad(what + ": cada valor debe tener 120 caracteres o menos.")
		}
		out = append(out, s)
	}
	if len(out) > 50 {
		return nil, catBad(what + ": máximo 50 valores.")
	}
	return out, nil
}

func catSortOrderOK(n *int) bool {
	return n == nil || (*n >= math.MinInt32 && *n <= math.MaxInt32)
}

// catCheckIDs ordena, quita repetidos y exige que todos los ids existan en table
// (texto fijo del código); si no, devuelve un 400 con msg.
func catCheckIDs(ctx context.Context, q DBTX, table string, in []int64, msg string) ([]int64, error) {
	ids := append([]int64{}, in...)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) > 1000 || (len(ids) > 0 && ids[0] <= 0) {
		return nil, catBad(msg)
	}
	found, err := undoIDList(ctx, q, "select id from "+table+" where id = any($1)", ids)
	if err != nil {
		return nil, err
	}
	if len(found) != len(ids) {
		return nil, catBad(msg)
	}
	return ids, nil
}

// catLock bloquea la fila id de table y devuelve su nombre; 404 con msg si no existe.
// Para editar basta "for no key update", que no frena a las llaves foráneas; para
// borrar va "for update".
func catLock(ctx context.Context, tx pgx.Tx, table, lock string, id int64, msg string) (string, error) {
	var name string
	err := tx.QueryRow(ctx, "select name from "+table+" where id = $1 "+lock, id).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", undoFail(http.StatusNotFound, msg)
	}
	return name, err
}

// ---------- Visibilidad (course_hidden_badges) ----------

func catCourseHidden(ctx context.Context, q DBTX, courseID int64) ([]int64, error) {
	return undoIDList(ctx, q, `select badge_id from course_hidden_badges where course_id = $1 order by badge_id`, courseID)
}

func catBadgeHidden(ctx context.Context, q DBTX, badgeID int64) ([]int64, error) {
	return undoIDList(ctx, q, `select course_id from course_hidden_badges where badge_id = $1 order by course_id`, badgeID)
}

// catSetCourseHidden deja ocultas en el curso exactamente esas insignias (las que existan).
func catSetCourseHidden(ctx context.Context, q DBTX, courseID int64, badgeIDs []int64) error {
	if _, err := q.Exec(ctx, `delete from course_hidden_badges where course_id = $1`, courseID); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `insert into course_hidden_badges (course_id, badge_id)
		select $1, b.id from badges b where b.id = any($2)`, courseID, badgeIDs)
	return err
}

// catSetBadgeHidden deja la insignia oculta exactamente en esos cursos (los que existan).
func catSetBadgeHidden(ctx context.Context, q DBTX, badgeID int64, courseIDs []int64) error {
	if _, err := q.Exec(ctx, `delete from course_hidden_badges where badge_id = $1`, badgeID); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `insert into course_hidden_badges (course_id, badge_id)
		select c.id, $1 from courses c where c.id = any($2)`, badgeID, courseIDs)
	return err
}

// ---------- Cursos ----------

const catCourseCols = `c.id, c.name, c.sheet_name, c.sheet_aliases, c.side_image_url, c.logo_url,
	c.hidden_badge_keywords, c.active, c.sort_order,
	(select count(*) from students s where s.course_id = c.id),
	coalesce((select array_agg(h.badge_id order by h.badge_id) from course_hidden_badges h
		where h.course_id = c.id), '{}'),
	c.created_at, c.updated_at`

func catScanCourse(row pgx.Row) (adminCourseJSON, error) {
	var c adminCourseJSON
	err := row.Scan(&c.ID, &c.Name, &c.SheetName, &c.SheetAliases, &c.SideImageURL, &c.LogoURL,
		&c.HiddenBadgeKeywords, &c.Active, &c.SortOrder, &c.StudentCount, &c.HiddenBadgeIDs,
		&c.CreatedAt, &c.UpdatedAt)
	return c, err
}

// catCourseBody arma la respuesta {course, auditId} con el curso ya guardado.
func catCourseBody(ctx context.Context, q DBTX, id, auditID int64) (any, error) {
	course, err := catScanCourse(q.QueryRow(ctx, `select `+catCourseCols+` from courses c where c.id = $1`, id))
	return map[string]any{"course": course, "auditId": auditID}, err
}

type catCourseInput struct {
	Name                *string   `json:"name"`
	SheetName           *string   `json:"sheetName"`
	SheetAliases        *[]string `json:"sheetAliases"`
	SideImageURL        *string   `json:"sideImageUrl"`
	LogoURL             *string   `json:"logoUrl"`
	HiddenBadgeKeywords *[]string `json:"hiddenBadgeKeywords"`
	Active              *bool     `json:"active"`
	SortOrder           *int      `json:"sortOrder"`
}

type catCourseRow struct {
	Name, SheetName       string
	SheetAliases          []string
	SideImageURL, LogoURL string
	HiddenBadgeKeywords   []string
	Active                bool
	SortOrder             int
}

// catApplyCourse copia al curso los campos presentes y lo valida. Las imágenes ya
// vienen revisadas por catImages.
func catApplyCourse(in catCourseInput, c *catCourseRow) error {
	if in.Name != nil {
		c.Name = strings.TrimSpace(*in.Name)
	}
	if in.SheetName != nil {
		c.SheetName = strings.TrimSpace(*in.SheetName)
	}
	switch {
	case c.Name == "":
		return catBad("El nombre del curso es obligatorio.")
	case utf8.RuneCountInString(c.Name) > 120:
		return catBad("El nombre del curso es demasiado largo (máximo 120 caracteres).")
	case c.SheetName == "":
		return catBad("La hoja de Sheets es obligatoria.")
	case utf8.RuneCountInString(c.SheetName) > 100:
		return catBad("El nombre de la hoja es demasiado largo (máximo 100 caracteres).")
	case !catSortOrderOK(in.SortOrder):
		return catBad("El orden no es válido.")
	}
	var err error
	if in.SheetAliases != nil {
		if c.SheetAliases, err = catList(*in.SheetAliases, "Alias de la hoja"); err != nil {
			return err
		}
	}
	if in.HiddenBadgeKeywords != nil {
		if c.HiddenBadgeKeywords, err = catList(*in.HiddenBadgeKeywords, "Palabras para ocultar insignias"); err != nil {
			return err
		}
	}
	if in.SideImageURL != nil {
		c.SideImageURL = *in.SideImageURL
	}
	if in.LogoURL != nil {
		c.LogoURL = *in.LogoURL
	}
	if in.Active != nil {
		c.Active = *in.Active
	}
	if in.SortOrder != nil {
		c.SortOrder = *in.SortOrder
	}
	return nil
}

func (a *App) handleAdminCoursesList(w http.ResponseWriter, r *http.Request, adminID int64) {
	rows, err := a.DB.Query(r.Context(), `select `+catCourseCols+` from courses c order by c.sort_order, c.id`)
	if err != nil {
		a.catFail(w, "listar cursos", err)
		return
	}
	courses, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (adminCourseJSON, error) {
		return catScanCourse(row)
	})
	if courses == nil {
		courses = []adminCourseJSON{}
	}
	a.catRespond(w, "listar cursos", http.StatusOK, map[string]any{"courses": courses}, err)
}

func (a *App) handleAdminCoursesCreate(w http.ResponseWriter, r *http.Request, adminID int64) {
	ctx := r.Context()
	var in catCourseInput
	if !decodeJSON(w, r, &in) {
		return
	}
	// Sin hoja propia, la hoja se llama como el curso.
	if in.SheetName == nil || strings.TrimSpace(*in.SheetName) == "" {
		in.SheetName = in.Name
	}
	c := catCourseRow{Active: true, SheetAliases: []string{}, HiddenBadgeKeywords: []string{}}
	err := a.catImages(ctx, in.SideImageURL, in.LogoURL)
	if err == nil {
		err = catApplyCourse(in, &c)
	}
	if err != nil {
		a.catFail(w, "crear curso", err)
		return
	}
	body, err := a.catTx(ctx, func(tx pgx.Tx) (any, error) {
		if in.SortOrder == nil {
			if err := tx.QueryRow(ctx, `select coalesce(max(sort_order), 0) + 1 from courses`).Scan(&c.SortOrder); err != nil {
				return nil, err
			}
		}
		var id int64
		if err := tx.QueryRow(ctx, `insert into courses (name, sheet_name, sheet_aliases, side_image_url, logo_url,
			hidden_badge_keywords, active, sort_order) values ($1, $2, $3, $4, $5, $6, $7, $8) returning id`,
			c.Name, c.SheetName, c.SheetAliases, c.SideImageURL, c.LogoURL, c.HiddenBadgeKeywords, c.Active,
			c.SortOrder).Scan(&id); err != nil {
			return nil, err
		}
		after, err := snapshotRow(ctx, tx, "courses", id)
		if err != nil {
			return nil, err
		}
		auditID, err := a.auditAdmin(ctx, tx, adminID, AuditEntry{
			Action: "create", Entity: "course", EntityID: itoa64(id), Summary: "Curso creado: " + c.Name,
			After: after, Undoable: true,
		})
		if err != nil {
			return nil, err
		}
		return catCourseBody(ctx, tx, id, auditID)
	})
	a.catRespond(w, "crear curso", http.StatusCreated, body, err)
}

func (a *App) handleAdminCoursesUpdate(w http.ResponseWriter, r *http.Request, adminID int64) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusNotFound, "Curso no encontrado.")
		return
	}
	var in catCourseInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if in == (catCourseInput{}) {
		writeError(w, http.StatusBadRequest, "No hay cambios que guardar.")
		return
	}
	if err := a.catImages(ctx, in.SideImageURL, in.LogoURL); err != nil {
		a.catFail(w, "actualizar curso", err)
		return
	}
	body, err := a.catTx(ctx, func(tx pgx.Tx) (any, error) {
		var c catCourseRow
		err := tx.QueryRow(ctx, `select name, sheet_name, sheet_aliases, side_image_url, logo_url,
			hidden_badge_keywords, active, sort_order from courses where id = $1 for no key update`, id).Scan(
			&c.Name, &c.SheetName, &c.SheetAliases, &c.SideImageURL, &c.LogoURL, &c.HiddenBadgeKeywords,
			&c.Active, &c.SortOrder)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, undoFail(http.StatusNotFound, "Curso no encontrado.")
		}
		if err != nil {
			return nil, err
		}
		if err := catApplyCourse(in, &c); err != nil {
			return nil, err
		}
		before, err := snapshotRow(ctx, tx, "courses", id)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `update courses set name = $2, sheet_name = $3, sheet_aliases = $4,
			side_image_url = $5, logo_url = $6, hidden_badge_keywords = $7, active = $8, sort_order = $9,
			updated_at = now() where id = $1`,
			id, c.Name, c.SheetName, c.SheetAliases, c.SideImageURL, c.LogoURL, c.HiddenBadgeKeywords,
			c.Active, c.SortOrder); err != nil {
			return nil, err
		}
		after, err := snapshotRow(ctx, tx, "courses", id)
		if err != nil {
			return nil, err
		}
		auditID, err := a.auditAdmin(ctx, tx, adminID, AuditEntry{
			Action: "update", Entity: "course", EntityID: itoa64(id), Summary: "Curso actualizado: " + c.Name,
			Before: before, After: after, Undoable: true,
		})
		if err != nil {
			return nil, err
		}
		return catCourseBody(ctx, tx, id, auditID)
	})
	a.catRespond(w, "actualizar curso", http.StatusOK, body, err)
}

func (a *App) handleAdminCoursesDelete(w http.ResponseWriter, r *http.Request, adminID int64) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusNotFound, "Curso no encontrado.")
		return
	}
	body, err := a.catTx(ctx, func(tx pgx.Tx) (any, error) {
		// "for update" frena altas de alumnos en este curso mientras se borra.
		name, err := catLock(ctx, tx, "courses", "for update", id, "Curso no encontrado.")
		if err != nil {
			return nil, err
		}
		var students int
		if err := tx.QueryRow(ctx, `select count(*) from students where course_id = $1`, id).Scan(&students); err != nil {
			return nil, err
		}
		if students > 0 {
			return nil, undoFail(http.StatusConflict, "El curso tiene alumnos. Muévelos o desactiva el curso.")
		}
		row, err := snapshotRow(ctx, tx, "courses", id)
		if err != nil {
			return nil, err
		}
		hidden, err := catCourseHidden(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `delete from courses where id = $1`, id); err != nil {
			return nil, err
		}
		auditID, err := a.auditAdmin(ctx, tx, adminID, AuditEntry{
			Action: "delete", Entity: "course", EntityID: itoa64(id), Summary: "Curso eliminado: " + name,
			Before: map[string]any{"row": row, "hidden_badge_ids": hidden}, Undoable: true,
		})
		return map[string]any{"success": true, "auditId": auditID}, err
	})
	a.catRespond(w, "eliminar curso", http.StatusOK, body, err)
}

func (a *App) handleAdminCoursesHiddenBadges(w http.ResponseWriter, r *http.Request, adminID int64) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusNotFound, "Curso no encontrado.")
		return
	}
	var in struct {
		BadgeIDs *[]int64 `json:"badgeIds"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.BadgeIDs == nil {
		writeError(w, http.StatusBadRequest, "Falta la lista de insignias ocultas (badgeIds).")
		return
	}
	body, err := a.catTx(ctx, func(tx pgx.Tx) (any, error) {
		name, err := catLock(ctx, tx, "courses", "for no key update", id, "Curso no encontrado.")
		if err != nil {
			return nil, err
		}
		ids, err := catCheckIDs(ctx, tx, "badges", *in.BadgeIDs, "Alguna de las insignias no existe.")
		if err != nil {
			return nil, err
		}
		before, err := catCourseHidden(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if err := catSetCourseHidden(ctx, tx, id, ids); err != nil {
			return nil, err
		}
		auditID, err := a.auditAdmin(ctx, tx, adminID, AuditEntry{
			Action: "update", Entity: "visibility", EntityID: itoa64(id),
			Summary:  "Insignias ocultas actualizadas en " + name,
			Before:   map[string]any{"course_id": id, "badge_ids": before},
			After:    map[string]any{"course_id": id, "badge_ids": ids},
			Undoable: true,
		})
		if err != nil {
			return nil, err
		}
		return catCourseBody(ctx, tx, id, auditID)
	})
	a.catRespond(w, "ocultar insignias", http.StatusOK, body, err)
}

func (a *App) handleAdminCoursesReorder(w http.ResponseWriter, r *http.Request, adminID int64) {
	a.catReorder(w, r, adminID, "courses", "course", "Curso movido")
}

// catReorder aplica el orden recibido (sort_order = posición) y deja una entrada por
// cada fila que se movió, todas en un mismo lote. table viene del código, no del cliente.
func (a *App) catReorder(w http.ResponseWriter, r *http.Request, adminID int64, table, entity, label string) {
	ctx := r.Context()
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if len(in.IDs) == 0 || len(in.IDs) > 1000 {
		writeError(w, http.StatusBadRequest, "La lista de ids debe tener entre 1 y 1000 elementos.")
		return
	}
	if sorted := slices.Sorted(slices.Values(in.IDs)); len(slices.Compact(sorted)) != len(in.IDs) {
		writeError(w, http.StatusBadRequest, "La lista tiene ids repetidos.")
		return
	}
	body, err := a.catTx(ctx, func(tx pgx.Tx) (any, error) {
		found, err := undoIDList(ctx, tx, "select id from "+table+" where id = any($1) for no key update", in.IDs)
		if err != nil {
			return nil, err
		}
		if len(found) != len(in.IDs) {
			return nil, catBad("Alguno de los elementos no existe.")
		}
		batch := newBatchID()
		for i, id := range in.IDs {
			pos := i + 1
			before, err := snapshotRow(ctx, tx, table, id)
			if err != nil {
				return nil, err
			}
			if n, _ := undoInt(before["sort_order"]); n == int64(pos) {
				continue
			}
			if _, err := tx.Exec(ctx, "update "+table+" set sort_order = $2, updated_at = now() where id = $1", id, pos); err != nil {
				return nil, err
			}
			after, err := snapshotRow(ctx, tx, table, id)
			if err != nil {
				return nil, err
			}
			name, _ := before["name"].(string)
			if _, err := a.auditAdmin(ctx, tx, adminID, AuditEntry{
				Action: "update", Entity: entity, EntityID: itoa64(id),
				Summary: label + " a la posición " + strconv.Itoa(pos) + ": " + name,
				Before:  before, After: after, BatchID: batch, Undoable: true,
			}); err != nil {
				return nil, err
			}
		}
		return map[string]any{"success": true, "batchId": batch}, nil
	})
	a.catRespond(w, "reordenar", http.StatusOK, body, err)
}

// ---------- Insignias ----------

const catBadgeCols = `b.id, b.name, b.slug, b.stock, b.cost, b.image_url, b.active, b.sort_order,
	coalesce((select array_agg(h.course_id order by h.course_id) from course_hidden_badges h
		where h.badge_id = b.id), '{}'),
	coalesce((select sum(p.quantity) from purchases p where p.badge_id = b.id and p.reverted_at is null), 0),
	b.created_at, b.updated_at`

func catScanBadge(row pgx.Row) (adminBadgeJSON, error) {
	var b adminBadgeJSON
	err := row.Scan(&b.ID, &b.Name, &b.Slug, &b.Stock, &b.Cost, &b.ImageURL, &b.Active, &b.SortOrder,
		&b.HiddenCourseIDs, &b.PurchasedCount, &b.CreatedAt, &b.UpdatedAt)
	return b, err
}

// catBadgeBody arma la respuesta {badge, auditId} con la insignia ya guardada.
func catBadgeBody(ctx context.Context, q DBTX, id, auditID int64) (any, error) {
	badge, err := catScanBadge(q.QueryRow(ctx, `select `+catBadgeCols+` from badges b where b.id = $1`, id))
	return map[string]any{"badge": badge, "auditId": auditID}, err
}

// catBadges lista insignias; where y order son texto fijo del código.
func catBadges(ctx context.Context, q DBTX, where, order string) ([]adminBadgeJSON, error) {
	rows, err := q.Query(ctx, `select `+catBadgeCols+` from badges b `+where+` order by `+order)
	if err != nil {
		return nil, err
	}
	badges, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (adminBadgeJSON, error) {
		return catScanBadge(row)
	})
	if badges == nil {
		badges = []adminBadgeJSON{}
	}
	return badges, err
}

// catBadgeSnapshot es la fila de la insignia más los cursos donde está oculta, así
// deshacer una edición también regresa la visibilidad.
func catBadgeSnapshot(ctx context.Context, q DBTX, id int64) (map[string]any, error) {
	row, err := snapshotRow(ctx, q, "badges", id)
	if err != nil {
		return nil, err
	}
	row[undoHiddenKey], err = catBadgeHidden(ctx, q, id)
	return row, err
}

type catBadgeInput struct {
	Name            *string  `json:"name"`
	Stock           *int     `json:"stock"`
	Cost            *int     `json:"cost"`
	ImageURL        *string  `json:"imageUrl"`
	Active          *bool    `json:"active"`
	SortOrder       *int     `json:"sortOrder"`
	HiddenCourseIDs *[]int64 `json:"hiddenCourseIds"`
}

type catBadgeRow struct {
	Name, ImageURL string
	Stock, Cost    int
	Active         bool
	SortOrder      int
}

// catApplyBadge copia a la insignia los campos presentes y la valida.
func catApplyBadge(in catBadgeInput, b *catBadgeRow) error {
	if in.Name != nil {
		b.Name = strings.TrimSpace(*in.Name)
	}
	switch {
	case b.Name == "":
		return catBad("El nombre de la insignia es obligatorio.")
	case utf8.RuneCountInString(b.Name) > 120:
		return catBad("El nombre de la insignia es demasiado largo (máximo 120 caracteres).")
	case slugify(b.Name) == "":
		return catBad("El nombre de la insignia debe tener letras o números.")
	case in.Stock != nil && (*in.Stock < 0 || *in.Stock > math.MaxInt32):
		return catBad("El stock debe ser un entero de 0 en adelante.")
	case in.Cost != nil && (*in.Cost < 0 || *in.Cost > math.MaxInt32):
		return catBad("El costo debe ser un entero de 0 en adelante.")
	case !catSortOrderOK(in.SortOrder):
		return catBad("El orden no es válido.")
	}
	if in.Stock != nil {
		b.Stock = *in.Stock
	}
	if in.Cost != nil {
		b.Cost = *in.Cost
	}
	if in.ImageURL != nil {
		b.ImageURL = *in.ImageURL
	}
	if in.Active != nil {
		b.Active = *in.Active
	}
	if in.SortOrder != nil {
		b.SortOrder = *in.SortOrder
	}
	return nil
}

// catKeywordCourses devuelve los cursos que ocultan una insignia nueva con ese nombre
// (alguna de sus palabras clave aparece en el nombre normalizado).
func catKeywordCourses(ctx context.Context, q DBTX, name string) ([]int64, error) {
	rows, err := q.Query(ctx, `select id, hidden_badge_keywords from courses`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	label := normalizeLabel(name)
	var out []int64
	for rows.Next() {
		var id int64
		var keywords []string
		if err := rows.Scan(&id, &keywords); err != nil {
			return nil, err
		}
		for _, kw := range keywords {
			if k := normalizeLabel(kw); k != "" && strings.Contains(label, k) {
				out = append(out, id)
				break
			}
		}
	}
	return out, rows.Err()
}

func (a *App) handleAdminBadgesList(w http.ResponseWriter, r *http.Request, adminID int64) {
	badges, err := catBadges(r.Context(), a.DB, "", "b.sort_order, b.name, b.id")
	a.catRespond(w, "listar insignias", http.StatusOK, map[string]any{"badges": badges}, err)
}

func (a *App) handleAdminBadgesCreate(w http.ResponseWriter, r *http.Request, adminID int64) {
	ctx := r.Context()
	var in catBadgeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	b := catBadgeRow{Active: true}
	err := a.catImages(ctx, in.ImageURL)
	if err == nil {
		err = catApplyBadge(in, &b)
	}
	if err != nil {
		a.catFail(w, "crear insignia", err)
		return
	}
	body, err := a.catTx(ctx, func(tx pgx.Tx) (any, error) {
		var hidden []int64
		if in.HiddenCourseIDs != nil {
			var err error
			if hidden, err = catCheckIDs(ctx, tx, "courses", *in.HiddenCourseIDs, "Alguno de los cursos no existe."); err != nil {
				return nil, err
			}
		}
		keyword, err := catKeywordCourses(ctx, tx, b.Name)
		if err != nil {
			return nil, err
		}
		if in.SortOrder == nil {
			if err := tx.QueryRow(ctx, `select coalesce(max(sort_order), 0) + 1 from badges`).Scan(&b.SortOrder); err != nil {
				return nil, err
			}
		}
		var id int64
		if err := tx.QueryRow(ctx, `insert into badges (name, slug, stock, cost, image_url, active, sort_order)
			values ($1, $2, $3, $4, $5, $6, $7) returning id`,
			b.Name, slugify(b.Name), b.Stock, b.Cost, b.ImageURL, b.Active, b.SortOrder).Scan(&id); err != nil {
			return nil, err
		}
		if err := catSetBadgeHidden(ctx, tx, id, undoIDs(append(hidden, keyword...))); err != nil {
			return nil, err
		}
		after, err := catBadgeSnapshot(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		auditID, err := a.auditAdmin(ctx, tx, adminID, AuditEntry{
			Action: "create", Entity: "badge", EntityID: itoa64(id), Summary: "Insignia creada: " + b.Name,
			After: after, Undoable: true,
		})
		if err != nil {
			return nil, err
		}
		return catBadgeBody(ctx, tx, id, auditID)
	})
	a.catRespond(w, "crear insignia", http.StatusCreated, body, err)
}

func (a *App) handleAdminBadgesUpdate(w http.ResponseWriter, r *http.Request, adminID int64) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusNotFound, "Insignia no encontrada.")
		return
	}
	var in catBadgeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if in == (catBadgeInput{}) {
		writeError(w, http.StatusBadRequest, "No hay cambios que guardar.")
		return
	}
	if err := a.catImages(ctx, in.ImageURL); err != nil {
		a.catFail(w, "actualizar insignia", err)
		return
	}
	body, err := a.catTx(ctx, func(tx pgx.Tx) (any, error) {
		var b catBadgeRow
		err := tx.QueryRow(ctx, `select name, stock, cost, image_url, active, sort_order from badges
			where id = $1 for no key update`, id).Scan(&b.Name, &b.Stock, &b.Cost, &b.ImageURL, &b.Active, &b.SortOrder)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, undoFail(http.StatusNotFound, "Insignia no encontrada.")
		}
		if err != nil {
			return nil, err
		}
		if err := catApplyBadge(in, &b); err != nil {
			return nil, err
		}
		var hidden []int64
		if in.HiddenCourseIDs != nil {
			if hidden, err = catCheckIDs(ctx, tx, "courses", *in.HiddenCourseIDs, "Alguno de los cursos no existe."); err != nil {
				return nil, err
			}
		}
		before, err := catBadgeSnapshot(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `update badges set name = $2, slug = $3, stock = $4, cost = $5, image_url = $6,
			active = $7, sort_order = $8, updated_at = now() where id = $1`,
			id, b.Name, slugify(b.Name), b.Stock, b.Cost, b.ImageURL, b.Active, b.SortOrder); err != nil {
			return nil, err
		}
		// hiddenCourseIds reemplaza la visibilidad completa; las palabras clave solo aplican al crear.
		if in.HiddenCourseIDs != nil {
			if err := catSetBadgeHidden(ctx, tx, id, hidden); err != nil {
				return nil, err
			}
		}
		after, err := catBadgeSnapshot(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		auditID, err := a.auditAdmin(ctx, tx, adminID, AuditEntry{
			Action: "update", Entity: "badge", EntityID: itoa64(id), Summary: "Insignia actualizada: " + b.Name,
			Before: before, After: after, Undoable: true,
		})
		if err != nil {
			return nil, err
		}
		return catBadgeBody(ctx, tx, id, auditID)
	})
	a.catRespond(w, "actualizar insignia", http.StatusOK, body, err)
}

func (a *App) handleAdminBadgesDelete(w http.ResponseWriter, r *http.Request, adminID int64) {
	ctx := r.Context()
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusNotFound, "Insignia no encontrada.")
		return
	}
	body, err := a.catTx(ctx, func(tx pgx.Tx) (any, error) {
		name, err := catLock(ctx, tx, "badges", "for update", id, "Insignia no encontrada.")
		if err != nil {
			return nil, err
		}
		row, err := snapshotRow(ctx, tx, "badges", id)
		if err != nil {
			return nil, err
		}
		hidden, err := catBadgeHidden(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		// Las compras se quedan (con badge_name) y la base les quita el enlace; deshacer lo repone.
		purchases, err := undoIDList(ctx, tx, `select id from purchases where badge_id = $1 order by id`, id)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `delete from badges where id = $1`, id); err != nil {
			return nil, err
		}
		auditID, err := a.auditAdmin(ctx, tx, adminID, AuditEntry{
			Action: "delete", Entity: "badge", EntityID: itoa64(id), Summary: "Insignia eliminada: " + name,
			Before:   map[string]any{"row": row, undoHiddenKey: hidden, "purchase_ids": purchases},
			Undoable: true,
		})
		return map[string]any{"success": true, "auditId": auditID}, err
	})
	a.catRespond(w, "eliminar insignia", http.StatusOK, body, err)
}

func (a *App) handleAdminBadgesReorder(w http.ResponseWriter, r *http.Request, adminID int64) {
	a.catReorder(w, r, adminID, "badges", "badge", "Insignia movida")
}

// ---------- Niveles ----------

func catReadLevels(ctx context.Context, q DBTX) ([]Level, error) {
	rows, err := q.Query(ctx, `select level, min_exp from levels order by level`)
	if err != nil {
		return nil, err
	}
	levels, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Level])
	if levels == nil {
		levels = []Level{}
	}
	return levels, err
}

// catCheckLevels aplica las reglas del spec §4.3 a niveles ya ordenados por nivel.
func catCheckLevels(levels []Level) string {
	if len(levels) < 1 || len(levels) > 20 {
		return "Debe haber entre 1 y 20 niveles."
	}
	for i, l := range levels {
		switch {
		case l.Level != i:
			return "Los niveles deben ir de 0 en adelante, sin huecos ni repetidos."
		case i == 0 && l.MinExp != 0:
			return "El nivel 0 debe empezar en 0 EXP."
		case i > 0 && l.MinExp <= levels[i-1].MinExp:
			return "La EXP mínima debe crecer en cada nivel."
		case l.MinExp > math.MaxInt32:
			return "La EXP mínima es demasiado grande."
		}
	}
	return ""
}

// catWriteLevels reemplaza toda la tabla de niveles.
func catWriteLevels(ctx context.Context, q DBTX, levels []Level) error {
	lv := make([]int32, len(levels))
	exp := make([]int32, len(levels))
	for i, l := range levels {
		lv[i], exp[i] = int32(l.Level), int32(l.MinExp)
	}
	if _, err := q.Exec(ctx, `delete from levels`); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `insert into levels (level, min_exp) select * from unnest($1::int[], $2::int[])`, lv, exp)
	return err
}

// catLevelsSnapshot es el formato de la bitácora: [{"level":0,"min_exp":0}, ...].
func catLevelsSnapshot(levels []Level) []map[string]any {
	out := make([]map[string]any, len(levels))
	for i, l := range levels {
		out[i] = map[string]any{"level": l.Level, "min_exp": l.MinExp}
	}
	return out
}

func (a *App) handleAdminLevelsGet(w http.ResponseWriter, r *http.Request, adminID int64) {
	levels, err := catReadLevels(r.Context(), a.DB)
	a.catRespond(w, "leer niveles", http.StatusOK, map[string]any{"levels": levels}, err)
}

func (a *App) handleAdminLevelsPut(w http.ResponseWriter, r *http.Request, adminID int64) {
	ctx := r.Context()
	var in struct {
		Levels []Level `json:"levels"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	levels := slices.Clone(in.Levels)
	slices.SortFunc(levels, func(x, y Level) int { return x.Level - y.Level })
	if msg := catCheckLevels(levels); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	body, err := a.catTx(ctx, func(tx pgx.Tx) (any, error) {
		// Dos guardados a la vez no deben mezclar sus filas.
		if _, err := tx.Exec(ctx, `lock table levels in share row exclusive mode`); err != nil {
			return nil, err
		}
		before, err := catReadLevels(ctx, tx)
		if err != nil {
			return nil, err
		}
		if err := catWriteLevels(ctx, tx, levels); err != nil {
			return nil, err
		}
		auditID, err := a.auditAdmin(ctx, tx, adminID, AuditEntry{
			Action: "update", Entity: "level", Summary: "Niveles actualizados",
			Before: catLevelsSnapshot(before), After: catLevelsSnapshot(levels), Undoable: true,
		})
		return map[string]any{"levels": levels, "auditId": auditID}, err
	})
	if err == nil {
		a.invalidateLevels()
	}
	a.catRespond(w, "guardar niveles", http.StatusOK, body, err)
}

// ---------- Avatares ----------

func (a *App) handleAdminAvatarsList(w http.ResponseWriter, r *http.Request, adminID int64) {
	sql, args := `select code, level, url, updated_at from avatars`, []any{}
	if code := strings.TrimSpace(r.URL.Query().Get("code")); code != "" {
		sql, args = sql+` where code = $1`, append(args, code)
	}
	rows, err := a.DB.Query(r.Context(), sql+` order by code, level`, args...)
	if err != nil {
		a.catFail(w, "listar avatares", err)
		return
	}
	avatars, err := pgx.CollectRows(rows, pgx.RowToStructByPos[avatarJSON])
	if avatars == nil {
		avatars = []avatarJSON{}
	}
	a.catRespond(w, "listar avatares", http.StatusOK, map[string]any{"avatars": avatars}, err)
}

func (a *App) handleAdminAvatarsPut(w http.ResponseWriter, r *http.Request, adminID int64) {
	ctx := r.Context()
	var in struct {
		Code  string `json:"code"`
		Level *int   `json:"level"`
		URL   string `json:"url"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	code := strings.TrimSpace(in.Code)
	switch {
	case code == "":
		writeError(w, http.StatusBadRequest, "Falta el código del alumno.")
		return
	case in.Level == nil:
		writeError(w, http.StatusBadRequest, "Falta el nivel.")
		return
	}
	level := *in.Level
	url, err := a.catCheckImage(ctx, in.URL, true)
	if err != nil {
		a.catFail(w, "guardar avatar", err)
		return
	}
	body, err := a.catTx(ctx, func(tx pgx.Tx) (any, error) {
		var codeOK, levelOK bool
		if err := tx.QueryRow(ctx, `select $1 in ('DefaultH', 'DefaultF') or exists (select 1 from students where code = $1),
			exists (select 1 from levels where level = $2)`, code, level).Scan(&codeOK, &levelOK); err != nil {
			return nil, err
		}
		if !codeOK {
			return nil, catBad("No hay ningún alumno con ese código (usa DefaultH o DefaultF para los predeterminados).")
		}
		if !levelOK {
			return nil, catBad("Ese nivel no existe.")
		}
		key := code + ":" + strconv.Itoa(level)
		var before map[string]any
		err := tx.QueryRow(ctx, `select to_jsonb(t) from avatars t where code = $1 and level = $2 for update`,
			code, level).Scan(&before)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		var av avatarJSON
		if err := tx.QueryRow(ctx, `insert into avatars (code, level, url) values ($1, $2, $3)
			on conflict (code, level) do update set url = excluded.url, updated_at = now()
			returning code, level, url, updated_at`, code, level, url).Scan(&av.Code, &av.Level, &av.URL, &av.UpdatedAt); err != nil {
			return nil, err
		}
		after, err := snapshotRow(ctx, tx, "avatars", key)
		if err != nil {
			return nil, err
		}
		action, summary := "create", "Avatar agregado: "
		if before != nil {
			action, summary = "update", "Avatar cambiado: "
		}
		auditID, err := a.auditAdmin(ctx, tx, adminID, AuditEntry{
			Action: action, Entity: "avatar", EntityID: key,
			Summary: summary + code + " nivel " + strconv.Itoa(level),
			Before:  before, After: after, Undoable: true,
		})
		return map[string]any{"avatar": av, "auditId": auditID}, err
	})
	a.catRespond(w, "guardar avatar", http.StatusOK, body, err)
}

func (a *App) handleAdminAvatarsDelete(w http.ResponseWriter, r *http.Request, adminID int64) {
	ctx := r.Context()
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	level, err := strconv.Atoi(r.URL.Query().Get("level"))
	if code == "" || err != nil {
		writeError(w, http.StatusBadRequest, "Faltan el código o el nivel del avatar.")
		return
	}
	body, err := a.catTx(ctx, func(tx pgx.Tx) (any, error) {
		var before map[string]any
		err := tx.QueryRow(ctx, `select to_jsonb(t) from avatars t where code = $1 and level = $2 for update`,
			code, level).Scan(&before)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, undoFail(http.StatusNotFound, "Avatar no encontrado.")
		}
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `delete from avatars where code = $1 and level = $2`, code, level); err != nil {
			return nil, err
		}
		auditID, err := a.auditAdmin(ctx, tx, adminID, AuditEntry{
			Action: "delete", Entity: "avatar", EntityID: code + ":" + strconv.Itoa(level),
			Summary: "Avatar eliminado: " + code + " nivel " + strconv.Itoa(level),
			Before:  before, Undoable: true,
		})
		return map[string]any{"success": true, "auditId": auditID}, err
	})
	a.catRespond(w, "eliminar avatar", http.StatusOK, body, err)
}

// ---------- Resumen ----------

type catCount struct {
	Total  int `json:"total"`
	Active int `json:"active"`
}

type catCourseStat struct {
	CourseID   int64   `json:"courseId"`
	CourseName string  `json:"courseName"`
	Active     bool    `json:"active"`
	Students   int     `json:"students"`
	AvgLevel   float64 `json:"avgLevel"`
	TotalExp   int64   `json:"totalExp"`
}

func (a *App) handleAdminStats(w http.ResponseWriter, r *http.Request, adminID int64) {
	ctx := r.Context()
	var students, courses, badges catCount
	var coins int64
	var last7, total int
	// Las compras revertidas no cuentan; las monedas son las de alumnos activos.
	err := a.DB.QueryRow(ctx, `select
		(select count(*) from students), (select count(*) from students where active),
		(select count(*) from courses), (select count(*) from courses where active),
		(select count(*) from badges), (select count(*) from badges where active),
		(select coalesce(sum(monedas), 0) from students where active),
		(select count(*) from purchases where reverted_at is null and created_at >= now() - interval '7 days'),
		(select count(*) from purchases where reverted_at is null)`).Scan(
		&students.Total, &students.Active, &courses.Total, &courses.Active, &badges.Total, &badges.Active,
		&coins, &last7, &total)
	if err != nil {
		a.catFail(w, "resumen", err)
		return
	}
	lowStock, err := catBadges(ctx, a.DB, "where b.active and b.stock <= 3", "b.stock, b.sort_order, b.name")
	if err != nil {
		a.catFail(w, "resumen", err)
		return
	}
	// Por curso, solo alumnos activos; el nivel sale de la tabla de niveles.
	rows, err := a.DB.Query(ctx, `select c.id, c.name, c.active, count(s.id),
			coalesce(round(avg(lv.level), 2), 0)::float8, coalesce(sum(s.exp), 0)
		from courses c
		left join students s on s.course_id = c.id and s.active
		left join lateral (select coalesce(max(l.level), 0) as level from levels l where l.min_exp <= s.exp) lv
			on s.id is not null
		group by c.id
		order by c.sort_order, c.id`)
	if err != nil {
		a.catFail(w, "resumen", err)
		return
	}
	perCourse, err := pgx.CollectRows(rows, pgx.RowToStructByPos[catCourseStat])
	if err != nil {
		a.catFail(w, "resumen", err)
		return
	}
	if perCourse == nil {
		perCourse = []catCourseStat{}
	}
	recent, _, err := undoEntries(ctx, a.DB, undoFilter{limit: 10})
	a.catRespond(w, "resumen", http.StatusOK, map[string]any{
		"students":           students,
		"courses":            courses,
		"badges":             map[string]any{"total": badges.Total, "active": badges.Active, "lowStock": lowStock},
		"coinsInCirculation": coins,
		"purchases":          map[string]int{"last7Days": last7, "total": total},
		"perCourse":          perCourse,
		"recentActivity":     recent,
	}, err)
}
