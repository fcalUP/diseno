package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func decodeOKHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if !decodeJSON(w, r, &body) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
}

func TestCSRF(t *testing.T) {
	a := appNoDB(t)
	h := a.middleware(decodeOKHandler())
	try := func(method, path, contentType, origin, body string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	cases := []struct {
		method, path, ct, origin string
		want                     int
	}{
		{"POST", "/api/login", "", "", http.StatusForbidden},
		{"POST", "/api/login", "text/plain", "", http.StatusForbidden},
		{"DELETE", "/api/admin/avatars", "", "", http.StatusForbidden},
		{"POST", "/api/login", "application/json; charset=utf-8", "", http.StatusOK},
		{"POST", "/api/login", "application/json", "http://localhost:5173", http.StatusOK},
		{"POST", "/api/login", "application/json", "http://127.0.0.1:8080", http.StatusOK},
		{"POST", "/api/login", "application/json", "https://evil.example", http.StatusForbidden},
		{"POST", "/api/login", "application/json", "null", http.StatusForbidden},
		{"GET", "/api/me", "", "https://evil.example", http.StatusOK},
		{"PUT", "/api/uploads/local", "image/png", "http://localhost:5173", http.StatusOK},
		{"PUT", "/api/uploads/local", "image/png", "https://evil.example", http.StatusForbidden},
	}
	for _, c := range cases {
		if got := try(c.method, c.path, c.ct, c.origin, "{}"); got != c.want {
			t.Errorf("%s %s ct=%q origin=%q: %d, esperaba %d", c.method, c.path, c.ct, c.origin, got, c.want)
		}
	}

	// En producción solo vale el origen exacto de PUBLIC_URL.
	a.Cfg.PublicURL = "https://diseno.example.com"
	if got := try("POST", "/api/login", "application/json", "http://localhost:5173", "{}"); got != http.StatusForbidden {
		t.Errorf("localhost en producción: %d", got)
	}
	if got := try("POST", "/api/login", "application/json", "https://diseno.example.com", "{}"); got != http.StatusOK {
		t.Errorf("origen propio en producción: %d", got)
	}
}

func TestBodyLimitAndDecode(t *testing.T) {
	h := appNoDB(t).middleware(decodeOKHandler())
	send := func(body string) int {
		req := httptest.NewRequest("POST", "/api/x", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if got := send(`{"a":"` + strings.Repeat("x", 70<<10) + `"}`); got != http.StatusRequestEntityTooLarge {
		t.Errorf("cuerpo de 70 KB: %d", got)
	}
	if got := send(""); got != http.StatusOK {
		t.Errorf("cuerpo vacío: %d", got)
	}
	if got := send("{nope"); got != http.StatusBadRequest {
		t.Errorf("JSON roto: %d", got)
	}
}

func TestSecurityHeadersAndRecover(t *testing.T) {
	a := appNoDB(t)
	a.Cfg.UploadOrigin = "https://b.s3.us-east-1.amazonaws.com"
	h := a.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic("boom") }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/x", nil))
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), `"error"`) {
		t.Fatalf("panic: %d %s", rec.Code, rec.Body)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "connect-src 'self' https://b.s3.us-east-1.amazonaws.com;") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("CSP: %s", csp)
	}
	for k, v := range map[string]string{"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY", "Referrer-Policy": "strict-origin-when-cross-origin"} {
		if rec.Header().Get(k) != v {
			t.Errorf("%s = %q", k, rec.Header().Get(k))
		}
	}
}

func TestClientIP(t *testing.T) {
	req := func(remote string, headers map[string]string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		return r
	}
	cases := []struct {
		r     *http.Request
		trust bool
		want  string
	}{
		{req("10.0.0.1:5000", map[string]string{"CloudFront-Viewer-Address": "1.2.3.4:443"}), false, "10.0.0.1"},
		{req("10.0.0.1:5000", map[string]string{"CloudFront-Viewer-Address": "1.2.3.4:443"}), true, "1.2.3.4"},
		// Regresión (H1): una IPv6 cuenta como su /64, venga de donde venga.
		{req("10.0.0.1:5000", map[string]string{"CloudFront-Viewer-Address": "[2001:db8::1]:443"}), true, "2001:db8::"},
		{req("10.0.0.1:5000", map[string]string{"CloudFront-Viewer-Address": "2001:db8::1:443"}), true, "2001:db8::"},
		{req("10.0.0.1:5000", map[string]string{"CloudFront-Viewer-Address": "[2001:db8:1:2:aaaa:bbbb:cccc:dddd]:443"}), true, "2001:db8:1:2::"},
		{req("10.0.0.1:5000", map[string]string{"CloudFront-Viewer-Address": "[2001:db8:1:3::1]:443"}), true, "2001:db8:1:3::"},
		{req("10.0.0.1:5000", map[string]string{"X-Forwarded-For": "2001:db8:1:2::ffff"}), true, "2001:db8:1:2::"},
		{req("10.0.0.1:5000", map[string]string{"CloudFront-Viewer-Address": "[::ffff:1.2.3.4]:443"}), true, "1.2.3.4"},
		{req("10.0.0.1:5000", map[string]string{"X-Forwarded-For": "5.6.7.8, 10.0.0.2"}), true, "5.6.7.8"},
		{req("10.0.0.1:5000", map[string]string{"X-Forwarded-For": "basura"}), true, "10.0.0.1"},
		{req("[2001:db8:9:9::5]:5000", nil), false, "2001:db8:9:9::"},
		{req("[::1]:5000", nil), true, "::"},
	}
	for i, c := range cases {
		if got := clientIP(c.r, c.trust); got != c.want {
			t.Errorf("caso %d: %q, esperaba %q", i, got, c.want)
		}
	}
}

func TestPathID(t *testing.T) {
	mux := http.NewServeMux()
	var id int64
	var ok bool
	mux.HandleFunc("GET /x/{id}", func(w http.ResponseWriter, r *http.Request) { id, ok = pathID(r, "id") })
	for in, want := range map[string]bool{"12": true, "0": false, "-3": false, "abc": false} {
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x/"+in, nil))
		if ok != want {
			t.Errorf("pathID(%q) = %d, %v", in, id, ok)
		}
	}
}
