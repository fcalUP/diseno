package app

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Regresión: cuando el admin cambiaba de página con el Resumen cargando, el log
// marcaba ERROR "context canceled" y la respuesta era 500. Que el navegador corte
// la petición no es una falla del servidor.
func TestCanceledRequestIsNotAnError(t *testing.T) {
	e := admSetup(t)
	var logs bytes.Buffer
	e.a.Log = slog.New(slog.NewTextHandler(&logs, nil))

	sid := e.student(e.courseID("TAR"), "C001", "Cancela", 0, 0)
	rec := httptest.NewRecorder()
	e.a.setSession(rec, kindStudent, sid)
	stu := rec.Result().Cookies()[0]

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, c := range []struct {
		path   string
		cookie *http.Cookie
	}{
		{"/api/admin/stats", e.cookie},
		{"/api/admin/students", e.cookie},
		{"/api/leaderboard", stu},
	} {
		req := httptest.NewRequest("GET", c.path, nil).WithContext(ctx)
		req.AddCookie(c.cookie)
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		if rec.Code == http.StatusInternalServerError {
			t.Errorf("%s cancelada: no debe ser 500 (%s)", c.path, rec.Body)
		}
	}

	// Los tres escritores de errores de los manejadores, con el error envuelto como llega de pgx.
	wrapped := fmt.Errorf("consulta: %w", context.Canceled)
	for name, write := range map[string]func(http.ResponseWriter){
		"admFail":        func(w http.ResponseWriter) { e.a.admFail(w, "prueba", wrapped) },
		"undoWriteError": func(w http.ResponseWriter) { e.a.undoWriteError(w, "prueba", wrapped) },
		"stuInternal":    func(w http.ResponseWriter) { e.a.stuInternal(w, "prueba", wrapped) },
	} {
		rec := httptest.NewRecorder()
		write(rec)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: esperaba 400, llegó %d", name, rec.Code)
		}
	}
	if strings.Contains(logs.String(), "level=ERROR") {
		t.Fatalf("una petición cancelada no debe ir al log como error:\n%s", logs.String())
	}

	// Un error real sí se sigue registrando.
	e.a.admFail(httptest.NewRecorder(), "prueba", fmt.Errorf("falla de verdad"))
	if !strings.Contains(logs.String(), "level=ERROR") {
		t.Fatal("un error real debe ir al log")
	}
}
