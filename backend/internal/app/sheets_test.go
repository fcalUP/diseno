package app

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// El ID predeterminado de Ajustes, para que la vista previa sin ID también funcione.
const impTestSheetID = "1I6pVLSBav-U7c86FLavh0tikPghLDVrWCFuru-qwQ4Y"

const impTestEmail = "importador@diseno-pruebas.iam.gserviceaccount.com"

type impFakeTab struct {
	title string
	rows  [][]string
}

// impFake imita lo poco que usamos de Google: el token JWT, los metadatos y
// values:batchGet (filas cortas y celdas vacías omitidas, como el de verdad).
type impFake struct {
	srv   *httptest.Server
	block chan struct{} // si no es nil, los metadatos esperan a que se cierre

	mu      sync.Mutex
	id      string
	title   string
	tabs    []impFakeTab
	fail    int        // estado de error forzado para la API de Sheets
	failMsg string     // mensaje de ese error ("" = el de permisos)
	batches [][]string // rangos pedidos en cada batchGet
}

var impTestKey = sync.OnceValue(func() string {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		panic(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
})

// impCredentials es un JSON de cuenta de servicio de prueba. Su token_uri
// apunta a un puerto cerrado: si el cliente lo usara, la prueba fallaría.
func impCredentials() string {
	b, _ := json.Marshal(map[string]string{
		"type":           "service_account",
		"project_id":     "diseno-pruebas",
		"private_key_id": "clave-1",
		"private_key":    impTestKey(),
		"client_email":   impTestEmail,
		"token_uri":      "http://127.0.0.1:1/no-usar",
	})
	return string(b)
}

func newImpFake(t *testing.T, tabs []impFakeTab) *impFake {
	t.Helper()
	f := &impFake{id: impTestSheetID, title: "Gamificación 2026", tabs: tabs}
	f.srv = httptest.NewServer(f)
	t.Cleanup(f.srv.Close)
	oldURL, oldToken, oldClient := impSheetsURL, impTokenURL, impHTTPClient
	impSheetsURL, impTokenURL, impHTTPClient = f.srv.URL, f.srv.URL+"/token", f.srv.Client()
	t.Cleanup(func() { impSheetsURL, impTokenURL, impHTTPClient = oldURL, oldToken, oldClient })
	return f
}

func (f *impFake) setTab(title string, rows [][]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.tabs {
		if f.tabs[i].title == title {
			f.tabs[i].rows = rows
			return
		}
	}
	f.tabs = append(f.tabs, impFakeTab{title, rows})
}

func (f *impFake) setFail(status int) {
	f.mu.Lock()
	f.fail = status
	f.mu.Unlock()
}

func impGoogleError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":{"code":%d,"message":%q,"status":"ERROR"}}`, status, msg)
}

func (f *impFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/token" {
		if r.Method != http.MethodPost || r.FormValue("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || r.FormValue("assertion") == "" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"access_token":"tok-prueba","token_type":"Bearer","expires_in":3600}`)
		return
	}
	if r.Header.Get("Authorization") != "Bearer tok-prueba" {
		impGoogleError(w, http.StatusUnauthorized, "Request had invalid authentication credentials.")
		return
	}
	rest, _ := strings.CutPrefix(r.URL.Path, "/v4/spreadsheets/")
	id, op, _ := strings.Cut(rest, "/")
	if op == "" && f.block != nil {
		<-f.block
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case f.fail != 0:
		msg := f.failMsg
		if msg == "" {
			msg = "The caller does not have permission"
		}
		impGoogleError(w, f.fail, msg)
	case id != f.id:
		impGoogleError(w, http.StatusNotFound, "Requested entity was not found.")
	case op == "":
		if r.URL.Query().Get("fields") != "properties.title,sheets.properties.title" {
			impGoogleError(w, http.StatusBadRequest, "fields inesperado")
			return
		}
		var sheets []map[string]any
		for _, tab := range f.tabs {
			sheets = append(sheets, map[string]any{"properties": map[string]string{"title": tab.title}})
		}
		json.NewEncoder(w).Encode(map[string]any{"properties": map[string]string{"title": f.title}, "sheets": sheets})
	case op == "values:batchGet":
		ranges := r.URL.Query()["ranges"]
		f.batches = append(f.batches, ranges)
		var out []map[string]any
		for _, rg := range ranges {
			vr, ok := f.valueRange(rg)
			if !ok {
				impGoogleError(w, http.StatusBadRequest, "Unable to parse range: "+rg)
				return
			}
			out = append(out, vr)
		}
		json.NewEncoder(w).Encode(map[string]any{"spreadsheetId": f.id, "valueRanges": out})
	default:
		impGoogleError(w, http.StatusNotFound, "Not found")
	}
}

// valueRange entiende 'Pestaña'!A:N y recorta las filas al ancho pedido.
func (f *impFake) valueRange(rg string) (map[string]any, bool) {
	i := strings.LastIndex(rg, "'!")
	if !strings.HasPrefix(rg, "'") || i < 1 {
		return nil, false
	}
	title := strings.ReplaceAll(rg[1:i], "''", "'")
	width := map[string]int{"A:N": 14, "A:C": 3, "A:E": 5}[rg[i+2:]]
	for _, tab := range f.tabs {
		if tab.title != title || width == 0 {
			continue
		}
		vr := map[string]any{"range": rg, "majorDimension": "ROWS"}
		var values [][]string
		for _, row := range tab.rows {
			values = append(values, row[:min(len(row), width)])
		}
		if len(values) > 0 { // una pestaña vacía no trae "values"
			vr["values"] = values
		}
		return vr, true
	}
	return nil, false
}

func TestImpRange(t *testing.T) {
	cases := map[string]string{
		"Analisis de Datos": "'Analisis de Datos'!A:N",
		"O'Brien":           "'O''Brien'!A:N",
		"TAR":               "'TAR'!A:N",
	}
	for tab, want := range cases {
		if got := impRange(tab, "A:N"); got != want {
			t.Errorf("impRange(%q) = %q, esperaba %q", tab, got, want)
		}
	}
}

func TestImpParseCredentials(t *testing.T) {
	conf, err := impParseCredentials(impCredentials())
	if err != nil {
		t.Fatal(err)
	}
	if conf.Email != impTestEmail || conf.TokenURL != impTokenURL {
		t.Fatalf("config inesperada: email=%q tokenURL=%q", conf.Email, conf.TokenURL)
	}
	bad := []string{
		"no es json",
		`{"type":"authorized_user","client_email":"x@y.z"}`,
		`{"type":"service_account","client_email":"x@y.z","private_key":"basura"}`,
		`{"type":"service_account","private_key":"` + strings.ReplaceAll(impTestKey(), "\n", `\n`) + `"}`,
	}
	for _, raw := range bad {
		var ie *impError
		if _, err := impParseCredentials(raw); !errors.As(err, &ie) || ie.status != http.StatusBadRequest {
			t.Errorf("credenciales %.30q: esperaba error 400, llegó %v", raw, err)
		}
	}
	if impClientEmail(impCredentials()) != impTestEmail || impClientEmail("{") != "" {
		t.Fatal("impClientEmail no lee client_email")
	}
}

func TestImpSheetsClient(t *testing.T) {
	f := newImpFake(t, []impFakeTab{
		{"O'Brien", [][]string{{"ID", "NOMBRE"}, {"1", "Uno"}, {}, {"2"}}},
		{"Vacía", nil},
	})
	conf, err := impParseCredentials(impCredentials())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	s := impNewSheets(ctx, conf)

	meta, err := s.metadata(ctx, impTestSheetID)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != "Gamificación 2026" || strings.Join(meta.Tabs, "|") != "O'Brien|Vacía" {
		t.Fatalf("metadatos: %+v", meta)
	}
	got, err := s.batchGet(ctx, impTestSheetID, []string{impRange("O'Brien", "A:N"), impRange("Vacía", "A:C")})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || len(got[0]) != 4 || got[0][1][1] != "Uno" || len(got[0][2]) != 0 || len(got[0][3]) != 1 || len(got[1]) != 0 {
		t.Fatalf("batchGet: %q", got)
	}

	// Errores de Google traducidos para el admin.
	expect := func(err error, status int, msg string) {
		t.Helper()
		var ie *impError
		if !errors.As(err, &ie) || ie.status != status || ie.msg != msg {
			t.Fatalf("esperaba %d %q, llegó %v", status, msg, err)
		}
	}
	_, err = s.metadata(ctx, "1otraHojaQueNoExisteParaNadaXYZ")
	expect(err, http.StatusBadRequest, "No existe la hoja con ese ID.")
	f.setFail(http.StatusForbidden)
	_, err = s.metadata(ctx, impTestSheetID)
	expect(err, http.StatusBadRequest, "La cuenta de servicio "+impTestEmail+" no tiene acceso a la hoja. Compártela con ese correo.")
	f.mu.Lock()
	f.failMsg = "Google Sheets API has not been used in project 123 before or it is disabled."
	f.mu.Unlock()
	_, err = s.metadata(ctx, impTestSheetID)
	expect(err, http.StatusBadRequest, "La API de Google Sheets no está habilitada en el proyecto de la cuenta de servicio. Actívala en Google Cloud.")
	f.mu.Lock()
	f.failMsg = ""
	f.mu.Unlock()
	f.setFail(http.StatusInternalServerError)
	_, err = s.batchGet(ctx, impTestSheetID, []string{impRange("O'Brien", "A:N")})
	expect(err, http.StatusBadGateway, "Google Sheets respondió con error 500. The caller does not have permission")

	// Si Google rechaza el token, el mensaje habla de las credenciales.
	f.setFail(0)
	impTokenURL = f.srv.URL + "/token-malo"
	conf, _ = impParseCredentials(impCredentials())
	_, err = impNewSheets(ctx, conf).metadata(ctx, impTestSheetID)
	expect(err, http.StatusBadRequest, "Google rechazó las credenciales de la cuenta de servicio. Revisa que la clave siga activa.")
}

func TestImpCell(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{{nil, ""}, {"1,000", "1,000"}, {true, "true"}, {float64(12), "12"}, {1.5, "1.5"}}
	for _, c := range cases {
		if got := impCell(c.in); got != c.want {
			t.Errorf("impCell(%v) = %q, esperaba %q", c.in, got, c.want)
		}
	}
}

func TestImpParseDate(t *testing.T) {
	mx := impMexico
	cases := map[string]time.Time{
		"1/2/2026, 3:04:05 PM":    time.Date(2026, 1, 2, 15, 4, 5, 0, mx), // toLocaleString en inglés
		"1/2/2026, 3:04:05 PM":    time.Date(2026, 1, 2, 15, 4, 5, 0, mx), // Node 20+ usa espacio angosto
		"2/1/2026, 15:04:05":      time.Date(2026, 1, 2, 15, 4, 5, 0, mx), // toLocaleString en español
		"2/1/2026, 3:04:05 p. m.": time.Date(2026, 1, 2, 15, 4, 5, 0, mx),
		"2/1/2026, 9:00:00 a.m.":  time.Date(2026, 1, 2, 9, 0, 0, 0, mx),
		"1/2/2026 15:04:05":       time.Date(2026, 1, 2, 15, 4, 5, 0, mx), // formato de Sheets
		"25/2/2026 15:04:05":      time.Date(2026, 2, 25, 15, 4, 5, 0, mx),
		"2026-01-02 15:04:05":     time.Date(2026, 1, 2, 15, 4, 5, 0, mx),
		" 2026-01-02 ":            time.Date(2026, 1, 2, 0, 0, 0, 0, mx),
	}
	for in, want := range cases {
		got, ok := impParseDate(in)
		if !ok || !got.Equal(want) {
			t.Errorf("impParseDate(%q) = %v, %v; esperaba %v", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "ayer", "31/31/2026", "2026-13-01"} {
		if _, ok := impParseDate(bad); ok {
			t.Errorf("impParseDate(%q) debió fallar", bad)
		}
	}
}

func TestImpAvatarName(t *testing.T) {
	cases := map[string]struct {
		code  string
		level int
		ok    bool
	}{
		"02514123.png":     {"0251412", 3, true},
		"02798126.png":     {"0279812", 6, true},
		"DefaultH5.png":    {"DefaultH", 5, true},
		"ana0.JPEG":        {"ana", 0, true},
		"x1.webp":          {"x", 1, true},
		"5.png":            {"", 0, false},
		"0251412.txt":      {"", 0, false},
		"sinnivel.png":     {"", 0, false},
		"sub/02514120.png": {"", 0, false},
	}
	for in, want := range cases {
		code, level, ok := impAvatarName(in)
		if code != want.code || level != want.level || ok != want.ok {
			t.Errorf("impAvatarName(%q) = %q, %d, %v", in, code, level, ok)
		}
	}
}
