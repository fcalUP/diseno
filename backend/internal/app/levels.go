package app

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

type Level struct {
	Level  int `json:"level"`
	MinExp int `json:"minExp"`
}

// Levels devuelve los niveles ordenados por nivel (con caché).
func (a *App) Levels(ctx context.Context) ([]Level, error) {
	a.levelsMu.Lock()
	defer a.levelsMu.Unlock()
	if a.levelsAt.IsZero() || time.Since(a.levelsAt) > cacheTTL {
		rows, err := a.DB.Query(ctx, "select level, min_exp from levels order by level")
		if err != nil {
			return nil, fmt.Errorf("leer niveles: %w", err)
		}
		levels, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Level])
		if err != nil {
			return nil, fmt.Errorf("leer niveles: %w", err)
		}
		a.levels, a.levelsAt = levels, time.Now()
	}
	return slices.Clone(a.levels), nil
}

func (a *App) invalidateLevels() {
	a.levelsMu.Lock()
	a.levelsAt = time.Time{}
	a.levelsMu.Unlock()
}

// CalcLevel da el nivel más alto cuyo minExp <= exp (0 si no alcanza ninguno).
func CalcLevel(exp int, levels []Level) int {
	lvl := 0
	for _, l := range levels {
		if l.MinExp <= exp && l.Level > lvl {
			lvl = l.Level
		}
	}
	return lvl
}
