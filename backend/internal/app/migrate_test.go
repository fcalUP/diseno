package app

import (
	"context"
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
	if applied != 2 || levels != 6 || courses != 5 || activeCourses != 3 {
		t.Fatalf("migraciones=%d niveles=%d cursos=%d activos=%d", applied, levels, courses, activeCourses)
	}
}
