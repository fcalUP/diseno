package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSPA(t *testing.T) {
	a := appNoDB(t)
	files := map[string]string{
		"index.html":    "<html>app</html>",
		"assets/app.js": "console.log(1)",
		"favicon.ico":   "ico",
	}
	for name, body := range files {
		p := filepath.Join(a.Cfg.WebDir, filepath.FromSlash(name))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := a.Routes()
	cases := []struct {
		method, path string
		code         int
		body, cache  string
	}{
		{"GET", "/", 200, "<html>app</html>", "no-cache"},
		{"GET", "/admin/alumnos", 200, "<html>app</html>", "no-cache"},
		{"GET", "/index.html", 200, "<html>app</html>", "no-cache"},
		{"GET", "/assets/app.js", 200, "console.log(1)", "public, max-age=31536000, immutable"},
		{"GET", "/assets/viejo.js", 404, "", ""},
		{"GET", "/favicon.ico", 200, "ico", "public, max-age=3600"},
		{"POST", "/algo", 405, "", ""},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))
		if rec.Code != c.code || (c.body != "" && rec.Body.String() != c.body) || rec.Header().Get("Cache-Control") != c.cache {
			t.Errorf("%s %s: %d %q cache=%q", c.method, c.path, rec.Code, rec.Body, rec.Header().Get("Cache-Control"))
		}
	}
}

// s3Like es cualquier Storage que no sea local: así no se registra /media/.
type s3Like struct{ Storage }

func TestSPAWithoutWebDir(t *testing.T) {
	a := appNoDB(t)
	a.Cfg.WebDir = filepath.Join(t.TempDir(), "no-existe")
	a.Store = s3Like{}
	h := a.Routes()
	for _, p := range []string{"/", "/admin", "/media/legacy/logo.png"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code != http.StatusNotFound || strings.Contains(rec.Header().Get("Content-Type"), "json") {
			t.Errorf("GET %s: %d %s", p, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
}
