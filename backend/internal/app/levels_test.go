package app

import (
	"context"
	"testing"
)

// Umbrales sembrados en 0002_seed.sql; deben dar lo mismo que calculateLevel del server.js viejo.
var legacySeedLevels = []Level{{0, 0}, {1, 10}, {2, 25}, {3, 50}, {4, 75}, {5, 100}}

func legacyCalculateLevel(exp int) int {
	switch {
	case exp >= 100:
		return 5
	case exp >= 75:
		return 4
	case exp >= 50:
		return 3
	case exp >= 25:
		return 2
	case exp >= 10:
		return 1
	}
	return 0
}

func TestCalcLevel(t *testing.T) {
	cases := map[int]int{0: 0, 9: 0, 10: 1, 24: 1, 25: 2, 100: 5, 250: 5, -3: 0}
	for exp, want := range cases {
		if got := CalcLevel(exp, legacySeedLevels); got != want {
			t.Errorf("CalcLevel(%d) = %d, esperaba %d", exp, got, want)
		}
	}
	for exp := -5; exp <= 300; exp++ {
		if CalcLevel(exp, legacySeedLevels) != legacyCalculateLevel(exp) {
			t.Fatalf("difiere del código viejo con exp=%d", exp)
		}
	}
	if CalcLevel(50, nil) != 0 {
		t.Fatal("sin niveles debe dar 0")
	}
}

func TestLevelsFromDB(t *testing.T) {
	a := isolatedApp(t)
	ctx := context.Background()
	levels, err := a.Levels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(levels) != len(legacySeedLevels) {
		t.Fatalf("niveles = %v", levels)
	}
	for i := range levels {
		if levels[i] != legacySeedLevels[i] {
			t.Fatalf("niveles = %v", levels)
		}
	}
	// Con caché: un cambio en la tabla se ve solo después de invalidar.
	if _, err := a.DB.Exec(ctx, "insert into levels (level, min_exp) values (6, 150)"); err != nil {
		t.Fatal(err)
	}
	if levels, _ = a.Levels(ctx); len(levels) != 6 {
		t.Fatal("esperaba el valor en caché")
	}
	a.invalidateLevels()
	if levels, _ = a.Levels(ctx); len(levels) != 7 || CalcLevel(150, levels) != 6 {
		t.Fatalf("tras invalidar: %v", levels)
	}
}
