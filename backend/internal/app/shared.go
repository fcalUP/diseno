package app

// Formas JSON compartidas entre los manejadores del panel. Son el espejo de
// frontend/src/lib/types.ts: si cambia una, cambia la otra.

import (
	"context"
	"time"
)

type adminStudentJSON struct {
	ID             int64      `json:"id"`
	Code           string     `json:"code"`
	CourseID       int64      `json:"courseId"`
	CourseName     string     `json:"courseName"`
	Name           string     `json:"name"`
	Sexo           string     `json:"sexo"`
	Email          string     `json:"email"`
	Tareas         int        `json:"tareas"`
	Monedas        int        `json:"monedas"`
	Asistencias    int        `json:"asistencias"`
	Games          int        `json:"games"`
	Exp            int        `json:"exp"`
	Level          int        `json:"level"`
	LastLoginLevel int        `json:"lastLoginLevel"`
	Badges         int        `json:"badges"`
	Comments       string     `json:"comments"`
	Active         bool       `json:"active"`
	LastLoginAt    *time.Time `json:"lastLoginAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	AvatarLevels   []int      `json:"avatarLevels"`
}

type adminCourseJSON struct {
	ID                  int64     `json:"id"`
	Name                string    `json:"name"`
	SheetName           string    `json:"sheetName"`
	SheetAliases        []string  `json:"sheetAliases"`
	SideImageURL        string    `json:"sideImageUrl"`
	LogoURL             string    `json:"logoUrl"`
	HiddenBadgeKeywords []string  `json:"hiddenBadgeKeywords"`
	Active              bool      `json:"active"`
	SortOrder           int       `json:"sortOrder"`
	StudentCount        int       `json:"studentCount"`
	HiddenBadgeIDs      []int64   `json:"hiddenBadgeIds"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

type adminBadgeJSON struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	Slug            string    `json:"slug"`
	Stock           int       `json:"stock"`
	Cost            int       `json:"cost"`
	ImageURL        string    `json:"imageUrl"`
	Active          bool      `json:"active"`
	SortOrder       int       `json:"sortOrder"`
	HiddenCourseIDs []int64   `json:"hiddenCourseIds"`
	PurchasedCount  int       `json:"purchasedCount"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type avatarJSON struct {
	Code      string    `json:"code"`
	Level     int       `json:"level"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type adminPurchaseJSON struct {
	ID          int64      `json:"id"`
	StudentID   int64      `json:"studentId"`
	StudentCode string     `json:"studentCode"`
	StudentName string     `json:"studentName"`
	CourseID    int64      `json:"courseId"`
	CourseName  string     `json:"courseName"`
	BadgeID     *int64     `json:"badgeId"`
	BadgeName   string     `json:"badgeName"`
	Quantity    int        `json:"quantity"`
	UnitCost    int        `json:"unitCost"`
	TotalCost   int        `json:"totalCost"`
	CoinsBefore *int       `json:"coinsBefore"`
	CoinsAfter  *int       `json:"coinsAfter"`
	Source      string     `json:"source"`
	CreatedAt   time.Time  `json:"createdAt"`
	RevertedAt  *time.Time `json:"revertedAt"`
}

type adminAccountJSON struct {
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	Active      bool       `json:"active"`
	LastLoginAt *time.Time `json:"lastLoginAt"`
	CreatedAt   time.Time  `json:"createdAt"`
}

type auditEntryJSON struct {
	ID         int64      `json:"id"`
	At         time.Time  `json:"at"`
	ActorType  string     `json:"actorType"`
	ActorLabel string     `json:"actorLabel"`
	Action     string     `json:"action"`
	Entity     string     `json:"entity"`
	EntityID   *string    `json:"entityId"`
	Summary    string     `json:"summary"`
	Before     any        `json:"before"`
	After      any        `json:"after"`
	BatchID    *string    `json:"batchId"`
	Undoable   bool       `json:"undoable"`
	UndoneAt   *time.Time `json:"undoneAt"`
	RevertsID  *int64     `json:"revertsId"`
}

type secretStatusJSON struct {
	Configured  bool   `json:"configured"`
	Source      string `json:"source"`
	ClientEmail string `json:"clientEmail,omitempty"`
}

// adminLabel devuelve el usuario del admin para la bitácora; si ya no existe, "admin #id".
func (a *App) adminLabel(ctx context.Context, q DBTX, adminID int64) string {
	var username string
	if err := q.QueryRow(ctx, `select username from admins where id = $1`, adminID).Scan(&username); err != nil {
		return "admin #" + itoa64(adminID)
	}
	return username
}

// auditAdmin registra una entrada hecha por un admin llenando los datos del actor.
func (a *App) auditAdmin(ctx context.Context, q DBTX, adminID int64, e AuditEntry) (int64, error) {
	id := adminID
	e.ActorType = "admin"
	e.ActorID = &id
	if e.ActorLabel == "" {
		e.ActorLabel = a.adminLabel(ctx, q, adminID)
	}
	return a.Audit(ctx, q, e)
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
