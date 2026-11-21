package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Ayudas para pruebas; B1/B2/B3 pueden usarlas (ver docs/routes_contract.md).

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// isolatedConfig es una Config de pruebas: almacenamiento local en carpetas temporales.
func isolatedConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		Port:          "8080",
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		SessionSecret: strings.Repeat("x", 32),
		PublicURL:     "http://localhost:5173",
		WebDir:        t.TempDir(),
		StorageDriver: "local",
		MediaDir:      t.TempDir(),
		S3Prefix:      "media/",
		AWSRegion:     "us-east-1",
		AdminUsername: "Admin",
	}
}

// freshSchema abre un pool sobre un esquema vacío y propio de la prueba; lo borra al final.
// Se salta la prueba si no hay DATABASE_URL.
func freshSchema(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL no definida")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := "t_" + randomToken(6)
	if _, err := admin.Exec(ctx, "create schema "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "drop schema "+schema+" cascade")
		admin.Close()
	})
	return pool
}

// isolatedDB es freshSchema con las migraciones ya aplicadas (incluye los datos semilla).
func isolatedDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	db := freshSchema(t)
	if err := Migrate(context.Background(), db, quietLogger()); err != nil {
		t.Fatal(err)
	}
	return db
}

// isolatedApp arma un App completo sobre isolatedDB.
func isolatedApp(t *testing.T) *App {
	t.Helper()
	db := isolatedDB(t)
	a, err := New(context.Background(), isolatedConfig(t), db, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// appNoDB sirve para probar lo que no toca la base.
func appNoDB(t *testing.T) *App {
	t.Helper()
	a, err := New(context.Background(), isolatedConfig(t), nil, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestRoutesUnknownAPI(t *testing.T) {
	h := appNoDB(t).Routes() // registrar todas las rutas no debe hacer panic
	for _, method := range []string{"GET", "DELETE"} {
		req := httptest.NewRequest(method, "/api/no-existe", nil)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"error"`) {
			t.Fatalf("%s: esperaba 404 JSON, llegó %d %s", method, rec.Code, rec.Body)
		}
	}
}

func TestHealth(t *testing.T) {
	h := isolatedApp(t).Routes()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/health", nil))
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"ok":true}` {
		t.Fatalf("health: %d %s", rec.Code, rec.Body)
	}
}

func TestBootstrapAdmin(t *testing.T) {
	a := isolatedApp(t)
	ctx := context.Background()

	a.Cfg.AdminPassword = "corta"
	if err := a.BootstrapAdmin(ctx); err == nil {
		t.Fatal("una contraseña de menos de 8 caracteres debió fallar")
	}

	a.Cfg.AdminPassword = "una-clave-larga"
	for range 2 { // la segunda vez no hace nada
		if err := a.BootstrapAdmin(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	var hash string
	if err := a.DB.QueryRow(ctx, "select count(*), max(password_hash) from admins where username = 'Admin'").Scan(&n, &hash); err != nil {
		t.Fatal(err)
	}
	if n != 1 || !checkPassword(hash, "una-clave-larga") {
		t.Fatalf("admins=%d, contraseña correcta=%v", n, checkPassword(hash, "una-clave-larga"))
	}
	var undoable bool
	if err := a.DB.QueryRow(ctx, "select undoable from audit_log where entity = 'admin' and action = 'create'").Scan(&undoable); err != nil {
		t.Fatal(err)
	}
	if undoable {
		t.Fatal("la creación del admin inicial no debe poder deshacerse")
	}
}
