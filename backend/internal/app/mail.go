package app

// Correo con el API HTTP de Resend y las plantillas heredadas.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
)

// Base del API de Resend; las pruebas la apuntan a un httptest.
var mailResendBaseURL = "https://api.resend.com"

// resendMailer lee la API key de Ajustes en cada envío, así un cambio hecho
// en el panel aplica sin reiniciar.
type resendMailer struct {
	a      *App
	url    string
	client *http.Client
}

func newResendMailer(a *App) Mailer {
	return &resendMailer{a: a, url: mailResendBaseURL + "/emails", client: &http.Client{Timeout: 10 * time.Second}}
}

func (m *resendMailer) Send(ctx context.Context, msg Mail) error {
	s, err := m.a.Settings(ctx)
	if err != nil {
		return err
	}
	if s.ResendAPIKey == "" {
		return errors.New("no hay API key de Resend configurada")
	}
	body, err := json.Marshal(struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		HTML    string   `json:"html,omitempty"`
		Text    string   `json:"text,omitempty"`
	}{msg.From, msg.To, msg.Subject, msg.HTML, msg.Text})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.ResendAPIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "diseno-portal/1.0") // Resend rechaza peticiones sin User-Agent
	res, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("resend: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		detail, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("resend respondió %d: %s", res.StatusCode, bytes.TrimSpace(detail))
	}
	return nil
}

// mailAsync manda m en segundo plano con su propio límite de tiempo. Un fallo
// solo queda en el log; nunca toca la respuesta de quien lo pidió.
func (a *App) mailAsync(m Mail, what string) {
	mailer := a.Mail
	go func() {
		defer func() {
			if v := recover(); v != nil {
				a.Log.Error("panic al enviar correo", "tipo", what, "err", fmt.Sprint(v))
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := mailer.Send(ctx, m); err != nil {
			a.Log.Error("no se pudo enviar el correo", "tipo", what, "err", err)
		}
	}()
}

// mailRecipients: el correo del alumno (matrícula@dominio si no tiene) más las copias de Ajustes.
func mailRecipients(s Settings, email, code string) []string {
	to := []string{strings.TrimSpace(email)}
	if to[0] == "" {
		to[0] = code + "@" + s.StudentEmailDomain
	}
	for _, cc := range s.EmailCc {
		if cc = strings.TrimSpace(cc); cc != "" && !slices.Contains(to, cc) {
			to = append(to, cc)
		}
	}
	return to
}

// mailResetCode es el correo HTML de siempre, con la vigencia que diga Ajustes.
func mailResetCode(from string, to []string, code string, ttlMinutes int) Mail {
	unit := "minutos"
	if ttlMinutes == 1 {
		unit = "minuto"
	}
	html := fmt.Sprintf(`<h2>Código de restablecimiento de contraseña</h2>
<p>Tu código de verificación es:</p>
<h1 style="color:#ff6600;">%s</h1>
<p>Este código expirará en <strong>%d %s</strong>.</p>`, code, ttlMinutes, unit)
	return Mail{From: from, To: to, Subject: "Código de restablecimiento de contraseña", HTML: html}
}

type mailItem struct {
	Name     string
	Quantity int
}

// mailPurchase es el aviso de compra en texto plano del servidor viejo; ahora
// lista todas las insignias del carrito. to[0] es el correo del alumno.
func mailPurchase(from string, to []string, career string, items []mailItem, before, after int) Mail {
	subject, what := "Confirmación de compra de insignia", "insignia"
	if len(items) > 1 {
		subject, what = "Confirmación de compra de insignias", "insignias"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Hola,\n\nSe ha realizado una compra de %s:\n\n- Alumno: %s\n- Carrera: %s\n", what, to[0], career)
	for _, it := range items {
		fmt.Fprintf(&b, "- Insignia: %s\n- Cantidad: %d\n", it.Name, it.Quantity)
	}
	fmt.Fprintf(&b, "- Costo total: %d monedas\n- Monedas anteriores: %d\n- Monedas restantes: %d\n\nGracias.",
		before-after, before, after)
	return Mail{From: from, To: to, Subject: subject, Text: b.String()}
}
