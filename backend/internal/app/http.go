package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

const (
	maxBodyBytes   = 64 << 10 // cuerpo máximo de una petición al API
	maxUploadBytes = 25 << 20 // solo PUT /api/uploads/local
	localUploadURL = "/api/uploads/local"
)

// decodeJSON lee un único valor JSON en dst. Un cuerpo vacío cuenta como {}.
// Si falla ya respondió 400 (o 413) y devuelve false.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(dst)
	if errors.Is(err, io.EOF) {
		return true
	}
	if err == nil {
		// Decode acepta streams: leer hasta EOF evita ignorar otro valor o basura
		// después del primero y comprueba el límite también en los espacios finales.
		var extra json.RawMessage
		err = decoder.Decode(&extra)
		if errors.Is(err, io.EOF) {
			return true
		}
	}
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeError(w, http.StatusRequestEntityTooLarge, "La solicitud es demasiado grande.")
		return false
	}
	writeError(w, http.StatusBadRequest, "JSON inválido.")
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeCanceled contesta si el navegador ya cortó la petición (cambió de página o
// cerró la pestaña). No es una falla del servidor, así que no va al log como error.
func writeCanceled(w http.ResponseWriter, err error) bool {
	if !errors.Is(err, context.Canceled) {
		return false
	}
	writeError(w, http.StatusBadRequest, "La solicitud se canceló.")
	return true
}

// pathID lee un id numérico positivo de la ruta ("/students/{id}").
func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id, err == nil && id > 0
}

// clientIP da la IP del visitante para los límites. Una IPv6 cuenta como su /64:
// quien tiene una /64 estrena dirección en cada petición sin esfuerzo, así que
// contarlas sueltas no frena a nadie. La IPv4 queda igual.
func clientIP(r *http.Request, trustProxy bool) string {
	addr := clientAddr(r, trustProxy)
	if ip := net.ParseIP(addr); ip != nil && ip.To4() == nil {
		return ip.Mask(net.CIDRMask(64, 128)).String()
	}
	return addr
}

// clientAddr da la dirección del visitante. Detrás de CloudFront (trustProxy) usa
// CloudFront-Viewer-Address y luego X-Forwarded-For; si no, RemoteAddr.
func clientAddr(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if ip := viewerAddressIP(r.Header.Get("CloudFront-Viewer-Address")); ip != "" {
			return ip
		}
		first, _, _ := strings.Cut(r.Header.Get("X-Forwarded-For"), ",")
		if ip := net.ParseIP(strings.TrimSpace(first)); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// viewerAddressIP entiende "ip:puerto", "[ipv6]:puerto" y también "ipv6:puerto" sin corchetes.
func viewerAddressIP(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(v); err == nil {
		v = host
	} else if i := strings.LastIndex(v, ":"); i > 0 && net.ParseIP(v[:i]) != nil {
		v = v[:i]
	}
	if ip := net.ParseIP(v); ip != nil {
		return ip.String()
	}
	return ""
}

// middleware arma la cadena del spec §5.4: recover → log → cabeceras → límite → CSRF → mux.
func (a *App) middleware(next http.Handler) http.Handler {
	h := a.csrfCheck(next)
	h = limitBody(h)
	h = securityHeaders(a.csp(), h)
	h = a.logRequests(h)
	return a.recoverPanics(h)
}

func (a *App) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if v == http.ErrAbortHandler {
				panic(v)
			}
			a.Log.Error("panic en handler", "method", r.Method, "path", r.URL.Path,
				"err", fmt.Sprint(v), "stack", string(debug.Stack()))
			writeError(w, http.StatusInternalServerError, "Error interno del servidor.")
		}()
		next.ServeHTTP(w, r)
	})
}

// statusWriter guarda el código de respuesta para el log.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// logRequests registra método, ruta, estado y duración. Nunca cuerpos ni query
// (el token de subida local viaja en la query).
func (a *App) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		// El healthcheck de Docker corre cada pocos segundos; solo interesa si falla.
		if r.URL.Path == "/api/health" && sw.status == http.StatusOK {
			return
		}
		a.Log.Info("http", "method", r.Method, "path", r.URL.Path, "status", sw.status,
			"ms", time.Since(start).Milliseconds())
	})
}

func (a *App) csp() string {
	connect, script := "'self'", "'self'"
	if a.Cfg.UploadOrigin != "" {
		connect += " " + a.Cfg.UploadOrigin
	}
	// El SDK del WAF baja su script y resuelve los retos contra su propio dominio.
	if a.Cfg.WafSDKOrigin != "" {
		connect += " " + a.Cfg.WafSDKOrigin
		script += " " + a.Cfg.WafSDKOrigin
	}
	return "default-src 'self'; script-src " + script + "; worker-src 'self' blob:; " +
		"img-src 'self' data: blob:; " +
		"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
		"font-src https://fonts.gstatic.com; connect-src " + connect + "; " +
		"object-src 'none'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
}

func securityHeaders(csp string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", csp)
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

func isLocalUpload(r *http.Request) bool {
	return r.Method == http.MethodPut && r.URL.Path == localUploadURL
}

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := int64(maxBodyBytes)
		if isLocalUpload(r) {
			limit = maxUploadBytes
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}

// csrfCheck: toda mutación del API rechaza solicitudes cross-site del navegador,
// exige JSON y, si trae Origin, que sea el nuestro. Los clientes sin Fetch Metadata
// conservan las comprobaciones de Content-Type y Origin.
func (a *App) csrfCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			next.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
			writeError(w, http.StatusForbidden, "Origen no permitido.")
			return
		}
		if !isLocalUpload(r) {
			mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mt != "application/json" {
				writeError(w, http.StatusForbidden, "Content-Type debe ser application/json.")
				return
			}
		}
		if o := r.Header.Get("Origin"); o != "" && !a.originAllowed(o) {
			writeError(w, http.StatusForbidden, "Origen no permitido.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// originAllowed compara esquema y host con PUBLIC_URL. Si PUBLIC_URL es
// localhost, se acepta cualquier http://localhost:* o http://127.0.0.1:*.
func (a *App) originAllowed(origin string) bool {
	o, err := url.Parse(origin)
	if err != nil || o.Host == "" {
		return false
	}
	pub, err := url.Parse(a.Cfg.PublicURL)
	if err != nil {
		return false
	}
	if strings.EqualFold(o.Scheme, pub.Scheme) && strings.EqualFold(o.Host, pub.Host) {
		return true
	}
	return pub.Scheme == "http" && isLoopbackName(pub.Hostname()) &&
		o.Scheme == "http" && isLoopbackName(o.Hostname())
}

func isLoopbackName(h string) bool {
	return h == "localhost" || h == "127.0.0.1"
}
