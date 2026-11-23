package app

// Importación desde el Google Sheets del sistema viejo (alumnos, insignias y
// compras) y de la media heredada (avatares e imágenes de insignias). Lee la
// hoja igual que legacy/server.js, pero nunca escribe en ella.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"path"
	"regexp"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // la zona de México no depende de lo que traiga la imagen

	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2/jwt"
)

const (
	impAvatarsPrefix = "legacy/avatars/"
	impBadgesPrefix  = "legacy/badges/"
	impMaxWarnings   = 500
	impBusy          = "Ya hay una importación en curso. Espera a que termine."
)

// ---------- Formas JSON (espejo de types.ts) ----------

type impInclude struct {
	Students  bool `json:"students"`
	Badges    bool `json:"badges"`
	Purchases bool `json:"purchases"`
	Media     bool `json:"media"`
}

type impRequest struct {
	SpreadsheetID   string      `json:"spreadsheetId"`
	CredentialsJSON string      `json:"credentialsJson"`
	Include         *impInclude `json:"include"`
	Mode            string      `json:"mode"`
}

type impCourseReport struct {
	CourseID   int64   `json:"courseId"`
	CourseName string  `json:"courseName"`
	Sheet      *string `json:"sheet"`
	Rows       int     `json:"rows"`
	Created    int     `json:"created"`
	Updated    int     `json:"updated"`
	Unchanged  int     `json:"unchanged"`
	Skipped    int     `json:"skipped"`
}

type impBadgeReport struct {
	Rows      int `json:"rows"`
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	Skipped   int `json:"skipped"`
}

type impPurchaseReport struct {
	Rows      int `json:"rows"`
	Created   int `json:"created"`
	Skipped   int `json:"skipped"`
	Unmatched int `json:"unmatched"`
}

type impMediaReport struct {
	Avatars      int `json:"avatars"`
	BadgeImages  int `json:"badgeImages"`
	CourseImages int `json:"courseImages"`
}

type impReport struct {
	DryRun           bool              `json:"dryRun"`
	BatchID          *string           `json:"batchId"`
	SpreadsheetTitle string            `json:"spreadsheetTitle"`
	SheetsFound      []string          `json:"sheetsFound"`
	Courses          []impCourseReport `json:"courses"`
	Badges           impBadgeReport    `json:"badges"`
	Purchases        impPurchaseReport `json:"purchases"`
	Media            impMediaReport    `json:"media"`
	Warnings         []string          `json:"warnings"`
}

func newImpReport(dry bool) *impReport {
	return &impReport{DryRun: dry, SheetsFound: []string{}, Courses: []impCourseReport{}, Warnings: []string{}}
}

type impStatusJSON struct {
	SpreadsheetID string           `json:"spreadsheetId"`
	Credentials   secretStatusJSON `json:"credentials"`
	LegacyMedia   struct {
		Available bool `json:"available"`
		Avatars   int  `json:"avatars"`
		Badges    int  `json:"badges"`
	} `json:"legacyMedia"`
	LastImport *auditEntryJSON `json:"lastImport"`
}

type impJobJSON struct {
	ID         string     `json:"id"`
	Status     string     `json:"status"` // running | done | error
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
	Report     *impReport `json:"report"`
	Error      *string    `json:"error"`
}

type impJob struct {
	mu  sync.Mutex
	val impJobJSON
}

type impMediaResponse struct {
	Media    impMediaReport `json:"media"`
	Warnings []string       `json:"warnings"`
	BatchID  *string        `json:"batchId"`
}

// ---------- Manejadores ----------

func (a *App) handleAdminImportStatus(w http.ResponseWriter, r *http.Request, adminID int64) {
	ctx := r.Context()
	s, err := a.Settings(ctx)
	if err != nil {
		a.impWriteErr(w, "estado", err)
		return
	}
	var st impStatusJSON
	st.SpreadsheetID = s.SpreadsheetID
	st.Credentials = secretStatusJSON{
		Configured:  s.GoogleServiceAccount != "",
		Source:      s.GoogleSource,
		ClientEmail: impClientEmail(s.GoogleServiceAccount),
	}
	avatars, badges, err := a.impLegacyCounts(ctx)
	if err != nil {
		// Sin almacenamiento el resto del estado sigue sirviendo.
		a.Log.Warn("importación: no se pudo listar la media heredada", "err", err)
	}
	st.LegacyMedia.Avatars, st.LegacyMedia.Badges = avatars, badges
	st.LegacyMedia.Available = avatars+badges > 0
	if st.LastImport, err = impLastImport(ctx, a.DB); err != nil {
		a.impWriteErr(w, "estado", err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (a *App) handleAdminImportPreview(w http.ResponseWriter, r *http.Request, adminID int64) {
	var req impRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	src, opt, err := a.impPrepare(r.Context(), req, adminID)
	if err != nil {
		a.impWriteErr(w, "vista previa", err)
		return
	}
	// La simulación también carga la hoja y las instantáneas de la base en
	// memoria: comparte el único turno con las corridas y la importación de media.
	if !a.importMu.TryLock() {
		writeError(w, http.StatusConflict, impBusy)
		return
	}
	defer a.importMu.Unlock()
	// Cada vista previa consulta a Google y corre bcrypt por alumno en todos los núcleos.
	if !a.Limiter.Allow("import-preview:"+itoa64(adminID), 10, time.Minute) {
		writeError(w, http.StatusTooManyRequests, "Demasiadas vistas previas seguidas. Espera un minuto.")
		return
	}
	opt.dry = true
	ctx, cancel := context.WithTimeout(r.Context(), 100*time.Second)
	defer cancel()
	rep, err := a.impExecute(ctx, src, opt)
	if err != nil {
		a.impWriteErr(w, "vista previa", err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (a *App) handleAdminImportRun(w http.ResponseWriter, r *http.Request, adminID int64) {
	var req impRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	src, opt, err := a.impPrepare(r.Context(), req, adminID)
	if err != nil {
		a.impWriteErr(w, "importación", err)
		return
	}
	if !a.importMu.TryLock() {
		writeError(w, http.StatusConflict, impBusy)
		return
	}
	a.impPrune()
	id := randomToken(12)
	job := &impJob{val: impJobJSON{ID: id, Status: "running", StartedAt: time.Now().UTC()}}
	a.importJobs.Store(id, job)
	go a.impBackground(job, src, opt)
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": id})
}

func (a *App) handleAdminImportJob(w http.ResponseWriter, r *http.Request, adminID int64) {
	v, ok := a.importJobs.Load(r.PathValue("jobId"))
	if !ok {
		writeError(w, http.StatusNotFound, "No existe esa importación.")
		return
	}
	job := v.(*impJob)
	job.mu.Lock()
	val := job.val
	job.mu.Unlock()
	writeJSON(w, http.StatusOK, val)
}

func (a *App) handleAdminImportMedia(w http.ResponseWriter, r *http.Request, adminID int64) {
	if !a.importMu.TryLock() {
		writeError(w, http.StatusConflict, impBusy)
		return
	}
	defer a.importMu.Unlock()
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		a.impWriteErr(w, "media", err)
		return
	}
	defer tx.Rollback(ctx)

	run := a.newImpRun(ctx, impOptions{include: impInclude{Media: true}, adminID: adminID}, newImpReport(false))
	run.tx, run.batch = tx, newBatchID()
	run.actor = a.adminLabel(ctx, tx, adminID)
	err = run.load()
	if err == nil {
		err = run.importMedia()
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		a.impWriteErr(w, "media", err)
		return
	}
	run.capWarnings()
	res := impMediaResponse{Media: run.rep.Media, Warnings: run.rep.Warnings}
	if run.audits > 0 {
		res.BatchID = &run.batch
	}
	writeJSON(w, http.StatusOK, res)
}

// ---------- Errores, trabajos y preparación ----------

// impStatus da el código y el mensaje para el admin; lo inesperado va al log
// (solo el error, nunca datos de alumnos ni credenciales).
func (a *App) impStatus(what string, err error) (int, string) {
	var ie *impError
	switch {
	case errors.As(err, &ie):
		return ie.status, ie.msg
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, "La importación tardó demasiado y se canceló."
	case errors.Is(err, context.Canceled):
		return http.StatusBadRequest, "La solicitud se canceló."
	}
	a.Log.Error("importación: "+what, "err", err)
	return http.StatusInternalServerError, "Error interno del servidor."
}

func (a *App) impWriteErr(w http.ResponseWriter, what string, err error) {
	status, msg := a.impStatus(what, err)
	writeError(w, status, msg)
}

// impBackground corre la importación con su propio contexto (la petición ya
// respondió 202). El candado se suelta antes de publicar el resultado: quien
// ya vio "done" puede lanzar otra sin toparse con un 409.
func (a *App) impBackground(job *impJob, src impSource, opt impOptions) {
	rep, err := func() (*impReport, error) {
		defer a.importMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		return a.impSafeExecute(ctx, src, opt)
	}()
	now := time.Now().UTC()
	job.mu.Lock()
	defer job.mu.Unlock()
	job.val.FinishedAt = &now
	if err != nil {
		_, msg := a.impStatus("importación", err)
		job.val.Status, job.val.Error = "error", &msg
		return
	}
	job.val.Status, job.val.Report = "done", rep
}

// impSafeExecute evita que un panic dentro de la gorutina tumbe el servidor.
func (a *App) impSafeExecute(ctx context.Context, src impSource, opt impOptions) (rep *impReport, err error) {
	defer func() {
		if v := recover(); v != nil {
			a.Log.Error("importación: panic", "err", fmt.Sprint(v), "stack", string(debug.Stack()))
			rep, err = nil, errors.New("panic durante la importación")
		}
	}()
	return a.impExecute(ctx, src, opt)
}

// impPrune olvida los trabajos que terminaron hace más de un día.
func (a *App) impPrune() {
	cutoff := time.Now().Add(-24 * time.Hour)
	a.importJobs.Range(func(k, v any) bool {
		job := v.(*impJob)
		job.mu.Lock()
		old := job.val.FinishedAt != nil && job.val.FinishedAt.Before(cutoff)
		job.mu.Unlock()
		if old {
			a.importJobs.Delete(k)
		}
		return true
	})
}

type impSource struct {
	spreadsheetID string
	conf          *jwt.Config
}

type impOptions struct {
	include impInclude
	mode    string // "upsert" | "create_only"
	dry     bool
	adminID int64
}

var (
	impSheetIDRe  = regexp.MustCompile(`^[A-Za-z0-9_-]{10,200}$`)
	impSheetURLRe = regexp.MustCompile(`/spreadsheets/d/([A-Za-z0-9_-]+)`)
)

// impPrepare valida la petición. Credenciales e ID salen de la petición y, si
// no vienen, de Ajustes. Las credenciales de la petición no se guardan.
func (a *App) impPrepare(ctx context.Context, req impRequest, adminID int64) (impSource, impOptions, error) {
	opt := impOptions{include: impInclude{Students: true, Badges: true, Purchases: true, Media: true}, mode: req.Mode, adminID: adminID}
	if req.Include != nil {
		opt.include = *req.Include
	}
	if inc := opt.include; !inc.Students && !inc.Badges && !inc.Purchases && !inc.Media {
		return impSource{}, opt, impFail(http.StatusBadRequest, "Elige al menos una parte para importar.")
	}
	if opt.mode == "" {
		opt.mode = "upsert"
	}
	if opt.mode != "upsert" && opt.mode != "create_only" {
		return impSource{}, opt, impFail(http.StatusBadRequest, "Modo de importación inválido.")
	}
	s, err := a.Settings(ctx)
	if err != nil {
		return impSource{}, opt, err
	}
	creds := strings.TrimSpace(req.CredentialsJSON)
	if creds == "" {
		creds = strings.TrimSpace(s.GoogleServiceAccount)
	}
	if creds == "" {
		return impSource{}, opt, impFail(http.StatusBadRequest, "Configura las credenciales de Google en Ajustes o sube el archivo JSON.")
	}
	conf, err := impParseCredentials(creds)
	if err != nil {
		return impSource{}, opt, err
	}
	id := strings.TrimSpace(req.SpreadsheetID)
	if id == "" {
		id = strings.TrimSpace(s.SpreadsheetID)
	}
	if m := impSheetURLRe.FindStringSubmatch(id); m != nil {
		id = m[1] // pegaron la url completa de la hoja
	}
	if id == "" {
		return impSource{}, opt, impFail(http.StatusBadRequest, "Falta el ID de la hoja de cálculo.")
	}
	if !impSheetIDRe.MatchString(id) {
		return impSource{}, opt, impFail(http.StatusBadRequest, "El ID de la hoja de cálculo no es válido.")
	}
	return impSource{spreadsheetID: id, conf: conf}, opt, nil
}

// ---------- La importación ----------

type impCourse struct {
	ID        int64
	Name      string
	SheetName string
	Aliases   []string
	Keywords  []string
}

type impStudent struct {
	ID, CourseID                                        int64
	Code, Name, Sexo, Email, Hash, Comments             string
	Tareas, Monedas, Asistencias, Exp, LastLevel, Games int
	Snap                                                map[string]any // to_jsonb de la fila, para la bitácora
}

type impBadge struct {
	ID        int64
	Name      string
	Slug      string
	ImageURL  string
	Stock     int
	Cost      int
	SortOrder int
	Snap      map[string]any
}

type impKey struct {
	course int64
	code   string
}

type impRun struct {
	a      *App
	ctx    context.Context
	tx     pgx.Tx
	opt    impOptions
	rep    *impReport
	batch  string
	actor  string // usuario del admin, se busca una sola vez
	audits int

	settings     Settings
	courses      []impCourse // por sort_order
	students     map[impKey]*impStudent
	byCode       map[string][]*impStudent
	badges       map[string]*impBadge // por slug
	badgeList    []*impBadge          // en orden de id, las nuevas al final
	maxSort      int
	refs         map[string]bool // source_ref de compras ya importadas
	legacyBadges map[string]bool // slugs con legacy/badges/{slug}.png
	fake         int64
}

func (a *App) newImpRun(ctx context.Context, opt impOptions, rep *impReport) *impRun {
	return &impRun{
		a: a, ctx: ctx, opt: opt, rep: rep,
		students:     map[impKey]*impStudent{},
		byCode:       map[string][]*impStudent{},
		badges:       map[string]*impBadge{},
		refs:         map[string]bool{},
		legacyBadges: map[string]bool{},
	}
}

// impExecute lee la hoja y aplica todo en una sola transacción. En la vista
// previa (opt.dry) hace exactamente lo mismo sin escribir: la transacción es
// de solo lectura y lo "creado" vive en memoria con ids negativos.
func (a *App) impExecute(ctx context.Context, src impSource, opt impOptions) (*impReport, error) {
	rep := newImpReport(opt.dry)
	sheets := impNewSheets(ctx, src.conf)
	meta, err := sheets.metadata(ctx, src.spreadsheetID)
	if err != nil {
		return nil, err
	}
	rep.SpreadsheetTitle, rep.SheetsFound = meta.Title, meta.Tabs

	run := a.newImpRun(ctx, opt, rep)
	if run.courses, err = impLoadCourses(ctx, a.DB); err != nil {
		return nil, err
	}
	tabs := run.pickTabs(meta.Tabs)
	values, err := sheets.batchGet(ctx, src.spreadsheetID, tabs.ranges())
	if err != nil {
		return nil, err
	}
	for i, tab := range tabs.all {
		tab.rows = values[i]
	}

	// Google ya respondió; la transacción se abre hasta ahora.
	txOpts := pgx.TxOptions{}
	if opt.dry {
		txOpts.AccessMode = pgx.ReadOnly // aunque hubiera un error aquí, la vista previa no podría escribir
	}
	tx, err := a.DB.BeginTx(ctx, txOpts)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	run.tx = tx
	if !opt.dry {
		run.batch = newBatchID()
		rep.BatchID = &run.batch
		run.actor = a.adminLabel(ctx, tx, opt.adminID)
	}

	if err := run.load(); err != nil {
		return nil, err
	}
	if opt.include.Students {
		if err := run.importStudents(tabs.byCourse); err != nil {
			return nil, err
		}
	}
	if opt.include.Badges {
		if err := run.importBadges(tabs.badges); err != nil {
			return nil, err
		}
	}
	if opt.include.Purchases {
		if err := run.importPurchases(tabs.purchases); err != nil {
			return nil, err
		}
	}
	if opt.include.Media {
		if err := run.importMedia(); err != nil {
			return nil, err
		}
	}
	run.capWarnings()
	if opt.dry {
		return rep, nil
	}
	if err := run.audit("import", "import", "", run.summary(), nil, rep); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return rep, nil
}

func impLoadCourses(ctx context.Context, q DBTX) ([]impCourse, error) {
	rows, err := q.Query(ctx, `select id, name, sheet_name, sheet_aliases, hidden_badge_keywords
		from courses order by sort_order, id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[impCourse])
}

// load trae lo que la importación necesita. En la corrida real bloquea las
// filas de alumnos e insignias para que nadie las cambie a la mitad.
func (r *impRun) load() error {
	lock := ""
	if !r.opt.dry {
		lock = " for update"
	}
	inc := r.opt.include
	var err error
	if r.settings, err = r.a.Settings(r.ctx); err != nil {
		return err
	}
	if inc.Students || inc.Purchases {
		rows, err := r.tx.Query(r.ctx, `select id, course_id, code, name, sexo, email, password_hash, comments,
			tareas, monedas, asistencias, exp, last_login_level, games, to_jsonb(s)
			from students s order by id`+lock)
		if err != nil {
			return err
		}
		list, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByPos[impStudent])
		if err != nil {
			return err
		}
		for _, s := range list {
			r.addStudent(s)
		}
	}
	if inc.Badges || inc.Purchases || inc.Media {
		rows, err := r.tx.Query(r.ctx, `select id, name, slug, image_url, stock, cost, sort_order, to_jsonb(b)
			from badges b order by id`+lock)
		if err != nil {
			return err
		}
		if r.badgeList, err = pgx.CollectRows(rows, pgx.RowToAddrOfStructByPos[impBadge]); err != nil {
			return err
		}
		for _, b := range r.badgeList {
			r.badges[b.Slug] = b
			r.maxSort = max(r.maxSort, b.SortOrder)
		}
	}
	if inc.Badges || inc.Media {
		set, err := r.a.impLegacyBadges(r.ctx)
		switch {
		case err == nil:
			r.legacyBadges = set
		case inc.Media:
			return err
		default:
			r.a.Log.Warn("importación: no se pudo listar legacy/badges", "err", err)
			r.warn("No se pudo revisar la media heredada; las insignias nuevas quedan sin imagen.")
		}
	}
	if inc.Purchases {
		rows, err := r.tx.Query(r.ctx, `select source_ref from purchases where source_ref like 'sheets:%'`)
		if err != nil {
			return err
		}
		refs, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		for _, ref := range refs {
			r.refs[ref] = true
		}
	}
	return nil
}

func (r *impRun) addStudent(s *impStudent) {
	r.students[impKey{s.CourseID, s.Code}] = s
	r.byCode[s.Code] = append(r.byCode[s.Code], s)
}

// fakeID numera en negativo lo que la vista previa "crea"; nunca llega a la base.
func (r *impRun) fakeID() int64 {
	r.fake--
	return r.fake
}

func (r *impRun) warn(format string, args ...any) {
	r.rep.Warnings = append(r.rep.Warnings, fmt.Sprintf(format, args...))
}

// capWarnings recorta la lista, que también se guarda en la bitácora.
func (r *impRun) capWarnings() {
	if n := len(r.rep.Warnings); n > impMaxWarnings {
		r.rep.Warnings = append(r.rep.Warnings[:impMaxWarnings], fmt.Sprintf("… y %d avisos más.", n-impMaxWarnings))
	}
}

// audit registra una entrada del lote a nombre del admin; en la vista previa no hace nada.
func (r *impRun) audit(action, entity, id, summary string, before, after any) error {
	if r.opt.dry {
		return nil
	}
	r.audits++
	_, err := r.a.auditAdmin(r.ctx, r.tx, r.opt.adminID, AuditEntry{
		ActorLabel: r.actor,
		Action:     action,
		Entity:     entity,
		EntityID:   id,
		Summary:    summary,
		Before:     before,
		After:      after,
		BatchID:    r.batch,
		Undoable:   true,
	})
	return err
}

func (r *impRun) summary() string {
	var created, updated int
	for _, c := range r.rep.Courses {
		created += c.Created
		updated += c.Updated
	}
	b, p, m := r.rep.Badges, r.rep.Purchases, r.rep.Media
	return fmt.Sprintf("Importación desde Google Sheets (%s): %d alumnos nuevos y %d actualizados, "+
		"%d insignias nuevas y %d actualizadas, %d compras, %d avatares y %d imágenes de insignias.",
		r.rep.SpreadsheetTitle, created, updated, b.Created, b.Updated, p.Created, m.Avatars, m.BadgeImages)
}

// ---------- Pestañas ----------

type impTab struct {
	name, cols string
	rows       [][]string
}

type impTabs struct {
	byCourse  map[int64]*impTab
	badges    *impTab
	purchases *impTab
	all       []*impTab // en el orden en que se piden a Google
}

func (t *impTabs) add(name, cols string) *impTab {
	tab := &impTab{name: name, cols: cols}
	t.all = append(t.all, tab)
	return tab
}

func (t *impTabs) ranges() []string {
	out := make([]string, len(t.all))
	for i, tab := range t.all {
		out[i] = impRange(tab.name, tab.cols)
	}
	return out
}

// pickTabs decide qué leer: la pestaña de cada curso, Badges y Purchases.
// El servidor viejo creaba las pestañas que faltaban; aquí solo se avisa.
func (r *impRun) pickTabs(titles []string) *impTabs {
	t := &impTabs{byCourse: map[int64]*impTab{}}
	inc := r.opt.include
	if inc.Students {
		used := map[string]string{} // pestaña → curso que ya la tomó
		for _, c := range r.courses {
			cr := impCourseReport{CourseID: c.ID, CourseName: c.Name}
			if tab := r.courseTab(c, titles, used); tab != "" {
				used[tab] = c.Name
				t.byCourse[c.ID] = t.add(tab, "A:N")
				cr.Sheet = &tab
			}
			r.rep.Courses = append(r.rep.Courses, cr)
		}
	}
	find := func(want string) string {
		for _, title := range titles {
			if normalizeLabel(title) == want {
				return title
			}
		}
		return ""
	}
	if inc.Badges {
		if tab := find("badges"); tab != "" {
			t.badges = t.add(tab, "A:C")
		} else {
			r.warn("La hoja no tiene pestaña Badges; no se importaron insignias.")
		}
	}
	if inc.Purchases {
		if tab := find("purchases"); tab != "" {
			t.purchases = t.add(tab, "A:E")
		} else {
			r.warn("La hoja no tiene pestaña Purchases; no se importaron compras.")
		}
	}
	return t
}

// courseTab busca la pestaña cuyo nombre normalizado es el de la hoja del
// curso o el de alguno de sus alias. La que se llama igual que la hoja gana.
func (r *impRun) courseTab(c impCourse, titles []string, used map[string]string) string {
	want := map[string]bool{normalizeLabel(c.SheetName): true}
	for _, al := range c.Aliases {
		if n := normalizeLabel(al); n != "" {
			want[n] = true
		}
	}
	var found []string
	for _, title := range titles {
		if want[normalizeLabel(title)] {
			found = append(found, title)
		}
	}
	if i := slices.Index(found, c.SheetName); i > 0 {
		found[0], found[i] = found[i], found[0]
	}
	for _, tab := range found {
		if _, taken := used[tab]; !taken {
			if len(found) > 1 {
				r.warn("El curso %s coincide con las pestañas %s; se usó %s.", c.Name, strings.Join(found, ", "), tab)
			}
			return tab
		}
	}
	if len(found) > 0 {
		r.warn("La pestaña %s ya se usó para el curso %s; el curso %s se omitió.", found[0], used[found[0]], c.Name)
	} else {
		r.warn("No hay pestaña para el curso %s (se buscó: %s).", c.Name, strings.Join(append([]string{c.SheetName}, c.Aliases...), ", "))
	}
	return ""
}

// impText limpia una celda de una sola línea. La hoja vieja se acepta tal cual
// (sin las reglas del registro), pero sin caracteres de control.
func impText(s string) string { return strings.TrimSpace(stripControl(s)) }

// impCells lee celdas sin salirse de la fila: Sheets omite las vacías del final.
func impCells(row []string) func(int) string {
	return func(i int) string {
		if i < len(row) {
			return row[i]
		}
		return ""
	}
}

func impBlank(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

var impPlainInt = regexp.MustCompile(`^[+-]?[0-9]+$`)

// impOdd dice si una celda numérica trae algo que parseInt ignora ("1,000" → 1).
func impOdd(s string) bool { return s != "" && !impPlainInt.MatchString(s) }

// num lee un entero como el servidor viejo (parseInt) y avisa cuando la celda
// traía algo más.
func (r *impRun) num(where, field, raw string) int {
	s := strings.TrimSpace(raw)
	n := parseIntLoose(s)
	if impOdd(s) {
		r.warn("%s: %s %q se leyó como %d.", where, field, s, n)
	}
	return n
}

// ---------- Alumnos ----------

type impStudentRow struct {
	cr       *impCourseReport
	course   *impCourse
	where    string // "TAR, fila 5 (0251412)" para los avisos
	code     string
	name     string
	sexo     string
	email    string
	password string
	comments string

	tareas, monedas, asistencias, exp, lastLevel, games int

	cur       *impStudent // la fila que ya existe, o nil
	hash      string
	changed   bool // la contraseña cambia
	generated bool // la hoja no traía contraseña
	err       error
}

// importStudents lee columnas A–N de cada pestaña de curso, igual que el
// servidor viejo. I y M se ignoran (I era un conteo derivado de compras).
func (r *impRun) importStudents(tabs map[int64]*impTab) error {
	var rows []*impStudentRow
	for i := range r.rep.Courses {
		cr, course := &r.rep.Courses[i], &r.courses[i]
		tab := tabs[course.ID]
		if tab == nil {
			continue
		}
		seen := map[string]bool{}
		for n, cells := range tab.rows {
			if n == 0 || impBlank(cells) {
				continue // la fila 1 es el encabezado
			}
			cr.Rows++
			cell := impCells(cells)
			where := fmt.Sprintf("%s, fila %d", tab.name, n+1)
			code := impText(cell(0))
			if code == "" {
				cr.Skipped++
				r.warn("%s: sin código de alumno; se omitió.", where)
				continue
			}
			if seen[code] {
				cr.Skipped++
				r.warn("%s: el código %s está repetido; se conservó la primera fila.", where, code)
				continue
			}
			seen[code] = true
			row := &impStudentRow{cr: cr, course: course, where: where + " (" + code + ")", code: code,
				cur: r.students[impKey{course.ID, code}]}
			if row.cur != nil && r.opt.mode == "create_only" {
				cr.Skipped++
				continue
			}
			row.name = impText(cell(1))
			if row.sexo = impText(cell(2)); row.sexo == "" {
				row.sexo = "N/A"
			}
			if row.email = impText(cell(3)); row.email == "" {
				row.email = code + "@" + r.settings.StudentEmailDomain
			}
			row.password = cell(4)
			if len(row.password) > 72 { // bcrypt no acepta más
				r.warn("%s: la contraseña de la hoja es demasiado larga; se ignoró.", row.where)
				row.password = ""
			}
			row.tareas = r.num(row.where, "tareas", cell(5))
			row.monedas = r.num(row.where, "monedas", cell(6))
			row.asistencias = r.num(row.where, "asistencias", cell(7))
			row.exp = r.num(row.where, "EXP", cell(9))
			row.lastLevel = r.num(row.where, "último nivel", cell(10))
			row.games = r.num(row.where, "juegos", cell(11))
			row.comments = cell(13)
			rows = append(rows, row)
		}
	}

	r.a.impResolvePasswords(r.ctx, rows, r.opt.dry, r.settings.PasswordPattern)
	for _, row := range rows {
		if row.err != nil {
			return row.err
		}
		if row.cur == nil {
			if err := r.createStudent(row); err != nil {
				return err
			}
			row.cr.Created++
			continue
		}
		changed, err := r.updateStudent(row)
		if err != nil {
			return err
		}
		if changed {
			row.cr.Updated++
		} else {
			row.cr.Unchanged++
		}
	}
	return nil
}

// impResolvePasswords decide el hash de cada fila. bcrypt tarda decenas de
// milisegundos por llamada, así que se reparte entre los núcleos con los
// turnos de la app. sem deja a lo más GOMAXPROCS filas esperando turno: un
// login que llega a media importación no queda detrás de cientos de ellas.
func (a *App) impResolvePasswords(ctx context.Context, rows []*impStudentRow, dry bool, pattern string) {
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	var wg sync.WaitGroup
	for _, row := range rows {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := a.withBcrypt(ctx, func() { row.resolvePassword(dry, pattern) }); err != nil {
				row.err = err
			}
		})
	}
	wg.Wait()
}

// resolvePassword: en la hoja la contraseña está en claro. Si coincide con la
// actual se conserva el hash; si viene vacía, un alumno existente se queda con
// la suya y uno nuevo recibe una al azar.
func (row *impStudentRow) resolvePassword(dry bool, pattern string) {
	if row.cur != nil && (row.password == "" || checkPassword(row.cur.Hash, row.password)) {
		row.hash = row.cur.Hash
		return
	}
	plain := row.password
	if plain == "" {
		row.generated = true
		var err error
		if plain, err = generatePassword(pattern); err != nil {
			plain = randomToken(8) // la regla no admite generadas; igual hay que restablecerla
		}
	}
	row.changed = true
	if !dry {
		row.hash, row.err = hashPassword(plain)
	}
}

func (r *impRun) createStudent(row *impStudentRow) error {
	st := &impStudent{CourseID: row.course.ID, Code: row.code, Name: row.name, Sexo: row.sexo,
		Email: row.email, Hash: row.hash, Comments: row.comments, Tareas: row.tareas,
		Monedas: row.monedas, Asistencias: row.asistencias, Exp: row.exp, LastLevel: row.lastLevel,
		Games: row.games}
	if r.opt.dry {
		st.ID = r.fakeID()
	} else {
		err := r.tx.QueryRow(r.ctx, `insert into students (course_id, code, name, sexo, email, password_hash,
			tareas, monedas, asistencias, exp, last_login_level, games, comments)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			returning id, to_jsonb(students)`,
			st.CourseID, st.Code, st.Name, st.Sexo, st.Email, st.Hash, st.Tareas, st.Monedas,
			st.Asistencias, st.Exp, st.LastLevel, st.Games, st.Comments).Scan(&st.ID, &st.Snap)
		if err != nil {
			return err
		}
		summary := fmt.Sprintf("Importación: alumno %s creado en %s", st.Code, row.course.Name)
		if err := r.audit("create", "student", itoa64(st.ID), summary, nil, st.Snap); err != nil {
			return err
		}
	}
	if row.generated {
		r.warn("%s: sin contraseña en la hoja; queda una al azar. Restablécela en Alumnos para dársela al alumno.", row.where)
	}
	r.addStudent(st)
	return nil
}

// updateStudent aplica la fila de la hoja; false si no había nada distinto.
func (r *impRun) updateStudent(row *impStudentRow) (bool, error) {
	cur := row.cur
	var fields []string
	diff := func(label string, changed bool) {
		if changed {
			fields = append(fields, label)
		}
	}
	diff("nombre", cur.Name != row.name)
	diff("sexo", cur.Sexo != row.sexo)
	diff("correo", cur.Email != row.email)
	diff("contraseña", row.changed)
	diff("tareas", cur.Tareas != row.tareas)
	diff("monedas", cur.Monedas != row.monedas)
	diff("asistencias", cur.Asistencias != row.asistencias)
	diff("EXP", cur.Exp != row.exp)
	diff("último nivel visto", cur.LastLevel != row.lastLevel)
	diff("juegos", cur.Games != row.games)
	diff("comentarios", cur.Comments != row.comments)
	if len(fields) == 0 {
		return false, nil
	}
	if !r.opt.dry {
		var after map[string]any
		err := r.tx.QueryRow(r.ctx, `update students set name = $2, sexo = $3, email = $4,
			password_hash = $5, tareas = $6, monedas = $7, asistencias = $8, exp = $9,
			last_login_level = $10, games = $11, comments = $12, updated_at = now()
			where id = $1 returning to_jsonb(students)`,
			cur.ID, row.name, row.sexo, row.email, row.hash, row.tareas, row.monedas,
			row.asistencias, row.exp, row.lastLevel, row.games, row.comments).Scan(&after)
		if err != nil {
			return false, err
		}
		summary := fmt.Sprintf("Importación: alumno %s (%s) actualizado: %s", cur.Code, row.course.Name, strings.Join(fields, ", "))
		if err := r.audit("update", "student", itoa64(cur.ID), summary, cur.Snap, after); err != nil {
			return false, err
		}
		cur.Snap = after
	}
	cur.Name, cur.Sexo, cur.Email, cur.Hash, cur.Comments = row.name, row.sexo, row.email, row.hash, row.comments
	cur.Tareas, cur.Monedas, cur.Asistencias = row.tareas, row.monedas, row.asistencias
	cur.Exp, cur.LastLevel, cur.Games = row.exp, row.lastLevel, row.games
	return true, nil
}

// ---------- Insignias ----------

// importBadges lee la pestaña Badges: A nombre, B existencias, C costo. La
// llave es el slug, el mismo nombre de archivo que usaba el frontend viejo.
func (r *impRun) importBadges(tab *impTab) error {
	if tab == nil {
		return nil
	}
	rep := &r.rep.Badges
	seen := map[string]bool{}
	for n, cells := range tab.rows {
		if n == 0 || impBlank(cells) {
			continue
		}
		rep.Rows++
		cell := impCells(cells)
		where := fmt.Sprintf("%s, fila %d", tab.name, n+1)
		name := strings.TrimSpace(cell(0))
		slug := slugify(name)
		if slug == "" {
			rep.Skipped++
			r.warn("%s: la insignia no tiene un nombre válido; se omitió.", where)
			continue
		}
		if seen[slug] {
			rep.Skipped++
			r.warn("%s: la insignia %s está repetida; se conservó la primera.", where, name)
			continue
		}
		seen[slug] = true
		cur := r.badges[slug]
		if cur != nil && r.opt.mode == "create_only" {
			rep.Skipped++
			continue
		}
		where += " (" + name + ")"
		stock := r.nonNegative(where, "existencias", cell(1))
		cost := r.nonNegative(where, "costo", cell(2))
		if cur == nil {
			if err := r.createBadge(name, slug, stock, cost); err != nil {
				return err
			}
			rep.Created++
			continue
		}
		changed, err := r.updateBadge(cur, name, stock, cost)
		if err != nil {
			return err
		}
		if changed {
			rep.Updated++
		} else {
			rep.Unchanged++
		}
	}
	return nil
}

// nonNegative: la tabla no admite existencias ni costos negativos.
func (r *impRun) nonNegative(where, field, raw string) int {
	n := r.num(where, field, raw)
	if n < 0 {
		r.warn("%s: %s negativo; se dejó en 0.", where, field)
		return 0
	}
	return n
}

// impHiddenFor dice si una insignia nueva nace oculta en el curso: su nombre
// normalizado contiene alguna de las palabras del curso.
func impHiddenFor(c impCourse, badgeName string) bool {
	name := normalizeLabel(badgeName)
	for _, k := range c.Keywords {
		if k = normalizeLabel(k); k != "" && strings.Contains(name, k) {
			return true
		}
	}
	return false
}

func (r *impRun) createBadge(name, slug string, stock, cost int) error {
	r.maxSort++
	b := &impBadge{Name: name, Slug: slug, Stock: stock, Cost: cost, SortOrder: r.maxSort}
	if r.legacyBadges[slug] {
		b.ImageURL = mediaURL(impBadgesPrefix + slug + ".png")
	}
	var hiddenIDs []int64
	var hiddenNames []string
	for _, c := range r.courses {
		if impHiddenFor(c, name) {
			hiddenIDs = append(hiddenIDs, c.ID)
			hiddenNames = append(hiddenNames, c.Name)
		}
	}
	if r.opt.dry {
		b.ID = r.fakeID()
	} else {
		err := r.tx.QueryRow(r.ctx, `insert into badges (name, slug, stock, cost, image_url, sort_order)
			values ($1, $2, $3, $4, $5, $6) returning id, to_jsonb(badges)`,
			b.Name, b.Slug, b.Stock, b.Cost, b.ImageURL, b.SortOrder).Scan(&b.ID, &b.Snap)
		if err != nil {
			return err
		}
		if len(hiddenIDs) > 0 {
			if _, err := r.tx.Exec(r.ctx, `insert into course_hidden_badges (course_id, badge_id)
				select unnest($1::bigint[]), $2 on conflict do nothing`, hiddenIDs, b.ID); err != nil {
				return err
			}
		}
		summary := "Importación: insignia " + name + " creada"
		if len(hiddenNames) > 0 {
			summary += " (oculta en " + strings.Join(hiddenNames, ", ") + ")"
		}
		if err := r.audit("create", "badge", itoa64(b.ID), summary, nil, b.Snap); err != nil {
			return err
		}
	}
	r.badges[slug] = b
	r.badgeList = append(r.badgeList, b)
	return nil
}

func (r *impRun) updateBadge(b *impBadge, name string, stock, cost int) (bool, error) {
	var fields []string
	if b.Name != name {
		fields = append(fields, "nombre")
	}
	if b.Stock != stock {
		fields = append(fields, "existencias")
	}
	if b.Cost != cost {
		fields = append(fields, "costo")
	}
	if len(fields) == 0 {
		return false, nil
	}
	if !r.opt.dry {
		var after map[string]any
		err := r.tx.QueryRow(r.ctx, `update badges set name = $2, stock = $3, cost = $4, updated_at = now()
			where id = $1 returning to_jsonb(badges)`, b.ID, name, stock, cost).Scan(&after)
		if err != nil {
			return false, err
		}
		summary := fmt.Sprintf("Importación: insignia %s actualizada: %s", name, strings.Join(fields, ", "))
		if err := r.audit("update", "badge", itoa64(b.ID), summary, b.Snap, after); err != nil {
			return false, err
		}
		b.Snap = after
	}
	b.Name, b.Stock, b.Cost = name, stock, cost
	return true, nil
}

// ---------- Compras ----------

var impMexico = func() *time.Location {
	if loc, err := time.LoadLocation("America/Mexico_City"); err == nil {
		return loc
	}
	return time.FixedZone("CST", -6*60*60)
}()

// El servidor viejo guardaba new Date().toLocaleString() con USER_ENTERED, así
// que la celda trae ese texto o la fecha ya reformateada por Sheets. Se prueba
// en orden; el primero que entienda la fecha gana.
var impDateLayouts = []string{
	"1/2/2006, 3:04:05 PM", // toLocaleString en inglés
	"1/2/2006 3:04:05 PM",
	"2/1/2006, 15:04:05", // toLocaleString en español
	"1/2/2006 15:04:05",  // fecha y hora de Sheets
	"2/1/2006 15:04:05",
	"1/2/2006, 15:04:05",
	"2006-01-02 15:04:05",
	"1/2/2006 15:04",
	"2/1/2006 15:04",
	"1/2/2006",
	"2/1/2006",
	"2006-01-02",
}

// "a. m." / "p. m." solo salen de toLocaleString en español, que pone el día primero.
var (
	impMeridiem      = strings.NewReplacer("A. M.", "AM", "P. M.", "PM", "A.M.", "AM", "P.M.", "PM")
	impDateLayoutsES = []string{"2/1/2006, 3:04:05 PM", "2/1/2006 3:04:05 PM"}
)

// impParseDate interpreta la fecha en hora del centro de México.
func impParseDate(s string) (time.Time, bool) {
	s = strings.ToUpper(impSpaces(s))
	layouts := impDateLayouts
	if es := impMeridiem.Replace(s); es != s {
		s, layouts = es, impDateLayoutsES
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, impMexico); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// impSpaces colapsa los espacios, incluido el angosto que Node pone antes de "PM".
func impSpaces(s string) string {
	return strings.TrimFunc(collapseRuns(s, isJSSpace, " "), isJSSpace)
}

// importPurchases lee Purchases: A fecha, B código, C insignia, D cantidad,
// E carrera (las filas más viejas no la traen). Esas compras ya ocurrieron:
// no se descuentan monedas ni existencias.
func (r *impRun) importPurchases(tab *impTab) error {
	if tab == nil {
		return nil
	}
	rep := &r.rep.Purchases
	seen := map[string]int{}     // cuántas veces apareció cada compra idéntica
	unknown := map[string]bool{} // insignias fuera del catálogo, un aviso por nombre
	now := time.Now()
	for n, cells := range tab.rows {
		if n == 0 || impBlank(cells) {
			continue
		}
		rep.Rows++
		cell := impCells(cells)
		where := fmt.Sprintf("%s, fila %d", tab.name, n+1)
		date := impSpaces(cell(0))
		code := impText(cell(1)) // igual que en importStudents, para que casen
		badgeName := strings.TrimSpace(cell(2))
		qtyRaw := strings.TrimSpace(cell(3))
		career := strings.TrimSpace(cell(4))
		qty := parseIntLoose(qtyRaw)
		if code == "" || badgeName == "" || qty <= 0 {
			rep.Skipped++
			r.warn("%s: falta el alumno, la insignia o una cantidad válida; se omitió.", where)
			continue
		}

		// La huella no usa el número de fila, así reimportar no duplica aunque
		// las filas se muevan. Dos compras idénticas se distinguen por su orden.
		key := strings.Join([]string{date, code, normalizeLabel(badgeName), strconv.Itoa(qty), normalizeLabel(career)}, "\x1f")
		k := seen[key]
		seen[key] = k + 1
		sum := sha256.Sum256([]byte(key + "\x1f" + strconv.Itoa(k)))
		ref := "sheets:" + hex.EncodeToString(sum[:])
		if r.refs[ref] {
			rep.Skipped++ // ya se importó antes
			continue
		}

		st, why := r.matchStudent(code, career)
		if st == nil {
			rep.Unmatched++
			r.warn("%s: %s", where, why)
			continue
		}
		slug := slugify(badgeName)
		b := r.badges[slug]
		unit := 0
		if b != nil {
			unit = b.Cost
		}
		if int64(unit)*int64(qty) > math.MaxInt32 {
			rep.Skipped++
			r.warn("%s: la cantidad %q está fuera de rango; se omitió.", where, qtyRaw)
			continue
		}
		if impOdd(qtyRaw) {
			r.warn("%s: cantidad %q se leyó como %d.", where, qtyRaw, qty)
		}
		if b == nil && !unknown[slug] {
			unknown[slug] = true
			r.warn("La insignia %q de %s no está en el catálogo; sus compras quedan sin insignia ligada.", badgeName, tab.name)
		}
		at, ok := impParseDate(date)
		if !ok {
			at = now
			r.warn("%s: no se entendió la fecha %q; se usó la de la importación.", where, date)
		}
		if err := r.insertPurchase(st, b, badgeName, qty, unit, ref, at); err != nil {
			return err
		}
		r.refs[ref] = true
		rep.Created++
	}
	return nil
}

// matchStudent ubica al alumno de una compra. Con carrera se busca en ese
// curso; si falta, no se reconoce o el alumno no está ahí, sirve la única
// inscripción del código (el servidor viejo contaba las compras solo por código).
func (r *impRun) matchStudent(code, career string) (*impStudent, string) {
	if c := r.courseForCareer(career); c != nil {
		if st := r.students[impKey{c.ID, code}]; st != nil {
			return st, ""
		}
	}
	all := r.byCode[code]
	switch len(all) {
	case 1:
		return all[0], ""
	case 0:
		return nil, "no existe el alumno " + code + "; se omitió la compra."
	}
	if career == "" {
		return nil, "el alumno " + code + " está en varios cursos y la fila no trae carrera; se omitió la compra."
	}
	return nil, fmt.Sprintf("el alumno %s está en varios cursos y la carrera %q no indica cuál; se omitió la compra.", code, career)
}

// courseForCareer es getSheetNameCandidates del servidor viejo, pero con los
// datos de los cursos: primero coincidencia exacta con nombre, hoja o alias;
// luego el primer curso (por orden) con un alias contenido en la carrera.
func (r *impRun) courseForCareer(career string) *impCourse {
	n := normalizeLabel(career)
	if n == "" {
		return nil
	}
	for i := range r.courses {
		c := &r.courses[i]
		if n == normalizeLabel(c.Name) || n == normalizeLabel(c.SheetName) ||
			slices.ContainsFunc(c.Aliases, func(al string) bool { return normalizeLabel(al) == n }) {
			return c
		}
	}
	for i := range r.courses {
		c := &r.courses[i]
		if slices.ContainsFunc(c.Aliases, func(al string) bool {
			al = normalizeLabel(al)
			return al != "" && strings.Contains(n, al)
		}) {
			return c
		}
	}
	return nil
}

func (r *impRun) insertPurchase(st *impStudent, b *impBadge, badgeName string, qty, unit int, ref string, at time.Time) error {
	if r.opt.dry {
		return nil
	}
	var badgeID *int64
	var stock any // null si la insignia no está en el catálogo
	if b != nil {
		badgeID, stock = &b.ID, b.Stock
	}
	var id int64
	var snap map[string]any
	err := r.tx.QueryRow(r.ctx, `insert into purchases (student_id, badge_id, badge_name, quantity,
		unit_cost, total_cost, source, source_ref, created_at)
		values ($1, $2, $3, $4, $5, $6, 'sheets', $7, $8) returning id, to_jsonb(purchases)`,
		st.ID, badgeID, badgeName, qty, unit, unit*qty, ref, at).Scan(&id, &snap)
	if err != nil {
		return err
	}
	// Mismo formato que las compras de la app: la fila más monedas y existencias de ese momento.
	snap["student_coins"] = st.Monedas
	snap["badge_stock"] = stock
	summary := fmt.Sprintf("Importación: compra de %d × %s de %s", qty, badgeName, st.Code)
	return r.audit("create", "purchase", itoa64(id), summary, nil, snap)
}

// ---------- Media heredada ----------

// impAvatarName parte "02514123.png" en el código "0251412" y el nivel 3,
// igual que el frontend viejo armaba /avatars/{id}{level}.png.
func impAvatarName(file string) (code string, level int, ok bool) {
	ext := path.Ext(file)
	switch strings.ToLower(ext) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
	default:
		return "", 0, false
	}
	base := strings.TrimSuffix(file, ext)
	if len(base) < 2 || strings.Contains(base, "/") {
		return "", 0, false
	}
	d := base[len(base)-1]
	if d < '0' || d > '9' {
		return "", 0, false
	}
	return base[:len(base)-1], int(d - '0'), true
}

// impLegacyBadges da los slugs que tienen legacy/badges/{slug}.png.
func (a *App) impLegacyBadges(ctx context.Context) (map[string]bool, error) {
	objs, err := a.Store.List(ctx, impBadgesPrefix)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, o := range objs {
		slug, ok := strings.CutSuffix(strings.TrimPrefix(o.Key, impBadgesPrefix), ".png")
		if ok && slug != "" && !strings.Contains(slug, "/") {
			set[slug] = true
		}
	}
	return set, nil
}

func (a *App) impLegacyCounts(ctx context.Context) (avatars, badges int, err error) {
	objs, err := a.Store.List(ctx, impAvatarsPrefix)
	if err != nil {
		return 0, 0, err
	}
	for _, o := range objs {
		if _, _, ok := impAvatarName(strings.TrimPrefix(o.Key, impAvatarsPrefix)); ok {
			avatars++
		}
	}
	set, err := a.impLegacyBadges(ctx)
	return avatars, len(set), err
}

// importMedia registra los avatares de legacy/avatars/ en las casillas que aún
// no tienen uno (nunca pisa lo que subió un admin) y pone imagen heredada a las
// insignias que no tienen.
func (r *impRun) importMedia() error {
	objs, err := r.a.Store.List(r.ctx, impAvatarsPrefix)
	if err != nil {
		return err
	}
	levels, err := r.a.Levels(r.ctx)
	if err != nil {
		return err
	}
	known := map[int]bool{}
	for _, l := range levels {
		known[l.Level] = true
	}
	rows, err := r.tx.Query(r.ctx, `select code || ':' || level from avatars`)
	if err != nil {
		return err
	}
	slots, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	taken := map[string]bool{}
	for _, s := range slots {
		taken[s] = true
	}

	picked := map[string]string{} // casilla → archivo que la tomó
	for _, o := range objs {
		file := strings.TrimPrefix(o.Key, impAvatarsPrefix)
		code, level, ok := impAvatarName(file)
		if !ok {
			r.warn("Avatar heredado con nombre no reconocido: %s; se omitió.", file)
			continue
		}
		if !known[level] {
			r.warn("Avatar %s: el nivel %d no existe en Niveles; se omitió.", file, level)
			continue
		}
		slot := code + ":" + strconv.Itoa(level)
		if prev, dup := picked[slot]; dup {
			r.warn("Avatar %s: %s ya ocupa ese código y nivel; se omitió.", file, prev)
			continue
		}
		picked[slot] = file
		if taken[slot] {
			continue
		}
		if !r.opt.dry {
			var snap map[string]any
			err := r.tx.QueryRow(r.ctx, `insert into avatars (code, level, url) values ($1, $2, $3)
				on conflict do nothing returning to_jsonb(avatars)`, code, level, mediaURL(o.Key)).Scan(&snap)
			if errors.Is(err, pgx.ErrNoRows) {
				continue // alguien la ocupó mientras tanto
			}
			if err != nil {
				return err
			}
			summary := fmt.Sprintf("Importación: avatar heredado de %s, nivel %d", code, level)
			if err := r.audit("create", "avatar", slot, summary, nil, snap); err != nil {
				return err
			}
		}
		taken[slot] = true
		r.rep.Media.Avatars++
	}

	for _, b := range r.badgeList {
		if b.ImageURL != "" || !r.legacyBadges[b.Slug] {
			continue
		}
		url := mediaURL(impBadgesPrefix + b.Slug + ".png")
		if !r.opt.dry {
			var after map[string]any
			err := r.tx.QueryRow(r.ctx, `update badges set image_url = $2, updated_at = now()
				where id = $1 returning to_jsonb(badges)`, b.ID, url).Scan(&after)
			if err != nil {
				return err
			}
			summary := "Importación: imagen heredada para la insignia " + b.Name
			if err := r.audit("update", "badge", itoa64(b.ID), summary, b.Snap, after); err != nil {
				return err
			}
			b.Snap = after
		}
		b.ImageURL = url
		r.rep.Media.BadgeImages++
	}
	return nil
}

// ---------- Estado ----------

// impLastImport devuelve la entrada resumen de la última importación.
func impLastImport(ctx context.Context, q DBTX) (*auditEntryJSON, error) {
	var e auditEntryJSON
	var before, after []byte
	err := q.QueryRow(ctx, `select id, at, actor_type, actor_label, action, entity, entity_id, summary,
		before, after, batch_id::text, undoable, undone_at, reverts_id
		from audit_log where entity = 'import' and action = 'import' order by id desc limit 1`).
		Scan(&e.ID, &e.At, &e.ActorType, &e.ActorLabel, &e.Action, &e.Entity, &e.EntityID, &e.Summary,
			&before, &after, &e.BatchID, &e.Undoable, &e.UndoneAt, &e.RevertsID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if before != nil {
		e.Before = json.RawMessage(before)
	}
	if after != nil {
		e.After = json.RawMessage(after)
	}
	return &e, nil
}
