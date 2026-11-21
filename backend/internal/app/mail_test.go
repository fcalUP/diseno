package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

// mailFakeReq es lo que recibió el Resend falso.
type mailFakeReq struct {
	Method, Path, Auth, UserAgent, ContentType string
	Body                                       struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		HTML    string   `json:"html"`
		Text    string   `json:"text"`
	}
}

// mailFake levanta un Resend falso que contesta status, pone una API key de
// prueba y deja a.Mail apuntando a él. Cada correo recibido llega por el canal.
func mailFake(t *testing.T, a *App, status int) <-chan mailFakeReq {
	t.Helper()
	got := make(chan mailFakeReq, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := mailFakeReq{
			Method:      r.Method,
			Path:        r.URL.Path,
			Auth:        r.Header.Get("Authorization"),
			UserAgent:   r.Header.Get("User-Agent"),
			ContentType: r.Header.Get("Content-Type"),
		}
		_ = json.NewDecoder(r.Body).Decode(&req.Body)
		got <- req
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"id":"prueba"}`))
	}))
	t.Cleanup(srv.Close)

	old := mailResendBaseURL
	mailResendBaseURL = srv.URL
	a.Mail = newResendMailer(a) // toma la URL al crearse
	mailResendBaseURL = old

	a.Cfg.ResendAPIKey = "re_prueba"
	a.invalidateSettings()
	return got
}

func mailWait(t *testing.T, ch <-chan mailFakeReq) mailFakeReq {
	t.Helper()
	select {
	case m := <-ch:
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("el correo no llegó al Resend falso")
	}
	return mailFakeReq{}
}

func TestMailResendSend(t *testing.T) {
	a := isolatedApp(t)
	ctx := context.Background()
	got := mailFake(t, a, http.StatusOK)

	msg := Mail{From: "portal@up.edu.mx", To: []string{"a@up.edu.mx", "b@up.edu.mx"}, Subject: "Hola", Text: "texto"}
	if err := a.Mail.Send(ctx, msg); err != nil {
		t.Fatal(err)
	}
	m := mailWait(t, got)
	if m.Method != http.MethodPost || m.Path != "/emails" || m.Auth != "Bearer re_prueba" ||
		m.ContentType != "application/json" || m.UserAgent == "" {
		t.Fatalf("petición inesperada: %+v", m)
	}
	if m.Body.From != msg.From || !slices.Equal(m.Body.To, msg.To) || m.Body.Subject != "Hola" ||
		m.Body.Text != "texto" || m.Body.HTML != "" {
		t.Fatalf("cuerpo inesperado: %+v", m.Body)
	}

	// La key guardada en Ajustes gana al env y aplica sin reiniciar.
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SaveSettings(ctx, tx, map[string]any{"resend_api_key": "re_panel"}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	a.invalidateSettings()
	if err := a.Mail.Send(ctx, msg); err != nil {
		t.Fatal(err)
	}
	if m := mailWait(t, got); m.Auth != "Bearer re_panel" {
		t.Fatalf("Authorization = %q", m.Auth)
	}
}

func TestMailResendErrors(t *testing.T) {
	a := isolatedApp(t)
	ctx := context.Background()
	msg := Mail{From: "portal@up.edu.mx", To: []string{"a@up.edu.mx"}, Subject: "Hola", HTML: "<p>x</p>"}

	got := mailFake(t, a, http.StatusUnprocessableEntity)
	if err := a.Mail.Send(ctx, msg); err == nil || !strings.Contains(err.Error(), "422") {
		t.Fatalf("esperaba error con el status de Resend, llegó %v", err)
	}
	mailWait(t, got)

	// Sin API key no se intenta nada.
	a.Cfg.ResendAPIKey = ""
	a.invalidateSettings()
	if err := a.Mail.Send(ctx, msg); err == nil {
		t.Fatal("sin API key debió fallar")
	}
	select {
	case m := <-got:
		t.Fatalf("no debió llamar a Resend: %+v", m)
	default:
	}
}

func TestMailTemplates(t *testing.T) {
	s := defaultSettings()
	s.EmailCc = []string{" profe@up.edu.mx ", "", "ana@correo.mx"}
	if got := mailRecipients(s, " ana@correo.mx ", "A001"); !slices.Equal(got, []string{"ana@correo.mx", "profe@up.edu.mx"}) {
		t.Fatalf("destinatarios: %q", got)
	}
	if got := mailRecipients(s, "", "A001"); got[0] != "A001@up.edu.mx" {
		t.Fatalf("sin correo debe usar matrícula@dominio: %q", got)
	}

	m := mailResetCode("portal@up.edu.mx", []string{"a@up.edu.mx"}, "042137", 10)
	if m.Subject != "Código de restablecimiento de contraseña" || m.Text != "" ||
		!strings.Contains(m.HTML, `<h1 style="color:#ff6600;">042137</h1>`) ||
		!strings.Contains(m.HTML, "<strong>10 minutos</strong>") {
		t.Fatalf("correo de código: %+v", m)
	}
	if m := mailResetCode("x", nil, "1", 1); !strings.Contains(m.HTML, "<strong>1 minuto</strong>") {
		t.Fatalf("singular: %s", m.HTML)
	}

	// Con una sola insignia el texto queda igual que el del servidor viejo.
	one := mailPurchase("portal@up.edu.mx", []string{"A001@up.edu.mx", "fcal@up.edu.mx"}, "TAR",
		[]mailItem{{Name: "Lápiz", Quantity: 2}}, 50, 30)
	want := "Hola,\n\nSe ha realizado una compra de insignia:\n\n- Alumno: A001@up.edu.mx\n- Carrera: TAR\n" +
		"- Insignia: Lápiz\n- Cantidad: 2\n- Costo total: 20 monedas\n- Monedas anteriores: 50\n" +
		"- Monedas restantes: 30\n\nGracias."
	if one.Text != want || one.Subject != "Confirmación de compra de insignia" || one.HTML != "" || one.From != "portal@up.edu.mx" {
		t.Fatalf("compra de una insignia:\n%s\n%+v", one.Text, one)
	}
	many := mailPurchase("x", []string{"A001@up.edu.mx"}, "TAR",
		[]mailItem{{Name: "Lápiz", Quantity: 2}, {Name: "Goma", Quantity: 1}}, 50, 5)
	if many.Subject != "Confirmación de compra de insignias" ||
		!strings.Contains(many.Text, "- Insignia: Lápiz\n- Cantidad: 2\n- Insignia: Goma\n- Cantidad: 1\n- Costo total: 45 monedas") {
		t.Fatalf("compra de varias:\n%s", many.Text)
	}
}
