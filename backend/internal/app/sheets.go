package app

// Cliente mínimo de Google Sheets por REST: solo lectura con una cuenta de
// servicio y dos llamadas (metadatos y values:batchGet).

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
)

const impSheetsScope = "https://www.googleapis.com/auth/spreadsheets.readonly"

// Las pruebas cambian estas tres por un servidor falso.
var (
	impSheetsURL  = "https://sheets.googleapis.com"
	impTokenURL   = google.JWTTokenURL
	impHTTPClient = &http.Client{Timeout: time.Minute}
)

// impError es un error que se le muestra al admin tal cual, con su código HTTP.
type impError struct {
	status int
	msg    string
}

func (e *impError) Error() string { return e.msg }

func impFail(status int, msg string) error { return &impError{status: status, msg: msg} }

// impParseCredentials valida el JSON de la cuenta de servicio. El token_uri
// del archivo se ignora y siempre se pide el token a Google, así un JSON
// alterado no puede hacer que el servidor llame a otra dirección.
func impParseCredentials(raw string) (*jwt.Config, error) {
	conf, err := google.JWTConfigFromJSON([]byte(raw), impSheetsScope)
	if err != nil || conf.Email == "" {
		return nil, impFail(http.StatusBadRequest, "Las credenciales de Google no son válidas: se espera el JSON de una cuenta de servicio.")
	}
	if block, _ := pem.Decode(conf.PrivateKey); block == nil {
		return nil, impFail(http.StatusBadRequest, "Las credenciales de Google no traen una private_key válida.")
	}
	conf.TokenURL = impTokenURL
	return conf, nil
}

// impClientEmail saca client_email del JSON guardado; "" si no se puede leer.
func impClientEmail(raw string) string {
	var f struct {
		ClientEmail string `json:"client_email"`
	}
	if json.Unmarshal([]byte(raw), &f) != nil {
		return ""
	}
	return f.ClientEmail
}

type impSheets struct {
	client *http.Client
	email  string // client_email, para los mensajes de error
}

// impNewSheets arma el cliente. ctx debe vivir mientras se use el cliente,
// porque con él se pide el token.
func impNewSheets(ctx context.Context, conf *jwt.Config) *impSheets {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, impHTTPClient)
	return &impSheets{client: conf.Client(ctx), email: conf.Email}
}

type impSpreadsheet struct {
	Title string
	Tabs  []string
}

func (s *impSheets) metadata(ctx context.Context, id string) (impSpreadsheet, error) {
	var out struct {
		Properties struct {
			Title string `json:"title"`
		} `json:"properties"`
		Sheets []struct {
			Properties struct {
				Title string `json:"title"`
			} `json:"properties"`
		} `json:"sheets"`
	}
	q := url.Values{"fields": {"properties.title,sheets.properties.title"}}
	if err := s.get(ctx, "/v4/spreadsheets/"+url.PathEscape(id), q, &out); err != nil {
		return impSpreadsheet{}, err
	}
	sp := impSpreadsheet{Title: out.Properties.Title, Tabs: []string{}}
	for _, sh := range out.Sheets {
		sp.Tabs = append(sp.Tabs, sh.Properties.Title)
	}
	return sp, nil
}

// batchGet lee varios rangos en una sola llamada, en el mismo orden pedido.
// Usa el render por omisión (FORMATTED_VALUE), igual que el servidor viejo:
// cada celda llega como el texto que se ve en la hoja.
func (s *impSheets) batchGet(ctx context.Context, id string, ranges []string) ([][][]string, error) {
	if len(ranges) == 0 {
		return nil, nil
	}
	var out struct {
		ValueRanges []struct {
			Values [][]any `json:"values"`
		} `json:"valueRanges"`
	}
	q := url.Values{"ranges": ranges}
	if err := s.get(ctx, "/v4/spreadsheets/"+url.PathEscape(id)+"/values:batchGet", q, &out); err != nil {
		return nil, err
	}
	if len(out.ValueRanges) != len(ranges) {
		return nil, impFail(http.StatusBadGateway, "Google Sheets devolvió una respuesta inesperada.")
	}
	res := make([][][]string, len(ranges))
	for i, vr := range out.ValueRanges {
		rows := make([][]string, len(vr.Values))
		for j, row := range vr.Values {
			cells := make([]string, len(row))
			for k, c := range row {
				cells[k] = impCell(c)
			}
			rows[j] = cells
		}
		res[i] = rows
	}
	return res, nil
}

// impCell pasa una celda a texto; con FORMATTED_VALUE ya debería serlo.
func impCell(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// impRange arma 'Pestaña'!A:N; una comilla simple en el nombre se escribe doble.
func impRange(tab, cols string) string {
	return "'" + strings.ReplaceAll(tab, "'", "''") + "'!" + cols
}

func (s *impSheets) get(ctx context.Context, path string, q url.Values, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, impSheetsURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	res, err := s.client.Do(req)
	if err != nil {
		return impTransportError(ctx, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return impTransportError(ctx, err)
	}
	if res.StatusCode != http.StatusOK {
		return s.apiError(res.StatusCode, body)
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return impFail(http.StatusBadGateway, "Google Sheets devolvió una respuesta inesperada.")
	}
	return nil
}

// apiError traduce los errores de Google a mensajes para el admin.
func (s *impSheets) apiError(status int, body []byte) error {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &e)
	switch {
	case status == http.StatusForbidden && strings.Contains(e.Error.Message, "disabled"):
		// Google responde 403 también cuando la API no está activa en el proyecto.
		return impFail(http.StatusBadRequest, "La API de Google Sheets no está habilitada en el proyecto de la cuenta de servicio. Actívala en Google Cloud.")
	case status == http.StatusForbidden:
		return impFail(http.StatusBadRequest, "La cuenta de servicio "+s.email+" no tiene acceso a la hoja. Compártela con ese correo.")
	case status == http.StatusNotFound:
		return impFail(http.StatusBadRequest, "No existe la hoja con ese ID.")
	case status == http.StatusUnauthorized:
		return impFail(http.StatusBadRequest, "Google rechazó las credenciales de la cuenta de servicio.")
	case status == http.StatusTooManyRequests:
		return impFail(http.StatusServiceUnavailable, "Google Sheets está limitando las consultas. Intenta de nuevo en un minuto.")
	case status == http.StatusBadRequest:
		// Pasa, por ejemplo, si el ID es de un Excel subido a Drive y no de una hoja de Google.
		return impFail(http.StatusBadRequest, strings.TrimSpace("Google Sheets rechazó la consulta. "+e.Error.Message))
	}
	return impFail(http.StatusBadGateway, strings.TrimSpace(fmt.Sprintf("Google Sheets respondió con error %d. %s", status, e.Error.Message)))
}

func impTransportError(ctx context.Context, err error) error {
	var re *oauth2.RetrieveError
	var ne net.Error
	switch {
	case errors.As(err, &re):
		return impFail(http.StatusBadRequest, "Google rechazó las credenciales de la cuenta de servicio. Revisa que la clave siga activa.")
	case ctx.Err() != nil:
		return ctx.Err()
	case errors.As(err, &ne) && ne.Timeout():
		return impFail(http.StatusGatewayTimeout, "Google Sheets tardó demasiado en responder.")
	}
	return impFail(http.StatusBadGateway, "No se pudo conectar con Google Sheets.")
}
