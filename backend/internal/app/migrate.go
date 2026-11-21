package app

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Número fijo para pg_advisory_lock: si arrancan dos procesos, uno espera al otro.
const migrationLockID int64 = 72540311

// Migrate aplica en orden alfabético los .sql que falten, cada uno en su
// propia transacción, y los anota en schema_migrations.
func Migrate(ctx context.Context, db *pgxpool.Pool, log *slog.Logger) error {
	conn, err := db.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("migraciones: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "select pg_advisory_lock($1)", migrationLockID); err != nil {
		return fmt.Errorf("migraciones (lock): %w", err)
	}
	defer conn.Exec(context.Background(), "select pg_advisory_unlock($1)", migrationLockID)

	if _, err := conn.Exec(ctx, `create table if not exists schema_migrations (
		version    text primary key,
		applied_at timestamptz not null default now()
	)`); err != nil {
		return fmt.Errorf("migraciones: %w", err)
	}

	entries, err := fs.ReadDir(migrationFiles, "migrations") // ya vienen ordenadas por nombre
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		var done bool
		if err := conn.QueryRow(ctx, "select exists (select 1 from schema_migrations where version = $1)", name).Scan(&done); err != nil {
			return fmt.Errorf("migraciones: %w", err)
		}
		if done {
			continue
		}
		sql, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		// Sin argumentos pgx usa el protocolo simple, que admite varias sentencias.
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migración %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, "insert into schema_migrations (version) values ($1)", name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migración %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("migración %s: %w", name, err)
		}
		log.Info("migración aplicada", "version", name)
	}
	return nil
}
