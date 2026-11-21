package app

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX lo cumplen *pgxpool.Pool y pgx.Tx.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type AuditEntry struct {
	ActorType  string // "admin" | "student" | "system"
	ActorID    *int64
	ActorLabel string // usuario, matrícula o "sistema"
	Action     string // create|update|delete|purchase|revert|restore|import|undo|redo|login...
	Entity     string // student|course|badge|visibility|level|avatar|setting|admin|purchase|import
	EntityID   string // "" si no aplica; avatar = "code:level"
	Summary    string // en español, para personas
	Before     any    // instantánea o nil (se guarda como jsonb)
	After      any
	BatchID    string // "" o uuid
	Undoable   bool
	RevertsID  *int64
}

// Audit inserta una entrada en audit_log y devuelve su id. Solo registra; deshacer vive en undo.go.
func (a *App) Audit(ctx context.Context, q DBTX, e AuditEntry) (int64, error) {
	before, err := jsonParam(e.Before)
	if err != nil {
		return 0, fmt.Errorf("auditoría (before): %w", err)
	}
	after, err := jsonParam(e.After)
	if err != nil {
		return 0, fmt.Errorf("auditoría (after): %w", err)
	}
	var id int64
	err = q.QueryRow(ctx, `insert into audit_log
		(actor_type, actor_id, actor_label, action, entity, entity_id, summary,
		 before, after, batch_id, undoable, reverts_id)
		values ($1, $2, $3, $4, $5, nullif($6, ''), $7, $8, $9, nullif($10, '')::uuid, $11, $12)
		returning id`,
		e.ActorType, e.ActorID, e.ActorLabel, e.Action, e.Entity, e.EntityID, e.Summary,
		before, after, e.BatchID, e.Undoable, e.RevertsID).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("auditoría: %w", err)
	}
	return id, nil
}

// jsonParam convierte v a jsonb; nil (o algo que serializa a null) queda como NULL.
func jsonParam(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return nil, err
	}
	return json.RawMessage(b), nil
}

// newBatchID genera un uuid v4 para agrupar entradas de auditoría.
func newBatchID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Tablas que snapshotRow acepta y cómo se ubica la fila. Los avatares tienen
// llave compuesta: id es el texto "code:level", igual que AuditEntry.EntityID.
var snapshotWhere = map[string]string{
	"students":  "t.id = $1",
	"courses":   "t.id = $1",
	"badges":    "t.id = $1",
	"purchases": "t.id = $1",
	"admins":    "t.id = $1",
	"avatars":   "t.code || ':' || t.level = $1::text",
}

// snapshotRow devuelve la fila como to_jsonb (columnas en snake_case).
// Si no existe devuelve pgx.ErrNoRows.
func snapshotRow(ctx context.Context, q DBTX, table string, id any) (map[string]any, error) {
	where, ok := snapshotWhere[table]
	if !ok {
		return nil, fmt.Errorf("snapshotRow: tabla no permitida %q", table)
	}
	var row map[string]any
	err := q.QueryRow(ctx, "select to_jsonb(t) from "+table+" t where "+where, id).Scan(&row)
	if err != nil {
		return nil, err
	}
	return row, nil
}
