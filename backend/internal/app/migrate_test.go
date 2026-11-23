package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateTwice(t *testing.T) {
	db := freshSchema(t)
	ctx := context.Background()
	for i := range 2 { // la segunda pasada no debe hacer nada
		if err := Migrate(ctx, db, quietLogger()); err != nil {
			t.Fatalf("pasada %d: %v", i+1, err)
		}
	}
	var applied, levels, courses, activeCourses int
	err := db.QueryRow(ctx, `select
		(select count(*) from schema_migrations),
		(select count(*) from levels),
		(select count(*) from courses),
		(select count(*) from courses where active)`).Scan(&applied, &levels, &courses, &activeCourses)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 3 || levels != 6 || courses != 5 || activeCourses != 3 {
		t.Fatalf("migraciones=%d niveles=%d cursos=%d activos=%d", applied, levels, courses, activeCourses)
	}
}

func TestMigrateDefaultAvatars(t *testing.T) {
	db := isolatedDB(t)
	ctx := context.Background()
	rows, err := db.Query(ctx, `select code, level, url from avatars order by code, level`)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	for rows.Next() {
		var code, url string
		var level int
		if err := rows.Scan(&code, &level, &url); err != nil {
			t.Fatal(err)
		}
		if (code != "DefaultH" && code != "DefaultF") || level < 0 || level > 5 {
			t.Fatalf("avatar predeterminado inesperado: %s %d", code, level)
		}
		filename := filepath.Join("..", "..", "..", "seed", "media", strings.TrimPrefix(url, "/media/"))
		if _, err := os.Stat(filename); err != nil {
			t.Fatalf("avatar predeterminado sin archivo: %s: %v", url, err)
		}
		n++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if n != 12 {
		t.Fatalf("esperaba 12 avatares por defecto, hay %d", n)
	}
	// Simula volver a ejecutar la actualización sobre una instalación que ya
	// tiene el avatar personalizado del profesor: no debe sobrescribirse.
	if _, err := db.Exec(ctx, `update avatars set url = '/media/u/custom.png' where code = 'DefaultF' and level = 2;
		delete from schema_migrations where version = '0003_default_avatars.sql'`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db, quietLogger()); err != nil {
		t.Fatal(err)
	}
	var url string
	if err := db.QueryRow(ctx, `select url from avatars where code = 'DefaultF' and level = 2`).Scan(&url); err != nil {
		t.Fatal(err)
	}
	if url != "/media/u/custom.png" {
		t.Fatalf("se sobrescribió el avatar personalizado: %s", url)
	}
}
